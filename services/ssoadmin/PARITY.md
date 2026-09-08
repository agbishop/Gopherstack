---
service: ssoadmin
sdk_module: aws-sdk-go-v2/service/ssoadmin@v1.43.1
last_audit_commit: 1d7169f66
last_audit_date: 2026-08-07
overall: A            # multiple severe client-breaking wire-shape bugs found and fixed 2026-07-24 sweep.
                      # 2026-08-21 (gopherstack-c8ge, Scope B): fixed UpdateTrustedTokenIssuer reusing
                      # Create's OIDC config shape for Update, wholesale-replacing the stored config and
                      # wiping the immutable IssuerUrl on every config change. See the op row.
                      # 2026-08-21 (gopherstack-1vv2): fixed UpdateApplication wholesale-replacing
                      # PortalOptions on every call (even ones never mentioning it), erasing
                      # Visibility -- a field UpdateApplicationInput.PortalOptions can never carry.
                      # See the UpdateApplication op row.
                      # gopherstack-dbwi pass: implemented the ProvisioningStatus filter on
                      # ListPermissionSetsProvisionedToAccount/ListAccountsForProvisionedPermissionSet
                      # (real provisioned-vs-edited-since-provisioned drift tracking) and
                      # DescribeInstance's EncryptionConfigurationDetails.
                      # This pass (gopherstack-gt9o, part of the gopherstack-u8my sdk_module pin
                      # sweep): DescribeInstance/UpdateInstance PermissionSetsEnabled and
                      # ListInstances' Regions are now real; both DescribeInstance and UpdateInstance
                      # move from wire: partial to wire: ok. See ops table + gaps below.
                      # 2026-09-04 (gopherstack-2fa, delete-surface sweep): fixed a resource leak in
                      # cascadeDeleteInstance (DeleteInstance's cascade). Unlike DeletePermissionSet,
                      # DeleteInstance has no "no live assignments" precondition, so it can force-delete
                      # a permission set that still has account assignments -- but it only cleaned
                      # b.assignments/b.customerManagedPolicies/b.permissionBoundaries/b.permissionSets,
                      # leaving that permission set's b.provisionedAt and b.assignmentCreationIDs rows
                      # (both hand-rolled maps, no cleanup path of their own) behind forever. Repeated
                      # instance create/delete cycles grew both maps without bound. See
                      # TestCascadeDeleteInstance_PurgesProvisioningState.
ops:
  # --- fixed this sweep ---
  CreateAccountAssignment: {wire: ok, errors: ok, state: ok, persist: ok, note: "AccountAssignmentCreationStatus used invented 'AccountId' field; real AccountAssignmentOperationStatus uses 'TargetId'/'TargetType'. A real client previously got nil TargetId. Fixed."}
  DeleteAccountAssignment: {wire: ok, errors: ok, state: ok, persist: ok, note: "same AccountAssignmentDeletionStatus TargetId/TargetType fix as CreateAccountAssignment"}
  DescribeAccountAssignmentCreationStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "same TargetId/TargetType fix"}
  DescribeAccountAssignmentDeletionStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "same TargetId/TargetType fix"}
  ListAccountAssignmentCreationStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unpaginated (MaxResults ignored, NextToken always nil) and returned the full singular-status shape instead of the real slim AccountAssignmentOperationStatusMetadata (CreatedDate/RequestId/Status only). Both fixed; pagination preserves the backend's CreatedDate-descending order via new paginateOrdered helper (does not re-sort like paginateBy). FIXED (2026-08-30 wrapper-key-sweep pagination-reproducibility pass) — paginateOrdered's resume cursor (RequestID) is genuinely unique, but the sort feeding it wasn't: ListAccountAssignmentCreationStatus sorted b.creationStatuses.All() (a store.Table map walk, unspecified order) by CreatedDate descending only, and CreatedDate can tie (e.g. bulk-provisioning several assignments in the same instant). Because listProvisioningStatusMetadata re-fetches and re-sorts from scratch on every request, a tied pair could land in a different relative order between two calls even with nothing changed, and paginateOrdered's resume-by-key scan would then find a different split point -- dropping or duplicating a tied record (mirrors the elbv2 listener/rule marker-fed-by-tie-prone-sort bug from an earlier pass). Fixed by tiebreaking on RequestID (the store.Table's own key) after CreatedDate. See TestListAccountAssignmentCreationStatus_PaginationStableAcrossTiedCreatedDate (account_assignment_status_pagination_internal_test.go), hand-reverted to confirm it fails against the unfixed sort (duplicated a record 3x on the first of 30 iterations), then restored."}
  ListAccountAssignmentDeletionStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "same slim-metadata-shape + pagination fix as ListAccountAssignmentCreationStatus. FIXED (2026-08-30 wrapper-key-sweep pagination-reproducibility pass) — same CreatedDate-tie-over-map-walk bug as ListAccountAssignmentCreationStatus (b.deletionStatuses.All()), fixed identically with a RequestID tiebreak. See TestListAccountAssignmentDeletionStatus_PaginationStableAcrossTiedCreatedDate, hand-reverted to confirm it fails against the unfixed sort, then restored."}
  ListAccountAssignmentsForPrincipal: {wire: ok, errors: ok, state: ok, persist: ok, note: "Filter.AccountId and MaxResults/NextToken pagination were both ignored (real op supports both); now implemented"}
  ProvisionPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "PermissionSetProvisioningStatus shape confirmed correct (uses AccountId, unlike AccountAssignmentOperationStatus -- these two 'status' shapes diverge on the real API and had been conflated into one Go view type)"}
  DescribePermissionSetProvisioningStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "same shape confirmed correct"}
  ListPermissionSetProvisioningStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unpaginated and returned the full status shape instead of the real slim PermissionSetProvisioningStatusMetadata (CreatedDate/RequestId/Status only); fixed with paginateOrdered (preserves CreatedDate-descending order). FIXED (2026-08-30 wrapper-key-sweep pagination-reproducibility pass) — same CreatedDate-tie-over-map-walk bug as ListAccountAssignmentCreationStatus (b.provisioningStatuses.All()), fixed identically with a RequestID tiebreak. See TestListPermissionSetProvisioningStatus_PaginationStableAcrossTiedCreatedDate, hand-reverted to confirm it fails against the unfixed sort, then restored."}
  ListPermissionSetsProvisionedToAccount: {wire: ok, errors: ok, state: fixed, persist: ok, note: "MaxResults/NextToken were ignored (prior pass); now paginated. FIXED this pass (gopherstack-dbwi): ProvisioningStatus filter (LATEST_PERMISSION_SET_PROVISIONED/LATEST_PERMISSION_SET_NOT_PROVISIONED) was accepted and silently ignored; now real, backed by a new PermissionSet.ModifiedDate (internal bookkeeping, bumped by every content-changing op) compared against a new provisionedAt map (stamped by CreateAccountAssignment's implicit provisioning and explicit ProvisionPermissionSet). ssoadminSnapshotVersion bumped 2->3 for both new persisted fields."}
  ListAccountsForProvisionedPermissionSet: {wire: ok, errors: ok, state: fixed, persist: ok, note: "Same ProvisioningStatus filter fix as ListPermissionSetsProvisionedToAccount, same underlying drift-tracking mechanism."}
  CreateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE: response wrapped the full application under an invented 'Application' object; real CreateApplicationOutput is exactly {ApplicationArn, IdentityStoreArn, InstanceArn} flat, and IdentityStoreArn was never returned. Fixed; backend now derives ApplicationAccount/CreatedFrom/IdentityStoreArn."}
  DescribeApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE: entire response was nested one level too deep under an invented 'Application' wrapper (plus a fabricated 'Tags' member) -- a real aws-sdk-go-v2 client parsing this would get every DescribeApplicationOutput field nil. Real shape is flat: ApplicationAccount/ApplicationArn/ApplicationProviderArn/CreatedDate/CreatedFrom/Description/IdentityStoreArn/InstanceArn/Name/PortalOptions/Status, no Tags. Fixed; tags now only reachable via ListTagsForResource like every other taggable resource."}
  UpdateApplication: {wire: ok, errors: ok, state: ok, persist: fixed, note: "response echoed a full invented 'Application' object; real UpdateApplicationOutput is void. Fixed to {}. 2026-08-21 (gopherstack-1vv2): persist was accept-and-corrupt — UpdateApplicationInput.PortalOptions is types.UpdateApplicationPortalOptions (SignInOptions only, no Visibility, unlike Create-side types.PortalOptions), and the handler wholesale-replaced app.PortalOptions with a freshly-decoded struct on EVERY UpdateApplication call, even ones that never mentioned PortalOptions at all — silently zeroing Visibility and SignInOptions every time. Fixed: PortalOptions is now a nil-able pointer at decode time and the backend merges only SignInOptions into the existing PortalOptions, leaving Visibility untouched. See TestUpdateApplication_PreservesVisibility."}
  ListApplications: {wire: ok, errors: ok, state: ok, persist: ok, note: "was missing ApplicationAccount/CreatedFrom/IdentityStoreArn (present on the real per-item Application type) and MaxResults/NextToken pagination; both fixed. FIXED 2026-08-29 (wrapper-key sweep): ListApplicationsInput.Filter (ApplicationAccount/ApplicationProvider, types.ListApplicationsFilter, serializers.go:5111) was a real wire field the handler's request struct didn't even declare -- json.Unmarshal silently dropped it, so every call returned every application in the instance regardless of the filter. Now reads Filter.ApplicationAccount/Filter.ApplicationProvider and matches against Application.ApplicationAccount/ApplicationProviderArn. See TestListApplications_Filter."}
  DescribeApplicationAssignment: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE: response nested under an invented 'ApplicationAssignment' wrapper; real DescribeApplicationAssignmentOutput is flat {ApplicationArn, PrincipalId, PrincipalType}. Fixed."}
  ListApplicationAssignments: {wire: ok, errors: ok, state: ok, persist: ok, note: "MaxResults/NextToken were ignored; now paginated"}
  ListApplicationAssignmentsForPrincipal: {wire: ok, errors: ok, state: ok, persist: ok, note: "Filter.ApplicationArn and MaxResults/NextToken pagination were both ignored; now implemented"}
  PutApplicationAssignmentConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "request accepted an invented 'AssignmentRequiredForAllIdentities' field with no counterpart on the real PutApplicationAssignmentConfigurationInput (exactly {ApplicationArn, AssignmentRequired}); removed"}
  PutApplicationSessionConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE: entire operation modeled a fabricated 'SessionDuration' concept that does not exist on the real API at all. Real Put/GetApplicationSessionConfiguration operate on 'UserBackgroundSessionApplicationStatus' (ENABLED/DISABLED), confused here with the unrelated PermissionSet.SessionDuration field. Fixed end-to-end (backend, interface, handlers, validation)."}
  GetApplicationSessionConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "same UserBackgroundSessionApplicationStatus fix; also removed an invented nested 'ApplicationSessionConfiguration' wrapper -- real GetApplicationSessionConfigurationOutput is flat"}
  ListApplicationProviders: {wire: ok, errors: ok, state: ok, persist: ok, note: "FederationProtocol was populated on every seeded catalog entry but silently dropped on the wire; also MaxResults/NextToken were ignored. Both fixed."}
  DescribeApplicationProvider: {wire: ok, errors: ok, state: ok, persist: ok, note: "same FederationProtocol drop bug as ListApplicationProviders; fixed"}
  ListApplicationAccessScopes: {wire: ok, errors: ok, state: ok, persist: ok, note: "MaxResults/NextToken were ignored; now paginated"}
  GetApplicationAuthenticationMethod: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE: response double-wrapped the stored union body under an extra invented 'AuthenticationMethodType'/'AuthenticationMethod' pair one level too deep. Real GetApplicationAuthenticationMethodOutput is exactly {AuthenticationMethod: <union>} with NO sibling AuthenticationMethodType (unlike AuthenticationMethodItem, the ListApplicationAuthenticationMethods item shape, which legitimately has both as siblings). A real client's union deserializer would never find the 'Iam' tag through the old double-wrap. Fixed."}
  GetApplicationGrant: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE: same double-wrap bug as GetApplicationAuthenticationMethod -- real GetApplicationGrantOutput is exactly {Grant: <union>}, no sibling GrantType (unlike GrantItem, the ListApplicationGrants item shape). Fixed."}
  CreateTrustedTokenIssuer: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed already correct: flat {TrustedTokenIssuerArn}"}
  DescribeTrustedTokenIssuer: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE: response nested under an invented 'TrustedTokenIssuer' wrapper with fabricated InstanceArn and Tags members; real DescribeTrustedTokenIssuerOutput is flat {Name, TrustedTokenIssuerArn, TrustedTokenIssuerConfiguration, TrustedTokenIssuerType}, no InstanceArn, no Tags. Fixed; tags now only reachable via ListTagsForResource."}
  UpdateTrustedTokenIssuer: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "response echoed a full invented 'TrustedTokenIssuer' object (with a fabricated InstanceArn); real UpdateTrustedTokenIssuerOutput is void. Fixed to {}. 2026-08-21 (gopherstack-c8ge, Scope B: confirmed live candidate of gopherstack-1vv2's class, drilled to member level): real UpdateTrustedTokenIssuerInput.TrustedTokenIssuerConfiguration is types.OidcJwtUpdateConfiguration -- it has NO IssuerUrl member at all (immutable post-creation) and its remaining 3 fields are independently optional -- but the handler reused Create's all-required, IssuerUrl-carrying shape and the backend wholesale-replaced the stored OidcJwtConfiguration, so any config Update wiped IssuerUrl and rejected single-field updates that omitted it (validateOIDCJWTConfig required IssuerUrl unconditionally). Modeled OidcJwtUpdateConfiguration/TrustedTokenIssuerUpdateConfiguration distinct from the Create-side types and merge field by field. See TestUpdateTrustedTokenIssuer_FieldsSurviveIndependentUpdates and the corrected TestUpdateTrustedTokenIssuerWithConfig."}
  ListTrustedTokenIssuers: {wire: ok, errors: ok, state: ok, persist: ok, note: "per-item shape (types.TrustedTokenIssuerMetadata) had an invented InstanceArn member that doesn't exist on the real type (Name/TrustedTokenIssuerArn/TrustedTokenIssuerType only); also MaxResults/NextToken were ignored. Both fixed."}
  DescribeInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "response included an invented 'Tags' member; real DescribeInstanceOutput has none. Fixed (prior pass); tags now only reachable via ListTagsForResource. FIXED (gopherstack-dbwi): EncryptionConfigurationDetails was entirely unpopulated; now returns the real, constant default (EncryptionStatus=ENABLED, KeyType=AWS_OWNED_KMS_KEY) since this SDK version has no Put/UpdateInstanceEncryptionConfiguration op at all -- every instance this backend can produce genuinely has this state, so it's a real default, not fabricated per-instance data. StatusReason (a separate top-level member, documented as useful for non-ACTIVE instance status) remains correctly omitted -- this backend's instances are always ACTIVE, so omitting it is wire-correct, not a gap. FIXED this pass (gopherstack-gt9o): PermissionSetsEnabled (*bool, api_op_DescribeInstance.go:77) is now echoed, sourced from Instance.PermissionSetsEnabled (a new *bool field, set only by UpdateInstance); omitted from the wire entirely (not a fabricated false) until the first UpdateInstance call that supplies it."}
  ListInstances: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass (gopherstack-gt9o): types.InstanceMetadata (types/types.go:495) gained PrimaryRegion/Regions since v1.38.0; ListInstances did not populate either. Regions (types/types.go:522, []RegionMetadata) is now populated from this instance's real AddRegion state via ListRegions -- genuine data, not invented -- and empty (field omitted) for an instance that never called AddRegion. PrimaryRegion (types/types.go:518, *string) is modeled shape-only and permanently left unset: this backend has no caller-settable source for it and RegionMetadata.IsPrimaryRegion is always false (see gaps), so there is no real value to derive it from; an invented region a client reads and acts on is worse than an absent field."}
  ListRegions: {wire: ok, errors: ok, state: ok, persist: ok, note: "MaxResults/NextToken were ignored; now paginated"}
  # --- error-status class fix (every op) ---
  "*": {wire: ok, errors: ok, state: ok, persist: ok, note: "ALL ops: ResourceNotFoundException/ConflictException were mapped to HTTP 404/409. ssoadmin is the plain 'json' (awsjson1.1) protocol with no per-exception @httpError override in its Smithy model (verified against botocore's sso-admin service-2.json): every exception without fault=true (i.e. everything except InternalServerException) is a client fault and real AWS returns HTTP 400 for ALL of them, matching the convention already used in services/secretsmanager (another pure-JSON-protocol service, single http.StatusBadRequest for its whole handler) and DynamoDB (returns 400 for ResourceNotFoundException in production). Fixed handleBackendError + 6 other writeError call sites + every test asserting on the old codes."}
  # --- previously fixed (last sweep, still verified ok) ---
  DescribeRegion: {wire: ok, errors: ok, state: ok, persist: ok}
  AddRegion: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveRegion: {wire: ok, errors: ok, state: ok, persist: ok}
  PutApplicationAccessScope: {wire: ok, errors: ok, state: ok, persist: ok}
  GetApplicationAccessScope: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePermissionSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed nested under 'PermissionSet' -- this IS correct per the real CreatePermissionSetOutput shape, unlike the Application/TrustedTokenIssuer wrapper bugs above"}
  DescribePermissionSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "same 'PermissionSet' wrapper confirmed correct"}
  ListPermissionSets: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePermissionSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePermissionSet: {wire: ok, errors: ok, state: ok, persist: ok}
  AttachManagedPolicyToPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DetachManagedPolicyFromPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok}
  ListManagedPoliciesInPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "MaxResults/NextToken were ignored; now paginated (dedup'd with ListCustomerManagedPolicyReferencesInPermissionSet via generic listPermissionSetSubItems helper)"}
  ListCustomerManagedPolicyReferencesInPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "same pagination fix"}
  PutInlinePolicyToPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok}
  GetInlinePolicyForPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed flat, correct"}
  PutPermissionsBoundaryToPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPermissionsBoundaryForPermissionSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed nested under 'PermissionsBoundary' -- correct"}
  GetApplicationAssignmentConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed flat {AssignmentRequired}, correct"}
  CreateInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed flat {InstanceArn}, correct"}
  UpdateInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed void {} response, correct. FIXED this pass (gopherstack-gt9o): request input gained PermissionSetsEnabled (*bool, api_op_UpdateInstance.go:64, since v1.38.0); handleUpdateInstance now threads it as a *bool, storing whatever value is supplied verbatim and leaving the stored value untouched when the field is omitted. Real AWS documents 'the only accepted value is true' as a business rule (once enabled it cannot be disabled) but that is not a wire-level constraint pinned by the SDK -- not enforced here (being stricter than AWS is a known recurring bug class in this repo); an explicit false is stored as given. EncryptionConfiguration/PermissionSetsEnabled mutual exclusivity is real-API documented but EncryptionConfiguration itself remains entirely unmodeled on this input (out of scope for gopherstack-gt9o -- see gaps)."}
families:
  Instance: {status: ok, note: "CreateInstance/DescribeInstance(fixed)/DeleteInstance/ListInstances(fixed)/UpdateInstance(fixed). PermissionSetsEnabled and ListInstances' Regions now real this pass (gopherstack-gt9o); see ops table."}
  PermissionSet+Policies: {status: ok, note: "managed/inline/customer-managed/permissions-boundary attach-detach + pagination fixed on the two List ops that needed it"}
  AccountAssignment: {status: ok, note: "SEVERE TargetId/TargetType wire-shape bug fixed across Create/Delete/Describe*/List*Status; ListAccountAssignmentsForPrincipal Filter+pagination added"}
  ProvisionPermissionSet+Status: {status: ok, note: "PermissionSetProvisioningStatus shape confirmed correctly distinct from AccountAssignmentOperationStatus (previously incorrectly shared one Go view type using AccountAssignment's field name); List variants slimmed to the real Metadata shape + paginated. NEW this pass: provisioned-vs-edited-since-provisioned drift tracking (PermissionSet.ModifiedDate + backend.provisionedAt) backs the ProvisioningStatus filter on both ListPermissionSetsProvisionedToAccount and ListAccountsForProvisionedPermissionSet -- see those ops' notes."}
  Application+Assignment+AccessScope+AuthMethod+Grant+SessionConfig: {status: ok, note: "SEVERE: Create/Describe/Update Application and DescribeApplicationAssignment all had invented wrapper objects/fields that would break a real client's deserializer (found and fixed 4 of these 'invented wrapper' bugs across Application+AccountAssignment+AuthMethod+Grant+TrustedTokenIssuer -- same bug class recurring across the file, all now fixed). PutApplicationSessionConfiguration/GetApplicationSessionConfiguration fully re-modeled around the real UserBackgroundSessionApplicationStatus field (previously a fabricated SessionDuration concept). GetApplicationAuthenticationMethod/GetApplicationGrant double-wrap bugs fixed. Pagination added to ListApplicationAssignments(ForPrincipal)/ListApplicationAccessScopes/ListApplicationProviders."}
  TrustedTokenIssuer: {status: ok, note: "SEVERE: DescribeTrustedTokenIssuer/UpdateTrustedTokenIssuer had the same invented-wrapper bug class (plus fabricated InstanceArn/Tags members); fixed. ListTrustedTokenIssuers item shape also had an invented InstanceArn; fixed + paginated."}
  InstanceAccessControlAttributeConfiguration: {status: ok}
  Region: {status: ok, note: "ListRegions pagination added this sweep"}
  Tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource confirmed as the sole tag-retrieval path for Application/Instance/TrustedTokenIssuer (all three previously had fabricated inline Tags members on their Describe/singular-Get responses, now removed)"}
gaps:
  - "FIXED (gopherstack-gt9o): DescribeInstanceOutput/UpdateInstanceInput's PermissionSetsEnabled and ListInstances' InstanceMetadata.Regions are now threaded/populated; see DescribeInstance/UpdateInstance/ListInstances ops entries."
  - "InstanceMetadata.PrimaryRegion (ListInstances) remains permanently unset -- no caller-settable or derivable source in this backend (see ListInstances op note). UpdateInstanceInput.EncryptionConfiguration remains entirely unmodeled (pre-existing, out of scope for gopherstack-gt9o -- see UpdateInstance op note)."
  - "RegionMetadata.IsPrimaryRegion is always false -- known simplification, unchanged from prior sweep (bd: none filed). This is also why InstanceMetadata.PrimaryRegion above has no real data to derive from."
  - "ListApplicationAuthenticationMethods/ListApplicationGrants/ListTagsForResource support NextToken on the real API but have no MaxResults member at all (unlike every other List op in this service); gopherstack still returns everything in one page with a nil NextToken for these three. Low-value: there is no MaxResults contract to violate (a real caller can never request a capped page), and this mirrors the same intentional simplification already accepted for other AWS emulators in this codebase. Re-examined this pass (gopherstack-dbwi) and confirmed still not worth building: there is no real behavior gap to close, only a self-imposed pagination-everywhere convention this service already deviates from correctly. (bd: gopherstack-dbwi, considered and left as-is)"
deferred: []
leaks: {status: fixed, note: "no new goroutines/janitors introduced this sweep; all fixes are pure request-parsing/response-shape/backend-field changes inside the existing coarse-lock methods. identityStoreArn() helper reads b.instances while b.mu is already held by the caller (CreateApplication/AddApplicationInternal) -- safe because store.Table.Get has no internal locking (backend-level coarse lock only, confirmed in pkgs/store/table.go), consistent with every other Table access pattern in this backend. 2026-09-04 (gopherstack-2fa): fixed cascadeDeleteInstance leaking b.provisionedAt/b.assignmentCreationIDs rows for force-deleted permission sets (see ops table note above); everything else re-checked this pass (all Delete* handlers) confirmed clean -- tags live as a struct field (immune), applicationScopes/AuthMethods/Grants/AssignConfig/Sessions and customerManagedPolicies/permissionBoundaries are all cleaned on both DeleteApplication/DeletePermissionSet and the DeleteInstance cascade."}
---

## Notes (2026-08-22, gopherstack-r80d batch 30 -- required-output-member audit)

ssoadmin (6 required output fields / 79 ops, 6 ops-with-required per a fresh
`cmd/requiredoutputfields` run, cross-checked against an independent
brace-depth awk walk of `ssoadmin@v1.43.1`'s `api_op_*.go` files -- both
agreed exactly at 6). Read all 6 flagged ops end to end against their
handlers: `DescribeApplicationProvider` (`ApplicationProviderArn`),
`GetApplicationAccessScope` (`Scope`),
`GetApplicationAssignmentConfiguration` (`AssignmentRequired`),
`GetApplicationGrant` (`Grant`), `ListApplicationAccessScopes` (`Scopes`),
`ListApplicationGrants` (`Grants`).

**Protocol question asked and answered no:** every one of these 6 handlers
builds its response as a `map[string]any` literal
(`handler_application_access_scopes.go`, `handler_application_grants.go`,
`handler_applications.go`, `handler_application_assignments.go`), not a
tagged struct marshaled by `encoding/json`/`encoding/xml` -- so the
`omitempty`-on-a-required-field tag-rule this campaign is built on does not
apply here at all: a map-literal key is written unconditionally regardless
of its value's zero-ness, so there is no tag to go stale. Every required
key is present in every success-path literal (confirmed by reading each
handler directly, not grepped).

**Followed one wrapped type below the flat scan:** `ListApplicationAccessScopesOutput.Scopes`
is `[]types.ScopeDetails` (ssoadmin@v1.43.1 types/types.go:821-832), whose
own `Scope` member is separately required -- invisible to the flat op-level
scan since the array field itself isn't required-flagged at the item
level. gopherstack's own `ScopeDetails` (models.go:341) is a genuine tagged
struct (`Scope string \`json:"Scope"\`` with no `omitempty`), correctly
never omitting the key even for a zero-value scope. `GrantItem`'s `Grant`
union member (types.go:412-455) is passed through as opaque
`json.RawMessage` (never reconstructed), which is architecturally correct
for a union type gopherstack cannot decode -- `PutApplicationGrant`
defensively substitutes literal `"null"` for an empty body rather than
storing a truly missing value, and any real client-side omission of the
required `Grant` field on `PutApplicationGrant` would already be rejected
by the real SDK's own input validator before reaching this backend (same
disqualifying rule as batch 29's `PutScalingPolicy` gap), so this null-body
edge case is unreachable via any real client and not a bug.

**Result: 0 bugs.** All 6 ops confirmed already correct; no code changes.
This service was already A-graded with multiple prior wire-shape sweeps
(see `overall:` above); this pass adds nothing to that history except
closing out this specific bug class by name.

## Notes (2026-08-07 pass, gopherstack-dbwi)

### ProvisioningStatus filter: provisioned-vs-edited-since-provisioned drift tracking

The prior sweep's gap entry called this "a much larger feature" and left it unimplemented.
Re-examined this pass and found it genuinely buildable at a scope proportionate to the rest
of this service's op surface, not requiring a new subsystem:

- `PermissionSet` gained an internal-only `ModifiedDate` field (not a real AWS wire field --
  `types.PermissionSet` has no such member -- confirmed by reading the SDK's struct
  directly). `bumpModified` (`permission_sets.go`) stamps it to now, called from every op
  that changes a permission set's effective content: `UpdatePermissionSet` (only when a
  field actually changes), `Attach`/`DetachManagedPolicyToPermissionSet`,
  `Attach`/`DetachCustomerManagedPolicyReferenceToPermissionSet`,
  `Put`/`DeleteInlinePolicyToPermissionSet`, `Put`/`DeletePermissionsBoundaryToPermissionSet`.
  `CreatePermissionSet`/`AddPermissionSetInternal` initialize it to `CreatedDate`.
- A new backend map, `provisionedAt` (keyed by `instanceArn|permissionSetArn|accountID`),
  records the last time a permission set was (re-)provisioned to a specific account.
  `CreateAccountAssignment` stamps it implicitly (real AWS documents that this op also
  provisions the permission set to the account if not already provisioned).
  `ProvisionPermissionSet` stamps it explicitly: `AWS_ACCOUNT` marks the one target account;
  `ALL_PROVISIONED_ACCOUNTS` marks every account currently assigned the permission set (this
  backend has no broader "every account in the org" concept, so `ALL_PROVISIONED_ACCOUNTS` is
  scoped to already-provisioned accounts, matching its own name). `DeleteAccountAssignment`
  removes the `provisionedAt` entry once no assignment for that account+permission-set pair
  remains, avoiding a ghost row.
- `ListPermissionSetsProvisionedToAccount`/`ListAccountsForProvisionedPermissionSet` compare
  `ModifiedDate` against `provisionedAt` per (account, permission set) pair: not-after means
  `LATEST_PERMISSION_SET_PROVISIONED`, after (or no `provisionedAt` record at all --
  conservative default) means `LATEST_PERMISSION_SET_NOT_PROVISIONED`.
- Both `backendSnapshot` (Assignments/ProvisionedAt) and `PermissionSet.ModifiedDate` are new
  persisted shapes; `ssoadminSnapshotVersion` bumped `2 -> 3`.
- The handler layer validates `ProvisioningStatus` is empty or one of the two real enum
  values (`LATEST_PERMISSION_SET_PROVISIONED`/`LATEST_PERMISSION_SET_NOT_PROVISIONED`) rather
  than silently accepting anything, matching this service's existing validation conventions.

### DescribeInstance's EncryptionConfigurationDetails

Checked whether a `Put`/`UpdateInstanceEncryptionConfiguration` op exists anywhere in the
pinned `aws-sdk-go-v2/service/ssoadmin@v1.38.0` -- it does not. Encryption configuration is
entirely read-only via this API version, so every instance this backend can ever produce has
exactly the same real default AWS documents for an instance whose KMS key was never (and, via
this API, can never be) switched to customer-managed: `EncryptionStatus: ENABLED`,
`KeyType: AWS_OWNED_KMS_KEY`, no `KmsKeyArn`. This is populated as a real constant, not
per-instance fabricated data -- there is nothing dynamic to get wrong since no write path
exists. `StatusReason` (a separate top-level `DescribeInstanceOutput` member, distinct from
`EncryptionConfigurationDetails.EncryptionStatusReason`) is documented as useful specifically
for a non-`ACTIVE` instance status; this backend's instances are always `ACTIVE` (no
`CREATE_FAILED`/error-during-delete path modeled), so omitting it remains wire-correct --
re-classified from "gap" to "correct by construction," not fixed as a bug.

## Notes (2026-07-24 sweep)

This sweep re-audited every op's wire shape byte-for-byte against
`aws-sdk-go-v2/service/ssoadmin@v1.38.0`'s real `deserializers.go` (not just the exported
`types` package structs, which don't reveal wrapper nesting) and found a recurring, severe bug
class that the prior sweep's `overall: A` rating missed entirely:

**Invented response wrappers ("double-nesting").** Several singular Describe/Update/Get/Create
handlers wrapped their entire response payload one level too deep under a fabricated top-level
key (e.g. `{"Application": {...}}`, `{"ApplicationAssignment": {...}}`,
`{"TrustedTokenIssuer": {...}}`) that does not exist anywhere in the real SDK. The real ops in
question (`DescribeApplication`, `DescribeApplicationAssignment`, `DescribeTrustedTokenIssuer`,
`UpdateApplication`, `UpdateTrustedTokenIssuer`, `CreateApplication`) are all **flat** or **void**
outputs per their real `awsAwsjson11_deserializeOpDocument*Output` functions. A real
`aws-sdk-go-v2` client calling any of these against the old code would get every single output
field `nil`/zero-valued -- not a subtle bug, a total break of the operation. Root-caused by
reading only the exported Go struct shape and assuming AWS's common "wrap the resource under its
type name" convention (which IS correct for `CreatePermissionSet`/`DescribePermissionSet`/
`GetPermissionsBoundaryForPermissionSet`, confirmed still correct this sweep) applies universally
-- it doesn't; each op's wrapping is independently modeled in Smithy and has to be checked against
the deserializer, not inferred from a pattern seen elsewhere in the same service.

A second, narrower instance of the same class: `GetApplicationAuthenticationMethod` and
`GetApplicationGrant` union-typed responses were wrapped in an *extra* discriminator pair
(`{"AuthenticationMethodType": ..., "AuthenticationMethod": ...}`) copied from the sibling `List*`
op's per-item shape (`AuthenticationMethodItem`/`GrantItem`, which legitimately has both fields as
siblings) -- but the singular `Get*Output` shapes have **only** the union member, no sibling type
field. This is the same underlying mistake (pattern-matching one op's shape onto a
similar-but-different op) at one level of nesting instead of two.

**Fabricated fields on otherwise-correct shapes.** Independent of the wrapper bug:
`DescribeInstance`/`DescribeApplication`/`DescribeTrustedTokenIssuer` all echoed a `Tags` array
inline -- none of the three real `Describe*Output` shapes have a `Tags` member at all (tags are
always a separate `ListTagsForResource` call in real ssoadmin, confirmed for every taggable
resource type). `DescribeTrustedTokenIssuer`/`UpdateTrustedTokenIssuer`/`ListTrustedTokenIssuers`
also fabricated an `InstanceArn` member that doesn't exist on `TrustedTokenIssuerMetadata` or
`DescribeTrustedTokenIssuerOutput`. `PutApplicationAssignmentConfiguration` accepted an invented
`AssignmentRequiredForAllIdentities` request field with no real counterpart.
`AccountAssignmentOperationStatus` (the Create/Delete/Describe*Status shape for account
assignments) used a fabricated `AccountId` field where the real member is `TargetId`/`TargetType`
-- confirmed via the real `awsAwsjson11_deserializeDocumentAccountAssignmentOperationStatus`,
which has no `AccountId` case at all. All three `List*Status` metadata shapes
(`AccountAssignmentOperationStatusMetadata`, `PermissionSetProvisioningStatusMetadata`) are
slimmer than their singular counterparts (`CreatedDate`/`RequestId`/`Status` only) and were
previously returning the full fat shape.

**A whole fabricated concept.** `PutApplicationSessionConfiguration`/
`GetApplicationSessionConfiguration` modeled a `SessionDuration` string (confused with the
unrelated, legitimate `PermissionSet.SessionDuration` field) that does not exist anywhere on
these two real operations. The real member is `UserBackgroundSessionApplicationStatus`
(`ENABLED`/`DISABLED` enum), confirmed via both operations' real Go SDK source. Fully re-modeled
end-to-end (interface signature doc-comment, backend validation, both handlers, both wire
responses) since there were no external callers of the backend method outside this package
(verified via repo-wide grep before changing the signature's semantics).

**Missing pagination.** ~13 List operations that the real SDK models with `MaxResults`/`NextToken`
were silently ignoring both and always returning every result with a nil `NextToken` --
technically tolerated by the Go SDK client (which just gets everything on "page 1"), but a real
`MaxResults` cap is a hard client-side contract that was being violated. Fixed via the existing
`paginateBy` helper for alphabetically-orderable list items, and a new `paginateOrdered` helper
(does NOT re-sort, only locates the resume point by a unique key) for the three
`ProvisioningStatus`-derived lists that must preserve the backend's `CreatedDate`-descending
order -- using `paginateBy` there would have silently scrambled `TestListPermissionSetProvisioningStatusSorted`'s
established chronological-order contract into alphabetical-by-RequestId order, since RequestId is
a random UUID with no correlation to creation time.

**Error-status class bug (every op).** `ResourceNotFoundException`/`ConflictException` were
mapped to HTTP 404/409 throughout the handler. ssoadmin's Smithy model
(`botocore/data/sso-admin/2020-07-20/service-2.json`, cross-checked) has no per-exception
`error.httpStatusCode` override on any of its 7 exception shapes; for the plain `"json"`
(awsjson1.1) protocol this means every exception without `"fault": true` (i.e. everything except
`InternalServerException`) is a client fault that real AWS returns as **HTTP 400**, full stop --
this is the same behavior confirmed for DynamoDB (another pure-JSON-protocol service) and already
implemented correctly in `services/secretsmanager` (single `http.StatusBadRequest` for its entire
handler, no per-exception-name status logic at all). The `aws-sdk-go-v2` client itself doesn't
care about the exact status code (it dispatches on the body's `__type`/error-code string as long
as the status is non-2xx), so this was invisible to Go-SDK-based tests, but any HTTP-status-aware
caller (a different SDK, curl, a test harness asserting on raw status) would see the wrong code.
Fixed `handleBackendError` (the shared error-mapping function) plus 6 other inline
`writeError(..., http.StatusConflict/StatusNotFound, ...)` call sites that bypassed it, and every
test in the package asserting on the old codes (~50 call sites via blanket `sed`, verified each
was actually a ssoadmin-exception-status assertion, not an unrelated HTTP semantic, before the
bulk edit).

**Dedup.** The three `ProvisioningStatus` metadata list handlers (`ListAccountAssignmentCreationStatus`/
`ListAccountAssignmentDeletionStatus`/`ListPermissionSetProvisioningStatus`) and the two
permission-set-subresource list handlers (`ListManagedPoliciesInPermissionSet`/
`ListCustomerManagedPolicyReferencesInPermissionSet`) were each near-identical once the pagination
fix was applied (flagged by `golangci-lint`'s `dupl` linter). Extracted into two generic
package-level helpers (`listProvisioningStatusMetadata[T]`, `listPermissionSetSubItems[T]`) in
`handler.go` rather than suppressed with `//nolint:dupl`; each handler is now a 5-8 line call into
the shared helper.

- Persistence: no snapshot-shape changes this sweep (`ssoadminSnapshotVersion` unchanged);
  `Application` gained three new backend-only fields (`ApplicationAccount`/`CreatedFrom`/
  `IdentityStoreArn`) which round-trip automatically since the whole struct is already
  JSON-snapshotted by value.
- Carried over unmodified from the prior sweep (still verified correct): the awsjson1.1
  `X-Amz-Target` routing, epoch-seconds timestamp handling, and the Region family's
  ADDING/ACTIVE/REMOVING lazy-transition + lazy-prune pattern.

## gopherstack-zquj (2026-08-22): type-checked hand-written-key sweep, clean

Direct followup to the required-output-member note above, which found
ssoadmin's map-literal responses immune to `omitempty` bugs but flagged
that immunity as exposure to a *different*, unchecked class: a hand-typed
map key with a typo or wrong case is dropped on the wire by a real client
and caught by nothing in this repo. 89 `map[string](any|interface{}|
string)` literal openings in the non-test package (`grep -rEo
'map\[string\](any|interface\{\}|string)\{'`), against ssoadmin@v1.43.1.

Built a type-checked scanner (`keycheck`, same tool as shield's zquj note
above; not committed into this repo this pass, see that note for why) that
diffs every hand-written key against the real key set derived from
`ssoadmin@v1.43.1`'s `deserializers.go` AST (`awsAwsjson11_
deserializeOpDocument*`/`awsAwsjson11_deserializeDocument*` case-switch
lists), not the Go struct field names. Instrument validated in two parts
before trusting a clean result: the SDK-side case-list parser against the
known-bad scheduler casing bug (gopherstack-r80d batch 32, commit
`8469dcdd9` -- see shield's zquj note for detail); the handler-side
map-literal/`X["key"]=` walker against a synthetic fixture with a planted
typo'd key, caught with zero false positives on correctly-keyed siblings.

**Two scanner false-positive classes found and excluded before trusting a
clean result** (same as shield): `map[string]struct{}`/`map[string]bool`
validation-set literals excluded by value type; the shared
`__type`/`message` error-envelope keys (written by `writeError`,
handler.go:702-706, reachable from nearly every op's error path) excluded
as protocol-reserved -- confirmed by grep that `"message"`/`"__type"`
never appear as a case key in any *success*-path `OpDocument*Output`
deserializer in this SDK version, only in exception-type deserializers, so
excluding them cannot mask a real success-path bug.

**Result: 73 real ops resolved (79 dispatched minus 6 ops whose
`*Output` struct is genuinely empty -- `DeleteApplicationAccessScope`/
`DeleteApplicationAuthenticationMethod`/`DeleteApplicationGrant`/
`PutApplicationAccessScope`/`PutApplicationAuthenticationMethod`/
`PutApplicationGrant` -- confirmed by reading each `api_op_*.go`: the
output struct holds only `ResultMetadata`/`noSmithyDocumentSerde`, so the
SDK's codegen emits no `OpDocument*Output` deserializer at all, and the
handlers correctly write `map[string]any{}`), 129 tracked written keys, 0
keys outside the op's real reachable wire shape.**

Spot-checked the highest-surface ops by hand rather than trusting the tool
alone: `DescribeApplication`'s 11 written keys
(handler_applications.go:128-140, already carrying a comment citing
`awsAwsjson11_deserializeOpDocumentDescribeApplicationOutput`,
deserializers.go:14289) match exactly; `CreateAccountAssignment` and its
sibling status ops (written=1, allowed=10) turned out to wrap an
already-audited tagged struct (`accountAssignmentStatusView`,
handler.go:549-559) inside one `map[string]any` key -- the low written
count is the scanner correctly not descending into a struct literal it
doesn't need to check (that construction is covered by the required-field
sweep above), not a missed site.

**Known blind spot, disclosed**: the scanner checks "does this key exist
anywhere in the op's reachable shape," not "at the right nesting level" --
a same-named key misplaced one level off would not be caught. None found
on manual spot-check of the largest-surface ops (above). Site count here
(89) differs from the 622 cited when gopherstack-zquj was filed; per
zquj's own warning that grep-derived scopes in this campaign have been
wrong by as much as 11x, that gap is expected and is exactly why this
sweep verified per-op against the real deserializer rather than trusting
either number.

## 2026-08-30 (wrapper-key sweep): exhaustive request-field-read audit, no new bugs

Method: derived the operation list from the handler.go dispatch table
(`handler.go` map-literal registrations, `(*Handler).handle*` entries) rather than
trusting this file's prose or a naive whole-token regex over PARITY.md -- 79 dispatched
ops, matching the zquj sweep's independently-derived count. For every request struct
(named or `var req struct{...}`/`var body struct{...}` anonymous, both patterns used in
this handler) across every non-test `.go` file, cross-referenced each JSON-tagged field
against a combined-text search of the whole non-test package for `.FieldName` usage
anywhere -- catching the "declared field never read at all" shape without trusting any
single file's local context (a per-file-only version of this scan false-positived on
structs whose fields are used in a different file). Confirmed protocol directly from the
pinned SDK: `awsAwsjson11_*` prefix throughout `ssoadmin@v1.43.1/deserializers.go` --
plain JSON-RPC 1.1 over `X-Amz-Target`, no legacy/query path reachable by any real client
for this service.

**Result: zero unread request fields found.** Every request struct's fields (including
the six `Filter.*`/pagination fields already fixed by prior passes -- `ListApplications`'
`Filter.ApplicationAccount`/`Filter.ApplicationProvider`, `ListAccountAssignmentsForPrincipal`'s
`Filter.AccountId`, `ListApplicationAssignmentsForPrincipal`'s `Filter.ApplicationArn`, both
`ProvisioningStatus` filters) are consumed. Consistent with this file's own extensive prior
sweep history (2026-07-24 through 2026-08-30) already having driven this specific bug class to
zero; this pass's contribution is an independent re-derivation confirming that, not new fixes.

**Negative checks, explicitly:**
- **Listing that never consults its store**: none -- every `handle(List|Describe|Get)*`
  function reaches `h.Backend.*`, scripted check across every `handler_*.go`, zero
  exceptions.
- **Handler that discards its entire request**: none -- every `handle*(ctx, in *Type)`
  function references at least one `in.Field`, scripted check, zero exceptions.
- **Ordering / tie-prone sorts**: the three `ProvisioningStatus`-metadata list ops'
  CreatedDate-tie-over-map-walk bug (see ops table, "wrapper-key-sweep
  pagination-reproducibility pass" above) is this service's real instance of the class and
  is already fixed this same day. `listPermissionSetSubItems`/`paginateBy`-backed lists sort
  by each resource's own Name/Arn (unique), so no further tiebreak is needed.

No code changed this pass. Gates: `go build ./services/ssoadmin/...`, `go vet
./services/ssoadmin/...` and `go vet ./...` (repo-wide), `go test -race -count=1
./services/ssoadmin/...`, `golangci-lint run ./services/ssoadmin/...`.

## 2026-09-08: writeJSON/writeError/handleBackendError nil-on-write fall-through audit (gopherstack-246v) -- clean

Part of the sweep following the elasticache fix (gopherstack-8haq): `writeJSON`
(`handler.go:692`) writes headers/body and unconditionally `return nil`s. `writeError`
(`handler.go:702`) wraps it directly. `handleBackendError` (`handler.go:616`) maps a
backend sentinel/string error to a wire type and status, always via `return writeError(...)`.
Any helper that rejects through one of these and is called by code storing and checking the
result would get a silent nil and fall through past the rejection.

**Method (mechanical).** A `go/parser`/`go/ast` script over every non-test file in this flat
package (16k lines, no subdirectories) computed the fixed-point closure seeded with
`{writeJSON, writeError, handleBackendError}`: find every function with a bare
`return <sink>(...)`, add it, repeat to convergence. `Handler` (`handler.go:405`) is
ssoadmin's dispatch entry, registered directly with echo; its own unrecognized-action
fallback is a direct `return writeError(...)`, and `dispatch` (`handler.go:532`) — the
switch `Handler` calls into — likewise ends its default case in a direct return, so both,
and every `handleXxx` op handler `dispatch` calls, were pulled into the closure
automatically wherever their own body ends in one of these direct-return shapes; their call
sites were checked in the same sweep rather than partitioned separately.

The closure converged at 86 functions (the 3 seeds, `Handler`, `dispatch`, and 81 discovered
`handleXxx` op handlers). Every call site of every one of those 86 was re-walked and
classified: 299 total call sites (production and test). 294 are `return <fn>(...)`
(direct-return, safe). The remaining 5 are `h.Handler()` calls in test files
(`handler_sdk_route_table_test.go:171`, `handler_test.go:47,77,326`,
`iam_enforcement_test.go:85`) obtaining the exported `echo.HandlerFunc`-returning method —
a name collision with the discovered closure, not a stored-then-checked error from any
response-writer helper.

**No instance of the broken shape exists in ssoadmin.** No code changed. Gates:
`GOTOOLCHAIN=go1.27.0 golangci-lint run ./services/ssoadmin/...` 0 issues;
`GOTOOLCHAIN=go1.27.0 go test -race ./services/ssoadmin/...` ok.
