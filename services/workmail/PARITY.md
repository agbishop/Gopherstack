---
service: workmail
sdk_module: aws-sdk-go-v2/service/workmail@v1.39.4
last_audit_commit: dc877102
# 2026-08-30: pagination-tie sweep (separate from the cursor-population sweep below -- this one
# asks whether a name-sorted List op can lose or duplicate a record at a page boundary when two
# records tie on the sort key). Of the 16 backend List* methods, 14 source from a store.Index
# (*ByOrg.Get/byOrgEntity.Get), whose order does not vary between calls (pkgs/store/index.go),
# so a tie-prone sort (e.g. ListGroups/ListUsers/ListResources by Name, where the table's own key
# is GroupID/UserID/ResourceID, not Name) still cannot reorder or drop a record between two
# separate List calls. ListGroupMembers and ListResourceDelegates walk a raw
# map[orgID]map[parentID]map[childID]bool set and sort by that same childID (MemberID/
# DelegateID) -- since a Go map cannot hold two entries under one key, that sort can never tie
# regardless of iteration order. ListOrganizations is the one real map-walk
# (store.Table.All()) sorted by a field (Alias) other than the table's own key (OrgID): confirmed
# safe because CreateOrganization explicitly rejects a duplicate Alias
# (`b.orgsByAlias[alias]` check, organizations.go) before insert, so Alias is unique by
# construction. No fixes needed; 0 code changes. Existing tests construct only distinct
# names/aliases, so none could have exercised a tie even where one is possible in principle.
# 2026-08-30: cursor-population sweep (does every List response struct that DECLARES a NextToken
# actually SET one before the collection can exceed a page?). Enumerated all 15 SDK ops whose
# Input/Output declare NextToken (ListAliases, ListAvailabilityConfigurations, ListGroupMembers,
# ListGroupsForEntity, ListGroups, ListImpersonationRoles, ListMailboxExportJobs,
# ListMailboxPermissions, ListMailDomains, ListMobileDeviceAccessOverrides, ListOrganizations,
# ListPersonalAccessTokens, ListResourceDelegates, ListResources, ListUsers). Found genuinely
# clean: every one of the 15 backend methods sorts its result deterministically then delegates to
# a single shared `paginate[T any]` helper (store.go), and every one of the 15 handlers reads
# req.NextToken/MaxResults and returns the resulting token -- no exceptions, no bypasses, no
# handler that discards the params. workmail and ram both already had this shared-helper pattern
# and came back clean; workspaces had no such helper at all (10 bugs found), and mgn/cognitoidp
# each had one op that bypassed their otherwise-correct shared helper.
# No fixes needed this pass; 0 code changes.
overall: A            # 6 gaps + 1 (already-fixed, stale-labeled) deferred item closed; 1 real leak class fixed; banned nolint removed
                      # 2026-08-29: errcodeaudit ERROR-path sweep. 2 confident findings expanded on inspection: the shared ErrConflict sentinel (fabricated EntityAlreadyExistsException, a type this SDK defines nowhere) was used by 9 different creation ops, each modeling a DIFFERENT real code. Split into ErrNameUnavailable (NameAvailabilityException: CreateAvailabilityConfiguration/CreateGroup/CreateOrganization/CreateResource/CreateUser), ErrEmailInUse (EmailAddressInUseException: CreateAlias/RegisterToWorkMail), ErrMailDomainInUse (MailDomainInUseException: RegisterMailDomain). CreateImpersonationRole kept ErrConflict/the fabricated code: its own model defines no AlreadyExists exception at all, no replacement invented. The default-500 "InternalServiceError" code left unchanged: WorkMail models no generic internal-error type across all 92 ops, so no code choice here is ever errors.As-matchable regardless. 4 existing tests (handler_users_test.go, handler_aliases_test.go, handler_organizations_test.go, handler_availability_config_test.go) previously asserted the fabricated "AlreadyExists"-shaped string as correct; corrected. EnableInteroperability (this pass's assigned follow-up check): independently reverified already fixed (gopherstack-sm09, closed same day) -- CreateOrganization threads it onto InteroperabilityEnabled and DescribeOrganization echoes it back correctly.
ops:
  CreateOrganization: {wire: ok, errors: ok, state: ok, persist: ok, note: "Domains was []string; real wire is [{DomainName,HostedZoneId}] objects -- json.Unmarshal failed for any client-specified domain (500 InternalServiceError). Fixed (prior pass). Default + client-specified domains now also populate DkimVerificationStatus/Records (see GetMailDomain). errcodeaudit 2026-08-29 FIX: duplicate-alias rejection emitted the fabricated EntityAlreadyExistsException; switched to the real NameAvailabilityException this op's own model defines."}
  DescribeOrganization: {wire: ok, errors: ok, state: ok, persist: ok, note: "added MigrationAdmin field (types.DescribeOrganizationOutput.MigrationAdmin). Field-diffed the whole SDK surface: no operation in aws-sdk-go-v2/service/workmail@v1.37.2 ever sets MigrationAdmin (it's populated out-of-band by an Exchange interoperability/migration flow this backend doesn't simulate), so it is correctly always empty/omitted -- matches every real org that never configured migration. Not a stub: the field is modeled and wired, it's just never non-empty because nothing in the real API's surface can make it non-empty either."}
  DeleteOrganization: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade-delete now also purges tags (org's own + every contained user/group/resource's, via ARN-prefix match) and globalAliases rows (primary emails + CreateAlias aliases) for the whole org -- previously both were left as permanent ghost rows post-delete (DeleteOrganization's own doc comment said tags were 'deliberately left untouched'). See leaks below."}
  ListOrganizations: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "now wires FirstName/LastName/IdentityProviderUserId/HiddenFromGlobalAddressList from CreateUserInput -- previously accepted on the wire but silently discarded (never reached the User struct, so DescribeUser could never surface them even before this pass' DescribeUser fix). errcodeaudit 2026-08-29 FIX: duplicate-name rejection emitted the fabricated EntityAlreadyExistsException (no such type in this SDK); switched to the real NameAvailabilityException CreateUser's own model defines. Verified via TestCreateUser_NameUnavailable (real client, errors.As)."}
  DescribeUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED: City/Company/Country/Department/Initials/JobTitle/Office/Street/Telephone/ZipCode/HiddenFromGlobalAddressList/IdentityProviderIdentityStoreId/IdentityProviderUserId/MailboxProvisionedDate/MailboxDeprovisionedDate all now modeled on User and wired through DescribeUser's response. MailboxProvisionedDate/MailboxDeprovisionedDate are set alongside EnabledDate/DisabledDate in RegisterToWorkMail/DeregisterFromWorkMail (real WorkMail provisions/deprovisions the mailbox at the same time it enables/disables WorkMail use)."}
  UpdateUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "now accepts City/Company/Country/Department/Initials/JobTitle/Office/Street/Telephone/ZipCode/IdentityProviderUserId/Role/HiddenFromGlobalAddressList (UpdateUserInput's full field set) -- previously only DisplayName/FirstName/LastName were wired."}
  DeleteUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "now cascade-cleans CreateAlias-created aliases + their globalAliases rows, mailbox permissions (as target entity AND as grantee), group memberships, resource delegate listings, mailboxQuotas, and tags -- see leaks below. Also now clears the user's primary email from globalAliases on delete (was previously the only one of the three entity types that skipped this; verified unreachable in practice since delete requires DISABLED state, which only follows DeregisterFromWorkMail, which already clears Email -- defensive fix for consistency with DeleteGroup/DeleteResource, not a live bug)."}
  ListUsers: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED: Filters (DisplayNamePrefix/PrimaryEmailPrefix/State/UsernamePrefix/IdentityProviderUserIdPrefix) now filter the result set (userMatchesFilter in users.go); previously accepted on the wire but silently ignored, returning the full unfiltered page."}
  RegisterToWorkMail: {wire: ok, errors: partial, state: ok, persist: ok, note: "verified real ENABLED transition + EnabledDate + email index writes, not a disguised no-op. Now also sets MailboxProvisionedDate for users. errcodeaudit 2026-08-29 FIX: email-in-use rejection emitted the fabricated EntityAlreadyExistsException; switched to the real EmailAddressInUseException, matching its own model and doc ('the email address ... is already created for a different user, group, or resource'). Verified via TestRegisterToWorkMail_EmailInUse. NOTED, not fixed: the op never checks whether the target entity itself is already registered (real WorkMail: 'performs no change if enabled, fails if deleted') before silently re-associating email -- a missing-validation gap, separate from the error-code bug, not fixed in this pass."}
  DeregisterFromWorkMail: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified real DISABLED transition + EnabledDate cleared. Now also sets MailboxDeprovisionedDate for users."}
  ResetPassword: {wire: ok, errors: ok, state: ok, persist: ok, note: "password intentionally not stored (matches other gopherstack auth-adjacent ops); existence is still validated."}
  GetMailboxDetails: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateMailboxQuota: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePrimaryEmailAddress: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "errcodeaudit 2026-08-29 FIX: duplicate-name rejection emitted the fabricated EntityAlreadyExistsException; switched to the real NameAvailabilityException CreateGroup's own model defines."}
  DescribeGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "now cascade-cleans aliases/globalAliases/permissions(target+grantee)/group-memberships-of-others/resource-delegate-listings/tags via the same cascadeCleanEntity helper DeleteUser/DeleteResource use -- see leaks below."}
  ListGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED: Filters (NamePrefix/PrimaryEmailPrefix/State) now filter the result set (groupMatchesFilter in groups.go); previously accepted but ignored."}
  AssociateMemberToGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateMemberFromGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ListGroupMembers: {wire: ok, errors: ok, state: ok, persist: ok}
  ListGroupsForEntity: {wire: ok, errors: ok, state: ok, persist: ok, note: "response reused the ListGroups item shape (Id/Name/Email/State); real shape is types.GroupIdentifier (GroupId/GroupName only) -- every field the SDK actually reads was zero-valued. Fixed with a dedicated groupIdentifierResp type (prior pass). GAP CLOSED this pass: Filters.GroupNamePrefix (the op's single filter dimension) now filters the result set; previously accepted but ignored."}
  CreateResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "errcodeaudit 2026-08-29 FIX: duplicate-name rejection emitted the fabricated EntityAlreadyExistsException; switched to the real NameAvailabilityException CreateResource's own model defines."}
  DescribeResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "BookingOptions / HiddenFromGlobalAddressList not modeled -- gap, not in this pass' declared 6; see gaps below."}
  UpdateResource: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "now cascade-cleans aliases/globalAliases/permissions(target+grantee)/group-memberships/other-resources'-delegate-listings/tags via cascadeCleanEntity -- see leaks below."}
  ListResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED: Filters (NamePrefix/PrimaryEmailPrefix/State) now filter the result set (resourceMatchesFilter in resources.go); previously accepted but ignored."}
  AssociateDelegateToResource: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateDelegateFromResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListResourceDelegates: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAlias: {wire: ok, errors: ok, state: ok, persist: ok, note: "errcodeaudit 2026-08-29 FIX: alias-in-use rejection emitted the fabricated EntityAlreadyExistsException; switched to the real EmailAddressInUseException CreateAlias's own model defines. gopherstack-gmny (2026-09-06): now enforces the documented, non-adjustable 100-aliases-per-user quota (maxAliasesPerUser, aliases.go) -- see LimitExceededException gap note below for sourcing."}
  DeleteAlias: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAliases: {wire: ok, errors: ok, state: ok, persist: ok, note: "primary email correctly included as first alias entry."}
  PutMailboxPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMailboxPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMailboxPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  RegisterMailDomain: {wire: ok, errors: ok, state: ok, persist: ok, note: "now sets DkimVerificationStatus=PENDING and populates Records (see GetMailDomain gap-close note). errcodeaudit 2026-08-29 FIX: duplicate-registration rejection emitted the fabricated EntityAlreadyExistsException; switched to the real MailDomainInUseException RegisterMailDomain's own model defines. Verified via TestRegisterMailDomain_InUse. gopherstack-gmny (2026-09-06): now enforces the documented, non-adjustable 1,000-domains-per-organization quota (maxDomainsPerOrganization, mail_domains.go) -- see LimitExceededException gap note below for sourcing."}
  DeregisterMailDomain: {wire: ok, errors: ok, state: ok, persist: ok, note: "default-domain protection verified (MailDomainStateException)."}
  GetMailDomain: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED: DkimVerificationStatus (PENDING on RegisterMailDomain, VERIFIED on the org's own domains from CreateOrganization) and Records (types.DnsRecord list: MX + SPF TXT + autodiscover CNAME + 3 DKIM CNAMEs, via dnsRecordsForDomain in mail_domains.go) now modeled and wired through the response. Record token/value contents are simulation-only placeholders (real WorkMail issues real per-domain DKIM tokens); the wire shape ({Hostname,Type,Value} per entry) is what a real SDK client actually reads and is correct. IsDefault/IsTestDomain/OwnershipVerificationStatus still correct (prior pass)."}
  ListMailDomains: {wire: ok, errors: ok, state: ok, persist: ok, note: "item shape is types.MailDomainSummary, wire key is DefaultDomain (not IsDefault) and there is no IsTestDomain field -- was silently emitting IsDefault=false/absent forever from the real client's point of view. Fixed (prior pass)."}
  UpdateDefaultMailDomain: {wire: ok, errors: ok, state: ok, persist: ok}
  PutAccessControlRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED: ImpersonationRoleIds/NotImpersonationRoleIds now accepted, stored on AccessControlRule, and echoed back by ListAccessControlRules; previously accepted by the real API (added after impersonation roles shipped) but not modeled anywhere on this backend."}
  DeleteAccessControlRule: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccessControlEffect: {wire: ok, errors: ok, state: ok, persist: ok, note: "creation-order rule evaluation, CIDR matching verified (prior pass). GAP CLOSED this pass: now accepts ImpersonationRoleId (GetAccessControlEffectInput's fifth condition input) and evaluates it against each rule's ImpersonationRoleIds/NotImpersonationRoleIds, matching the same ALL-non-empty-conditions-must-match semantics as Actions/IpRanges/UserIds."}
  ListAccessControlRules: {wire: ok, errors: ok, state: ok, persist: ok, note: "response used IPRanges/NotIPRanges (wrong casing); real wire is IpRanges/NotIpRanges -- an SDK client would see empty slices always. Fixed (prior pass). Now also echoes ImpersonationRoleIds/NotImpersonationRoleIds."}
  CreateImpersonationRole: {wire: ok, errors: partial, state: ok, persist: ok, note: "errcodeaudit 2026-08-29: duplicate-name rejection emits the fabricated EntityAlreadyExistsException. Left as-is: CreateImpersonationRole's own error model (deserializers.go awsAwsjson11_deserializeOpErrorCreateImpersonationRole) defines no AlreadyExists-shaped exception at all -- no replacement code invented."}
  GetImpersonationRole: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateImpersonationRole: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteImpersonationRole: {wire: ok, errors: ok, state: ok, persist: ok}
  ListImpersonationRoles: {wire: ok, errors: ok, state: ok, persist: ok, note: "response field was Items; real field is Roles. Fixed."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "RECLASSIFIED this pass: PARITY.md previously marked this 'persist: deferred' claiming b.tags was NOT in backendSnapshot. Independently re-verified by reading persistence.go directly: b.tags IS a field on backendSnapshot (json:\"tags\"), IS populated in Snapshot (Tags: b.tags), and IS restored in restoreCollectionMaps (b.tags = snap.Tags). This is confirmed by persistence_test.go's existing 'tags raw map' subtest, which round-trips a tag through Snapshot/Restore and passes. The prior audit's claim was stale/wrong, not a real gap -- tags were already correctly persisted before this pass touched anything. Also cascade-cleaned on DeleteUser/DeleteGroup/DeleteResource/DeleteOrganization this pass -- see leaks below."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEntity: {wire: ok, errors: ok, state: ok, persist: ok, note: "real API's only documented lookup key is Email; backend previously only matched by internal ID or Name, so a real client's DescribeEntity(Email=...) call always 404'd. Fixed to check the byEmail reverse-index maps first, falling back to ID/Name for compatibility."}
  CreateAvailabilityConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "errcodeaudit 2026-08-29 FIX: duplicate rejection emitted the fabricated EntityAlreadyExistsException; switched to the real NameAvailabilityException this op's own model defines."}
  DeleteAvailabilityConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAvailabilityConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAvailabilityConfigurations: {wire: ok, errors: ok, state: ok, persist: ok}
  TestAvailabilityConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "real endpoint/ARN validation, not a fabricated always-true stub."}
  CreateMobileDeviceAccessRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMobileDeviceAccessRule: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateMobileDeviceAccessRule: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMobileDeviceAccessRules: {wire: ok, errors: ok, state: ok, persist: ok}
  GetMobileDeviceAccessEffect: {wire: ok, errors: ok, state: ok, persist: ok}
  PutMobileDeviceAccessOverride: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMobileDeviceAccessOverride: {wire: ok, errors: ok, state: ok, persist: ok}
  GetMobileDeviceAccessOverride: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMobileDeviceAccessOverrides: {wire: ok, errors: ok, state: ok, persist: ok}
  PutEmailMonitoringConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEmailMonitoringConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEmailMonitoringConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  PutInboundDmarcSettings: {wire: ok, errors: ok, state: ok, persist: deferred, note: "inboundDmarc raw map is persisted (restoreCollectionMaps); ok."}
  DescribeInboundDmarcSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  PutRetentionPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRetentionPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDefaultRetentionPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  StartMailboxExportJob: {wire: ok, errors: ok, state: ok, persist: ok}
  CancelMailboxExportJob: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeMailboxExportJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMailboxExportJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "real Jobs[] item type is the FULL MailboxExportJob shape (same as Describe), not a summary; RoleArn/KmsKeyArn/S3Path/S3Prefix/EstimatedProgress/ErrorInfo were missing from the list response even though the backend already tracks them. Fixed."}
  CreateIdentityCenterApplication: {wire: ok, errors: ok, state: ok, persist: deferred, note: "identityCenterApps raw map IS persisted (restoreReverseLookupMaps); ok."}
  DeleteIdentityCenterApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  PutIdentityProviderConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteIdentityProviderConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeIdentityProviderConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePersonalAccessToken: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPersonalAccessTokenMetadata: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPersonalAccessTokens: {wire: ok, errors: ok, state: ok, persist: ok}
  GetImpersonationRoleEffect: {wire: ok, errors: ok, state: ok, persist: ok}
  AssumeImpersonationRole: {wire: ok, errors: ok, state: ok, persist: ok, note: "issued-token bookkeeping (issuedTokens table) is real, not fabricated; token itself is opaque (matches real API, which doesn't document a verifiable format)."}
families:
  route-matcher: {status: ok, note: "single X-Amz-Target-prefix POST endpoint (WorkMailService.<Op>); MatchPriority/RouteMatcher/ExtractOperation all verified against service.HandleTarget's shared dispatcher, not just a Handler() unit test. buildOps() was decomposed into 4 category builder funcs (buildOrgAndEntityOps/buildMailboxAndDomainOps/buildAccessAndImpersonationOps/buildConfigAndTokenOps) merged via maps.Copy to remove a //nolint:funlen -- purely a structural split, every op is still in the one flat dispatch map and still dispatch-reachable through service.HandleTarget -> h.dispatch. Re-verified op count unchanged (92) via HandlerOpsLen and TestSDKCompleteness (still green)."}
  persistence: {status: ok, note: "Handler.Snapshot/Restore already delegate to backend (fixed in an earlier phase per persistence.go's doc comments) so cli.go's setupPersistence picks WorkMail up. Verified all 15 org-nested/composite-keyed tables + 3 registry tables + raw maps (including tags -- see TagResource note above) round-trip through backendSnapshot; version-mismatch discard-and-reset path present. Additive struct fields (User/MailDomain/AccessControlRule/Organization) don't require a snapshot-version bump: old snapshots decode the new fields as zero values, which is the correct behavior (a pre-upgrade org genuinely never had DKIM records, migration admin, etc.)."}
  error-mapping: {status: ok, note: "every backend error path wraps one of ErrNotFound/ErrConflict/ErrValidation/ErrLimitExceeded/ErrMailDomainState/ErrEntityState; handleError's switch covers all six plus isUnknownOp -- no bare fmt.Errorf that would fall through to InternalServiceError found."}
gaps:
  - "CORRECTED 2026-08-23 (manifest-harvest pass): this bullet was stale. DescribeResource already models both BookingOptions and HiddenFromGlobalAddressList -- field-diffed against DescribeResourceOutput/types.BookingOptions (workmail@v1.39.4 api_op_DescribeResource.go:54-84, types/types.go:85-98): both fields present on handler_resources.go's describeResourceResp, BookingOptions carries all 3 real sub-fields (AutoAcceptRequests/AutoDeclineConflictingRequests/AutoDeclineRecurringRequests, interfaces.go), and CreateResource/UpdateResource both thread BookingOptions through. Already covered end-to-end by TestDescribeResource_BookingOptionsAndHiddenFromGAL (handler_resources_test.go), which passes. No code change needed -- the implementation predates this note and the note was never updated to match."
  - "Organization.State is hardcoded to ACTIVE (org creation is synchronous); real AWS transitions through Creating/Active/etc, but nothing in this backend ever leaves an org in a non-terminal state, so this is a non-issue in practice, not a hidden bug. Left as-is (re-verified this pass, not fixed -- there is nothing to fix: no code path produces an incorrect State)."
  - "ALREADY FIXED (2026-08-29 gopherstack-sm09 re-verification): this bullet was stale. CreateOrganization threads EnableInteroperability onto Organization.InteroperabilityEnabled (organizations.go:47, landed in fb80d66cd) and DescribeOrganization echoes it back (handler_organizations.go:78); TestCreateOrganization_EnableInteroperability (handler_organizations_test.go) proves both true and false round-trip through the real handler. No code change needed -- the fix predates this note and the note was never updated to match."
  - "LimitExceededException (gopherstack-gmny, 2026-09-06): declared on 10 ops but only 2 have a published, non-adjustable AWS quota to enforce. DOCUMENTATION-SOURCED, not SDK-verified (no wire field exists to check these numbers against) -- per docs.aws.amazon.com/workmail/latest/adminguide/workmail_limits.html: CreateAlias enforces 'Maximum number of aliases per user | 100. This is a hard quota and can't be changed.' (maxAliasesPerUser, aliases.go, scoped per entity from the existing b.aliases[orgID][entityID] bookkeeping); RegisterMailDomain enforces 'Number of domains per Amazon WorkMail organization | 1,000. This is a hard quota and can't be changed.' (maxDomainsPerOrganization, mail_domains.go, scoped per org from the existing b.mailDomainsByOrg index). CreateOrganization's published number (100 orgs/account) is explicitly NOT enforced: AWS states it 'Can be increased based on an organization's directory type' -- the same adjustable-quota shape already declined at services/efs/PARITY.md:76,80, so hardcoding it here would risk breaking legitimate high-volume use of the mock for no wire-shape benefit. The remaining 7 ops (CreateAvailabilityConfiguration, CreateImpersonationRole, CreateMobileDeviceAccessRule, PutAccessControlRule, PutRetentionPolicy, StartMailboxExportJob, UpdateImpersonationRole) have no published number anywhere on the quotas page or the API reference; durably blocked, recorded so nobody re-searches."
deferred: []
# The single previously-deferred item (Tags persistence) was independently
# re-verified this pass and found to be NOT actually deferred -- see the
# TagResource ops entry above. No items are deferred as of this audit.
leaks: {status: fixed, note: "Found and fixed a real cascade-cleanup leak class this pass: DeleteUser/DeleteGroup/DeleteResource removed the entity from its own table but left ghost rows behind in aliases/globalAliases (CreateAlias-created aliases, not primary emails, which were already cleaned), mailbox permissions (both as the target entity AND as a grantee on another entity's mailbox), other groups' membership sets, and other resources' delegate lists, plus tags keyed by the entity's ARN -- an alias or ARN belonging to a deleted entity could never be reused by anything else in the org. Fixed via a shared cascadeCleanEntity helper (store.go) called from all three Delete* ops. DeleteOrganization had the same class of bug at org scope: its own doc comment said tags were 'deliberately left untouched', and globalAliases rows for the org's users/groups/resources were never swept either (both the org and everything that could reference them were already gone, so nothing would ever clean them) -- fixed via deleteTagsForOrg (ARN-prefix match) and deleteGlobalAliasesForOrg (OrgID-field scan). Regression tests in cascade_cleanup_test.go. Everything else: no goroutines, tickers, or background janitors in this service; AssumeImpersonationRole issues tokens into issuedTokens with no TTL sweep, matching MobileDeviceAccessOverride/PersonalAccessToken's pattern of storing ExpiresAt/ExpiresTime as data without an active expiry janitor -- consistent with the rest of the service, not a new leak."}
---

## Notes

Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: WorkMailService.<Op>`.
Route matcher is a simple header-prefix check (`service.PriorityHeaderExact`); every
op in `buildOps()` is reachable through `service.HandleTarget` → `h.dispatch`, not just
through direct `Handler()` unit-test calls.

### Bugs fixed in the prior pass (2026-07-12, all in `services/workmail/`)

1. **`CreateOrganization` request-breaking wire bug** (`handler.go`): `Domains` was typed
   `[]string`, but the real SDK always serializes it as a list of `{DomainName,
   HostedZoneId}` objects (`aws-sdk-go-v2/service/workmail/types.Domain`). Any real
   client call that specified a domain would fail `json.Unmarshal` and surface as a 500
   `InternalServiceError` — this broke the single most common non-trivial
   `CreateOrganization` call shape. Fixed with a `domainReq` struct; `HostedZoneId` is
   accepted and discarded (Route53-only, no meaning for the in-memory backend).
2. **`ListAccessControlRules` response casing** (`handler.go`): emitted `IPRanges`/
   `NotIPRanges`; real wire keys are `IpRanges`/`NotIpRanges` (case-sensitive JSON
   deserialization means a real client always saw empty slices for these two fields).
3. **`ListMailDomains` response key** (`handler.go`): emitted `IsDefault`; the real
   `ListMailDomains` item type (`types.MailDomainSummary`) uses `DefaultDomain` and has
   no `IsTestDomain` field at all (that field only exists on the unrelated
   `GetMailDomainOutput` shape, which was already correct). A real client always saw
   `DefaultDomain: false` regardless of the actual default domain.
4. **`ListGroupsForEntity` response shape** (`handler.go`): reused the `ListGroups` item
   shape (`Id`/`Name`/`Email`/`State`); the real op returns `types.GroupIdentifier`
   (`GroupId`/`GroupName` only) — every field a real client actually reads
   (`GroupId`, `GroupName`) was always absent/zero-valued.
5. **`ListImpersonationRoles` response key** (`handler.go`): emitted `Items`; real field
   is `Roles`.
6. **`ListMailboxExportJobs` incomplete shape** (`handler.go`): real `Jobs[]` reuses the
   full `MailboxExportJob` type (same as `DescribeMailboxExportJob`), not a narrower
   summary. `RoleArn`, `KmsKeyArn`, `S3Path`, `S3Prefix`, `EstimatedProgress`, and
   `ErrorInfo` were missing even though the backend already tracks all of them.
7. **`DescribeEntity` never resolved by email** (`backend.go`): the real API's
   `DescribeEntityInput.Email` field is documented as "the email under which the entity
   exists" — it is an email lookup. The backend's `findUser`/`findGroup`/`findResource`
   only match by internal ID or by `Name`, never by `Email`, so a real client's
   `DescribeEntity` call (which can only pass an email) always 404'd unless the email
   happened to coincide with the entity's `Name`. Fixed by checking the existing
   `usersByEmail`/`groupsByEmail`/`resourcesByEmail` reverse-index maps first, falling
   back to the ID/Name lookup for backward compatibility with internal callers.
8. Added `ErrorMessage` to `DescribeOrganization`'s response (real field, backend
   already tracks `Organization.ErrorMessage`; cheap correctness fix, currently always
   empty in practice since org creation is synchronous).

Two existing tests (`TestAudit1_WorkMail_MailDomains`'s `list_mail_domains` case,
`TestAudit1_WorkMail_...`'s `list_groups_for_entity` and `list_impersonation_roles`
cases) asserted the **old, wrong** wire keys and were updated to assert the corrected
ones. New regression tests added in `handler_bugfix_test.go` cover all of the above.

### Gaps closed this pass (2026-07-23, all in `services/workmail/`)

All 6 gaps + the 1 deferred item listed in the 2026-07-12 PARITY.md were addressed:

1. **List filters** (`users.go`/`groups.go`/`resources.go`/`interfaces.go`/
   `handler_users.go`/`handler_groups.go`/`handler_resources.go`): `ListUsers`,
   `ListGroups`, `ListResources`, and `ListGroupsForEntity` all accept a `Filters`
   object on the wire (`ListUsersFilters`/`ListGroupsFilters`/`ListResourcesFilters`/
   `ListGroupsForEntityFilters`) that was previously parsed but never applied — a real
   client doing a prefix/state search got back the full unfiltered page. Added
   `UserFilter`/`GroupFilter`/`ResourceFilter` structs plus
   `userMatchesFilter`/`groupMatchesFilter`/`resourceMatchesFilter` (name/email prefix
   + state, all AND'd) and threaded `groupNamePrefix` through
   `ListGroupsForEntity` (its one filter dimension).
2. **`PutAccessControlRule`/`GetAccessControlEffect` impersonation conditions**
   (`access_control.go`/`interfaces.go`/`handler_access_control.go`): `AccessControlRule`
   now carries `ImpersonationRoleIDs`/`NotImpersonationRoleIDs` (wire keys
   `ImpersonationRoleIds`/`NotImpersonationRoleIds`), stored by `PutAccessControlRule`,
   echoed by `ListAccessControlRules`, and evaluated by `GetAccessControlEffect` against
   its new `ImpersonationRoleId` input (added to `matchesUserAndImpersonation`).
3. **`DescribeUser`/`CreateUser`/`UpdateUser` profile fields** (`interfaces.go`/
   `users.go`/`handler_users.go`): `User` gained `City`, `Company`, `Country`,
   `Department`, `Initials`, `JobTitle`, `Office`, `Street`, `Telephone`, `ZipCode`,
   `IdentityProviderIdentityStoreID`, `IdentityProviderUserID`,
   `HiddenFromGlobalAddressList`, `MailboxProvisionedDate`, `MailboxDeprovisionedDate`.
   `CreateUser`/`UpdateUser` now accept `CreateUserParams`/`UpdateUserParams` (matching
   `CreateUserInput`/`UpdateUserInput`'s full field sets, replacing the old 5- and
   5-positional-arg signatures) and `DescribeUser`'s response wires all of it.
   `MailboxProvisionedDate`/`MailboxDeprovisionedDate` are set in
   `RegisterToWorkMail`/`DeregisterFromWorkMail` alongside `EnabledDate`/`DisabledDate`.
4. **`GetMailDomain` DKIM/Records** (`mail_domains.go`/`organizations.go`/
   `handler_mail_domains.go`): added `DkimVerificationStatus` to `MailDomain` and a
   `dnsRecordsForDomain` helper producing the recommended `Records` list (MX + SPF TXT +
   autodiscover CNAME + 3 DKIM CNAMEs, matching `types.DnsRecord`'s
   `{Hostname,Type,Value}` shape) wired into both `RegisterMailDomain` (status
   `PENDING`) and `CreateOrganization`'s default + client-specified domains (status
   `VERIFIED`, matching their pre-existing `OwnershipVerificationStatus`).
5. **`DescribeOrganization` MigrationAdmin** (`interfaces.go`/`handler_organizations.go`):
   added the field. Field-diffed the whole `v1.37.2` SDK surface and confirmed no
   operation ever sets it in real AWS's public API either (interoperability/migration is
   admin-console-only) — the field is now modeled and wired, correctly always empty.
6. **`Organization.State` hardcoded to `ACTIVE`**: re-verified, confirmed non-issue (no
   code path ever produces an incorrect `State`); left as-is per the 2026-07-12 note.
7. **Tags "deferred" reclassified to `ok`**: the 2026-07-12 PARITY.md's `deferred` entry
   claimed `b.tags` was not in `backendSnapshot`. Read `persistence.go` directly this
   pass: `b.tags` IS a `backendSnapshot` field, IS populated in `Snapshot`, and IS
   restored in `restoreCollectionMaps` — already correct, not touched. The claim was
   stale/wrong, not a real gap. See the `TagResource` ops entry for the full
   verification trail.

### Leak fixed this pass (2026-07-23)

`DeleteUser`/`DeleteGroup`/`DeleteResource` (`store.go`'s new `cascadeCleanEntity`,
called from `users.go`/`groups.go`/`resources.go`) and `DeleteOrganization`
(`store.go`'s new `deleteTagsForOrg`/`deleteGlobalAliasesForOrg`, called from
`organizations.go`) previously left ghost rows behind in `aliases`/`globalAliases`,
mailbox `permissions` (as target entity AND as grantee), `groupMembers`, `delegates`,
and `tags` after an entity or whole organization was deleted — see the `leaks` block
above for the full description. Regression tests in `cascade_cleanup_test.go`.

### `//nolint:funlen` removed this pass (2026-07-23)

`handler.go`'s `buildOps` carried the service's one banned nolint
(`//nolint:funlen // existing issue.`, 141 lines / 92 ops). Decomposed into
`buildOrgAndEntityOps`/`buildMailboxAndDomainOps`/`buildAccessAndImpersonationOps`/
`buildConfigAndTokenOps`, merged via `maps.Copy` — purely structural, every op is still
in the one flat dispatch map. (An initial 5-way split triggered `dupl` on two
similarly-shaped map-literal builders despite entirely different keys/handlers; merged
back down to 4 to remove the coincidental structural match rather than paper over it
with another nolint.)

### Traps for the next auditor

- `GetMailDomain` (single-domain describe) and `ListMailDomains` (list) use **two
  different SDK types** with overlapping-but-different field sets — `IsDefault` is
  correct for `GetMailDomain`, wrong for `ListMailDomains`. Don't conflate them again.
- `ListGroups` and `ListGroupsForEntity` also use two different SDK types
  (`types.Group` vs `types.GroupIdentifier`) despite both being "list of groups" ops.
- The backend's per-entity `find*(orgID, entityID)` helpers (`findUser`, `findGroup`,
  `findResource`) intentionally accept either an internal ID or a `Name` — they do NOT
  search by email. Any op whose real AWS input field is documented as an email
  (`DescribeEntity`) needs the `usersByEmail`/`groupsByEmail`/`resourcesByEmail` maps
  instead, not `find*`.
- Before marking a PARITY.md `deferred`/`gap` entry as still-open, actually re-read the
  source it claims is wrong — the "Tags not persisted" deferred entry in the
  2026-07-12 PARITY.md was stale (persistence.go already persisted tags correctly) and
  would have been carried forward as a phantom gap if not independently re-verified.
- `cascadeCleanEntity` (store.go) is the one place that knows how to fully unlink an
  entity from every collection that references it by ID (aliases, permissions as either
  side, group memberships, resource delegate lists, tags). Any *new* op that creates
  another entity->entity reference (a new collection keyed by another entity's ID) needs
  a corresponding cleanup line added there, or it becomes the next leak.

### Wrapper-key sweep, gopherstack-6flj (this pass, 2026-08-15)

Full layer-1/2/3 pass over all 36 List/Describe/Get ops against
`aws-sdk-go-v2/service/workmail@v1.39.4`. Protocol: `awsAwsjson11_` (JSON-RPC
1.1), case-sensitive; confirmed `HandleDeserialize` reaches the real
`OpDocument*Output` deserializer directly for every op (no dead-wrapper
codegen layer in this service). 4 real bugs found and fixed:

1. `ListUsers` never emitted `IdentityProviderIdentityStoreId`/
   `IdentityProviderUserId` (real `types.User` members; the backend already
   tracked both, `DescribeUser` already emitted them correctly).
2. `ListGroupMembers` never emitted `EnabledDate`/`DisabledDate` (real
   `types.Member` members). The backend synthesized a fresh `Member` per
   membership without copying either date from the underlying user/group
   record it had already looked up.
3. `ListMailboxExportJobs` emitted `RoleArn`/`KmsKeyArn`/`S3Prefix`/
   `ErrorInfo` — an invented shape. The real `types.MailboxExportJob` (the
   List item type) is genuinely narrower than
   `DescribeMailboxExportJobOutput` and has none of those four members; the
   raw wire body leaked an IAM role ARN and a KMS key ARN on every list item
   that no real typed client could even decode. A prior "parity-4" pass's
   own doc comment incorrectly claimed these two shapes were identical —
   corrected.
4. `DescribeResource`/`UpdateResource` never modeled
   `HiddenFromGlobalAddressList` (a real, plain-storage member on both —
   unlike users/groups it is not settable on `CreateResource`, only via
   `UpdateResource`). Added the field to the `Resource` model and threaded
   it through.

**Disclosed, not fixed** (structural gaps, not renames):

- `DescribeResource`/`UpdateResource`'s `BookingOptions`
  (`AutoAcceptRequests`/`AutoDeclineConflictingRequests`/
  `AutoDeclineRecurringRequests`) — real members on both, but this backend
  has no resource-booking/scheduling concept to attach sensible defaults to,
  and the real API's default state for a never-configured resource isn't
  independently confirmed here.
- `DescribeOrganization`'s `InteroperabilityEnabled` always reports `false`
  — no cross-org interoperability concept exists anywhere in this backend;
  the field simply has no way to become non-default.
- `DescribeMailboxExportJobOutput`'s response carries a harmless extra
  `JobId` field not present on the real type (the client already knows the
  job ID from the request) — left as-is, matches this campaign's established
  "real client has no field to read an extra key into" non-bug precedent.
- `GetMailDomainOutput`'s response carries a harmless extra `DomainName`
  field not present on the real type, same reasoning.

No V1/V2 or other generational sibling pairs exist in this service. No
secret/credential-bearing field found beyond the RoleArn/KmsKeyArn leak
above (fixed). One ratifying test
(`TestBugfix_WorkMail_ListMailboxExportJobsFullShape`, from the same prior
"parity-4" pass that introduced finding #3) asserted the fabricated fields
as correct; rewritten as `...NarrowShape` to assert their absence instead.
Tests: `services/workmail/wire_field_fixes_test.go` (4 new real-SDK-client
tests via the existing `newWorkMailSDKClient` helper).

## 2026-08-30 WrapOp reflective-decode re-scan (gopherstack-4shm follow-up)

Re-scanned with `cmd/reqfieldscan` (resolves `WrapOp`'s generic parameter,
closing the literal-decode-anchored blind spot gopherstack-4shm was filed
for): 92/92 ops in the dispatch table, 92 request types, 313 fields.

2 fields flagged unread, both on `testAvailabilityConfigReq`: `EwsProvider`
and `LambdaProvider`. Real bug, fixed: "The request must contain either one
provider definition (EwsProvider or LambdaProvider) or the DomainName
parameter. If the DomainName parameter is provided, the configuration
stored under the DomainName will be tested." (workmail@v1.39.4
api_op_TestAvailabilityConfiguration.go) -- `handleTestAvailabilityConfiguration`
only ever used `DomainName`, so a client probing inline (not-yet-created)
credentials before a `CreateAvailabilityConfiguration` call always got
`EntityNotFoundException` instead of a real test result. Fixed by threading
`EwsProvider`/`LambdaProvider` through to `TestAvailabilityConfiguration`,
which now tests inline credentials directly when either is given, falling
back to the stored-config lookup otherwise; the endpoint/username/ARN
validation logic itself was already correct and is now shared (via new
`testEwsProvider`/`testLambdaProvider` helpers) between the inline and
stored-config paths instead of being duplicated. Tests:
`handler_availability_config_test.go`
(`TestAvailabilityConfigurationInlineProvider`, two cases: a valid inline
EWS provider against a domain with no stored config, and an invalid inline
Lambda ARN), confirmed failing (400 EntityNotFoundException) against
unmodified code before the fix.

Gates: `go build ./services/workmail/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/workmail/...` (pass),
`golangci-lint run ./services/workmail/...` (0 issues).

## 2026-08-31 Error-envelope sweep (gopherstack-6flj/uox6, errtargetaudit)

`errtargetaudit -dir workmail` reported 45 class-A findings (44
`EntityNotFoundException`, 1 `MailDomainStateException`), covering 63
individual sentinel-reference lines across 15 files. All 45 verified real
against workmail@v1.39.4's own per-op `awsAwsjson11_deserializeOpError*`
switches: the shared `ErrNotFound` sentinel (wire code
`EntityNotFoundException`) was used unconditionally for both
"organization does not exist" and "entity/resource does not exist within a
valid organization" checks across ~40 operations, but per-op verification
showed most operations declare `OrganizationNotFoundException` for the
first condition and either `ResourceNotFoundException`,
`MailDomainNotFoundException`, or **nothing** for the second — not
`EntityNotFoundException`. (`ErrNotFound` remains correct and untouched for
the ~48 other operations across this package whose own model does declare
`EntityNotFoundException`, e.g. `DescribeGroup`, `UpdateGroup`,
`AssociateMemberToGroup`, `UpdateImpersonationRole`,
`GetImpersonationRoleEffect`, all four Mobile-Device-Access-Override ops,
`DescribeResource`/`UpdateResource`, `DescribeUser`/`UpdateUser`,
`RegisterToWorkMail`/`DeregisterFromWorkMail`, `ResetPassword`,
`UpdatePrimaryEmailAddress`, `PutAccessControlRule`/`GetAccessControlEffect`
— checked individually, not assumed.)

**Three new sentinels added** (`errors.go`), each wired into
`handleError`'s switch ahead of the generic `ErrNotFound` case:
`ErrOrganizationNotFound` (`OrganizationNotFoundException`),
`ErrResourceNotFound` (`ResourceNotFoundException`),
`ErrMailDomainNotFound` (`MailDomainNotFoundException`). The shared
`ErrNotFound` sentinel itself is untouched.

**43 call sites fixed to `ErrOrganizationNotFound`** (the organization-ID
lookup in `CreateAvailabilityConfiguration`, `CreateGroup`,
`CreateMobileDeviceAccessRule`, `CreateResource`, `CreateUser`,
`DeleteAccessControlRule`, `DeleteAvailabilityConfiguration`,
`DeleteEmailMonitoringConfiguration`, `DeleteGroup`,
`DeleteIdentityProviderConfiguration`, `DeleteImpersonationRole`,
`DeleteMobileDeviceAccessRule`, `DeleteOrganization`,
`DeletePersonalAccessToken`, `DeleteResource`, `DeleteRetentionPolicy`,
`DeleteUser`, `DeregisterMailDomain`, `DescribeEmailMonitoringConfiguration`,
`DescribeIdentityProviderConfiguration`, `DescribeInboundDmarcSettings`,
`DescribeOrganization`, `GetImpersonationRole`, `GetMailDomain`,
`GetMobileDeviceAccessEffect`, `GetPersonalAccessTokenMetadata`,
`ListAccessControlRules`, `ListAvailabilityConfigurations`,
`ListImpersonationRoles`, `ListMailDomains`, `ListMailboxExportJobs`,
`ListMobileDeviceAccessRules`, `ListResources`, `ListUsers`,
`PutEmailMonitoringConfiguration`, `PutIdentityProviderConfiguration`,
`PutInboundDmarcSettings`, `PutRetentionPolicy`, `RegisterMailDomain`,
`TestAvailabilityConfiguration`, `UpdateAvailabilityConfiguration`,
`UpdateDefaultMailDomain`, `AssumeImpersonationRole`).

**6 call sites fixed to `ErrResourceNotFound`** (entity lookup on ops that
declare `ResourceNotFoundException`): `AssumeImpersonationRole` (role),
`GetImpersonationRole` (role), `UpdateAvailabilityConfiguration` (config),
`TestAvailabilityConfiguration` (config, stored-config path),
`DescribeIdentityProviderConfiguration` (config),
`GetPersonalAccessTokenMetadata` (token).

**2 call sites fixed to `ErrMailDomainNotFound`**: `GetMailDomain`,
`UpdateDefaultMailDomain` (both declare `MailDomainNotFoundException`).

**12 sites left unchanged and recorded** — the operation's own model
declares no fitting type for the specific condition, so no code was
substituted (comments added at each site naming the declared set):
`DeleteImpersonationRole` (role-not-found; only Organization* declared),
`DeleteGroup` (group-not-found), `DeleteResource` (resource-not-found),
`DeleteUser` (user-not-found), `DeleteAccessControlRule` (rule-not-found),
`DeleteAvailabilityConfiguration` (config-not-found),
`DeleteMobileDeviceAccessRule` (rule-not-found),
`DeletePersonalAccessToken` (token-not-found),
`DeleteRetentionPolicy` (policy-not-found),
`DeleteIdentityCenterApplication` (declares no not-found type of any kind —
not even `OrganizationNotFoundException`, since this op is not
organization-scoped in this backend at all),
`DeregisterMailDomain` ×2 (domain-not-found: no `MailDomainNotFoundException`
declared here despite `GetMailDomain`/`UpdateDefaultMailDomain` declaring
it for the same "domain not found in this org" condition; and
"cannot deregister the default domain": neither `MailDomainStateException`
nor `EntityNotFoundException` is declared, and `MailDomainInUseException`'s
own doc — "in use by ANOTHER user or organization" — describes a different
condition, so it was not substituted despite being the closest declared
conflict type). All 12: reason is "the operation's own model declares no
type for this condition", not a reachability or infrastructure gap.

**2 pre-existing tests corrected** (asserted only the wire `__type` string,
not a typed error — the class this sweep targets): `TestAssumeImpersonationRoleErrors`
/"org not found" and `TestAvailabilityConfigurationErrors`/"delete
nonexistent"+"update nonexistent" all expected `EntityNotFoundException`
for what is actually an organization-not-found path (in the availability-config
cases, the org in the request body was never created in that subtest, so
the org check — not an entity check — was what those subtests actually
exercised). Corrected the expected string to `OrganizationNotFoundException`
in all 3; assertion count unchanged (1 `assert.Contains` per subtest,
before and after).

**3 new real-client typed-error tests added**
(`error_envelope_fixes_test.go`): `TestDescribeOrganization_OrganizationNotFound_RealClient`,
`TestGetImpersonationRole_ResourceNotFound_RealClient`,
`TestGetMailDomain_MailDomainNotFound_RealClient` — each drives the real
`aws-sdk-go-v2/service/workmail` client and asserts `errors.As` against the
specific typed exception; confirmed failing against unmodified code
(temporarily reverted the corresponding sentinel, re-ran, restored).

Gates: `go build ./services/workmail/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/workmail/...` (pass),
`golangci-lint run ./services/workmail/...` (0 issues).

## 2026-08-31 errtargetaudit re-sweep: all 12 findings re-verified as the prior pass's recorded refusals

`errtargetaudit -dir workmail` (post-reachability-fix, post-sentinel-collision-fix)
reports 12 class-A findings (11 `EntityNotFoundException`, 1
`MailDomainStateException`): `DeleteAccessControlRule`,
`DeleteAvailabilityConfiguration`, `DeleteGroup`,
`DeleteIdentityCenterApplication`, `DeleteImpersonationRole`,
`DeleteMobileDeviceAccessRule`, `DeletePersonalAccessToken`,
`DeleteResource`, `DeleteRetentionPolicy`, `DeleteUser`, and
`DeregisterMailDomain` (both its `EntityNotFoundException` and
`MailDomainStateException` findings). These are exactly the 12 sites the
prior pass above ("2026-08-31 Error-envelope sweep") already found, checked
against each op's own declared error set, and deliberately left unchanged
with an explanatory comment at each site -- not a new backlog.

Independently re-verified all 12 directly against
`workmail@v1.39.4/deserializers.go`'s per-op
`awsAwsjson11_deserializeOpError<Op>` switch (not just re-reading the prior
PARITY note): none of `DeleteAccessControlRule`,
`DeleteAvailabilityConfiguration`, `DeleteImpersonationRole`,
`DeleteMobileDeviceAccessRule`, `DeletePersonalAccessToken`,
`DeleteRetentionPolicy`, or `DeregisterMailDomain` declare
`EntityNotFoundException`/`ResourceNotFoundException` at all (each declares
only `{OrganizationNotFoundException, OrganizationStateException}`, plus
`InvalidParameterException` on most); `DeleteGroup`/`DeleteUser`/
`DeleteResource` add `{DirectoryServiceAuthenticationFailedException,
DirectoryUnavailableException, EntityStateException,
UnsupportedOperationException}` but still no not-found type for the
entity itself; `DeleteIdentityCenterApplication` declares only
`{InvalidParameterException, OrganizationStateException}` -- no
`OrganizationNotFoundException` either, confirming the prior note's "not
even organization-scoped" claim; `DeregisterMailDomain` declares
`{InvalidCustomSesConfigurationException, InvalidParameterException,
MailDomainInUseException, OrganizationNotFoundException,
OrganizationStateException}` -- no `MailDomainNotFoundException` and no
`MailDomainStateException`, confirming both of its findings. Spot-checked
the source comments at each of `access_control.go:65` and
`mail_domains.go:96,105` -- still present, still accurate, no drift since
the prior pass.

**Verdict: 0 new findings, 12/12 previously-recorded refusals, no code
changed this pass.**

Gates: `go build ./services/workmail/...` (clean), `go vet
./services/iot/... ./services/workmail/...` (clean; repo-wide `go vet ./...`
was broken at the time of this pass by a concurrent, out-of-scope edit to
`services/codeconnections/handler_hosts.go` from another agent -- confirmed
via `git status`/`git diff --stat`, unrelated to this pass), `go test -race
-count=1 ./services/workmail/...` (pass, no new tests -- nothing to prove),
`golangci-lint run ./services/workmail/...` (0 issues).

## 2026-09-07 errtargetaudit re-sweep (gopherstack-hp83): 2 of 12 flip from refusal to fix on new evidence

Re-verified all 12 class-A findings the tool reports for workmail (same 12
as the 2026-08-31 pass: 11 `EntityNotFoundException` + 1
`MailDomainStateException`, all on `Delete*`/`DeregisterMailDomain`), again
against `workmail@v1.39.4/deserializers.go`'s per-op
`awsAwsjson11_deserializeOpError<Op>` switches. Confirms every prior
per-op wire-model claim unchanged: none of the 11 flagged ops declare
`EntityNotFoundException`/`ResourceNotFoundException`/
`MailDomainNotFoundException`, and `DeregisterMailDomain` declares no
`MailDomainStateException` either.

New this pass: also read each flagged op's doc comment in the pinned SDK's
own `api_op_<Op>.go` (not just its error-set switch, which only says what
code *isn't* used, not what the op actually does on a missing entity).
`api_op_DeleteAccessControlRule.go` and
`api_op_DeleteMobileDeviceAccessRule.go` state outright: "Deleting already
deleted and non-existing rules does not produce an error. In those cases,
the service sends back an HTTP 200 response with an empty HTTP body."
(`grep -l "does not produce an error" api_op_*.go` across all 92 workmail
ops matches exactly these two plus `DeleteMobileDeviceAccessOverride`,
which is out of scope -- its own model does declare
`EntityNotFoundException` and it isn't one of the 12 findings.) That is
direct, authoritative textual proof of intended behavior, not an inference
from an error-set omission, and it flips these 2 findings from "no correct
code, leave" to "the current code is wrong on both the code AND on
whether to error at all."

**Fixed** (`services/workmail/access_control.go` `DeleteAccessControlRule`,
`services/workmail/mobile_device_access.go`
`DeleteMobileDeviceAccessRule`): both now delete-if-present and return nil
unconditionally once the organization is confirmed to exist, matching the
documented idempotent-delete semantics, instead of returning
`EntityNotFoundException` (a code their own wire model never declares) when
the target rule is missing. Regression tests added:
`TestDeleteAccessControlRule_NonExistent`
(`handler_access_control_test.go`) and
`TestDeleteMobileDeviceAccessRule_NonExistent`
(`handler_mobile_device_access_test.go`), each asserting HTTP 200 with an
empty body (no `__type`) for a delete of a name/ID that was never created.
No pre-existing test exercised deleting a non-existent
rule for either op (the existing `delete_rule`/lifecycle tests only cover
deleting a rule that exists), so no pre-existing test needed correcting or
was pinning the old behavior for these two ops.

Neuter check (temporarily reverted each fix to the prior
`if !...Delete(...) { return fmt.Errorf(...ErrNotFound...) }` shape,
confirmed `go build ./services/workmail/...` still compiled, confirmed the
corresponding new test failed with `400`/`EntityNotFoundException` instead
of the expected `200`, then restored the fix): both lines behaved exactly
as required.

**Left unchanged** (10 findings: `DeleteAvailabilityConfiguration`,
`DeleteGroup`, `DeleteIdentityCenterApplication`, `DeleteImpersonationRole`,
`DeletePersonalAccessToken`, `DeleteResource`, `DeleteRetentionPolicy`,
`DeleteUser`, and `DeregisterMailDomain`'s both findings) -- none of these
ops' `api_op_<Op>.go` doc comments carry the "does not produce an error"
text or any other statement of missing-entity behavior (checked directly,
not assumed by analogy to the two fixed ops), so applying the same
idempotent-no-op fix to them would be guessing, which the 2026-08-31 pass
already correctly declined to do. The existing per-site comments citing
gopherstack-6flj/uox6 remain accurate and are left in place. Filed for the
issue author to decide whether to chase down non-SDK-doc evidence (e.g. AWS
admin-guide text) before guessing further.

Re-ran `errtargetaudit`: workmail class-A findings dropped from 12 to 10,
exactly the 2 fixed ops falling out; the remaining 10 are the pre-existing
documented refusals, unchanged.

Gates: `GOTOOLCHAIN=go1.26.6 go build ./services/workmail/...` (clean),
`GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/workmail/...`
(pass), `GOTOOLCHAIN=go1.26.6 golangci-lint run services/workmail/...` (0
issues).

### Addendum, same day: a third op shares the doc sentence but not the bug

`grep -l "does not produce an error" api_op_*.go` (used above to find the 2
fixed ops) actually matches three files, not two:
`DeleteAccessControlRule`, `DeleteMobileDeviceAccessRule`, and
`DeleteMobileDeviceAccessOverride`. The third was initially dismissed
without checking its own error set -- wrong. Checked directly:

```
awk "/deserializeOpErrorDeleteMobileDeviceAccessOverride\(/,/^}/" deserializers.go | grep -oE '"[A-Za-z0-9]+"'
"UnknownError"
"EntityNotFoundException"
"InvalidParameterException"
"OrganizationNotFoundException"
"OrganizationStateException"
```

Unlike the other two, `DeleteMobileDeviceAccessOverride` DOES declare
`EntityNotFoundException` in its own model -- even though its doc comment
carries the identical "Deleting already deleted and non-existing overrides
does not produce an error... HTTP 200... empty HTTP body" sentence. AWS's
own model and doc comment disagree with each other for this one op. Since
the modeled `errors` list is what a real client can even deserialize as a
typed exception (and is the same signal `errtargetaudit` and every
gopherstack error-envelope fix in this file has treated as authoritative
over free text), gopherstack's existing behavior -- returning
`EntityNotFoundException` when the override doesn't exist -- is correct as
written. **Not changed.** Added an in-place comment at
`mobile_device_access.go`'s `DeleteMobileDeviceAccessOverride` recording
the conflict so it isn't rediscovered as a false lead later.

This also answers why `errtargetaudit` flagged 2 of the 3 doc-sentence ops
and not the third: the tool's finding criterion is "emitted code absent
from the op's own declared error set," and `EntityNotFoundException` *is*
declared for `DeleteMobileDeviceAccessOverride` -- so by the tool's own
(wire-model-grounded) methodology this one is correctly a non-finding, not
a miss. Confirmed no pre-existing test pins a bug here either:
`TestMobileDeviceAccessOverrideErrors`'s "delete nonexistent override" case
(`handler_mobile_device_access_test.go`) already asserts
`EntityNotFoundException`/400, which matches the declared model -- it was
asserting correct behavior all along, not pinning a defect. No fix, no new
test needed; existing coverage already proves the modeled path.

Files changed this addendum: `services/workmail/mobile_device_access.go`
(comment only, no behavior change).

Gates re-run: `GOTOOLCHAIN=go1.26.6 go build ./services/workmail/...`
(clean), `GOTOOLCHAIN=go1.26.6 go test -race -count=1
./services/workmail/...` (pass, unchanged), `GOTOOLCHAIN=go1.26.6
golangci-lint run services/workmail/...` (0 issues).
