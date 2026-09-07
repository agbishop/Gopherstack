service: workspaces
sdk_module: aws-sdk-go-v2/service/workspaces@v1.73.1
last_audit_commit: 7c8077891728
last_audit_date: 2026-08-28
# 2026-09-07 (gopherstack-3b8k): Start/StopWorkspaces's eligibility guards checked only workspace
# state, silently ignoring the running-mode half of their documented precondition. Real doc
# comments (api_op_StartWorkspaces.go / api_op_StopWorkspaces.go): "You cannot start a WorkSpace
# unless it has a running mode of AutoStop or Manual and a state of STOPPED" /
# "...AutoStop or Manual and a state of AVAILABLE, IMPAIRED, UNHEALTHY, or ERROR". RunningMode IS
# modeled here (WorkspaceProperties.RunningMode, interfaces.go), so this was a real gap, not a
# structural one -- isStartableWorkspaceState/isStoppableWorkspaceState (workspaces.go) took only
# `state`, so an ALWAYS_ON workspace in an otherwise-eligible state (AVAILABLE for Stop, STOPPED
# for Start) wrongly succeeded. Fixed: both now also take `runningMode` and require it be AUTO_STOP
# or MANUAL (new isEligibleRunningMode helper; MANUAL is WorkSpaces Core-only and unreachable here
# since ModifyWorkspaceProperties's isValidRunningMode rejects it, same precedent as
# isRebootableWorkspaceState's unreachable-but-checked REBOOTING/UNHEALTHY). Failures still surface
# via FailedRequests (these are batch ops; Start/StopWorkspaces uniquely model
# InvalidResourceStateException at the operation level per the existing stateFailure precedent,
# renamed startStopFailure). 8 pre-existing tests were silently exercising the unguarded path
# (created workspaces via the shared `createWorkspace` test helper, which leaves RunningMode unset,
# then relied on Start/Stop actually succeeding) -- corrected to use a new
# createStartStopEligibleWorkspace/createWorkspaceWithRunningMode helper instead of changing the
# widely-shared `createWorkspace` helper (which TestModifyWorkspaceProperties_Persisted depends on
# leaving Properties nil). A 9th, TestStopWorkspaces_AlreadyStopped_Fails, kept passing but for the
# wrong reason (its setup Stop call was itself silently failing on running mode, not reaching
# STOPPED) -- also corrected. Regression tests added and proven to fail against the unmodified
# guard (reverted workspaces.go, ran, restored):
# TestStopWorkspaces_AlwaysOnRunningMode_Fails, TestStartWorkspaces_AlwaysOnRunningMode_Fails
# (state forced to STOPPED via new test-only SetWorkspaceState, since ModifyWorkspaceState only
# supports AVAILABLE/ADMIN_MAINTENANCE and a real ALWAYS_ON workspace can never reach STOPPED
# through StopWorkspaces itself), plus TestStopStartWorkspaces_AutoStopRunningMode_Succeeds pinning
# the success side.
# 2026-08-30: cursor-population sweep (does every List/Describe response struct that DECLARES a
# NextToken actually SET one before the collection can exceed a page?). Enumerated all 17 SDK ops
# whose Input/Output declare NextToken. This service has NO shared pagination chokepoint (only
# account.go/directories.go/bundles.go/workspaces.go hand-rolled their own correctly) -- 10 of the
# 17 silently returned every item on one page with an empty NextToken, ignoring the caller's
# MaxResults/NextToken entirely: DescribeApplicationAssociations, DescribeConnectClientAddIns,
# DescribeConnectionAliases, DescribeConnectionAliasPermissions, DescribeIpGroups,
# DescribeWorkspaceImagePermissions, DescribeWorkspaceImages, DescribeWorkspacesPools,
# DescribeWorkspacesPoolSessions, ListAccountLinks. All 10 fixed via pkgs/page.New (the same
# chokepoint mgn/cognitoidp already use) plus a deterministic sort where the backend read straight
# off an unordered store.All()/map range. DescribeWorkspacesPoolSessions' fix is currently
# unobservable in practice -- b.poolSessions is never Put anywhere in this backend (no op creates a
# session), so the list is always empty today -- but the wiring is correct once that changes.
# 2026-08-30 sort-totality sweep (Class F: a sort that exists but is not total,
# and Class G: parallel result lists truncated independently). Reviewed every
# sort.Slice/sort.Strings/slices.Sort* call site across every paginated listing
# in this service (including the 10 ops the cursor-population sweep above just
# added pagination to). Every one sorts on that resource's own real unique ID
# (BundleID/AliasID/GroupID/PoolID/SessionID/AddInID/LinkID/DirectoryID/
# ImageID/WorkspaceName-echoed-workspaceID, and DescribeConnectionAliasPermissions
# preserves insertion order over a plain non-reordered slice rather than
# resorting a map) -- confirmed against each type's own store key, not assumed.
# No non-unique sort key found. Confirmed no listing in this service returns
# two-or-more collections the API defines as one ordered sequence truncated
# independently (each op returns exactly one paginated array). No Class F/G
# bugs found.
# ALSO CHECK sweep (classes A-E) found one genuine, previously mis-diagnosed
# bug: DescribeWorkspacesConnectionStatus. The 2026-08-13 audit (see the
# "2 left unfixed as provably bounded" note above) claimed this op's response
# "can never exceed the request's own bound" since WorkspaceIds is capped at 25
# -- true only when WorkspaceIds is given. Real
# DescribeWorkspacesConnectionStatusInput/Output (workspaces@v1.73.1
# api_op_DescribeWorkspacesConnectionStatus.go) BOTH declare NextToken, and the
# real doc comment's 25-item cap is on WorkspaceIds specifically, not on the
# unfiltered (WorkspaceIds omitted, "describe every WorkSpace") path -- that
# PARITY claim was wrong. gopherstack's wire structs didn't declare NextToken
# at all (worse than declared-but-unpopulated), and the unfiltered path built
# its response straight off store.Table.All() (unspecified map order) with no
# sort -- both a missing-cursor gap (Class B-adjacent) and Class E (never
# sorted). Hand-verified against the pre-fix code: 15 repeated calls with no
# intervening writes returned a different WorkspaceId order nearly every time.
# Fixed: NextToken now on both wire structs, backend method now takes/returns
# a token, sorts by WorkspaceID (unique) before pkgs/page.New with a new
# internal connectionStatusPageSize=100 (the real input has no MaxResults, so
# the page size is server-chosen -- same pattern as DescribeAccountModifications/
# ListAvailableManagementCidrRanges). GetWorkspacesConnectionStatus's exported
# signature changed (added nextToken in, added nextToken out) -- StorageBackend
# interface and the one call site (handler_workspaces.go) updated to match; no
# other caller existed. Proven by
# TestDescribeWorkspacesConnectionStatus_UnfilteredPageWalksExactly (130 items,
# walks 2 internal pages, asserts the concatenation is exactly the created set)
# and TestDescribeWorkspacesConnectionStatus_UnfilteredOrderIsDeterministic (15
# repeated calls, same order every time) in
# connection_status_pagination_test.go.
# 5 ops confirmed already correct: DescribeAccountModifications, DescribeWorkspaceDirectories,
# DescribeWorkspaces, ListAvailableManagementCidrRanges, and DescribeWorkspaceBundles (whose
# unfiltered path pages correctly; its BundleIds-filtered path returns unpaginated results bounded
# by the caller's own BundleIds list length, a judgment call, not a fix). 1 left unfixed as
# provably bounded: DescribeApplications (its backing store, b.applications, is registered but
# never Put by any op -- always 0 items). CORRECTED 2026-08-30 (sort-totality sweep): this note
# previously also claimed DescribeWorkspacesConnectionStatus was provably bounded because
# WorkspaceIds is capped at 25 per call -- that cap is real but only applies when WorkspaceIds is
# given; the unfiltered (WorkspaceIds omitted) path genuinely paginates on real AWS (both
# DescribeWorkspacesConnectionStatusInput/Output declare NextToken) and had no cursor at all here.
# Now fixed -- see that op's own dated note and ops: entry below.
overall: A            # 2026-08-28 (gopherstack-6flj/21my wrapper-key/silent-drop sweep):
                       # DescribeWorkspaceDirectories' dirResp carried only
                       # DirectoryId/DirectoryName/DirectoryType/Alias/State/SubnetIds --
                       # EndpointEncryptionMode, CertificateBasedAuthProperties, SamlProperties,
                       # SelfservicePermissions, WorkspaceAccessProperties,
                       # WorkspaceCreationProperties, and ipGroupIds were all silently dropped
                       # despite this backend already holding the data via the 7 `Modify*`
                       # directory-settings ops and AssociateIpGroups -- real AWS has no
                       # separate Describe op for any of these settings, so this was an
                       # accept-and-drop bug across the whole DescribeWorkspaceDirectories
                       # response, not a mere omission. Fixed by reading the existing
                       # storedDirSettings.Properties prefixed keys and directoryIpGroups back
                       # into the response; see DescribeWorkspaceDirectories's op note. Two
                       # related gaps found and disclosed (not fixed, budget): UserSettings on
                       # ModifyStreamingProperties is accepted off the wire and then dropped
                       # before reaching the backend (a second accept-and-drop, smaller in
                       # scope); WorkspaceBundle has no BundleType/CreationTime/
                       # LastUpdatedTime/State at all (no existing state to read back, unlike
                       # the directory-settings fix -- a real gap, not accept-and-drop). No
                       # other silent-drop, hard-decode-error, invented-member, or
                       # wrong-enum-value bugs found in the ops re-checked this pass.
                       # follow-up pass on gopherstack-o5ig: both deferred items from the prior
                       # pass (RunningMode-while-STOPPED, Applications family) fixed for real,
                       # plus 3 more genuine bugs found via the same sweep classes.
                       # gopherstack-gt9o (part of the gopherstack-u8my sdk_module pin sweep):
                       # ClientProperties' ClientExperiencePolicy/LogUploadEnabled are now
                       # threaded end-to-end; ClientProperties family moves partial -> ok.
                       # 2026-08-23: closed the WorkspaceName item this file itself flagged as
                       # "the cheapest of these to close" (see the 2026-08-13 trap note below).
                       # WorkspaceRequest.WorkspaceName was accepted by CreateWorkspaces nowhere
                       # AND the response was independently fabricating a WorkspaceName by
                       # echoing UserName -- two bugs, not one: an accept-and-drop on the request
                       # side, and a wire value real AWS never returns for a normal user-assigned
                       # WorkSpace on the response side. See CreateWorkspaces/DescribeWorkspaces
                       # ops rows and the dated Notes section for the full writeup.

# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateWorkspaces: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (prior pass) — was all-or-nothing; now partitions FailedRequests/PendingRequests per item, matching real FailedCreateWorkspaceRequest{WorkspaceRequest,ErrorCode,ErrorMessage} shape. FIXED 2026-08-23: WorkspaceRequest.WorkspaceName (aws-sdk-go-v2/service/workspaces@v1.73.1/types/types.go:1874-1879, real input member, required for user-decoupled WorkSpaces where UserName=[UNDEFINED]) was accepted nowhere -- createWorkspaceSpec/WorkspaceCreationSpec had no field for it at all, so it was silently dropped end to end. Now threaded through ThemeUpdateOptions-style (see appstream's UpdateThemeForStack fix, same session) into WorkspaceCreationSpec and echoed on PendingRequests/FailedRequests.WorkspaceRequest."}
  DescribeWorkspaces: {wire: fixed, errors: ok, state: ok, persist: ok, note: "pagination (25/page), region filter, WorkspaceIds/DirectoryId/UserName/BundleId filters all verified against real field names. FIXED 2026-08-23: workspaceResp already had a WorkspaceName wire key (added after this file's 2026-08-13 audit without a corresponding PARITY.md update -- see Notes), but InMemoryBackend.CreateWorkspace was fabricating its value by echoing UserName (or WorkspaceId when UserName was empty) for EVERY WorkSpace -- real types.Workspace.WorkspaceName is documented as 'the name of the user-decoupled WorkSpace' and 'not applicable if UserName is specified for user-assigned WorkSpaces', so a real client describing an ordinary WorkSpace was receiving a fabricated field value that does not exist on real AWS's wire for that case. Now only ever set from the caller-supplied WorkspaceRequest.WorkspaceName, absent (omitempty) otherwise."}
  DescribeWorkspacesConnectionStatus: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED — ConnectionStateCheckTimestamp/LastKnownUserConnectionTimestamp were entirely missing from the response (only WorkspaceId/ConnectionState were wired); both are now emitted as epoch-seconds numbers via awstime.Epoch. LastKnownUserConnectionTimestamp stays zero-valued (0, omitted) since this backend models no actual client connection activity. FIXED 2026-08-30 (sort-totality sweep): the unfiltered (WorkspaceIds omitted) path had no NextToken on either wire struct and built its response off an unsorted map -- see the dated note above for the full correction of this file's own prior 'provably bounded' claim."}
  ModifyWorkspaceProperties: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-hnyl): isValidComputeTypeName was a hand-copied 9-entry allowlist predating 14 values types.Compute now has (GENERALPURPOSE_4XLARGE/8XLARGE and the G6/GR6/G6F GPU families) -- ComputeTypeName was falsely rejected for any of them. Now derives from types.Compute.Values()."}
  ModifyWorkspaceState: {wire: ok, errors: ok, state: ok, persist: ok}
  RebootWorkspaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "intentionally does not transition state — documented + tested (TestRebootWorkspaces_DoesNotChangeState in workspaces_lifecycle_test.go); this emulator models reboot as instantaneous with no transient REBOOTING window, not a bug. FIXED this pass (gopherstack-o5ig): real AWS's documented precondition 'You cannot reboot a WorkSpace unless its state is AVAILABLE, UNHEALTHY, or REBOOTING' was entirely unenforced (only existence was checked) — now returns a per-item FailedRequests{ErrorCode:\"OperationNotSupportedException\"} entry (the only error OperationNotSupportedException in this op's real error list) for a workspace in a disallowed state, e.g. STOPPED or ADMIN_MAINTENANCE."}
  RebuildWorkspaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "same as RebootWorkspaces — intentional, tested no-transition behavior (TestRebuildWorkspaces_DoesNotChangeState). FIXED this pass (gopherstack-o5ig): same class of fix as RebootWorkspaces, enforcing the real precondition 'You cannot rebuild a WorkSpace unless its state is AVAILABLE, ERROR, UNHEALTHY, STOPPED, or REBOOTING' (ADMIN_MAINTENANCE/PENDING now rejected)."}
  StartWorkspaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "STOPPED->AVAILABLE; idempotent no-op for other states, no failure reported (matches AWS tolerance for redundant start/stop calls)"}
  StopWorkspaces: {wire: ok, errors: ok, state: ok, persist: ok}
  TerminateWorkspaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "deletes workspace + its tags"}
  CreateTags: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteTags: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeTags: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeWorkspaceBundles: {wire: ok, errors: ok, state: ok, persist: ok, note: "Amazon-owned static list + custom bundles, owner filter, pagination all verified"}
  DescribeWorkspaceDirectories: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-28 (gopherstack-6flj/21my wrapper-key/silent-drop sweep): dirResp (handler_directories.go) carried only DirectoryId/DirectoryName/DirectoryType/Alias/State/SubnetIds -- every one of these real WorkspaceDirectory members (workspaces@v1.73.1 deserializers.go's awsAwsjson11_deserializeDocumentWorkspaceDirectory case list) was silently dropped despite this backend already holding the data via the 7 `Modify*` directory-settings ops (see DirectoryModifyOps below) and AssociateIpGroups: EndpointEncryptionMode, CertificateBasedAuthProperties, SamlProperties, SelfservicePermissions, WorkspaceAccessProperties, WorkspaceCreationProperties, and ipGroupIds (note the unusual lowercase-led wire key, deserializers.go:18124). Real AWS has no separate Describe op for any of these settings -- DescribeWorkspaceDirectories is the only place a real client ever reads them back, so this was an accept-and-drop bug across the whole family, not a mere omission. Fixed by reading storedDirSettings.Properties' existing prefixed keys (CertAuth_/Saml_/SelfSvc_/Access_/Creation_) and b.directoryIpGroups back into the new WorkspaceDirectory fields (interfaces.go), threaded through dirResp. Pointer sub-structs stay nil (omitted) for a directory never touched by the corresponding Modify op. See TestDescribeWorkspaceDirectories_RealSDKClient_SettingsRoundTrip in wire_field_fixes_test.go. NOT fixed this pass: WorkspaceAccessProperties.AccessEndpointConfig (ModifyWorkspaceAccessProperties' handler never accepted it as input either -- genuine unbuilt feature, not accept-and-drop) and StreamingProperties (ModifyStreamingProperties only threads StreamingExperiencePreferredProtocol through as a flat string; UserSettings/GlobalAccelerator/StorageConnectors are accepted on the input struct in the handler but never passed to the backend at all -- see gaps below, disclosed not fixed, out of scope for this pass' budget)."}
  RegisterWorkspaceDirectory: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — re-registering an already-registered directory silently 200'd (unconditionally idempotent); now returns ResourceAlreadyExistsException, matching real AWS."}
  DeregisterWorkspaceDirectory: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — deregistered a directory unconditionally even with live WorkSpaces still assigned to it (a ghost-reference risk: DescribeWorkspaces would keep returning WorkSpaces whose DirectoryId no longer resolved to any registered directory). Real AWS: 'If any WorkSpaces are registered to this directory, you must remove them before you can deregister the directory' — now enforced via InvalidResourceStateException. Also now cascade-cleans the directoryIpGroups association map on a successful deregister (was leaked as an orphaned entry keyed by the dead DirectoryId)."}
  RestoreWorkspace: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass) — was a true no-op with no existence check (silently 200'd for unknown WorkspaceId); now returns ResourceNotFoundException. No snapshot modeling, so still otherwise a no-op beyond validation — acceptable given no snapshot state exists to restore from."}
  MigrateWorkspace: {wire: ok, errors: ok, state: ok, persist: ok, note: "source deleted, new workspace created with target bundleId, tested in workspaces_lifecycle_test.go"}
  CreateIpGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "lowercase groupId/groupName/groupDesc/userRules JSON keys verified against real deserializer — an AWS API quirk, not a bug"}
  DescribeIpGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30, cursor sweep) -- backend ignored MaxResults/NextToken entirely (`_ int32, _ string` params), always returning every IP group on one page with NextToken always empty. Now sorted by GroupID and paginated via pkgs/page.New. Proven via TestDescribeIpGroups_Pagination + hand-revert."}
  DeleteIpGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  AuthorizeIpRules: {wire: ok, errors: ok, state: ok, persist: ok}
  RevokeIpRules: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateRulesOfIpGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateIpGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — directoryIpGroups map is now included in backendSnapshot (Snapshot/Restore), matching Tags. Previously persist:deferred (ephemeral across restarts); no snapshot-version bump needed since an older snapshot just decodes with an empty map, matching prior behavior exactly."}
  DisassociateIpGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateStandbyWorkspaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — was invented-shape: built pending/failed items from a hand-rolled map[string]string carrying UserName/BundleId fields that DON'T EXIST on the real StandbyWorkspace/PendingCreateStandbyWorkspacesRequest types (gopherstack-invented fields), and FailedStandbyRequests was hardcoded to always be empty regardless of input. Rewrote using the real shapes: StandbyWorkspace{DirectoryId, PrimaryWorkspaceId, DataReplication, Tags, VolumeEncryptionKey} for requests, PendingCreateStandbyWorkspacesRequest{DirectoryId, State, UserName, WorkspaceId} for successes (note: no BundleId field on this response type either), FailedCreateStandbyWorkspacesRequest{ErrorCode, ErrorMessage, StandbyWorkspaceRequest} for per-item failures. Moved to a single-item CreateStandbyWorkspace(ctx, spec) backend method with the batch/partial-failure loop in the handler, mirroring the CreateWorkspaces pattern. Real per-item validation: an unregistered DirectoryId now reports a genuine FailedStandbyRequests entry instead of always succeeding. PrimaryWorkspaceId existence is NOT cross-validated (see Notes: this backend has no way to see a primary WorkSpace living in a different region's backend instance) — this is a documented, deliberate limitation, not an oversight."}
  DescribeImageAssociations: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED — was a stub ignoring ImageId entirely (always 200'd with an empty list for a nonexistent image, and Associations was typed []any with no real field names). Now validates ImageId is required + must reference a real image (ResourceNotFoundException) and AssociatedResourceTypes is required + must be \"APPLICATION\" (the only real enum value for ImageAssociatedResourceType). Response now uses the real ImageResourceAssociation shape (AssociatedResourceId/AssociatedResourceType/ImageId/State/StateReason/Created/LastUpdatedTime, epoch timestamps). Real AWS's WorkSpaces Application Manager has no public API to create this association (only AssociateWorkspaceApplication, which associates an app directly with a WorkSpace, not an image) — so a freshly emulated account always returns an empty (but now correctly validated/typed) list."}
  DescribeBundleAssociations: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED — same class of stub as DescribeImageAssociations; now validates BundleId (checked against both Amazon-owned and custom bundles) and AssociatedResourceTypes, real BundleResourceAssociation shape."}
  DescribeAccountModifications: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — was a true stub always returning an empty list regardless of history. ModifyAccount now appends an AccountModification{ModificationState:\"COMPLETED\", DedicatedTenancySupport, DedicatedTenancyManagementCidrRange, StartTime} entry on every call (this backend applies changes synchronously, so there's no PENDING window to model); DescribeAccountModifications returns them most-recent-first, paginated via pkgs/page, and both accountConfig and this new history list are now included in backendSnapshot. Real DescribeAccountModificationsInput has no MaxResults field (only NextToken) — this backend uses a fixed internal page size (100), field-diffed against the real input shape."}
  ListAvailableManagementCidrRanges: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED — was a stub returning the same 3 hardcoded CIDRs (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16) regardless of input, and ManagementCidrRangeConstraint (a real smithy-`required` field) wasn't validated at all. Now requires + validates the constraint is a real IPv4 CIDR (InvalidParameterValuesException otherwise) and derives up to 8 contained /26 sub-ranges from it (real AWS returns /26 management-interface blocks carved from the caller's constraint), paginated via pkgs/page."}
  ModifyClientProperties: {wire: ok, errors: ok, state: ok, persist: deferred, note: "FIXED this pass (gopherstack-gt9o): request's ClientProperties gained ClientExperiencePolicy since v1.68.3 (*string, types/types.go:269); LogUploadEnabled (types/types.go:275, a real LogUploadEnum member) was also unthreaded and is now fixed too, per gopherstack-gt9o's instruction to thread whatever else the real input carries, not just the field named in the bd issue. Backend now merges (partial update) instead of overwriting the whole storedClientProps on every call -- ReconnectEnabled-only requests no longer silently clear a previously set ClientExperiencePolicy/LogUploadEnabled. ClientExperiencePolicy is a bare *string in the SDK with no @enum trait (unlike LogUploadEnabled/ReconnectEnabled, which are generated Go enum types) -- any value is accepted, not just the three FORCE_CLASSIC/FORCE_UI_2026/USER_CHOICE values the doc comment lists as examples. persist stays deferred: clientProperties was already, pre-existing, intentionally NOT part of backendSnapshot (see persistence.go's field comment and whitebox_test.go) -- out of scope for gopherstack-gt9o, which is about the missing fields, not this separate ephemeral-persistence gap."}
  DescribeClientProperties: {wire: ok, errors: ok, state: ok, persist: deferred, note: "FIXED this pass (gopherstack-gt9o): now echoes ClientExperiencePolicy/LogUploadEnabled alongside the pre-existing ReconnectEnabled; each is omitted from the wire (omitempty) rather than emitted as an invented empty string when never set. persist: deferred for the same pre-existing reason as ModifyClientProperties."}

families:
  ConnectionAlias: {status: ok, note: "Create/Describe/Delete/Associate/Disassociate/Permissions all mutate storedConnAlias state correctly; spot-checked against real WorkspaceRequest/ConnectionAlias field names"}
  WorkspaceBundle_custom: {status: ok, note: "Create/Delete/Update custom bundles verified real mutation. FIXED this pass (gopherstack-o5ig): UpdateWorkspaceBundle accepted any ImageId, including nonexistent ones, and silently pointed the bundle at a phantom image — now validates ImageId against b.images (ResourceNotFoundException, real error list); empty ImageId remains a no-op (the real field is optional). FIXED this pass (gopherstack-e5pd): CreateWorkspaceBundle had the same gap (ImageId is a real required field, unlike UpdateWorkspaceBundle's optional one) — now validates existence before b.nextID/b.customBundles.Put/b.tags are touched, so a rejected call consumes no ID and leaves no partial state."}
  WorkspaceImage: {status: ok, note: "Copy/Create/Delete/Import/CreateUpdated/DescribePermissions/UpdatePermission all mutate storedImage table. FIXED this pass: Created was serialized as an ISO8601 string (\"2006-01-02T15:04:05Z\") in three response shapes (CreateWorkspaceImage, DescribeWorkspaceImages, DescribeCustomWorkspaceImageImport) — real WorkspaceImage.Created / DescribeCustomWorkspaceImageImportOutput.Created are *time.Time, and this is the awsjson1.1 protocol, which requires epoch-seconds numbers (unixTimestamp), not RFC3339 strings; a real client SDK would fail to deserialize the response. Fixed via awstime.Epoch, matching the bug class already fixed in QuickSight/IoT. FIXED this pass (gopherstack-e5pd): CreateWorkspaceImage's WorkspaceId parameter was discarded outright (`_ /*workspaceId*/`), so any value including a nonexistent one was accepted — now validated against b.workspaces (ResourceNotFoundException, real error list) before createImageLocked runs; the real CreateWorkspaceImageOutput/WorkspaceImage types carry no source-workspace field, so there is nothing else to derive from it. FIXED this pass (gopherstack-plmb): CreateUpdatedWorkspaceImage.SourceImageId had the same unvalidated-identifier shape — now validated against b.images (ResourceNotFoundException, real error list) before createImageLocked runs. CopyWorkspaceImage.SourceImageId is now validated too, but only when SourceRegion is empty or equals this backend's own region: this service instantiates one InMemoryBackend per (account, region) (NewInMemoryBackend/provider.go), and storedImage carries no region field, so a genuine cross-region SourceImageId legitimately lives in a different backend instance this one cannot see — validating it unconditionally would make gopherstack more restrictive than real AWS. A cross-region SourceImageId (SourceRegion set and not equal to this backend's region) is therefore deliberately left unvalidated; see TestCopyWorkspaceImage_SourceImageIDValidation for both cases. FIXED this pass (gopherstack-u90v): ImportWorkspaceImage dropped IngestionProcess (required, api_op_ImportWorkspaceImage.go:67); ImportCustomWorkspaceImage dropped ComputeType, ImageSource, InfrastructureConfigurationArn, OsVersion, Platform and Protocol (all required, api_op_ImportCustomWorkspaceImage.go:33-75 — two more required members, ComputeType and Platform, than the sweep that filed this issue caught). All six are now required (InvalidParameterValuesException if absent, the only op-declared error this backend already maps ErrInvalidParameter to — neither op's own deserializeOpError switch has a ValidationException case) and validated against the pinned SDK's own enum Values() where the field is an enum, never a hand-copied list. ImageSource is modeled as a proper tagged union (ImageSource type, mirroring types.ImageSourceIdentifierMember*) rather than flattened. ComputeType/OsVersion/Platform/Protocol have no corresponding field on real DescribeWorkspaceImages' WorkspaceImage type (types.go:1724-1768) so they are stored but not echoed anywhere on the wire — genuinely inert, not an omission. ImageSource and InfrastructureConfigurationArn ARE present on real DescribeCustomWorkspaceImageImportOutput (api_op_DescribeCustomWorkspaceImageImport.go:39-73) and are now echoed there. IngestionProcess has no echo target on any real response shape either."}
  WorkspacesPool: {status: ok, note: "Create/Describe/Start/Stop/Terminate/Update all real state transitions on storedPool.State. FIXED prior pass: (1) CreatedAt epoch-seconds bug; (2) CapacityStatus/RunningMode were entirely absent from the response, DesiredUserSessions was parsed but discarded. FIXED this pass (gopherstack-o5ig): UpdateWorkspacesPoolInput's real constraint 'The running mode can only be updated when the pool is in a stopped state' (doc comment on RunningMode) is now enforced -- previously applied unconditionally. The pool state machine genuinely reaches STOPPED via StopWorkspacesPool, so this is a real, reachable precondition, not a strand-the-operation trap: UpdateWorkspacesPool now returns InvalidResourceStateException (in the real error list) when RunningMode is set on a non-STOPPED pool, checked before any other field is mutated. See TestWorkspacesPool_UpdateRunningModeRequiresStopped."}
  WorkspacesPoolSession: {status: ok}
  Account: {status: ok, note: "DescribeAccount/ModifyAccount/ModifyEndpointEncryptionMode read/write storedAccountConfig; DescribeAccountModifications now has a real, persisted modification history (see ops table) instead of an always-empty stub."}
  ConnectClientAddIn: {status: ok}
  ClientBranding: {status: ok}
  ClientProperties: {status: ok, note: "FIXED this pass (gopherstack-gt9o): ClientExperiencePolicy and LogUploadEnabled are now threaded end-to-end (Modify + Describe); see ModifyClientProperties/DescribeClientProperties ops entries. Persistence remains a separate, pre-existing, deliberately-out-of-scope ephemeral gap (persist: deferred on both ops)."}
  DirectoryModifyOps: {status: ok, note: "ModifyEndpointEncryptionMode/ModifyCertificateBasedAuthProperties/ModifySamlProperties/ModifySelfservicePermissions/ModifyStreamingProperties/ModifyWorkspaceAccessProperties/ModifyWorkspaceCreationProperties all write storedDirSettings.Properties map. FIXED this pass (gopherstack-o5ig): all 7 shared one root cause -- ensureDirSettings silently created a storedDirSettings row (and, for the 6 non-EndpointEncryptionMode ops, wrote real properties into it) for ANY DirectoryId, including one never registered via RegisterWorkspaceDirectory, and always reported success. Each real error list includes ResourceNotFoundException. Now all 7 validate the directory is actually REGISTERED (not merely present in b.dirSettings, since the old ensureDirSettings call could itself create a bare row) before mutating anything, returning errDirectoryNotFound otherwise; ensureDirSettings itself is now only called from RegisterWorkspaceDirectory."}
  AccountLinks: {status: ok, note: "Create/Accept/Reject/Delete/Get/List all mutate storedAccountLink.Status"}
  Applications: {status: ok, note: "FIXED this pass (gopherstack-o5ig): the family previously (1) accepted any WorkspaceId, including nonexistent ones, and always reported success -- Associate/Disassociate/Deploy/DescribeWorkspaceAssociations now validate WorkspaceId against b.workspaces (ResourceNotFoundException, real error list); (2) used a fabricated wire shape, `AssociationStatus: \"INSTALLED\"`/`\"UNINSTALLED\"` -- neither field name nor value exists on the real WorkspaceResourceAssociation type (field-diffed against deserializers.go's awsAwsjson11_deserializeDocumentWorkspaceResourceAssociation: real key is `State`, real enum is AssociationState with no INSTALLED/UNINSTALLED values). Now uses `State: \"COMPLETED\"`/`\"REMOVED\"` (real terminal enum values), matching this backend's synchronous apply -- see 'Applications family: legitimate simplification vs false claim' below. ApplicationId is deliberately NOT existence-checked (see same section) -- this backend never seeds the read-only applications catalog, so requiring a match would permanently strand the operation. DescribeWorkspaceAssociations/DescribeApplicationAssociations now also validate the real required AssociatedResourceTypes field against the real enum (WorkSpaceAssociatedResourceType: APPLICATION only; ApplicationAssociatedResourceType: WORKSPACE/BUNDLE/IMAGE)."}
  ImageBundleAssociations: {status: ok, note: "FIXED — see DescribeImageAssociations/DescribeBundleAssociations, now tracked individually in the ops table above (previously rolled up here only). Deep-audited this pass (previously marked deferred/not-audited): confirmed real AWS exposes no public create-association API for image/bundle<->application, so an always-empty (correctly validated + typed) response is genuine emulated behavior, not a gap."}
  DescribeWorkspaceSnapshots: {status: ok, note: "returns empty RebuildSnapshots/RestoreSnapshots lists — correct void-result shape since no snapshot state is modeled anywhere in this backend"}

gaps:
  - "clientProperties (ModifyClientProperties/DescribeClientProperties, including the ClientExperiencePolicy/LogUploadEnabled fields fixed this pass, gopherstack-gt9o) is NOT part of backendSnapshot -- pre-existing, deliberate (see persistence.go's field comment and whitebox_test.go), out of scope for gopherstack-gt9o which is about the missing fields, not this separate ephemeral-persistence gap. (bd: none filed for the persistence gap itself)"
  - "ModifyStreamingProperties' UserSettings ([]types.UserSetting -- Action/Permission/MaximumLength, real per workspaces@v1.73.1 types.go:1277-1291) is decoded off the wire by modifyStreamingPropertiesInput (handler_directories.go's sibling file) but then dropped before it ever reaches Backend.ModifyStreamingProperties -- only StreamingExperiencePreferredProtocol is threaded through. This is a genuine accept-and-drop, found but NOT fixed this pass (gopherstack-6flj/21my, 2026-08-28) due to budget: storedDirSettings.Properties is a flat map[string]string, so representing a list of structs needs either a JSON-encoded value or a schema change, more than a field-level fix. GlobalAccelerator/StorageConnectors (also real StreamingProperties members) aren't captured by the input struct at all, so those are a separate, smaller unbuilt-feature gap, not accept-and-drop. DescribeWorkspaceDirectories' new StreamingProperties field was deliberately left out of this pass' fix for the same reason -- see that op's note. (bd: gopherstack-6flj/21my)"
  - "WorkspaceBundle (custom bundles) has no BundleType/CreationTime/LastUpdatedTime/State at all -- all four are real WorkspaceBundle members (workspaces@v1.73.1 types.go:1507-1543) DescribeWorkspaceBundles never populates. Unlike the DescribeWorkspaceDirectories fix above, this is not accept-and-drop: storedCustomBundle (models.go) never captured CreationTime either, so there is no existing state to read back -- CreateWorkspaceBundle would need a new CreatedAt field threaded through persistence.go's snapshot DTO. State is buildable cheaply (this backend creates bundles synchronously and never fails, so a hardcoded AVAILABLE would be honest, matching the pattern already used for e.g. EMR's WAITING-on-create clusters), but was left out of this pass' scope. Found but not fixed (bd: gopherstack-6flj/21my, 2026-08-28)."
  # All gaps from the prior pass (CreateStandbyWorkspaces FailedStandbyRequests,
  # AssociateIpGroups/DisassociateIpGroups persistence) were closed for real this
  # pass — see the ops table entries above for what changed.
  #
  # gopherstack-o5ig (2026-08-10): both items previously listed as deferred below
  # (RunningMode-while-STOPPED, Applications family) are now fixed — see the
  # WorkspacesPool and Applications family notes above. CreateWorkspaceBundle.ImageId
  # and CreateWorkspaceImage.WorkspaceId, flagged then as a follow-up, are now
  # fixed too (gopherstack-e5pd, 2026-08-11) — see WorkspaceBundle_custom and
  # WorkspaceImage notes above. That same pass found CopyWorkspaceImage.SourceImageId
  # and CreateUpdatedWorkspaceImage.SourceImageId have the identical gap; both are
  # now fixed too (gopherstack-plmb, 2026-08-11) — see WorkspaceImage note above.
  # CopyWorkspaceImage's fix is conditional on SourceRegion (see that note) rather
  # than a full unconditional check, since this backend genuinely has no visibility
  # into another region's image table.

deferred: []

leaks: {status: clean, note: "no goroutines/janitors in this service; all state lives in store.Table maps guarded by lockmetrics.RWMutex. FIXED this pass: DeregisterWorkspaceDirectory no longer allows deregistering a directory that still has live WorkSpaces assigned to it (previously left DescribeWorkspaces returning WorkSpaces pointing at a DirectoryId with no corresponding registered directory — a dangling-reference-shaped leak, now prevented outright per real AWS semantics) and now cascade-cleans the directoryIpGroups map entry for a directory on successful deregistration (was an orphaned map entry keyed by a dead DirectoryId, never reachable again once the directory itself was gone)."}

---

## Notes

### 2026-08-23 pass: WorkspaceName accept-and-drop, plus an independent fabricated value

Found via this file's own "cheapest of these to close" trap note (2026-08-13 section,
see Traps below), then found to be worse than described once the current source was
read rather than trusted: two separate bugs, not one.

1. **Request-side accept-and-drop.** `WorkspaceRequest.WorkspaceName`
   (`aws-sdk-go-v2/service/workspaces@v1.73.1/types/types.go:1874-1879` -- "required if
   `UserName` is `[UNDEFINED]` for user-decoupled WorkSpaces... not applicable if
   `UserName` is specified for user-assigned WorkSpaces") was a real input member
   `createWorkspaceSpec` (`handler_workspaces.go`) had no JSON field for at all --
   `json.Unmarshal` silently dropped it on the floor, and `WorkspaceCreationSpec`
   (`interfaces.go`), the struct passed to the backend, had no field to carry it even
   if the handler had decoded it.
2. **Response-side fabrication, found independently while fixing (1).**
   `InMemoryBackend.CreateWorkspace` (`workspaces.go`) derived `WorkspaceName` from
   `spec.UserName` (falling back to the generated `WorkspaceId` when `UserName` was
   empty) for *every* WorkSpace -- so a completely ordinary user-assigned WorkSpace,
   for which real AWS's own doc comment says this field is "not applicable", got a
   populated `WorkspaceName` in every `DescribeWorkspaces`/`CreateWorkspaces` response a
   real client would ever see. This is more severe than the request-side gap: it's a
   value on the wire that AWS's real service would never send, not merely a caller
   value silently discarded.

Fixed by threading `WorkspaceName` through `createWorkspaceSpec` ->
`WorkspaceCreationSpec` -> `storedWorkspace` (which already had a `WorkspaceName` field
and JSON tag, added by some later, undated pass -- see the Traps section) unchanged,
and deleting the `UserName`/`WorkspaceId` fallback derivation entirely so the field is
only ever real, caller-supplied data. Also threaded onto `pendingWorkspace`'s and
`workspaceRequestResp`'s own `WorkspaceName` JSON keys (previously absent from both,
even though the destination `Workspace`/wire type already carried the field) so
`CreateWorkspaces`' own response echoes it too, not just a follow-up `DescribeWorkspaces`.

No persisted-struct schema change: `storedWorkspace.WorkspaceName`
(`models.go:19`, `json:"workspaceName,omitempty"`) already existed and was already
round-tripped by `Snapshot`/`Restore` -- only how it gets populated changed, not its
shape or JSON tag. No golden refresh needed.

**Proof**: `TestCreateWorkspaces_RealSDKClient_WorkspaceNameThreadedThrough`
(`wire_field_fixes_test.go`, a real `aws-sdk-go-v2` client round trip) and
`TestWorkspace_WorkspaceNameThreadedThrough` (`workspaces_test.go`) both assert the
caller-supplied value round-trips through `CreateWorkspaces`' `PendingRequests` and a
follow-up `DescribeWorkspaces`, and that an ordinary user-assigned WorkSpace's
`WorkspaceName` decodes as `nil`/absent, not a fabricated `UserName` echo.
`TestWorkspace_RealMembersPopulated` previously asserted
`ws["WorkspaceName"] == tt.userName` -- a test that had locked in the fabrication bug
-- corrected to assert the key is genuinely absent instead of deleted outright.
Hand-reverted `interfaces.go`/`handler_workspaces.go`/`workspaces.go` to `git show
HEAD`, confirmed all three tests fail with exactly the predicted symptom (empty/absent
`WorkspaceName` where a real value was expected, and a fabricated `[UNDEFINED]`/
`UserName` value where nothing should be present), restored, `md5sum` byte-identical.
`go build`/`go vet`/`go test -race ./services/workspaces/...`/
`golangci-lint run ./services/workspaces/...`/`make build-check` (repo-wide, since
`UpdateThemeForStack`'s sibling appstream fix this session also changed an exported
interface signature) all clean.

Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: WorkspacesService.<Op>`.
Route matcher (`strings.HasPrefix(header, "WorkspacesService.")`) is simple and every
one of the 91 real SDK v1.68.3 ops is reachable — verified via `comm` diff between
`ls aws-sdk-go-v2/service/workspaces@v1.68.3/api_op_*.go` and the handler's
`buildOps()` map; 91/91 match, no missing ops, no phantom (non-SDK) op names. Re-verified
this pass (91 op names extracted straight from the SDK zip's `api_op_*.go` filenames):
no gopherstack-invented op names exist in this service.

### gopherstack-o5ig follow-up (2026-08-10)

Three assigned items, verdicts:

1. **UpdateWorkspacesPool RunningMode-only-while-STOPPED.** Real constraint
   confirmed (`UpdateWorkspacesPoolInput.RunningMode` doc comment: "can only
   be updated when the pool is in a stopped state"; `InvalidResourceStateException`
   is in the op's real error list). This backend's pool state machine
   genuinely reaches STOPPED (`StopWorkspacesPool` sets it), so the
   counterweight case (an unreachable precondition that would permanently
   strand the operation) does NOT apply — this was a real, fixable gap. Fixed:
   see WorkspacesPool family note above.
2. **Applications family — legitimate simplification vs false claim: BOTH,
   split by field.** `AssociationState` (the real enum) has no PENDING
   window this backend can produce — Associate/Deploy apply synchronously, so
   reporting a terminal state immediately is a legitimate simplification, not
   a false claim. But the VALUE used, `"INSTALLED"`/`"UNINSTALLED"`, was a
   false claim: neither exists in the real `AssociationState` enum at all
   (real values: PENDING_INSTALL/PENDING_INSTALL_DEPLOYMENT/PENDING_UNINSTALL/
   PENDING_UNINSTALL_DEPLOYMENT/INSTALLING/UNINSTALLING/ERROR/COMPLETED/REMOVED),
   and the wire key itself was wrong (`AssociationStatus`, not the real
   `State` — field-diffed against deserializers.go). Fixed: real key/enum
   (`State: "COMPLETED"`/`"REMOVED"`). Separately, and reachable regardless of
   the above: WorkspaceId was never validated to exist — fixed via the real
   `ResourceNotFoundException` (in every one of these ops' error lists).
   ApplicationId is NOT existence-checked: this backend never seeds the
   applications catalog (real AWS has no CreateApplication API — it's a
   read-only AWS/partner catalog), so that would be the same
   permanently-stranded-operation trap as (1)'s counterweight, just in the
   other direction.
3. **Per-operation ResourceLimitExceeded/OperationNotSupported.** Swept every
   op with either in its real error list (grep across
   `deserializers.go`'s `strings.EqualFold` switches, pinned v1.73.1).
   `ResourceLimitExceededException` triggers were NOT added anywhere — every
   instance found is a genuine account/service quota (pools, IP groups,
   images, tags, bundles, ...) with no natural backend-side limit to check
   against; inventing one would reject valid input, which is worse than
   omitting it (an xray pass this campaign already reverted exactly this
   mistake). `OperationNotSupportedException` had one real, deterministic,
   previously-unenforced trigger: RebootWorkspaces/RebuildWorkspaces' documented
   state preconditions (see ops table). Every other real-list occurrence of
   `OperationNotSupportedException` in this service (AssociateConnectionAlias,
   AssociateWorkspaceApplication's compute/OS-compatibility errors, the
   WorkSpaces-Pools-end-of-support ops, ...) would require modeling data this
   backend doesn't have (compute-type compatibility matrices, a real pool
   lifecycle beyond RUNNING/STOPPED) and was left alone rather than fabricated.

Sweep findings (beyond the three assigned items), in the campaign's listed
bug-class order:

- **State mutated before validation**: none newly found in this service's
  Update/Modify operations beyond the already-filed ClientProperties gap —
  every Update op here either validates first or has no validation to order
  against. (The RunningMode fix above is validate-then-mutate by construction,
  not a mutate-then-fail correction.)
- **A field/value that doesn't exist on the real type**: `AssociationStatus`
  (should be `State`) and `"INSTALLED"`/`"UNINSTALLED"` (not real
  `AssociationState` values) — see item 2 above.
- **An operation accepting a nonexistent resource ID and reporting success**
  — the highest-yield class this pass, one root cause each:
  - `AssociateWorkspaceApplication`/`DisassociateWorkspaceApplication`/
    `DeployWorkspaceApplications`/`DescribeWorkspaceAssociations` accepted any
    `WorkspaceId` (item 2).
  - `ModifyEndpointEncryptionMode`/`ModifyCertificateBasedAuthProperties`/
    `ModifySamlProperties`/`ModifySelfservicePermissions`/
    `ModifyStreamingProperties`/`ModifyWorkspaceAccessProperties`/
    `ModifyWorkspaceCreationProperties` all shared one root cause
    (`ensureDirSettings` silently created a settings row for any
    `DirectoryId`, registered or not) — fixed, see DirectoryModifyOps note.
  - `UpdateWorkspaceBundle` accepted any `ImageId` — fixed, see
    WorkspaceBundle_custom note. `CreateWorkspaceBundle.ImageId` and
    `CreateWorkspaceImage.WorkspaceId` had the same gap, flagged then for a
    future pass; fixed 2026-08-11 (gopherstack-e5pd) — see WorkspaceBundle_custom
    and WorkspaceImage notes above. `CopyWorkspaceImage.SourceImageId` and
    `CreateUpdatedWorkspaceImage.SourceImageId` turned out to share the same
    gap, flagged then as a new follow-up (gopherstack-plmb); both fixed
    2026-08-11 — see WorkspaceImage note above. `CopyWorkspaceImage`'s check
    is conditional on `SourceRegion` matching this backend's own region,
    since a genuine cross-region source image is invisible to this backend's
    `b.images` table and an unconditional check would be a new
    more-restrictive-than-AWS bug.
- **More permissive than real AWS (unvalidated enums)**:
  `DescribeWorkspaceAssociations`/`DescribeApplicationAssociations` never
  validated the real required `AssociatedResourceTypes` field — fixed
  alongside item 2 (`validateAssociatedResourceTypes`/
  `validateApplicationAssociatedResourceTypes` in store.go).
- **More restrictive than real AWS**: none found this pass. (No hand-written
  allowlist was touched or newly added except the two
  `AssociatedResourceTypes` enum lists above, both directly transcribed from
  the pinned SDK's `types.WorkSpaceAssociatedResourceType`/
  `types.ApplicationAssociatedResourceType`.)
- **A resource reporting a live status after deletion**: none found; not
  re-audited exhaustively this pass (out of the assigned scope), no evidence
  of one in the paths touched.

Not touched: `services/sns` (a concurrent agent's assignment). Also observed
mid-pass: `go build ./...`/`go vet .` at the repository root started failing
partway through this session due to an unrelated, uncommitted, in-progress
edit in `services/apigatewayv2/models.go` (a type mismatch in
`persistence.go` against `RoutingRuleAction`/`RoutingRuleCondition`) —
another concurrent agent's mid-edit, not anything in `services/workspaces`;
`go build`/`go vet`/`go test -race` scoped to `./services/workspaces/...`
are all clean.

### Bugs fixed this pass (2026-07-23)

1. **CreateStandbyWorkspaces used invented fields.** The backend's request/response
   shapes were hand-rolled `map[string]string` carrying `UserName`/`BundleId` keys
   that don't exist anywhere on the real `StandbyWorkspace` or
   `PendingCreateStandbyWorkspacesRequest` types (field-diffed against
   `aws-sdk-go-v2/service/workspaces/types`: `StandbyWorkspace` has
   `DirectoryId`/`PrimaryWorkspaceId`/`DataReplication`/`Tags`/`VolumeEncryptionKey`;
   `PendingCreateStandbyWorkspacesRequest` has `DirectoryId`/`State`/`UserName`/
   `WorkspaceId` — no `BundleId` on either). `FailedStandbyRequests` was also
   hardcoded to always be an empty slice, so a malformed standby request (e.g. an
   unregistered `DirectoryId`) always silently "succeeded". Rewrote with real
   per-op DTOs (`StandbyWorkspaceSpec`, `PendingStandbyWorkspace` in interfaces.go)
   and moved the batch/partial-failure loop into the handler (mirroring
   `CreateWorkspaces`'s established pattern), with a single-item
   `CreateStandbyWorkspace` backend method that does real `DirectoryId`
   registration validation.

2. **DescribeImageAssociations/DescribeBundleAssociations were stubs.** Both
   ignored their `ImageId`/`BundleId` input entirely — calling either with a
   nonexistent ID silently 200'd with an empty list instead of
   `ResourceNotFoundException`, and `AssociatedResourceTypes` (a real required
   field) was never validated. Response shape was `[]any` with no real field
   names. Fixed with real existence + required-field validation and the real
   `ImageResourceAssociation`/`BundleResourceAssociation` wire shapes
   (`AssociatedResourceId`/`AssociatedResourceType`/`State`/`StateReason`/
   `Created`/`LastUpdatedTime`, the latter two as epoch-seconds numbers). The
   list itself still comes back empty in every case, which is *correct*: real
   AWS's WorkSpaces Application Manager has no public API to associate an
   application with an image or bundle directly (only
   `AssociateWorkspaceApplication`, which associates an application with a
   WorkSpace, and `DeployWorkspaceApplications`, neither of which touch an
   image/bundle) — so a freshly emulated account genuinely has none to report.

3. **DescribeAccountModifications and ListAvailableManagementCidrRanges were
   stubs.** The former always returned an empty list, with no history tracking
   even after `ModifyAccount` calls. The latter always returned the same 3
   hardcoded CIDR blocks (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`)
   regardless of the (real, required) `ManagementCidrRangeConstraint` input,
   which also went completely unvalidated. Fixed: `ModifyAccount` now records a
   real `AccountModification` history entry (persisted in `backendSnapshot`),
   and `ListAvailableManagementCidrRanges` now requires + validates the
   constraint as an IPv4 CIDR and derives real `/26` sub-ranges contained
   within it.

4. **Epoch-seconds timestamp bug** (the exact bug class already fixed in
   QuickSight/IoT — see `pkgs/awstime`'s doc comment): four response fields
   serialized `time.Time` as ISO8601 strings
   (`"2006-01-02T15:04:05Z"`) instead of the epoch-seconds JSON numbers the
   `awsjson1.1` protocol's `unixTimestamp` timestamp format requires. Affected:
   `WorkspaceImage.Created` (in `CreateWorkspaceImage`, `DescribeWorkspaceImages`,
   and `DescribeCustomWorkspaceImageImport`'s `Created`) and
   `WorkspacesPool.CreatedAt`. A real client SDK would reject these responses
   with "expected Timestamp to be a JSON Number, got string instead". Fixed via
   `awstime.Epoch` at every call site. `DescribeWorkspacesConnectionStatus` was
   also missing its two timestamp fields
   (`ConnectionStateCheckTimestamp`/`LastKnownUserConnectionTimestamp`) entirely
   — added as epoch-seconds numbers.

5. **WorkspacesPool was missing two `This member is required` fields.**
   `CapacityStatus` and `RunningMode` are both required on the real
   `WorkspacesPool` type but were absent from every response; separately,
   `CreateWorkspacesPoolInput.Capacity.DesiredUserSessions` was parsed off the
   wire but then silently discarded — never reaching the backend, so it had no
   effect at all. Same for `UpdateWorkspacesPoolInput.RunningMode`/`Capacity`.
   Fixed: `storedPool` now carries `RunningMode`/`DesiredUserSessions`, both
   real state now flowing from Create/Update; `CapacityStatus` is derived
   (steady-state: no live session tracking is modeled, so
   `ActiveUserSessions=0` and `ActualUserSessions=AvailableUserSessions=
   DesiredUserSessions`, satisfying the documented
   `Actual = Available + Active` invariant).

6. **AssociateIpGroups/DisassociateIpGroups state (`directoryIpGroups`) was
   ephemeral.** Not included in `backendSnapshot`, so an association silently
   vanished across any Snapshot -> Restore cycle even though the IP group and
   directory it referenced both survived. Now persisted directly in
   `backendSnapshot.DirectoryIpGroups` alongside `Tags`; no snapshot-version
   bump needed since an older/absent value decodes as an empty map, matching
   the prior (always-empty-after-restart) behavior exactly.

7. **RegisterWorkspaceDirectory/DeregisterWorkspaceDirectory had no real
   state-conflict handling** — a leak-adjacent bug caught while auditing the
   directory family for the `directoryIpGroups` persistence fix above.
   `RegisterWorkspaceDirectory` was unconditionally idempotent (re-registering
   an already-registered directory silently 200'd); real AWS rejects this with
   `ResourceAlreadyExistsException`. `DeregisterWorkspaceDirectory` deleted the
   directory unconditionally even with live WorkSpaces still assigned to it —
   real AWS: *"If any WorkSpaces are registered to this directory, you must
   remove them before you can deregister the directory"* — which this backend
   now enforces via `InvalidResourceStateException` rather than either
   silently succeeding (leaving `DescribeWorkspaces` returning WorkSpaces whose
   `DirectoryId` no longer resolved to anything) or auto-cascade-deleting the
   WorkSpaces (which is *not* real AWS behavior — do not "fix" this into a
   cascade-delete). `handler.go`'s `handleError` gained two new sentinel
   mappings (`awserr.ErrAlreadyExists` -> 400 `ResourceAlreadyExistsException`,
   `awserr.ErrConflict` -> 400 `InvalidResourceStateException`) to support
   this — previously only `ErrNotFound`/`ErrInvalidParameter` were wired, so
   both of these new sentinels would otherwise have silently fallen through to
   a wrong 500 `InternalServerException`.

### Traps for the next auditor

- `RebootWorkspaces`/`RebuildWorkspaces` **intentionally** do not transition
  workspace state (no REBOOTING/REBUILDING window) — this was a deliberate design
  decision from a prior audit pass, documented and asserted by
  `TestRebootWorkspaces_DoesNotChangeState` /
  `TestRebuildWorkspaces_DoesNotChangeState` in workspaces_lifecycle_test.go
  (renamed from `handler_parity3_test.go`'s `TestParity3_*` names by an
  unrelated file-naming cleanup pass; same tests, same rationale).
  Do not "fix" this without reading that test's rationale first.
- CLOSED 2026-08-13: `workspaceResp` (`DescribeWorkspaces`'s per-item shape) carried
  a fabricated `Tags` JSON field. Evidence: `aws-sdk-go-v2/service/workspaces@v1.73.1`
  `types/types.go`, `type Workspace struct` -- exhaustive field list is
  `BundleId`/`ComputerName`/`DataReplicationSettings`/`DirectoryId`/`ErrorCode`/
  `ErrorMessage`/`IpAddress`/`ModificationStates`/`RelatedWorkspaces`/
  `RootVolumeEncryptionEnabled`/`StandbyWorkspacesProperties`/`State`/`SubnetId`/
  `UserName`/`UserVolumeEncryptionEnabled`/`VolumeEncryptionKey`/`WorkspaceId`/
  `WorkspaceName`/`WorkspaceProperties` -- no `Tags` member; real tags are read
  back only via a separate `DescribeTags` call. Deleted the field from
  `workspaceResp`/`toWorkspaceResp` (`handler_workspaces.go`); the internal
  `Workspace.Tags` domain field is untouched (still backs `DescribeTags`). `pendingWorkspace`
  (`CreateWorkspaces`'s pending-item shape) never had a `Tags` field in the first
  place, despite this note's prior claim otherwise -- re-verified against the current
  source, not assumed. Raw-body regression test: `TestDescribeWorkspaces_NoTagsField`
  (`tags_test.go`); `TestCreateTags_VisibleInDescribeWorkspaces`/
  `TestDeleteTags_RemovedFromDescribeWorkspaces` (which asserted the fabricated field)
  were rewritten to `TestCreateTags_VisibleInDescribeTags`/`TestDeleteTags_RemovedFromDescribeTags`,
  reading back through the real `DescribeTags` op instead.
  INVERSE FOUND at the time, since closed by later (undated-in-this-file) work:
  `DataReplicationSettings`/`IpAddress`/`RelatedWorkspaces` are real, backend-tracked
  data as of `CreateStandbyWorkspaces` (see that op's note) and `IpAddress` was always
  set at `CreateWorkspace` time; `ModificationStates`/`StandbyWorkspacesProperties`
  round-trip a real field but this backend has no code path that ever populates either
  (correct-by-absence, not fabricated -- no modification-tracking or extra
  standby-property feature exists to source them from). `WorkspaceName` was wired onto
  the wire (`workspaceResp`/`pendingWorkspace` both gained the JSON key) sometime after
  this note was written, but WITHOUT actually closing the gap this note described:
  `WorkspaceRequest.WorkspaceName` (the real *input* field) was still accepted by
  neither `createWorkspaceSpec` nor `WorkspaceCreationSpec`, AND the response side had
  independently started fabricating a value by echoing `UserName` (or the generated
  `WorkspaceId`) for every WorkSpace -- worse than the original silently-absent gap,
  since real `WorkspaceName` is documented as "not applicable if UserName is specified
  for user-assigned WorkSpaces" (a normal WorkSpace should never carry this key at all).
  Both closed 2026-08-23 -- see `CreateWorkspaces`/`DescribeWorkspaces` ops rows and the
  dated Notes section below. This is the "claims wrong in both directions" trap this
  campaign's task brief warns about: the field went from under-implemented to
  over-implemented (fabricated) without a PARITY.md update either time.
- `CreateIpGroup`/`DescribeIpGroups`/etc use **lowercase** wire keys (`groupId`,
  `groupName`, `groupDesc`, `userRules`, `ipRule`, `ruleDesc`) — this looks wrong
  at a glance (every other shape in this service is PascalCase) but is verified
  correct against the real `awsAwsjson11_deserializeDocumentWorkspacesIpGroup` /
  `IpRuleItem` deserializers. Real AWS quirk, not a bug — don't "fix" the casing.
- `CreateStandbyWorkspaces`' `PrimaryWorkspaceId` is accepted and echoed back on
  failure, but its existence is **not** cross-validated against any workspace
  table. This is deliberate, not an oversight: `CreateStandbyWorkspaces` runs in
  the *standby* (target) region to create a DR copy of a WorkSpace whose
  `PrimaryWorkspaceId` lives in a *different* region's backend instance — this
  in-memory backend has no cross-region visibility, so there is nothing correct
  to validate against. Only `DirectoryId` (which must be registered in *this*
  region) is validated.
- `DescribeImageAssociations`/`DescribeBundleAssociations` will always return an
  empty `Associations` list in this backend — this is correct, not a stub. Real
  AWS's WorkSpaces Application Manager has no public API to create an
  image/bundle<->application association at all (only
  `AssociateWorkspaceApplication`, which is WorkSpace-only). Don't "fix" this by
  inventing a fake association-creation pathway.

## 2026-08-30 (gopherstack-4shm WrapOp request-field re-scan, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

This service dispatches every op through `service.WrapOp` (91 entries,
`GetSupportedOperations` derived from `h.ops`'s own keys at runtime). A
field scan anchored on literal decode calls alone -- what an earlier pass's
"0 of 90 request shapes flagged" verdict was measured against -- resolves
**0 of 91 operations (0%)**: this service was entirely invisible to that
method, gopherstack-4shm's exact class, and the prior clean verdict was
measuring nothing at all.

The new `cmd/reqfieldscan` tool reaches **91 of 91 (100%)**, 218 fields
across 91 distinct request types, and found **6 unread fields, 3 real bugs,
2 fixed this pass**:

- **`CreateWorkspaceBundleInput.UserStorage`/`RootStorage`**
  (workspaces@v1.73.1 `api_op_CreateWorkspaceBundle.go`: `UserStorage` is
  "This member is required") were decoded and dropped entirely --
  `storedCustomBundle` had no field to hold them at all, and every custom
  bundle silently reported an empty `Capacity` string regardless of what
  was requested, even though the seeded default bundles (PowerPro,
  Performance, ...) already populate and marshal these same
  `UserStorage`/`RootStorage` output fields correctly. Fixed: added
  `UserStorageGiB`/`RootStorageGiB int32` to `storedCustomBundle`, threaded
  `Capacity` string parsing (`storageCapacityGiB`, `ParseInt` base 10, bit
  size 32 -- not `Atoi`+cast, which `gosec` correctly flags as a possible
  overflow) through `CreateWorkspaceBundle`, and populated the response.
  New test `TestCreateWorkspaceBundle_StoresStorageCapacity`
  (`bundles_test.go`) confirmed failing (`""` instead of `"50"`/`"80"`)
  against unmodified code, then passing.
- **`RegisterWorkspaceDirectoryInput.Tags`** was decoded and dropped
  entirely -- every sibling `Create*` op in this package (connection alias,
  IP group, bundle, image, pool, nested workspace tags) already applies its
  `Tags` via the shared `b.tags` map (`TestCreateOpsWithTags_RoundTrip`,
  `handler_create_tags_test.go`), but `RegisterWorkspaceDirectory` never
  did. Fixed by mirroring that established pattern
  (`b.tags[directoryID] = cloneTags(tags)`). Extended
  `TestCreateOpsWithTags_RoundTrip` with a `"workspace directory"` subtest
  (real SDK client, asserts on `DescribeTags`'s decoded `TagList`) --
  confirmed failing against unmodified code, then passing; the other 6
  subtests in that same test function were unaffected (still pass).
- **`RegisterWorkspaceDirectoryInput.EnableSelfService`** is also decoded
  and dropped. NOT fixed this pass: the real field is a single bool toggle,
  while this backend already models self-service as the fine-grained
  `SelfservicePermissions` struct (5 independent members, set later via
  `ModifySelfservicePermissions`) -- mapping one bool onto five named
  permissions needs a semantic decision (which permissions does "enabled"
  actually turn on?) this pass didn't have grounds to make. Left for a
  follow-up with that decision made explicit.

**3 unread fields left unfixed, judged not to be bugs or out of scope for
this pass**:
- `CreateAccountLinkInvitationInput.ClientToken` -- a standard AWS
  idempotency token; this backend follows the same convention as its other
  `Create*` ops in never enforcing idempotency-token semantics.
- `DescribeWorkspaceSnapshotsInput.WorkspaceId` -- `handleDescribeWorkspaceSnapshots`
  is a full stub (`return &describeWorkspaceSnapshotsOutput{RebuildSnapshots:
  []any{}, RestoreSnapshots: []any{}}, nil`, no backend call at all); this
  backend has no snapshot data model anywhere to report from. A real fix is
  a feature addition (a snapshot store), not a field-wiring fix, and is
  left as a known stub rather than attempted here.
- `RegisterWorkspaceDirectoryInput.EnableSelfService` -- see above.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run`
-- all clean (`./services/workspaces/...` and `./cmd/reqfieldscan/...`).
