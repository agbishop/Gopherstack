---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: organizations
sdk_module: aws-sdk-go-v2/service/organizations@v1.53.5
last_audit_commit: 012f98aa
last_audit_date: 2026-08-30
overall: A            # 2026-08-30 (ordering pass): audited every List op's sort key against its actual
                      # unsorted source for tie-safety (Table.All() map walks are unspecified-order; a
                      # sort with no total-order comparator leaves ties to depend on that unspecified
                      # order, varying call to call, which page.New's index-based cursor can't tolerate --
                      # same bug class already fixed across cloudwatchlogs this same branch). Found and
                      # fixed 2: ListPolicies (sorted by PolicySummary.Name alone; CreatePolicy enforces no
                      # name uniqueness, so two same-type policies can tie -- added PolicySummary.ID as a
                      # secondary key) and ListDelegatedAdministrators' unfiltered branch (sorted by
                      # AccountID alone, but the table is keyed by ServicePrincipal+AccountID, so one
                      # account delegated for multiple services ties -- added ServicePrincipal as a
                      # secondary key). Every other List/Describe op's sort key was checked against its
                      # own source and confirmed already a total order over that source: ListAccounts/
                      # ListAccountsForParent (Account.ID, the table's own primary key), ListOrganizational-
                      # UnitsForParent (OrganizationalUnit.Name, sibling-name uniqueness enforced at
                      # CreateOrganizationalUnit), ListHandshakesForAccount/ListHandshakesForOrganization
                      # (Handshake.ID), ListAWSServiceAccessForOrganization (ServicePrincipal, the table's
                      # own primary key), ListTagsForResource (Tag.Key, map keys are inherently unique),
                      # ListTargetsForPolicy (PolicyTargetSummary.TargetID, sourced from a
                      # map[string][]string slice value -- deterministic insertion order, not a map walk,
                      # so no sort-totality risk regardless of key uniqueness), ListPoliciesForTarget (no
                      # sort at all, but same deterministic-slice source as ListTargetsForPolicy). Both
                      # fixes proven via new pagination_sort_totality_test.go (TestListPoliciesSortIsTotal:
                      # a real paginated HTTP walk with 3 same-named policies repeated 30x, proving the
                      # drop/duplicate-across-page-boundary directly on the wire;
                      # TestListDelegatedAdministratorsOrderIsStableAcrossCalls: asserts backend-internal
                      # return-order stability directly, since the real DelegatedAdministrator wire type
                      # has no ServicePrincipal member to distinguish same-account rows by, so a wire-level
                      # drop/duplicate count can't observe this one), both hand-reverted and confirmed to
                      # fail against unfixed code. See the ListPolicies/ListDelegatedAdministrators ops:
                      # entries for detail.
                      # --- 2026-08-29 (cursor-population sweep, same day, separate pass from the constraint-
                      # not-honoured one below -- this one reads response SHAPES, not filter semantics):
                      # every List/Describe op declaring a real NextToken (20 of 28, from the pinned SDK
                      # Output structs directly) already populates it through the shared page.New helper
                      # (handler_*.go). Two exceptions, both provably bounded and correctly left as-is:
                      # ListParents (api_op_ListParents.go's own doc comment -- "In the current release, a
                      # child can have only a single parent" -- so its declared-but-unset NextToken can
                      # never observably matter) and ListRoots (this backend's ListRoots always returns
                      # exactly b.root, a single value, matching AWS's real one-root-per-organization
                      # model). ListEffectivePolicyValidationErrors/ListAccountsWithInvalidEffectivePolicy/
                      # ListInboundResponsibilityTransfers are also unpaginated, but their backends always
                      # return zero items (stub-shaped, a separate no-stub-rule concern, not a cursor bug)
                      # so the gap is equally unobservable today -- not fixed, no code changed.
                      # --- wrapper-key-sweep, constraint-not-honoured class (2026-08-29) history below, preserved ---
                      # 2026-08-29 (wrapper-key-sweep, constraint-not-honoured class): ListHandshakesForAccount/
                      # ListHandshakesForOrganization never read Filter.ParentHandshakeId at all --
                      # any client filtering by it got the full unfiltered handshake list back.
                      # Fixed; see the two ops: entries. Every other List op's own filter/pagination
                      # parameters (ListPolicies.Filter, ListPoliciesForTarget.Filter,
                      # ListDelegatedAdministrators.ServicePrincipal, ListCreateAccountStatus.States,
                      # ListChildren.ChildType, etc.) were checked against their SDK Input structs and
                      # confirmed already correctly applied.
                      # RESTORED prior pass (gopherstack-0m6h): the 5 sibling responsibility-transfer
                      # ops (DescribeResponsibilityTransfer/ListInboundResponsibilityTransfers/
                      # ListOutboundResponsibilityTransfers/TerminateResponsibilityTransfer/
                      # UpdateResponsibilityTransfer) that downgraded this to B now model the real,
                      # distinct types.ResponsibilityTransfer shape (new ResponsibilityTransfer domain
                      # type + store.Table + backend methods + wire DTOs), not a Handshake reused
                      # across a real type boundary. Everything else unchanged from the prior pass.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateOrganization: {wire: ok, errors: ok, state: fixed, persist: ok, note: "management account ID now == backend.AccountID() (caller identity), not a synthetic counter-derived ID -- was previously fabricating 000000000001 regardless of the configured/caller account, breaking cross-service account-identity consistency (e.g. vs STS)"}
  DescribeOrganization: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteOrganization: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects when non-management member accounts remain, matches AWS"}
  EnableAllFeatures: {wire: ok, errors: ok, state: ok, persist: ok, note: "returns a synthetic ENABLE_ALL_FEATURES handshake in ACCEPTED state (no real multi-account approval flow needed for single-account emulation)"}
  ListRoots: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAccount: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreateAccountStatus transitions straight to SUCCEEDED (no stuck IN_PROGRESS poll trap)"}
  CreateGovCloudAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeCreateAccountStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  ListCreateAccountStatus: {wire: fixed, errors: ok, state: ok, persist: ok, note: "MaxResults/NextToken were parsed into the request but never applied -- handler always returned the full unfiltered set. Now wired through pkgs/page.New."}
  DescribeAccount: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Account.Paths now populated -- computed at read time from accountParent/ouParent (org tree is already fully modeled), not stored; see gaps entry below for format verification"}
  ListAccounts: {wire: fixed, errors: ok, state: ok, persist: ok, note: "already paginated via pkgs/page; Paths now populated per-account same as DescribeAccount"}
  RemoveAccountFromOrganization: {wire: ok, errors: ok, state: fixed, persist: ok, note: "cascades policyTargets/tags/delegated-admin cleanup. FIXED (gopherstack-3ahs, b8484292f) -- also left emailToAccountID[email] behind, permanently blocking CreateAccount from reusing that email even though no account held it any more (a wrong-answer rejection, not the usual inherited-ghost-state shape); now cleared. Regression: TestBackend_RemoveAccountFromOrganization_FreesEmailForReuse."}
  MoveAccount: {wire: ok, errors: ok, state: ok, persist: ok, note: "validates current parent == SourceParentId and dest existence before mutating both index directions"}
  CloseAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateOrganizationalUnit: {wire: fixed, errors: ok, state: ok, persist: ok, note: "depth-limit (root=0, OUs 1-5) and O(1) sibling-name uniqueness enforced; Path now populated on the returned OU"}
  DescribeOrganizationalUnit: {wire: fixed, errors: ok, state: ok, persist: ok, note: "OrganizationalUnit.Path now populated, same read-time computation as Account.Paths"}
  DeleteOrganizationalUnit: {wire: ok, errors: ok, state: fixed, persist: ok, note: "rejects non-empty OUs (child accounts or child OUs); now also cleans the reverse policyTargets index on delete -- previously left the deleted OU's ID as a ghost entry in every attached policy's target list, so ListTargetsForPolicy kept reporting a deleted OU as a live target"}
  UpdateOrganizationalUnit: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Path now populated on the returned OU"}
  ListOrganizationalUnitsForParent: {wire: fixed, errors: ok, state: ok, persist: ok, note: "already paginated; Path now populated per-OU same as DescribeOrganizationalUnit"}
  ListAccountsForParent: {wire: fixed, errors: ok, state: ok, persist: ok, note: "request/response DTOs already declared MaxResults/NextToken but the handler ignored both and returned everything -- wired page.New to match sibling ops; Paths now populated per-account same as DescribeAccount"}
  ListParents: {wire: ok, errors: ok, state: ok, persist: ok}
  ListChildren: {wire: ok, errors: ok, state: ok, persist: ok, note: "already paginated"}
  CreatePolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "content validated: json.Valid() syntax check -> MalformedPolicyDocumentException, and per-policy-type DEFAULT size quota -> ConstraintViolationException(POLICY_CONTENT_LIMIT_EXCEEDED), all values verified against the live 'Maximum size of a policy document' row of orgs_reference_limits.html: SCP 10240 (was wrongly 5120, shared with RCP -- real bug, fixed), RCP 5120, TAG/BACKUP/DECLARATIVE_POLICY_EC2 10000, AISERVICES_OPT_OUT_POLICY 2500, CHATBOT_POLICY/SECURITYHUB_POLICY 10000 (now independently confirmed, was previously an unverified guess that happened to be correct). Tags param validated via validateNewTags before any mutation (see TagResource note)."}
  DescribePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "Content, when supplied, goes through the same validatePolicyContent() as CreatePolicy (syntax+size, corrected SCP/RCP limits) before ANY field (name/description/content) is mutated, matching AWS's atomic per-request failure semantics."}
  DeletePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects deletion while still attached to any target"}
  ListPolicies: {wire: ok, errors: ok, state: fixed, persist: ok, note: "requires non-empty Filter, matches AWS; already paginated. 2026-08-30 (ordering pass): sort key was PolicySummary.Name alone, sourced from b.policies.All() (store.Table map walk, unspecified order); CreatePolicy enforces no name-uniqueness (real AWS Organizations doesn't require unique policy names either), so two same-type policies can tie on Name -- an untied comparator leaves relative order to depend on map-walk order, which varies call to call, and page.New's index-based cursor assumes a stably-ordered slice across calls. Fixed by adding PolicySummary.ID as a secondary sort key. TestListPoliciesSortIsTotal (pagination_sort_totality_test.go) reproduces the drop/duplicate-across-page-boundary via a real paginated HTTP walk with 3 same-named policies, repeated 30x; hand-reverted and confirmed to fail against unfixed code."}
  AttachPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "enforces AWS's 5-policies-per-type-per-target limit and duplicate-attachment rejection"}
  DetachPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPoliciesForTarget: {wire: fixed, errors: ok, state: ok, persist: ok, note: "MaxResults field was missing from the request DTO entirely and results were never truncated; added field + wired page.New"}
  ListTargetsForPolicy: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same gap as ListPoliciesForTarget -- fixed the same way"}
  EnablePolicyType: {wire: ok, errors: ok, state: fixed, persist: ok, note: "PolicyType now validated against validPolicyTypes() -- previously accepted any string and enabled it, more permissive than AWS's enum-constrained PolicyType"}
  DisablePolicyType: {wire: ok, errors: ok, state: fixed, persist: ok, note: "rejects disabling a type while policies of that type remain attached anywhere; PolicyType now validated against validPolicyTypes() (same fix as EnablePolicyType)"}
  TagResource: {wire: ok, errors: ok, state: fixed, persist: ok, note: "Tags validated (validateNewTags in tags.go) before merge: (1) key length 1-128 chars, value length 0-256 chars (TagKey/TagValue shape bounds, botocore organizations/2016-11-28) -> InvalidInputException -- NEW this pass, previously unvalidated; (2) reserved 'aws:' key prefix (case-insensitive) -> InvalidInputException(INVALID_SYSTEM_TAGS_PARAMETER); (3) duplicate key within one request's Tags list -> InvalidInputException(DUPLICATE_TAG_KEY); (4) resulting tag count > 50 (AWS's documented per-resource limit) -> ConstraintViolationException(MAX_TAG_LIMIT_EXCEEDED). Same validation gates CreateAccount/CreateGovCloudAccount/CreateOrganizationalUnit/CreatePolicy's Tags parameter, called before any resource is created so a bad tag list leaves nothing behind (matches AWS's 'the entire request fails' doc note on those Tags params)."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: fixed, errors: ok, state: ok, persist: ok, note: "response DTO already declared NextToken but it was never populated and the request had no NextToken field at all (real AWS ListTagsForResource paginates via NextToken, no MaxResults param); added the field and wired page.New with the service default page size"}
  EnableAWSServiceAccess: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableAWSServiceAccess: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAWSServiceAccessForOrganization: {wire: fixed, errors: ok, state: ok, persist: ok, note: "handler previously discarded the request body entirely (`_ []byte`), so MaxResults/NextToken were unreachable; added listAWSServiceAccessRequest + page.New wiring, guarded for empty body (matches ListHandshakesForAccount's pattern) since real SDK clients still send at least '{}'"}
  RegisterDelegatedAdministrator: {wire: ok, errors: ok, state: ok, persist: ok, note: "requires EnableAWSServiceAccess first, matches AWS's ErrServiceNotEnabled behavior"}
  DeregisterDelegatedAdministrator: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDelegatedAdministrators: {wire: fixed, errors: ok, state: ok, persist: ok, note: "MaxResults field missing from request DTO, results never truncated; added field + wired page.New. 2026-08-30 (ordering pass): the unfiltered branch (ServicePrincipal==\"\") sources from b.delegatedAdmins.All() (store.Table map walk), keyed by ServicePrincipal+AccountID (delegatedAdminKeyFn), NOT by AccountID alone -- so a single account registered as delegated admin for multiple different service principals (RegisterDelegatedAdministrator only rejects a duplicate servicePrincipal+accountID pair, never a repeat AccountID across services) produces multiple DelegatedAdmin rows tied on AccountID under a sort keyed on AccountID alone. Real types.DelegatedAdministrator (organizations@v1.53.5 types/types.go:192) has no ServicePrincipal member, so this can't be proven through the wire response the way ListPolicies' sibling bug can (every row for one account is AccountID-indistinguishable, and the table's total entry count doesn't change with reordering, so a wire-level drop/duplicate count is unaffected either way) -- proven instead via TestListDelegatedAdministratorsOrderIsStableAcrossCalls asserting InMemoryBackend.ListDelegatedAdministrators(\"\")'s own return order (via the exported, wire-excluded DelegatedAdmin.ServicePrincipal field) is identical across repeated calls with nothing changed in between, which page.New's index-based cursor requires and the map-walk source doesn't provide unaided. Fixed by adding ServicePrincipal as a secondary sort key. Hand-reverted and confirmed to fail against unfixed code."}
  ListDelegatedServicesForAccount: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same gap, fixed the same way"}
  AcceptHandshake: {wire: ok, errors: ok, state: ok, persist: ok}
  CancelHandshake: {wire: ok, errors: ok, state: ok, persist: ok}
  DeclineHandshake: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeHandshake: {wire: ok, errors: ok, state: ok, persist: ok}
  InviteAccountToOrganization: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-i8lo (2026-08-22): Target.Type (HandshakeParty, organizations@v1.53.5 types/types.go:420, required alongside Id:415) was decoded but never validated -- only Target.Id was checked. Now rejects a missing Target.Type with InvalidInputException."}
  LeaveOrganization: {wire: ok, errors: ok, state: ok, persist: ok}
  ListHandshakesForAccount: {wire: fixed, errors: ok, state: ok, persist: ok, note: "already paginated; empty-body-tolerant parsing pattern (len(body)>0 guard) reused for the new ListAWSServiceAccessForOrganization fix. 2026-08-29 (wrapper-key-sweep): Filter.ParentHandshakeId (types.HandshakeFilter.ParentHandshakeId, organizations@v1.53.5 types/types.go:390 -- \"only used for handshake types that are a child of another type\") was missing from the wire struct entirely, so any client filtering by it silently got the full unfiltered list back. Added the field; since this backend never creates a handshake with a parent (EnableAllFeatures synthesizes a single already-ACCEPTED handshake rather than the real ENABLE_ALL_FEATURES/APPROVE_ALL_FEATURES parent/child flow), a non-empty ParentHandshakeId now correctly excludes everything -- see TestHandshakeFilter_Handler."}
  ListHandshakesForOrganization: {wire: fixed, errors: ok, state: ok, persist: ok, note: "already paginated. 2026-08-29 (wrapper-key-sweep): same Filter.ParentHandshakeId gap as ListHandshakesForAccount -- fixed the same way."}
  DeleteResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutResourcePolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "Content now capped at 40,000 chars, the ResourcePolicyContent shape's hard max (botocore organizations/2016-11-28, not account-quota state like PolicyContent) -> ConstraintViolationException; was previously unbounded"}
  DescribeEffectivePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "walks the OU/root policy chain and merges per policy-type semantics (SCP intersection-style vs tag-style override)"}
  ListAccountsWithInvalidEffectivePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "correctly always empty -- this backend performs no policy-schema validation so no account can ever have an invalid effective policy; NOT a stub, a correct void result (parity-principles rule 4)"}
  ListEffectivePolicyValidationErrors: {wire: ok, errors: ok, state: ok, persist: ok, note: "same as above -- correct void result, not a stub"}
  DescribeResponsibilityTransfer: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-0m6h: now returns the real types.ResponsibilityTransfer shape (ActiveHandshakeId/Arn/EndTimestamp/Id/Name/Source/StartTimestamp/Status/Target/Type) under the correct 'ResponsibilityTransfer' envelope key -- was 'HandshakeDetails' holding a Handshake-shaped body (handshakeObject/toHandshakeObject), decoding as all-zero beyond Id/Arn against a real SDK client. Also fixed on the input side: real Id is the transfer's own rt-... id (DescribeResponsibilityTransferInput.Id), not the underlying HandshakeId this backend previously required. Verified against awsAwsjson11_deserializeOpDocumentDescribeResponsibilityTransferOutput/awsAwsjson11_deserializeDocumentResponsibilityTransfer, deserializers.go, aws-sdk-go-v2/service/organizations@v1.53.5."}
  InviteOrganizationToTransferResponsibility: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-4ggy: request dropped SourceName/StartTimestamp/Type (3 of 4 required members) entirely -- now validated and stored as HandshakeResource entries (RESPONSIBILITY_TRANSFER/TRANSFER_START_TIMESTAMP/TRANSFER_TYPE), matching how InviteAccountToOrganization/EnableAllFeatures embed their own extra fields. Also fixed a pre-existing bug found by reading the whole shape: the created Handshake's Action was hardcoded to APPROVE_ALL_FEATURES (copy-paste from EnableAllFeatures) instead of the real TRANSFER_RESPONSIBILITY (types/enums.go ActionType). Output.Handshake genuinely is types.Handshake -- this op's wire shape was already correct, unlike its 5 (now-fixed) siblings. gopherstack-0m6h additionally makes this op create the backing ResponsibilityTransfer domain record the other 5 ops now read/write. gopherstack-i8lo (2026-08-22): Target.Type (HandshakeParty, organizations@v1.53.5 types/types.go:420, required) had the same gap as InviteAccountToOrganization -- only Target.Id was checked; now validated the same way."}
  ListInboundResponsibilityTransfers: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-0m6h: element shape fixed same as DescribeResponsibilityTransfer; request now requires Type (real ListInboundResponsibilityTransfersInput.Type is required) and accepts Id/MaxResults/NextToken, all previously unparsed (handler took no body at all). Still honestly always returns empty: InviteOrganizationToTransferResponsibility can only be called by the sending account (doc comment) and this single-account backend has no path to simulate a transfer received from elsewhere -- see PARITY notes."}
  ListOutboundResponsibilityTransfers: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-0m6h: element shape fixed same as above; request now requires/filters on Type and paginates via MaxResults/NextToken (previously unparsed). Backed by the same ResponsibilityTransfer records InviteOrganizationToTransferResponsibility now creates, kept in sync with the underlying Handshake's Accept/Cancel/Decline/expire transitions."}
  TerminateResponsibilityTransfer: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-0m6h: now takes/returns the real transfer Id (rt-...), not a HandshakeId -- different ID space, previously conflated. Real TerminateResponsibilityTransferInput.EndTimestamp (optional) is now honored, defaulting to now() when omitted. State machine added: only an ACCEPTED transfer can be terminated (InvalidResponsibilityTransferTransitionException) and a transfer already terminated is rejected (ResponsibilityTransferAlreadyInStatusException) -- both declared on this op's own deserializeOpErrorTerminateResponsibilityTransfer switch, deserializers.go. Previously this op silently canceled any OPEN handshake regardless of the real op's semantics."}
  UpdateResponsibilityTransfer: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-0m6h: real UpdateResponsibilityTransferInput takes Id+Name and renames the transfer (api_op_UpdateResponsibilityTransfer.go doc comment) -- this backend previously took HandshakeId+Action(ACCEPT/DECLINE), reusing AcceptHandshake/DeclineHandshake semantics that belong to a different op entirely. Now renames ResponsibilityTransfer.Name by its own Id; allowed at any Status since this op's error switch declares no transition-related exception (unlike Terminate's)."}
# Families audited as a group (when per-op is impractical):
families:
  error_table: {status: ok, note: "getErrorTable() in handler.go covers all 28 sentinel errors defined in backend.go one-to-one; no gap that would surface as a 500 InternalFailure for a known error condition"}
  persistence: {status: ok, note: "Handler exposes Snapshot(ctx)/Restore(ctx,[]byte) exactly, delegating to InMemoryBackend's own Snapshot/Restore -- correctly registered by cli.go's setupPersistence. Versioned snapshot format (organizationsSnapshotVersion) discards incompatible old snapshots cleanly instead of partially decoding them."}
  arn_shapes: {status: ok, note: "all ARNs built via pkgs/arn.Build, organization/account/root/ou/policy/resource-policy/handshake resource paths verified against real SDK doc comments (global service, no region segment)"}
  id_formats: {status: ok, note: "12-digit account IDs, ou- root- p- h- o- prefixes match AWS patterns"}
  timestamps: {status: ok, note: "epochSeconds(t) in models.go now delegates to pkgs/awstime.Epoch (was a local float64(t.Unix()) reimplementation that truncated sub-second precision). Wire shape (JSON number, epoch seconds) unchanged and still correct; this closes the reuse-hygiene gap flagged in the prior audit. RE-VERIFIED 2026-08-29 (dedicated timestamp-encoding pattern hunt): protocol confirmed JSON-RPC 1.1 (awsAwsjson11_* serializer prefix, organizations@v1.53.5); all 12 *time.Time members across the whole SDK package (Account.JoinedTimestamp, CreateAccountStatus.{Completed,Requested}Timestamp, DelegatedAdministrator.{DelegationEnabledDate,JoinedTimestamp}, DelegatedService.DelegationEnabledDate, EffectivePolicy.LastUpdatedTimestamp, EnabledServicePrincipal.DateEnabled, Handshake.{Expiration,Requested}Timestamp, ResponsibilityTransfer.{End,Start}Timestamp) confirmed against deserializers.go's smithytime.ParseEpochSeconds calls and gopherstack's float64 wire structs -- all correct, 12-of-12. Request-side StartTimestamp/EndTimestamp (InviteOrganizationToTransferResponsibility, TerminateResponsibilityTransfer) parsed via time.Unix(int64(req.Field), 0).UTC(), matching serializers.go's smithytime.FormatEpochSeconds encoding -- also correct. ListEffectivePolicyValidationErrorsOutput.EvaluationTimestamp (a 13th member, Output-struct-only, not in types.go) is never emitted -- correctly ABSENT, not this pass's scope: the op always returns an empty EffectivePolicyValidationErrors list (no validation engine modeled) and there is no genuine 'last evaluated' instant to report without fabricating one."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "ListAccountsWithInvalidEffectivePolicy / ListEffectivePolicyValidationErrors don't paginate (MaxResults/NextToken silently accepted-but-ignored in the same way the 6 fixed ops used to be), but both are provably always-empty results given no real policy-schema validation exists, so pagination there is moot until schema validation is implemented (no bd issue filed yet)"
  - "AWS auto-creates and attaches a default 'FullAWSAccess' SCP to the root when the SERVICE_CONTROL_POLICY policy type is enabled (or org created with ALL features); this backend does not fabricate that default policy, so ListPolicies/ListPoliciesForTarget won't show it. Deep AWS behavior detail, not flagged as broken since no client mutation is silently dropped -- documented here for the next auditor (no bd issue filed yet)"
  - "Policy content size limits are modeled at AWS's DEFAULT per-type quota only (SCP 10240, RCP 5120, TAG/BACKUP/DECLARATIVE_POLICY_EC2/CHATBOT_POLICY/SECURITYHUB_POLICY 10000, AISERVICES_OPT_OUT_POLICY 2500 -- all independently verified against the live orgs_reference_limits.html 'Maximum size of a policy document' table this pass, including the SCP default itself, which was previously wrong at 5120/shared with RCP and has been fixed); this backend does not model the service-quota-increase path (e.g. SCP up to 20480 via a quota request) since there is no quota-management API call being emulated here. A client that successfully requested a real quota increase would see this backend reject documents AWS would accept -- legitimately unmodeled account state, not a bug (no bd issue filed yet)."
  - "DescribeEffectivePolicy does not validate its policyType argument against AWS's EffectivePolicyType enum (a different, larger enum than PolicyType -- includes INSPECTOR_POLICY/UPGRADE_ROLLOUT_POLICY/BEDROCK_POLICY/S3_POLICY/NETWORK_SECURITY_DIRECTOR_POLICY, excludes SCP/RCP), so an unrecognized value falls through to ErrEffectivePolicyNotFound instead of AWS's InvalidInputException; unlike EnablePolicyType/DisablePolicyType (fixed this pass against the existing validPolicyTypes() allowlist), adding this correctly needs a second, distinct allowlist and was left alone to avoid guessing at one under time pressure (no bd issue filed yet)"
  - "FIXED (gopherstack-gt9o): Account.Paths and OrganizationalUnit.Path are now computed at read time in paths.go, not stored (organizationsSnapshotVersion stays 1 -- both are json:\"-\" on the domain structs, derived from the already-persisted accountParent/ouParent trees). Format verified against the live AWS API Reference example responses for DescribeAccount ('Paths': ['o-exampleorgid/r-examplerootid111/555555555555/']) and DescribeOrganizationalUnit ('Path': 'o-exampleorgid/r-examplerootid111/ou-examplerootid111-exampleouid111/'), and against both types' published regex (^(o-[a-z0-9]{10,32}/r-[0-9a-z]{4,32}(/ou-[0-9a-z]{4,32}-[a-z0-9]{8,32})*(/\\d{12})*)/) -- the aws-sdk-go-v2 v1.53.5 Go doc comments alone ('The paths in the organization where the account exists.') don't pin the format, so the API Reference examples were load-bearing. Paths is list-typed but every real AWS example (and gopherstack's own single-parent tree -- accounts move via MoveAccount between exactly one source and one destination, matching AWS's no-multi-parenting model) yields exactly one element; gopherstack always returns a 1-element slice, never fabricating a second entry. Populated on DescribeAccount/ListAccounts/ListAccountsForParent/DescribeOrganizationalUnit/UpdateOrganizationalUnit/ListOrganizationalUnitsForParent/CreateOrganizationalUnit (found by grepping every func returning *Account/[]*Account/*OrganizationalUnit/[]*OrganizationalUnit, not by trusting the gap's named list); ListAccountsWithInvalidEffectivePolicy is exempt since it's provably always-empty (see families/gaps above) and ListChildren/ListParents return ChildSummary/ParentSummary, which AWS itself doesn't put Path on. A detached (dangling parent reference) or cyclic ouParent chain -- unreachable through this backend's own API surface, only via a hand-edited/corrupted Restore snapshot -- deterministically yields nil Paths / empty Path (bounded maxPathWalk traversal, never loops) rather than a fabricated string."
  - "FIXED (gopherstack-0m6h): the 5-op Handshake-vs-ResponsibilityTransfer structural gap noted below in the notes section is resolved -- see the ops table above and the dedicated notes entry."
  - "ResponsibilityTransfer.Source/Target directionality: this single-account backend can only originate transfers as the Source (self) inviting a Target (the invited party) -- see ListInboundResponsibilityTransfers' note and the responsibilityTransferDirectionOutbound const's doc comment (handshakes.go). This is inferred from the ARN's documented inbound/outbound path segment and the ListInbound/ListOutbound doc prose (both cross-checked against docs.aws.amazon.com, not just the Go SDK, since the SDK alone doesn't state which side of a transfer the inviting account ends up on); a genuinely two-account harness could observe the other account's Inbound-side view and confirm this independently. No bd issue filed -- documented here as a judgment call, not a known bug."
deferred: []               # both previously-deferred items (policy content validation, tag validation)
                            # were implemented and field-diffed this pass -- see CreatePolicy/UpdatePolicy/
                            # TagResource notes above and the residual-limitation gaps listed above.
leaks: {status: clean, note: "no goroutines, timers, or background janitors in this service; expireStaleHandshakesLocked runs synchronously inside the write lock on relevant ops, not on a ticker"}
---

## Notes

Freeform: AWS-behavior specifics worth remembering, and any "looks-wrong-but-correct" traps
so the next auditor doesn't re-flag them.

- **Protocol**: `awsjson1.1` (single POST endpoint, `X-Amz-Target: AWSOrganizationsV20161128.<Op>`
  dispatch in `handler.go`'s `RouteMatcher`/`ExtractOperation`). `GetSupportedOperations()` was
  cross-checked 1:1 against every `api_op_*.go` file in
  `aws-sdk-go-v2/service/organizations@v1.50.4` -- all 61 real ops are routed and reachable
  (`dispatch` → `dispatchOrg`/`dispatchAccount`/`dispatchOU`/`dispatchPolicy`/`dispatchMisc`/
  `dispatchNewOps` → `dispatchHandshakeOps`/`dispatchTransferOps`). No missing ops, no orphaned
  stub registrations to clean up.

- **Real bug #1 (fixed) -- management account identity**: `CreateOrganization` previously
  minted the org's management account ID from a local counter (`newAccountID(1)` ==
  `"000000000001"`), completely independent of `b.accountID` (the account this backend was
  constructed with via `service.AccountRegionOrDefault`, i.e. whatever account other services
  in this same gopherstack instance report as the caller identity, e.g. STS
  `GetCallerIdentity`). Real AWS: the account that calls `CreateOrganization` *becomes* the
  management account -- same ID. A client cross-checking account identity across services
  (e.g. comparing `sts:GetCallerIdentity` against `organizations:DescribeOrganization`'s
  `MasterAccountId`, which Terraform's provider and many IaC tools do) would see a mismatch.
  Fixed: `mgmtAcctID := b.accountID`. `accountCounter` still starts at
  `managementAccountCounter` (1) afterward so member accounts get sequential 12-digit IDs
  unrelated to the (now real) management account ID -- no behavior change there.

- **Real bug #2 (fixed) -- systemic pagination-wiring gap**: 6 of the 8 non-trivial list ops
  (`ListAccountsForParent`, `ListPoliciesForTarget`, `ListTargetsForPolicy`,
  `ListDelegatedAdministrators`, `ListCreateAccountStatus`, `ListDelegatedServicesForAccount`)
  had response DTOs that already declared a `NextToken` field (clearly intended for
  pagination, matching the pattern used correctly by `ListAccounts`,
  `ListOrganizationalUnitsForParent`, `ListChildren`, `ListPolicies`,
  `ListHandshakesForAccount`, `ListHandshakesForOrganization`) but the handlers never
  populated it and never truncated at `MaxResults`, silently returning the entire unpaginated
  result set every time. `ListTagsForResource` and `ListAWSServiceAccessForOrganization` had
  the same gap plus were missing `NextToken`/`MaxResults` fields from their request DTOs
  entirely (`ListAWSServiceAccessForOrganization`'s handler didn't even parse the request
  body). Fixed all 8 by wiring `pkgs/page.New` the same way the already-correct sibling ops
  do. Added `Test_ListOps_HonorMaxResultsAndNextToken` in
  `handler_pagination_gaps_test.go` covering MaxResults truncation + NextToken continuation
  for each.

- **Trap for next auditor**: `ListAccountsWithInvalidEffectivePolicy` and
  `ListEffectivePolicyValidationErrors` always return empty slices. This is **correct**, not a
  stub -- this backend does no real JSON-schema policy validation, so no account or policy can
  ever be "invalid". Per parity-principles rule 4, confirmed by reading the backend method
  before flagging.

- **CreateAccount lifecycle**: `CreateAccountStatus.State` goes straight to `SUCCEEDED` in the
  same call (no stuck `IN_PROGRESS` that would make a real client poll
  `DescribeCreateAccountStatus` forever) -- correct emulator behavior for this bug class called
  out in parity-principles.

- **Error table**: `getErrorTable()` (handler.go) has a 1:1 entry for all 28 sentinel errors
  declared in backend.go; verified there is no gap that would surface as a raw 500
  `InternalFailure` for a condition that should have a specific AWS exception code.

- **Real bug #3 (fixed) -- SCP given RCP's smaller default limit**: `policyContentMaxSize`
  matched `policyTypeSCP` and `policyTypeRCP` on the same case and returned a shared 5,120-char
  constant. The live orgs_reference_limits.html "Maximum size of a policy document" table gives
  SCP 10,240 and RCP 5,120 -- two different defaults. Effect: real AWS accepts an
  8,000-character SCP; this backend rejected it with
  `ConstraintViolationException(POLICY_CONTENT_LIMIT_EXCEEDED)` -- more restrictive than AWS.
  Split into `policyContentLimitSCP = 10240` / `policyContentLimitRCP = 5120`.
  `TestCreatePolicy_ContentSizeLimit`'s own `scp` case previously asserted the wrong (5,120)
  boundary -- exactly the "test written against gopherstack's own broken output" trap; fixed
  along with the code.

- **Real bug #4 (fixed) -- tag key/value string-length limits unvalidated**: only tag count
  (50/resource), duplicate-key, and `aws:`-prefix were enforced; the `TagKey`/`TagValue` shapes'
  own length bounds (botocore `organizations/2016-11-28/service-2.json.gz`: `TagKey` min 1/max
  128, `TagValue` min 0/max 256) were not. A 129-character key or a 257-character value that
  real AWS rejects was silently accepted and persisted. Added length checks to
  `validateNewTags` (tags.go), ahead of the existing prefix/duplicate/count checks so an invalid
  key/value still leaves the target unmutated.

- **Real bug #5 (fixed) -- PutResourcePolicy had no size cap**: `ResourcePolicyContent` (a
  *different* shape from `PolicyContent`) carries a hard `max: 40000` in the botocore model --
  unlike `PolicyContent`, this one is a real shape constraint, not account-quota state, and
  matches the docs page's "Maximum size of the resource-based delegation policy: 40,000
  characters" row exactly. `PutResourcePolicy` accepted content of any size. Added the check.

- **Real bug #6 (fixed) -- EnablePolicyType/DisablePolicyType accepted any string**: unlike
  `CreatePolicy`, neither validated `PolicyType` against `validPolicyTypes()`, so a garbage
  policy-type string would be silently "enabled" and stored on the root's `PolicyTypes` list --
  more permissive than AWS's enum-constrained `PolicyType`. Added the same
  `slices.Contains(validPolicyTypes(), policyType)` check `CreatePolicy` already used.

- **Real bug #7 (fixed) -- DeleteOrganizationalUnit left a ghost policy target**: deletion
  cleared the OU's own `targetPolicies` entry (OU -> attached policy IDs) but never walked those
  policies' `policyTargets` entries (policy -> attached target IDs) to remove the OU, unlike
  `RemoveAccountFromOrganization`, which does both directions. Effect: `ListTargetsForPolicy`
  kept listing the deleted OU as an attached target indefinitely (with an empty
  name/ARN, misreported as `Type: "ACCOUNT"` by `resolveTargetSummary`'s fallback branch).
  Fixed by mirroring `RemoveAccountFromOrganization`'s reverse-index cleanup.

- **Persistence**: `Handler.Snapshot`/`Restore` delegate directly to
  `InMemoryBackend.Snapshot`/`Restore` with the exact method signatures
  `Snapshot(ctx context.Context) []byte` / `Restore(ctx context.Context, []byte) error` that
  `cli.go`'s `setupPersistence` requires -- correctly registered, not the silent-unregistration
  bug class fixed elsewhere in the ~12-service sweep. Snapshot format is versioned
  (`organizationsSnapshotVersion`) and discards incompatible snapshots cleanly rather than
  partially decoding them.

- **This pass (2026-07-23) -- closed both previously-deferred items**:
  1. **Policy content validation** (`policies.go`): `CreatePolicy`/`UpdatePolicy` now run
     `validatePolicyContent(content, policyType)` before mutating any state.
     `json.Valid([]byte(content))` catches non-JSON content ->
     `MalformedPolicyDocumentException` (real AWS exception type, field-diffed against
     `types.MalformedPolicyDocumentException` in the SDK). A per-policy-type character-count
     ceiling (SCP/RCP 5120, TAG/BACKUP/DECLARATIVE_POLICY_EC2 10000,
     AISERVICES_OPT_OUT_POLICY 2500, per the Organizations quotas reference; CHATBOT_POLICY/
     SECURITYHUB_POLICY default to 10000 as an unverified best-effort value -- see gaps) ->
     `ConstraintViolationException` with `Reason: POLICY_CONTENT_LIMIT_EXCEEDED`, matching
     `types.ConstraintViolationExceptionReasonPolicyContentLimitExceeded` in the SDK enum.
     `UpdatePolicy` validates content *before* applying name/description so a rejected update
     doesn't partially mutate the policy (`TestUpdatePolicy_MalformedContent` asserts this).
  2. **Tag validation** (`tags.go`'s new `validateNewTags` helper, shared by `TagResource`,
     `CreateAccount`, `CreateGovCloudAccount`, `CreateOrganizationalUnit`, and `CreatePolicy`):
     rejects tag keys with the case-insensitive `aws:` reserved prefix
     (`InvalidInputException`/`INVALID_SYSTEM_TAGS_PARAMETER`, matching
     `types.InvalidInputExceptionReasonInvalidSystemTagsParameter`), rejects a duplicate key
     within one call's tag list (`InvalidInputException`/`DUPLICATE_TAG_KEY`, matching
     `types.InvalidInputExceptionReasonDuplicateTagKey`), and enforces AWS's documented
     50-tags-per-resource cap against the *merged* (existing + new) key set
     (`ConstraintViolationException`/`MAX_TAG_LIMIT_EXCEEDED`, matching
     `types.ConstraintViolationExceptionReasonMaxTagLimitExceeded`). Validation runs before any
     resource is created/mutated, so a rejected Tags parameter leaves nothing behind --
     verified by `TestCreateAccount_ReservedTagPrefixRejected`,
     `TestCreateOrganizationalUnit_DuplicateTagKeyRejected`,
     `TestCreatePolicy_ReservedTagPrefixRejected`, and
     `TestTagResource_MaxTagLimitExceeded_AcrossCalls`.
  3. **Reuse-hygiene**: `epochSeconds()` in `models.go` now delegates to `pkgs/awstime.Epoch`
     instead of reimplementing `float64(t.Unix())` (closes the prior audit's flagged gap;
     sub-second precision is now preserved, though every call site in this service only ever
     passes whole-second `time.Now()` values so there's no observable wire-format change).
  4. Not modeled (see `gaps`): the service-quota-increase path for policy size (a client that
     requested and received a real SCP size-limit increase would see this backend reject
     documents AWS would now accept), and per-tag key/value length limits (only count,
     duplicate-key, and reserved-prefix are enforced).

- **Real bug #8 (fixed, gopherstack-0m6h) -- 5 responsibility-transfer ops wired a Handshake
  where AWS uses a distinct type**: `DescribeResponsibilityTransfer`,
  `ListInboundResponsibilityTransfers`, `ListOutboundResponsibilityTransfers`,
  `TerminateResponsibilityTransfer`, and `UpdateResponsibilityTransfer` all serialized a
  `handshakeObject` under a `HandshakeDetails`/`ResponsibilityTransfers` key. Real AWS's element
  type there is `types.ResponsibilityTransfer` (`ActiveHandshakeId`/`Arn`/`EndTimestamp`/`Id`/
  `Name`/`Source`/`StartTimestamp`/`Status`/`Target`/`Type`,
  `awsAwsjson11_deserializeDocumentResponsibilityTransfer`, deserializers.go) -- a real SDK
  client decoded only the two overlapping key names (`Id`, `Arn`) and left everything else zero.
  Fixed with a full structural rebuild: a new `ResponsibilityTransfer`/`TransferParticipant`
  domain type (models.go), a `store.Table[ResponsibilityTransfer]` +
  `responsibilityTransfersByHandshake` secondary index (store_setup.go), new
  `responsibilityTransferObject`/`transferParticipantObject` wire DTOs, and backend methods
  keyed by the transfer's own `rt-...` `Id` (a distinct ID space from the `h-...` `HandshakeId`
  these ops previously, incorrectly, took as input).
  - `InviteOrganizationToTransferResponsibility` (already correct, unchanged wire shape) now
    additionally creates the backing `ResponsibilityTransfer` record (`Status: REQUESTED`,
    `Source`=self, `Target`=invited party) alongside the `Handshake` it already created.
  - `AcceptHandshake`/`CancelHandshake`/`DeclineHandshake`/the lazy-expiry sweep now sync a
    `TRANSFER_RESPONSIBILITY` handshake's state transition onto its linked
    `ResponsibilityTransfer.Status` (`syncResponsibilityTransferStatusLocked`, handshakes.go) --
    `HandshakeState`'s `OPEN` maps to `ResponsibilityTransferStatus`'s `REQUESTED`, every other
    value is shared verbatim between the two enums (types/enums.go).
  - `TerminateResponsibilityTransfer` gained a real state machine it never had as a
    disguised `CancelHandshake`: only an `ACCEPTED` transfer can be terminated
    (`InvalidResponsibilityTransferTransitionException`), and a transfer that already has an
    `EndTimestamp` can't be terminated again (`ResponsibilityTransferAlreadyInStatusException`)
    -- both exceptions are declared on this op's own
    `deserializeOpErrorTerminateResponsibilityTransfer` switch (deserializers.go), distinguishing
    the two failure modes. `EndTimestamp` (optional input) defaults to `time.Now()` when omitted;
    absent otherwise (never fabricated for a transfer that hasn't ended).
  - `UpdateResponsibilityTransfer` changed semantics entirely: real AWS only renames the transfer
    (`Id`+`Name`, `api_op_UpdateResponsibilityTransfer.go`'s doc comment); this backend previously
    hijacked `HandshakeId`+`Action(ACCEPT/DECLINE)`, which belongs to `AcceptHandshake`/
    `DeclineHandshake`. Renaming is allowed at any `Status` since that op's error switch declares
    no transition-related exception (unlike `Terminate`'s).
  - `ResponsibilityTransfer.Arn`'s documented pattern
    (`arn:...:organizations::<account>:transfer/o-.../(billing)/(inbound|outbound)/rt-...`,
    verified against the live AWS API Reference page since the Go SDK carries no ARN pattern
    constants) encodes direction per-account: this backend can only ever produce the outbound
    side of a transfer (see the `gaps` entry above), matching the pre-existing, unchanged
    `ListInboundResponsibilityTransfers`/`ListOutboundResponsibilityTransfers` action-filter fix
    from gopherstack-4ggy.
  - Checked whether `ResponsibilityTransfer` and `Handshake` diverge anywhere else in this
    service (the prompt for this fix, given kinesis/cloudformation/cloudwatch hit the same
    sibling-type-confusion bug class the same day): `InviteOrganizationToTransferResponsibility`,
    `AcceptHandshake`, `CancelHandshake`, and `DeclineHandshake` all genuinely operate on/return
    `types.Handshake` in real AWS (confirmed via their own `api_op_*.go` Output structs) -- no
    further confusion found.
  - 4 new/rewritten tests drive the real `aws-sdk-go-v2` client end-to-end
    (`TestResponsibilityTransfer_RoundTrip`,
    `TestInviteOrganizationToTransferResponsibility_RoundTrip`'s updated backend-level assertions,
    `handshakes_test.go`'s `TestBackend_DescribeResponsibilityTransfer`/
    `TestBackend_ResponsibilityTransfer_Lifecycle`, `handler_handshakes_test.go`'s
    `TestHandler_DescribeResponsibilityTransfer`); confirmed by hand-reverting the envelope key
    and the field-population that the round-trip test fails against the pre-fix shape (nil
    `ResponsibilityTransfer` / all-zero fields beyond `Id`/`Arn`).

### 2026-08-22 (gopherstack-i8lo): missing required-member validation

`HandshakeParty.Type` is marked `// This member is required.`
(`aws-sdk-go-v2/service/organizations@v1.53.5` `types/types.go:420`,
alongside `Id:415`, also required) and is checked by the real client's own
`validateHandshakeParty` (`validators.go:1189`) before a request is ever
sent. `InviteAccountToOrganization` and
`InviteOrganizationToTransferResponsibility` (`handler_handshakes.go`) both
decoded `Target.Type` but validated only `Target.Id`, so a request built by
hand (or by a client that skips local validation) with a Target missing
`Type` was silently accepted. Fixed by adding the same `== ""` rejection
already used for `Target.Id`, in both handlers.

No existing fixture supplied a `Target` with `Id` set but `Type` omitted
(all fixtures either provide both or omit the whole `Target`, which the
existing `Target.Id` check already caught) -- so no fixture was ratifying
this defect, but none tested it either. Added
`invite_missing_target_type` to `TestHandler_HandshakeErrors`
(`handler_handshakes_test.go`) and `"missing target type"` to
`TestHandler_InviteOrganizationToTransferResponsibility_MissingRequiredFields`
(`handler_transfer_responsibility_test.go`), both expecting `400`.

Confirmed via hand-revert: reverting `handler_handshakes.go` to `git show
HEAD:services/organizations/handler_handshakes.go` made both new subtests
fail (`expected: 400, actual: 200`); restored and `md5sum`-verified
identical to the fix.

The sweep's third reported item, glue `CreateDataCellsFilter`, does not
exist in this repo's `services/glue/` at all -- `CreateDataCellsFilter` is a
**Lake Formation** operation
(`aws-sdk-go-v2/service/lakeformation@v1.50.4/api_op_CreateDataCellsFilter.go`),
not Glue (`aws-sdk-go-v2/service/glue@v1.152.0` has no such op). The sweep
mislabeled the service; nothing to fix here, and `services/lakeformation/`
is outside this fix's scope.

## 2026-08-23 request-side accept-and-drop (gopherstack-n3zi)

InviteAccountToOrganization dropped Tags, a real body-bound member
(api_op_InviteAccountToOrganization.go:60-66). Accounts ARE taggable here, so
the account created by AcceptHandshake simply never received the invite-time
tags. Proven by a real-SDK-client round trip failing pre-fix with
"[]" should have 1 item(s), but has 0.

Gaps confirmed and NOT fixed, no backing state:
InviteOrganizationToTransferResponsibility's Tags and PutResourcePolicy's Tags
-- neither handshakes nor resource-policy IDs are registered taggable types
(resourceExistsLocked covers root, OU, account and policy only).

## 2026-09-07 (gopherstack-hg4i): no default FullAWSAccess policy modeled

Real AWS Organizations creates every organization with an AWS-managed SCP,
`p-FullAWSAccess`, attached to root (`api_op_DetachPolicy.go`'s doc: "Every
root, OU, and account must have at least one SCP attached. If you want to
replace the default FullAWSAccess policy with an SCP that limits the
permissions that can be delegated, you must attach the replacement SCP
before you can remove the default SCP"; `api_op_CreateOrganization.go`'s doc:
"By default (or if you set the FeatureSet parameter to ALL) ... service
control policies automatically enabled in the root"). This backend modeled
no such policy at all, and `CreatePolicy` hardcoded `AwsManaged: false` for
every policy it created -- correct for user-created policies, but there was
no path that ever produced an `AwsManaged: true` policy, so no code
anywhere could branch on it. The issue's premise that `DeletePolicy` had a
now-dead `AwsManaged` guard was itself wrong: `DeletePolicy` had no
`AwsManaged` check of any kind (dead or live) before this fix; `UpdatePolicy`
likewise had none, despite `types.PolicySummary.AwsManaged`'s doc comment
("you can attach the policy to roots, OUs, or accounts, but you cannot edit
it") only ever describing edit restrictions, not deletion.

Fixed by seeding `p-FullAWSAccess` (Name `FullAWSAccess`, Type
`SERVICE_CONTROL_POLICY`, `AwsManaged: true`) at `CreateOrganization`,
attached to root when `FeatureSet` is `ALL` (`seedFullAWSAccessPolicyLocked`,
policies.go) -- the SDK ships no policy id/name/content defaults of its own,
so the id/name/description match live `describe-policy` output and the
content is AWS's documented full-access SCP body, not SDK-verified. Added an
`AwsManaged` guard to both `DeletePolicy` and `UpdatePolicy`, returning
`AccessDeniedException` (`ErrAccessDeniedManagedPolicy`): neither op's
declared error set (`deserializers.go`) includes
`ConstraintViolationException`, and no `ConstraintViolationExceptionReason`
enum value fits "AWS-managed policy" either, so `AccessDeniedException`
(declared on both) is the only fit. `DetachPolicy` was left unguarded --
its own doc describes detaching the default SCP as a normal, supported
step in replacing it, not a restricted operation.

Seeding a real policy on every organization changed what `ListPolicies`,
`ListPoliciesForTarget`, and `DescribeEffectivePolicy` return by default,
and shifted the 5-SCP-per-target attachment ceiling on root down by one
already-occupied slot. 15 pre-existing tests needed correcting for the new
count/content (not weakened -- exact counts bumped, e.g. "attach 5 new SCPs
to root succeeds" became "attach 4 new SCPs succeeds, the 5th fails", and
`DescribeEffectivePolicy`'s "no policy attached returns not-found" case for
SCP specifically was rewritten to assert it now correctly resolves to the
inherited default instead, since real AWS never returns not-found for SCP
once an org exists). New coverage added in `default_policy_test.go`, all
HTTP-handler-driven: default policy identity/AwsManaged, its root
attachment via `ListPoliciesForTarget`, delete/update refusal with survivor
checks, and a too-broad-guard tripwire (a user-created policy must remain
`AwsManaged: false` and stay deletable). All new guard lines and the seed
call were individually neutered (commented out / forced true) and confirmed
to (a) still compile and (b) fail at least one test each, including a
simulated always-refuse `DeletePolicy` guard, which multiple existing tests
and the tripwire both caught.

Not implemented: `EnablePolicyType`/root `PolicyTypes` are not
auto-populated with `SERVICE_CONTROL_POLICY: ENABLED` at org creation, even
though `CreateOrganization`'s own doc also promises that for `FeatureSet:
ALL`. Left alone deliberately -- multiple existing tests
(`TestPolicyTypes`) call `EnablePolicyType(rootID, "SERVICE_CONTROL_POLICY")`
and require `NoError`, which a pre-enabled root would break via
`ErrPolicyTypeAlreadyEnabled`; recommend a separate, explicitly-scoped issue
rather than folding it into this fix.

A snapshot restored from before this change carries no `p-FullAWSAccess`
policy (Restore does not seed one; only `CreateOrganization` does), so a
long-lived organization restored across this change has zero SCPs where a
fresh one would have one. No migration was built for this -- flagged for
the operator to decide.
