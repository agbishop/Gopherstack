---
service: iam
sdk_module: aws-sdk-go-v2/service/iam@v1.63.0   # version audited against (go.mod pin; was stale
  # at v1.55.0 -- SimulateCustomPolicy/SimulatePrincipalPolicy's response shape
  # changed in v1.57.0 (top-level results now aggregate across resources instead
  # of one entry per resource); policy simulation was already flagged as NOT
  # re-verified this sweep (see items_still_open), so no live claim broke, but
  # its "already marked ok/PROVEN by sweeps 1-4" history is now stale too.
last_audit_commit: 202a5afdf
last_audit_date: 2026-08-29
overall: A   # sweep 13 (wrapper-key sweep, uncommitted as of this note): fixed
  # ListAttached{User,Role,Group}Policies dropping PathPrefix/Marker/MaxItems entirely
  # (silent unfiltered, unpaginated full list) and policyNameFromARN's wrong-separator
  # bug (PolicyName wire field polluted with Path segments for non-default-Path
  # policies). ListEntitiesForPolicy confirmed to share the same PathPrefix-drop shape;
  # closed separately (gopherstack-fjmw) with a small StorageBackend surface addition
  # (PermissionsBoundaryEntities). See items_still_open and the ops: entries for both
  # for detail.
  # sweep 12 (order-bug pattern hunt): all 8 List*Tags operations
  # (ListRoleTags, ListPolicyTags, ListUserTags, ListInstanceProfileTags, ListMFADeviceTags,
  # ListSAMLProviderTags, ListOpenIDConnectProviderTags, ListServerCertificateTags) built their
  # response by ranging a map[string]string directly with no sort -- raw Go map order, which can
  # differ between two calls with no mutation in between -- despite every one of these ops'
  # own doc comment stating "The returned list of tags is sorted by tag key." tagsMapToKV's own
  # doc comment already claimed "converts map[string]string to sorted svcTags.KV slice" while its
  # body did not sort at all. Fixed by making tagsMapToKV actually sort (slices.SortFunc by Key)
  # and routing every List*Tags handler through it (previously 3 of the 8 called it, the other 5
  # duplicated the same unsorted-range logic inline in resourceTagDispatch/handler_mfa.go).
  # Proven via TestListTags_SortedByKey (handler_create_tags_test.go): drives all 8 kinds through
  # the real SDK client with 3 out-of-order tag keys, asserts alphabetical order; 7 of 8 subtests
  # failed against the unfixed code (the 8th passed by map-iteration chance that run, underscoring
  # why this bug class survives a single-run test).
  # sweep 11 (gopherstack-iam-signing-cert-ownership follow-up, this pass): worked
  # items_still_open's named queue. Fixed ListSigningCertificates' disclosed Marker/MaxItems
  # pagination gap (sweep 10 left it deliberately unfixed) and, while implementing it, found
  # sibling ListSSHPublicKeys had a second real gap in the same area: its response never
  # echoed Marker at all despite genuinely paginating server-side, so IsTruncated=true carried
  # no continuation token a real client could use -- fixed both. Implemented
  # GetDelegationRequest/ListDelegationRequests for real (previously disclosed
  # validation-only/always-empty stubs) now that the delegation-request family's backend state
  # is fully real as of sweeps 7-9; found and fixed a latent "PENDING" status string with no
  # match in types.StateType (invisible until this sweep started actually serializing State).
  # Found and fixed GetAccountSummary's "SAMLProviders" key -- not a real SummaryKeyType, and
  # OIDCProviders was never surfaced at all; real IAM reports both under one "Providers" key.
  # Documented (but left as pure no-behavior-change dead code, matching the SSH-key
  # completeness-pass precedent) 2 more shadowed-duplicate dispatch entries found while
  # auditing access advisor. GetCredentialReport/GenerateCredentialReport re-verified clean
  # (empty real inputs; output field names/values all correct). sweep 10 (gopherstack-iam-signing-cert-ownership): fixed a real ownership-bypass
  # bug in UpdateSigningCertificate/DeleteSigningCertificate (see ops entry) found by the
  # required-member sweep this PARITY.md's items_still_open had flagged as "not re-verified
  # since sweeps 1-4" for the SSH key / signing certificate CRUD family.
  # sweep 9 (gopherstack-xh42): closed the last 2 delegation-family issues sweep 8
  # disclosed but left out of its named scope -- AssociateDelegationRequest's phantom
  # PolicyArn (removed, dead code, no real backend effect) and Accept/AssociateDelegationRequest
  # returning InvalidAction (400) instead of their own declared NoSuchEntity (404).
  # sweep 8: RejectDelegationRequest/SendDelegationToken/UpdateDelegationRequest
  # (gopherstack-qb3x) -- the other 3 members of the delegation-request family sweep 7
  # left flagged -- fixed the same silently-ignored-DelegationRequestId shape as
  # sweep 7's CreateDelegationRequest fix, and now genuinely mutate CreateDelegationRequest's
  # stored state instead of validating and discarding. Response shape for all 3 was
  # already correct (real Reject/Send/UpdateDelegationRequestOutput carry no members).
  # sweep 7: 4 required-member drops fixed (UploadServerCertificate.PrivateKey,
  # GetSSHPublicKey.Encoding, SetSecurityTokenServicePreferences.GlobalEndpointTokenVersion,
  # CreateDelegationRequest's 4 scalars + Permissions), plus a wrong wire shape
  # (CreateDelegationRequestResult) and GetHumanReadableSummary's first-ever entry.
protocol: aws-query -> XML
families:
  users_groups_roles: {status: ok, note: "CRUD + path/ARN verified; DeleteUser/DeleteRole/DeleteGroup/DeleteInstanceProfile now field-diffed against the real AWS 'before you delete' dependency lists (see ops below) instead of silently cascading"}
  policies:      {status: ok, note: managed+inline, versions, default version; PolicyXML has Tags field; DeletePolicy attachment check pre-existing and correct}
  access_keys:   {status: ok, note: create/rotate/status, secret only on create; DeleteUser no longer cascade-deletes keys (see ops)}
  providers:     {status: ok, note: SAML/OIDC CRUD, server certificates, login profile, password policy; tag-leak on delete/rename fixed this sweep}
ops:
  ListAttachedUserPolicies/ListAttachedGroupPolicies/ListAttachedRolePolicies: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 13, wrapper-key sweep). All three real Inputs (api_op_ListAttachedUserPolicies.go et al) declare PathPrefix, Marker, MaxItems; the handlers (handler_policies.go, handler_groups.go) read only UserName/GroupName/RoleName -- PathPrefix silently dropped (unfiltered full list returned regardless of the filter), and no pagination at all (IsTruncated always false, no Marker in the response struct -- ListAttached{User,Role,Group}PoliciesResult had no Marker field to begin with). Structural cause: StorageBackend's ListAttached*Policies(name) return []AttachedPolicy, which carries no Path -- fixed at the handler layer instead of widening the backend interface: new listAttachedPoliciesFiltered helper (handler_list_filters.go) resolves each attached policy's Path via the existing Backend.GetPolicy(arn) and paginates with pkgs/page, same page.New template used by the sibling ListUsers/ListRoles/ListGroups/ListInstanceProfiles fix (sweep 12's PathPrefix-family header comment). Added Marker to all 3 Result structs. In the course of writing the PathPrefix regression test, also found and fixed a second, independent bug in the same code path: policyNameFromARN (policies.go) returned everything after 'policy/' in the ARN instead of everything after the final '/', so any policy with a non-default Path (e.g. arn:...:policy/team/name) had its PolicyName wire field polluted with the path segments ('team/name' instead of 'name') in every one of these 3 list ops plus simulation.go's attached-policy resolution. Proven via TestListAttachedPolicies_PathPrefix (list_filter_params_test.go), a real-SDK-client test with one matching and one non-matching Path per resource kind (user/group/role); all 3 subtests fail against unmodified code (2 items returned instead of 1, and the surviving item's PolicyName wrong)."}
  ListEntitiesForPolicy: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-fjmw). Real ListEntitiesForPolicyInput (api_op_ListEntitiesForPolicy.go) declares PolicyArn, EntityFilter, PathPrefix, PolicyUsageFilter, Marker, MaxItems (wire keys identical to the Go field names, confirmed against serializers.go's awsAwsquery_serializeOpDocumentListEntitiesForPolicyInput). EntityFilter was already read and applied at the backend (InMemoryBackend.ListEntitiesForPolicy); PathPrefix/PolicyUsageFilter/Marker/MaxItems were parsed nowhere, IsTruncated was hardcoded false, and the Result struct had no Marker field at all. PathPrefix filters each returned ENTITY's own path (not the policy's, confirmed against the input's own doc comment), resolved per entry via the existing GetUser/GetGroup/GetRole accessors -- no new lookup needed there, since those accessors already existed on StorageBackend. PolicyUsageFilter (types.PolicyUsageType: PermissionsPolicy | PermissionsBoundary -- both legal, not inert) needed a genuinely new capability: the backend had no way to report which users/roles hold policyArn as their PERMISSIONS BOUNDARY (as opposed to a normal Attach*Policy attachment) -- groups have no permissions boundary concept in real IAM. New StorageBackend method PermissionsBoundaryEntities(policyArn) (policies.go), the one storage-surface addition, is a reverse scan of b.users/b.roles by PermissionsBoundary field, mirroring the existing PermissionsBoundaryARNs() pattern. This also fixed a second, independent correctness bug uncovered while designing the fix, not just a missing filter: entities that hold policyArn ONLY as their permissions boundary (never Attach*Policy'd) were entirely absent from the unfiltered listing before this fix, contradicting the input's own doc comment describing both usage kinds as in scope. New listEntitiesForPolicyFiltered (handler_list_filters.go) unions attached-usage and boundary-usage per entity, applies PathPrefix/PolicyUsageFilter, and concatenates User+Group+Role into ONE slice paginated by a single page.New call (not three independently-cut per-kind pages, which would misplace the page boundary between kinds). Entity names are stored directly (never derived by splitting an ARN), so the policyNameFromARN-class bug found in the ListAttached* sibling fix does not recur here. Proven via TestListEntitiesForPolicy_PathPrefix/_MarkerResumesAcrossPageBoundary/_PolicyUsageFilter (list_filter_params_test.go), real-SDK-client tests; all 3 fail against unmodified code (PathPrefix returns both entities instead of 1; PolicyUsageFilter=PermissionsBoundary returns the wrong entity because boundary-only entities were absent; a 4-entity walk at MaxItems=1 returns all 4 in one page instead of one per page)."}
  ListRoleTags/ListPolicyTags/ListUserTags/ListInstanceProfileTags/ListMFADeviceTags/ListSAMLProviderTags/ListOpenIDConnectProviderTags/ListServerCertificateTags: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 12, order-bug pattern hunt), first PARITY.md entry for this class. All 8 of IAM's List*Tags operations document 'The returned list of tags is sorted by tag key' (e.g. api_op_ListRoleTags.go:14) verbatim, but built their response by ranging a map[string]string with no sort -- raw Go map order, wrong per the doc and nondeterministic run to run. tagsMapToKV (handler_tags.go) already claimed 'sorted' in its own doc comment while not sorting; fixed to actually sort by key and routed every one of these 8 ops through it (resourceTagDispatch's generic List<kind>Tags closure and handler_mfa.go's ListMFADeviceTags closure previously duplicated the same unsorted logic inline instead of calling it). Proven by TestListTags_SortedByKey (handler_create_tags_test.go), a real-SDK-client round trip covering all 8 resource kinds with 3 out-of-order tag keys."}
  UpdateAccountPasswordPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-21 (gopherstack-c8ge, Scope A audit): singleton with no Create op, checked for the Update-vs-previous-Update merge bug. CONFIRMED CORRECT AS WHOLESALE REPLACE, not a bug: real UpdateAccountPasswordPolicyInput's own doc comment (api_op_UpdateAccountPasswordPolicy.go) states plainly 'This operation does not support partial updates. No parameters are required, but if you do not specify a parameter, that parameter's value reverts to its default value.' The existing b.passwordPolicy = &pp full-struct assignment already matches this documented contract exactly; no change made."}
  ListInstanceProfilesForRole: {wire: ok, errors: ok, state: ok, persist: ok, note: real backend-wired (fixed sweep 3)}
  GetAccountAuthorizationDetails: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 6): Marker/MaxItems/Filter now honored — Filter (User/Role/Group/LocalManagedPolicy/AWSManagedPolicy) restricts which of the 4 lists are populated (this mock has no AWS-managed-policy catalog, so AWSManagedPolicy always yields none); Marker/MaxItems paginate the combined Users+Groups+Roles+Policies sequence in XML field order, matching AWS's single Marker/MaxItems pair spanning all four lists. IsTruncated/Marker now populated in the response instead of always false/empty."}
  GetRole/GetRolePolicy/GetUserPolicy/GetGroupPolicy/GetPolicyVersion: {wire: ok, note: policy documents percent-encoded (RFC 3986) at wire boundary; stored plain}
  DeleteUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 5): previously cascade-deleted access keys/login profile/group membership instead of rejecting, diverging from real AWS (DeleteUser API doc: caller must remove password, access keys, signing cert, SSH key, service-specific creds, MFA device, inline policies, attached policies, group memberships FIRST or the call fails). Now returns DeleteConflict for every one of those 9 dependency kinds, in AWS's documented order, matching real behavior exactly. SSH-key/MFA checks read comprehensiveBackend before taking b.mu per the existing no-nested-lock convention."}
  DeleteRole: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 5): added the missing instance-profile-attachment DeleteConflict check documented by the real DeleteRole API (inline/attached-policy checks already existed). Previously a role attached to an instance profile could be deleted, leaving a ghost role-name string in InstanceProfile.Roles."}
  DeleteGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 5): real DeleteGroup requires 'the group must not contain any users or have any attached policies' — the group-membership half of that check was missing (members were silently cleared instead of blocking). Now returns DeleteConflict; policy-attachment check already existed."}
  DeleteInstanceProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 5): real DeleteInstanceProfile requires 'the instance profile must not have an associated role' — this check was entirely absent. Now returns DeleteConflict."}
  UpdateUser/UpdateGroup (rename): {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 5): renaming a user/group with an attached managed policy updated the forward index (userPolicies/groupPolicies) but left the reverse policyAttachments index (used by DeletePolicy's conflict check, ListEntitiesForPolicy, Detach*Policy) keyed by the OLD name — a ghost attachment that could never be cleared under the new name and could permanently block DeletePolicy with a stale conflict. New renamePolicyAttachmentsLocked helper keeps both indexes in sync; regression tests added."}
  tag-cleanup-on-delete (5 resource kinds): {wire: ok, state: ok, persist: ok, note: "Covers Delete/UpdateServerCertificate, DeleteInstanceProfile, DeleteSAMLProvider, DeleteOpenIDConnectProvider, DeleteVirtualMFADevice. FIXED (sweep 5): these 5 resource kinds are tagged via a Handler-level map (h.tags, keyed by \"prefix:name/ARN\") separate from the backend entity, because the backend model itself carries no Tags field for them. Delete handlers never cleared the entry, so a resource re-created with the same name/ARN after deletion silently inherited the deleted resource's tags (ghost row). Added Handler.deleteTags/renameTags and wired them into all 5 delete paths plus UpdateServerCertificate's rename path."}
  UploadServerCertificate: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 7): required PrivateKey (api_op_UploadServerCertificate.go:95) was read nowhere -- handler_server_certificates.go never even looked at it, so a request missing it (or containing garbage) succeeded with 200 and nothing validated a well-formed key was sent. Now validated in the handler for presence (InvalidInput, this op's own declared error) and PEM shape via encoding/pem (MalformedCertificate, also declared) -- the value itself is never stored, logged, or echoed back. It is a credential and real AWS never returns a private key either; no existing secret-handling pattern exists elsewhere in this service to follow (SecretAccessKey IS stored/returned, unlike a TLS private key), so validate-without-store is the deliberate choice here, not an oversight."}
  GetSSHPublicKey: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 7): required Encoding (api_op_GetSSHPublicKey.go:42, types.EncodingType SSH|PEM) was ignored -- the stored body was always returned verbatim regardless of the requested encoding. Now genuinely converts: UploadSSHPublicKey accepts either ssh-rsa (authorized_keys) or PEM SubjectPublicKeyInfo on upload per AWS's own doc ('must be encoded in ssh-rsa format or PEM format'), so GetSSHPublicKey detects which format is stored and converts to the requested one using golang.org/x/crypto/ssh + crypto/x509/pem (already a direct dependency: services/transfer, services/lightsail, services/ec2 all import it). A stored body that parses as neither format, or an Encoding value that is not SSH/PEM (including missing), returns UnrecognizedPublicKeyEncoding -- taken from this op's own declared error set (deserializers.go's awsAwsquery_deserializeOpErrorGetSSHPublicKey switch: NoSuchEntity, UnrecognizedPublicKeyEncoding), not a generic guess."}
  SetSecurityTokenServicePreferences: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 7): required GlobalEndpointTokenVersion (:63, v1Token|v2Token) was read nowhere and nothing stored it. Real IAM exposes no dedicated getter for this preference, but DOES surface it via GetAccountSummary's GlobalEndpointTokenVersion SummaryMap entry (types.SummaryKeyTypeGlobalEndpointTokenVersion) -- that natural home now exists (b.globalEndpointTokenVersion, persisted) and is observable end-to-end (Set -> GetAccountSummary shows 1 or 2), rather than validate-and-discard. Missing/unrecognized values return ValidationError -- this op's own declared error set is ServiceFailure-only (no per-op client-error exception modeled), so there is no op-specific code to borrow; ValidationError is this service's existing convention for that situation (see GetDelegationRequest/ListPoliciesGrantingServiceAccess in handler_account.go)."}
  ListPoliciesGrantingServiceAccess: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-lx5h), first PARITY.md entry for this op. models_account.go's listPGSAResult used xml:\"PolicyGroups>member\"; the real required element (deserializers.go's awsAwsquery_deserializeOpDocumentListPoliciesGrantingServiceAccessOutput, matched case-insensitively) is PoliciesGrantingServiceAccess. Was silent (list always empty either way) but would have broken the instant this validation-only stub grew real emulation. Field renamed to PoliciesGrantingServiceAccess/xml:\"PoliciesGrantingServiceAccess>member\"; kept as []string rather than the real []types.PolicyGrantingServiceAccess struct list since the list is always empty (an empty child element serializes identically for either Go type) and this op has no access-analysis state to populate it with — real emulation is out of scope, same as GetDelegationRequest. Arn/ServiceNamespaces required-input validation (handler_account.go) was already correct."}
  CreateDelegationRequest: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 7), first PARITY.md entry for this op. Previously read only OwnerAccountId and dropped required Description/NotificationChannel/RequestorWorkflowId/SessionDuration (api_op_CreateDelegationRequest.go:38,49,65,74) plus Permissions -- and returned a fabricated nested <DelegationRequest> wire element that does not exist in the real API (real CreateDelegationRequestOutput is flat ConsoleDeepLink+DelegationRequestId, confirmed against deserializers.go's awsAwsquery_deserializeOpDocumentCreateDelegationRequestOutput). Both bugs fixed: all 4 scalar required members validated (InvalidInput, declared for this op), Permissions validated for presence via at least one Permissions.* key -- the query wire form has no way to signal \"present but empty struct\" for a required-but-all-optional-fields member, an inherent protocol limitation rather than a validation gap -- and the response now matches the real flat shape. DECISION (see GetHumanReadableSummary below for the other half of this family's decision): this op is implemented for real, not disclosed-stub -- it is mechanical bookkeeping (generate an ID, store the request, mint a deep-link URL), nothing here requires fabricating content gopherstack cannot honestly produce."}
  GetHumanReadableSummary: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 7), first PARITY.md entry for this op. Previously ignored vals entirely, dropped required EntityArn (:49), and returned the generic empty iamSimpleTagResponse instead of the real GetHumanReadableSummaryResult{Locale,SummaryContent,SummaryState} -- a client could not distinguish AVAILABLE from IN_PROGRESS from FAILED. DECISION: this op uses an LLM to generate natural-language permission summaries (SDK doc: \"This method uses a Large Language Model (LLM) to generate the summary\"), which gopherstack cannot honestly produce -- fabricating summary prose would be exactly the invented-capability-is-worse-than-absent violation this ledger exists to prevent (mirrors lightsail's disclosed-stub precedent for e.g. GetCostEstimate/GetContainerServiceMetricData). Implemented the real request/response SHAPE with a truthful state machine instead: EntityArn is now required (InvalidInput, declared for this op) and resolved against real CreateDelegationRequest state via a synthetic-but-plausible delegation-request/<id> ARN suffix -- a known request returns SummaryState=NOT_SUPPORTED (a real enum value that does not claim an attempt was made or is in flight, unlike FAILED/IN_PROGRESS/AVAILABLE) with empty SummaryContent, never invented prose; an unresolvable EntityArn returns NoSuchEntity (also declared)."}
  RejectDelegationRequest: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 8, gopherstack-qb3x), first PARITY.md entry for this op. Previously ignored vals entirely -- required DelegationRequestId (api_op_RejectDelegationRequest.go:42) was read nowhere, so any call (even against a nonexistent request) succeeded with an empty 200. Response shape was already correct: real RejectDelegationRequestOutput carries no members beyond ResultMetadata (confirmed: no awsAwsquery_deserializeOpDocumentRejectDelegationRequestOutput exists in deserializers.go), so the existing empty iamSimpleTagResponse needed no change. Now DelegationRequestId is required (InvalidInput, declared for this op) and resolved against real CreateDelegationRequest state (NoSuchEntity if absent, also declared) -- a known request transitions Status to REJECTED and stores the optional Notes parameter, real mutation against CreateDelegationRequest's state rather than validate-and-discard. The doc comment ('once a request is rejected, it cannot be accepted or updated later') describes a state-machine precondition, but the op declares no error code for violating it (only ConcurrentModification/InvalidInput/NoSuchEntity/ServiceFailure, and ConcurrentModificationException's own doc is specifically about simultaneous writes, not stale state) -- so no such precondition is invented/enforced here, consistent with AcceptDelegationRequest/AssociateDelegationRequest not enforcing one either."}
  SendDelegationToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 8, gopherstack-qb3x), first PARITY.md entry for this op. Previously ignored vals entirely -- required DelegationRequestId (api_op_SendDelegationToken.go:44) was read nowhere. Response shape already correct (empty output, same confirmation method as RejectDelegationRequest above). Now DelegationRequestId is required (InvalidInput, declared) and resolved against real state (NoSuchEntity if absent, declared) -- a known request transitions Status to FINALIZED, matching the doc comment ('After the SendDelegationToken API call is successful, the request transitions to a FINALIZED state and cannot be rolled back'). The doc's ACCEPTED-state precondition is not enforced, same reasoning as RejectDelegationRequest: no declared error models a state-conflict rejection."}
  UpdateDelegationRequest: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 8, gopherstack-qb3x), first PARITY.md entry for this op. Previously ignored vals entirely -- required DelegationRequestId (api_op_UpdateDelegationRequest.go:38) was read nowhere. Response shape already correct (empty output, same confirmation method as RejectDelegationRequest above). Now DelegationRequestId is required (InvalidInput, declared) and resolved against real state (NoSuchEntity if absent, declared) -- a known request transitions Status to PENDING_APPROVAL and stores the optional Notes parameter, matching the doc comment ('When the delegation request is updated, it reaches the PENDING_APPROVAL state')."}
  AcceptDelegationRequest: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 9, gopherstack-xh42), first PARITY.md entry for this op. Previously returned ErrInvalidAction (code InvalidAction, 400) for an unknown DelegationRequestId, but api_op_AcceptDelegationRequest.go's own deserializeOpError switch declares only ConcurrentModification/NoSuchEntity/ServiceFailure -- no InvalidAction case exists for this op at all. Now returns ErrDelegationRequestNotFound (NoSuchEntity, 404), matching the fix RejectDelegationRequest/SendDelegationToken/UpdateDelegationRequest already got in sweep 8. Wire shape (DelegationRequestId in, no output members) and state mutation (Status -> ACCEPTED) were already correct."}
  AssociateDelegationRequest: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 9, gopherstack-xh42), first PARITY.md entry for this op. Two bugs, both disclosed but left unfixed in sweep 8's items_still_open: (1) the handler read a 'PolicyArn' form value that does not exist anywhere on the real AssociateDelegationRequestInput (api_op_AssociateDelegationRequest.go declares only DelegationRequestId) -- a phantom field gopherstack accepted that real AWS would reject. Checked for real backend effect the way gopherstack-8v8v's redshift-serverless DBName phantom field had one: it did not -- the stored PolicyArn was write-only, read back by no operation (GetDelegationRequest is validation-only and never reads delegationRequests state at all; ListDelegationRequests always returns an empty list; GetHumanReadableSummary only checks existence). A real SDK client can never populate AssociateDelegationRequestInput.PolicyArn in the first place (the field doesn't exist on the Go struct), so this was dead code reachable only by hand-crafted non-SDK requests -- same class as gopherstack-xou3's docdb-fields-copied-from-neptune, and removed for the same reason: there was no real shape to model inertly. DECISION: removed the parameter, the backend signature's second argument, and the DelegationRequest.PolicyArn model field entirely, rather than documenting it as a known gap -- unlike GetHumanReadableSummary's LLM-summary gap, there is no real AWS behavior here worth disclosing as absent, only a fabricated one worth deleting. (2) same InvalidAction-vs-NoSuchEntity bug as AcceptDelegationRequest above (api_op_AssociateDelegationRequest.go's switch: ConcurrentModification/InvalidInput/NoSuchEntity/ServiceFailure, no InvalidAction) -- same fix, now returns ErrDelegationRequestNotFound (404). The real AssociateDelegationRequest additionally documents storing the caller identity's ARN as the request's ownerId/ownerAccount on success; gopherstack has no caller-identity plumbing to populate that honestly, so (like Accept/Reject/Send/UpdateDelegationRequest's unenforced state-machine preconditions) this is left as a validate-and-confirm-existence op rather than fabricating an ownerId."}
  GetServiceLastAccessedDetailsWithEntities: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-r80d required-output-member sweep), first PARITY.md entry for this op. Required JobCompletionDate (api_op_GetServiceLastAccessedDetailsWithEntities.go:103-111) had no field at all on the wire struct (models_access_advisor.go's getSLADWithEntitiesResult) -- not merely unset, structurally absent, so no client could ever decode it. Added the field and populated it the same way the sibling non-Entities op already does (job treated as completing immediately, JobCompletionDate==JobCreationDate). EntityDetailsList remains always-empty (no access-advisor analytics engine backs it, same disclosed-mock rationale as GetInsightResults.ResultValues elsewhere in this codebase) and is typed []string rather than []types.EntityDetails since an empty child element serializes identically either way and there is no analysis data to populate real entries with -- unchanged by this fix, still honest, not the bug fixed here."}
  UpdateSigningCertificate/DeleteSigningCertificate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (sweep 10, gopherstack-iam-signing-cert-ownership), first PARITY.md entry for these ops. Both real inputs carry an optional UserName member alongside the required CertificateId (api_op_UpdateSigningCertificate.go, api_op_DeleteSigningCertificate.go) that scopes the certificate to its owner -- the exact shape Update/DeleteAccessKey and Update/DeleteSSHPublicKey already enforce via a `key.UserName != userName` -> NoSuchEntity check (access_keys.go, ssh_keys.go). The handler read neither op's UserName at all, and the backend methods took no userName parameter, so any caller could deactivate or delete ANY user's signing certificate by supplying only its CertificateId -- a correct-sibling-beside-a-broken-op case, and destructive: a value the client never scoped (or mis-scoped) was applied anyway. Both ops now take userName and reject a mismatch with the same ErrAccessKeyNotFound (NoSuchEntity/404) the sibling ops use. Proven via a real-SDK-client test (signing_certificate_ownership_whitebox_test.go): user mallory calls UpdateSigningCertificate/DeleteSigningCertificate with her own UserName against alice's/bob's CertificateId; confirmed both succeeded silently pre-fix (mallory could deactivate/delete a stranger's cert), now return smithy NoSuchEntity. Hand-reverted signing_certificates.go/handler_signing_certificates.go/store.go to confirm the pre-fix build+tests pass unmodified, then restored (md5sum identical)."}
  ListSigningCertificates: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 11), closing the gap sweep 10 disclosed and deliberately left open. ListSigningCertificates now takes (userName, marker, maxItems) and returns page.Page[SigningCertificate], matching the ListAccessKeys/ListSSHPublicKeys template exactly (sort by CertificateID, page.New(certs, marker, maxItems, iamDefaultMaxItems)); the handler reads Marker/MaxItems and the response sets Marker/IsTruncated from p.Next. StorageBackend interface signature change confirmed clean via `make build-check` (no external call sites). Proven via a real-SDK-client test (TestListSigningCertificates_Pagination, signing_certificate_ownership_whitebox_test.go): 3 certs uploaded, MaxItems=2 returns 2 with IsTruncated=true and a non-nil Marker, a second call with that Marker returns the remaining 1 with IsTruncated=false. Hand-reverted all 4 changed files (signing_certificates.go, handler_signing_certificates.go, store.go, models_ssh_signing_certs.go) to pre-fix content -- test fails to even compile against the old 1-arg signature (proving the signature genuinely couldn't paginate); a second, narrower revert (only dropping the response's new Marker field/line) reproduces a pure runtime assertion failure instead, isolating that half of the fix. Restored; md5sum identical to pre-revert copies."}
  ListSSHPublicKeys: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 11), found while implementing the ListSigningCertificates fix above. Real ListSSHPublicKeysOutput has a Marker member alongside IsTruncated (deserializers.go's awsAwsquery_deserializeOpDocumentListSSHPublicKeysOutput) -- sweep 10 verified this op's REQUEST side was clean (Marker/MaxItems genuinely paginated via pkgs/page) but never checked the RESPONSE side: listSSHPublicKeysResult had no Marker field at all, so a real client saw IsTruncated=true with no continuation token to page further with -- pagination was reachable one page deep and then silently stuck. Added Marker to listSSHPublicKeysResult and populated it from p.Next in the live iamSSHKeyListDeleteDispatch handler (left the shadowed iamSSHKeyCompletenessDispatch duplicate as-is, consistent with that function's own documented pure-reorganization/dead-code status)."}
  GetDelegationRequest: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 11), closing the gap this ledger's items_still_open named since sweep 9. Previously validation-only: checked DelegationRequestId was non-empty and returned an empty success response, even for a request ID that genuinely existed in CreateDelegationRequest's stored state (that state has been fully real since sweeps 7-9). Now looks the request up and returns real types.DelegationRequest fields gopherstack actually tracks (DelegationRequestId, CreateDate, Description, Notes, OnlySendByOwner, OwnerAccountId, Permissions{PolicyTemplateArn,Parameters}, RedirectUrl, RequestMessage, SessionDuration, State) -- fields it does not track (ApproverId, ExpirationTime, OwnerId, PermissionPolicy, RejectionReason, RequestorId, RequestorName, RolePermissionRestrictionArns, UpdatedTime) are honestly omitted (all optional on the wire), not fabricated. PermissionCheckResult/PermissionCheckStatus (DelegationPermissionCheck's async policy-coverage check) require a policy evaluator gopherstack does not have -- left unset, a modelling gap, not this sweep's bug. Unknown DelegationRequestId returns NoSuchEntity (already correct pre-fix, declared by this op). Found and fixed in passing: CreateDelegationRequest set Status='PENDING', a string with no match anywhere in types.StateType's real enum (UNASSIGNED/ASSIGNED/PENDING_APPROVAL/FINALIZED/ACCEPTED/REJECTED/EXPIRED) -- invisible until this sweep started actually serializing State for the first time. Now UNASSIGNED/ASSIGNED per whether OwnerAccountId was given, per GetDelegationRequest's own doc comment ('If a delegation request has no owner or owner account... can be called by any account'). Proven via real-SDK-client tests (TestGetDelegationRequest_SurfacesStoredState, delegation_requests_whitebox_test.go): known ID returns the real stored fields incl. nested Permissions; unknown ID is NoSuchEntity. Hand-reverted account.go/handler_account.go/models_account.go/store.go; both subtests fail pre-fix with a clean, quotable error ('GetDelegationRequestResult node not found' -- the SDK's own XML decoder rejecting the old empty envelope). Restored; md5sum identical."}
  ListDelegationRequests: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 11), the other half of the gap named above. Previously always returned an empty DelegationRequests list (typed []string) regardless of stored state; real ListDelegationRequestsOutput returns full []types.DelegationRequest objects (deserializers.go), not IDs. Now paginated (page.Page[DelegationRequest], sort by DelegationID, same page.New template as every other List op in this service) and returns the real objects via the same toDelegationRequestXML helper GetDelegationRequest uses. Real ListDelegationRequestsInput also carries an optional OwnerId filter; gopherstack has no caller-identity plumbing to ever populate a stored request's owner identity (same disclosed gap AssociateDelegationRequest's entry above already covers), so no stored request could ever match a real OwnerId regardless -- filter intentionally not applied, left as a named disclosed gap rather than implementing a filter path that would always yield empty. Proven via TestListDelegationRequests_SurfacesStoredState (delegation_requests_whitebox_test.go): fails pre-fix with '[]' should have 1 item(s), but has 0; passes post-fix."}
  GetAccountSummary: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (sweep 11). Real SummaryKeyType (types/enums.go) has no 'SAMLProviders' value -- SAML and OIDC identity providers are both counted under the single 'Providers' key. gopherstack emitted a fabricated 'SAMLProviders' key no real caller's SummaryKeyType-typed lookup would ever match, and never surfaced the separately-tracked OIDCProviders count at all (accept-and-drop: state the backend tracks but never surfaces). AWS's query/XML SummaryMap decodes into a bare map[string]int32 with no enum validation, so this never errored -- it just silently answered the wrong key. Now emits {Key: \"Providers\", Value: summary.SAMLProviders + summary.OIDCProviders}. Proven via a real-SDK-client test (TestGetAccountSummary_ProvidersKey, account_test.go): creates 1 SAML + 1 OIDC provider, asserts out.SummaryMap[\"Providers\"]==2 and out.SummaryMap[\"SAMLProviders\"] is absent; both assertions fail pre-fix (0 and present, respectively). Hand-reverted handler_providers.go, confirmed the failure, restored; md5sum identical. Every other SummaryMap key (Users/Groups/Roles/Policies/InstanceProfiles/MFADevices/GlobalEndpointTokenVersion) already matched the real enum exactly -- not re-audited beyond confirming that, no other bug found."}
  GenerateCredentialReport/GetCredentialReport: {wire: ok, errors: ok, state: ok, persist: n/a, note: "RE-VERIFIED (sweep 11), closing the 'not re-verified since sweep 4' flag for credential-report generation specifically. Both real Input structs are empty (noSmithyDocumentSerde only, no serializeOpDocument*Input function exists for either in serializers.go) -- nothing for a request-side member-drop bug to hide in. Output field names/values checked against api_op_GenerateCredentialReport.go/api_op_GetCredentialReport.go: State='COMPLETE' matches types.ReportStateTypeComplete exactly; Content/ReportFormat/GeneratedTime field names correct, ReportFormat='text/csv' matches types.ReportFormatTypeTextCsv exactly. No bug found."}
  GenerateServiceLastAccessedDetails/GetServiceLastAccessedDetails (dead-code documentation only): {wire: n/a, errors: n/a, state: n/a, persist: n/a, note: "Not a behavior fix -- while auditing access advisor (sweep 11), found 2 more shadowed-duplicate dispatch entries in the same shape as the SSH-key completeness-pass duplicates (see ListSSHPublicKeys entry above and its own pre-existing doc comment): iamOrgsReportDispatch (handler_account.go) has a 'GenerateServiceLastAccessedDetails' entry that ignores vals entirely and fabricates a JobID, and iamMiscDispatchTable (handler_providers.go) has a 'GetServiceLastAccessedDetails' entry that ignores JobId and always returns an empty ServicesLastAccessed list. Both are shadowed and dead: buildDispatchTable (handler.go) merges iamComprehensiveDispatchTable -- which carries the real, state-reading iamAccessAdvisorDispatch entries (handler_access_advisor.go) -- last, and that table's own doc comment already says so ('These entries override earlier stub implementations'). Confirmed live-vs-dead by merge-order tracing plus a discriminator test (TestIAMHandler_ServiceLastAccessedDetails_LiveHandlersWinOverShadowedStubs, access_advisor_test.go: pre-records a service access, drives Generate+Get through the real dispatch table, and only sees that access if the real handlers ran end to end -- they do). Added a short comment at each dead entry (matching the SSH-key precedent) so neither reads as live to a future pass; no behavior change, so no hand-revert applies. NOT fixed this sweep, left as a disclosed gap requiring a real access-advisor model rather than a request-completeness fix: GenerateServiceLastAccessedDetailsInput also serializes an optional Granularity (SERVICE_LEVEL|ACTION_LEVEL), and GetServiceLastAccessedDetailsInput also serializes Marker/MaxItems -- gopherstack's access-advisor backend (access_advisor.go) tracks only per-service data with no pagination concept and no per-action (ACTION_LEVEL) tracking at all, so honoring Granularity=ACTION_LEVEL would mean fabricating action-level data gopherstack cannot honestly produce (same invented-capability-is-worse-than-absent line as GetHumanReadableSummary's LLM content). See items_still_open."}
invented_ops_removed:
  - "GetUserPermissionsBoundary / GetRolePermissionsBoundary: not real IAM actions (no api_op_Get{User,Role}PermissionsBoundary.go in the SDK) — permissions-boundary info is returned as a field on GetUser/GetRole (types.User.PermissionsBoundary / types.Role.PermissionsBoundary), which gopherstack already does correctly. Deleted the fabricated duplicate getters, their GetSupportedOperations entries, and updated the 2 tests that called them to assert via GetUser/GetRole instead."
  - "TagGroup / UntagGroup / ListGroupTags: not real IAM actions — Group is not a taggable resource type in real AWS (aws-sdk-go-v2/service/iam/types.Group has no Tags field, no api_op_{Tag,Untag,ListGroupTags}.go exist). Deleted the fabricated backend methods (InMemoryBackend.TagGroup/UntagGroup), the StorageBackend interface methods, the dispatch entries, the Group.Tags / GroupXML.Tags model fields, and the 4 tests that exercised them."
gaps: []
leaks: {status: clean, note: "persistence leaks clean (unchanged); 2 leak classes found+fixed sweep 5 — see DeleteUser/DeleteRole/DeleteGroup/DeleteInstanceProfile ghost-row entries and the Handler-level tag leak entry above. go test -race passes."}
items_still_open:
  - "2026-08-29 constraint-parameter sweep fixed PathPrefix+pagination truncation across ListUsers/ListRoles/ListGroups/ListInstanceProfiles/ListPolicies, and ListPolicies' OnlyAttached/PolicyUsageFilter (see the sweep's own section above for detail). Sweep 13 closed ListAttached{User,Role,Group}Policies' PathPrefix (see its own ops: entry). ListEntitiesForPolicy's EntityFilter/PathPrefix/PolicyUsageFilter/Marker/MaxItems (confirmed present sweep 13, deliberately left open pending a StorageBackend surface change) is now also closed (gopherstack-fjmw, see its own ops: entry -- new PermissionsBoundaryEntities method) -- still open: the pagination-only params on ListMFADevices/ListAccessKeys/ListSigningCertificates/ListSSHPublicKeys/ListServiceSpecificCredentials (not re-checked)."
  - "Sweep 13 (wrapper-key sweep, iam+eventbridge scope): field-level enumeration via go/types selector-usage scan doesn't apply to IAM -- it's AWS Query/XML with no request struct types at all (handlers pull vals.Get(\"Key\") directly), unlike eventbridge's JSON *Input structs. Instead re-verified the known filter-after-pagination class (confirmed still fixed for the 5 ops sweep 12's PathPrefix-family header names) and found the same silent-full-list shape one layer over: ListAttached{User,Role,Group}Policies (fixed) and ListEntitiesForPolicy (confirmed, left open) both read PolicyArn/EntityType-only and ignore PathPrefix/PolicyUsageFilter/Marker/MaxItems entirely. Also fixed a wrong-Go-value bug found while writing the ListAttached* regression test: policyNameFromARN split on the wrong separator for any policy with a non-default Path. ListServerCertificates spot-checked clean (PathPrefix read and filtered correctly; no Marker/MaxItems support at all is a disclosed structural gap, not a filter-after-pagination bug -- there's no pagination to cut wrong). ListGroupsForUser spot-checked: hardcodes IsTruncated=false with no Marker/MaxItems read at all -- same disclosed structural gap, not fixed, not this sweep's named scope."
  - "Sweep 9 (gopherstack-xh42) closed both delegation-family issues sweep 8 disclosed but left out of its named scope (see AcceptDelegationRequest/AssociateDelegationRequest ops entries above for the fixes and reasoning). The delegation-request family (7 ops total: Create/Accept/Associate/Reject/Send/Update/GetHumanReadableSummary) is now fully covered across sweeps 7-9, with every op wire/error-verified against the pinned SDK. GetDelegationRequest/ListDelegationRequests remain disclosed validation-only/always-empty (unchanged, still out of scope -- no bd issue filed against them yet). STALE as of sweep 11 -- flagged by cmd/staleclaims (gopherstack-anjf): both are now real, see their own ops: entries above (\"FIXED (sweep 11)\") and the sweep-11 bullet below."
  - "This sweep (6) closed both remaining gopherstack-gjp/2sz3 items: (1) comprehensiveBackend's private sync.Mutex is gone — its fields (sshPublicKeys, mfaUserLinks, accessAdvisorJobs, serviceLastAccessed, orgReportJobs) are now guarded by the same coarse b.mu as every other backend map, per the one-coarse-lock convention (.claude/memories/pkgs-catalog.md). Two call sites (GetCredentialReport, ListMFADevicesForUser) previously nested c.mu inside a held b.mu.RLock; DeleteUser's dependency check ran entirely BEFORE taking b.mu, a real TOCTOU window between the SSH-key/MFA-device check and the delete. All three are now single atomic critical sections under b.mu. Snapshot()/Restore() also now read/write comprehensiveBackend state inside the same b.mu section as the rest of backend state, instead of a separate before/after step — Snapshot() gets one consistent point-in-time view (previously the comprehensive-state read and the rest-of-backend read were NOT atomic with each other). Covered by TestComprehensiveBackend_NoDataRace (-race, concurrent workers hitting both comprehensiveBackend and regular backend ops) and TestDeleteUser_SSHKeyConflictIsAtomic. (2) GetAccountAuthorizationDetails now honors Marker/MaxItems/Filter — see the ops entry above."
  - "NOT re-verified this sweep (no evidence of a bug found, but not field-diffed line-by-line either): policy simulation (SimulateCustomPolicy/SimulatePrincipalPolicy/evaluator.go), access advisor / service-last-accessed, credential report generation, account summary, condition-key evaluation (conditions.go), resource-policy evaluation (resource_arn.go). These were already marked ok/PROVEN by sweeps 1-4 and no new evidence surfaced against them. (SSH key / signing certificate CRUD -- the other family named in this line as of sweep 9 -- was field-diffed member-by-member in sweep 10: SSH key ops (Upload/Get/List/Update/DeleteSSHPublicKey) all read every serialized member correctly, no bug; signing certificates had a real ownership-bypass bug, now fixed, plus a disclosed pagination gap -- see ops entries above.)"
  - "Sweep 10 also confirmed policy evaluation itself (evaluator.go, conditions.go, resource_arn.go) and SimulatePrincipalPolicy/SimulateCustomPolicy remain untouched and out of scope: gopherstack has no real IAM policy evaluator, and building one is explicitly outside this campaign's charter (modelling gap, not a bug)."
  - "Sweep 11 closed 3 of this list's named items: ListSigningCertificates' disclosed pagination gap (now fixed, plus a second real gap found in the same area -- sibling ListSSHPublicKeys' response never echoed Marker despite genuinely paginating -- also fixed), and GetDelegationRequest/ListDelegationRequests (both now real, no longer disclosed stubs -- see ops entries above). Access advisor / credential report / account summary (named 'not re-verified since sweep 4' above) is now re-verified: GetCredentialReport/GenerateCredentialReport clean (no bug -- both real inputs are empty, output fields all correct); GetAccountSummary had a real bug, now fixed (fabricated 'SAMLProviders' key, OIDCProviders never surfaced -- see ops entry); GenerateServiceLastAccessedDetails/GetServiceLastAccessedDetails have 2 shadowed-dead-code duplicates now documented (no behavior change, see ops entry) plus a genuine, NOT-fixed disclosed gap: GenerateServiceLastAccessedDetailsInput's optional Granularity (SERVICE_LEVEL|ACTION_LEVEL) is not honored, and GetServiceLastAccessedDetailsInput's Marker/MaxItems are not paginated -- gopherstack's access-advisor backend tracks only per-service data with no per-action tracking and no pagination concept, so ACTION_LEVEL granularity would mean fabricating data gopherstack cannot honestly produce (same invented-capability-is-worse-than-absent line as GetHumanReadableSummary); Marker/MaxItems pagination is mechanical (same page.Page[T] template used everywhere else in this service) but was left out of this sweep's named scope to keep it focused. ListDelegationRequests' real OwnerId filter is also a disclosed, deliberately-unapplied gap (see its ops entry): gopherstack has no caller-identity plumbing to ever populate a stored request's owner identity, the same gap AssociateDelegationRequest already discloses. Condition-key evaluation (conditions.go) and resource-policy evaluation (resource_arn.go) remain NOT re-verified since sweep 4 -- out of this sweep's named scope, no evidence checked either way."
---

## Notes
- Sweep 11 (2026-08-23): worked items_still_open's named queue -- access advisor / credential
  report / account summary ("not re-verified since sweep 4"), GetDelegationRequest/
  ListDelegationRequests (disclosed stubs since sweep 9), and ListSigningCertificates'
  pagination gap (disclosed, deliberately left open by sweep 10). 5 real bugs fixed, plus
  2 shadowed-dead-code duplicates documented (no behavior change) and 2 modelling gaps
  newly disclosed (not fixed). See the ops entries above for
  ListSigningCertificates/ListSSHPublicKeys/GetDelegationRequest/ListDelegationRequests/
  GetAccountSummary/GenerateCredentialReport::GetCredentialReport/
  GenerateServiceLastAccessedDetails::GetServiceLastAccessedDetails for full detail on each.
  Every fix proven with a real-SDK-client test, hand-reverted against the pre-fix code to
  confirm a genuine failure, then restored with `md5sum` identical to the pre-revert copies.
  Gates: `go build ./...`, `go vet ./services/iam/...`, `gofmt -l` (clean), `go test -race
  -count=1 ./services/iam/...` (pass), `golangci-lint run ./services/iam/...` (0 issues),
  `go test ./pkgs/persistence/...` (pass -- no persisted struct changed, only method
  signatures), `make build-check` (clean -- `StorageBackend`/`ListSigningCertificates`/
  `GetDelegationRequest`/`ListDelegationRequests` are exported and iam is widely depended on;
  confirmed no call sites outside `services/iam/`).
- Sweep 10 (2026-08-23, gopherstack-iam-signing-cert-ownership): request-side audit of the
  credentials family (access keys already ok; SSH public keys and signing certificates
  field-diffed member-by-member against `iam@v1.58.1`'s `awsAwsquery_serializeOpDocument*`
  functions in serializers.go). SSH key ops were clean -- every serialized member
  (UserName, SSHPublicKeyId, SSHPublicKeyBody, Status, Encoding, Marker, MaxItems) is read
  by the handler and Marker/MaxItems are genuinely paginated via `pkgs/page`. Signing
  certificates had a real bug: `UpdateSigningCertificateInput`/`DeleteSigningCertificateInput`
  both serialize an optional `UserName` alongside the required `CertificateId`
  (`api_op_UpdateSigningCertificate.go`, `api_op_DeleteSigningCertificate.go`), used by real
  AWS to scope the certificate to its owner -- exactly the shape Update/DeleteAccessKey and
  Update/DeleteSSHPublicKey already enforce (`ak.UserName != userName` / `key.UserName !=
  userName` -> NoSuchEntity, in access_keys.go and ssh_keys.go respectively). The signing-cert
  handler never read `UserName` for either op, and the backend methods (`UpdateSigningCertificate`,
  `DeleteSigningCertificate`) took no `userName` parameter at all, so any caller could
  deactivate or delete a stranger's signing certificate by supplying only its `CertificateId`.
  Fixed both to take `userName` and reject a mismatch, matching the sibling ops exactly.
  Real-SDK-client proof in `signing_certificate_ownership_whitebox_test.go`: user `mallory`
  calls `UpdateSigningCertificate`/`DeleteSigningCertificate` with her own `UserName` against
  `alice`'s/`bob`'s certificate; confirmed both calls succeeded with no error pre-fix (and
  silently deactivated/deleted the victim's certificate), now return `smithy.APIError` with
  `ErrorCode()=="NoSuchEntity"`. Hand-reverted `signing_certificates.go`,
  `handler_signing_certificates.go`, and `store.go` to their pre-fix content, confirmed
  `go build`+`go test ./services/iam/...` pass unmodified in that state (i.e. this really is
  the bug, not a test artifact), then restored the fix (`md5sum` identical to the pre-revert
  copies). Also found, but left unfixed as a disclosed gap: `ListSigningCertificatesInput`
  also serializes `Marker`/`MaxItems`, but the handler reads only `UserName` and the backend
  has no pagination concept for this op at all (unlike sibling `ListAccessKeys`/
  `ListSSHPublicKeys`, both genuinely paginated via `page.Page[T]`) -- optional-member-dropped,
  silent no-op, not destructive, left out of this sweep's fix to keep its exported-signature
  blast radius minimal. Gates: `go build ./...` (repo-wide, since `UpdateSigningCertificate`/
  `DeleteSigningCertificate`/the `StorageBackend` interface are exported -- confirmed no
  call sites outside `services/iam/`), `go vet`, `gofmt -l` (clean), `go test -race -count=1
  ./services/iam/...` (pass), `golangci-lint run ./services/iam/...` (0 issues), `go test
  ./pkgs/persistence/...` (pass -- no persisted struct changed, only method signatures).
- Sweep 9 (2026-08-13, gopherstack-xh42): closed the 2 delegation-family issues sweep 8 disclosed in passing but left outside its named scope. AssociateDelegationRequest's handler read a `PolicyArn` form value with no counterpart on the real AssociateDelegationRequestInput (api_op_AssociateDelegationRequest.go: DelegationRequestId only) -- checked for real backend effect (the redshift-serverless DBName precedent, gopherstack-8v8v, is the reason this needs checking rather than assuming cosmetic) and found none: the stored value was write-only, read back by no operation. Removed the parameter, the backend method's second argument, and the DelegationRequest.PolicyArn model field, rather than documenting it as a disclosed gap -- there was no real shape here to preserve, only a fabricated one to delete (same call as gopherstack-xou3's docdb-fields-copied-from-neptune). Separately, AcceptDelegationRequest and AssociateDelegationRequest both returned InvalidAction (400) for an unknown DelegationRequestId despite neither op declaring that code in its own deserializeOpError switch (both declare NoSuchEntity); both now return NoSuchEntity (404), matching the fix Reject/Send/UpdateDelegationRequest already got in sweep 8. Two test bugs found in the same direction as the code bugs they covered: handler_extended_dispatch_test.go asserted `wantCode: http.StatusBadRequest` for both ops' not-found cases, encoding the same wrong expectation as the handler; corrected to StatusNotFound with a NoSuchEntity body assertion. Added a real-SDK-client test (delegation_requests_whitebox_test.go) driving both ops against an unknown ID and asserting smithy.APIError.ErrorCode()=="NoSuchEntity"; confirmed it fails against the pre-fix InvalidAction code by hand-reverting account.go.
- Sweep 8 (2026-08-13, gopherstack-qb3x): closed out the 3 delegation-request-family ops sweep 7 flagged but did not fix -- RejectDelegationRequest, SendDelegationToken, UpdateDelegationRequest all silently ignored their required DelegationRequestId (each op's own api_op_*.go), so any request (even against a nonexistent delegation request) succeeded. Confirmed each op's real *Output carries no members (deserializers.go has no awsAwsquery_deserializeOpDocument*Output for any of the 3), so unlike sweep 7's CreateDelegationRequest fix, the wire response shape needed no change -- only the input-side drop and the total absence of backend action. All 3 now validate DelegationRequestId (InvalidInput, declared by all 3), resolve it against CreateDelegationRequest's real stored state (NoSuchEntity if unknown, also declared), and genuinely mutate that state (REJECTED/FINALIZED/PENDING_APPROVAL respectively, plus storing the optional Notes parameter for Reject/Update) instead of validating and discarding. Found 2 more issues in the same family while reading the SDK for this sweep but left them unfixed as outside gopherstack-qb3x's named scope -- see items_still_open.
- Sweep 7 (2026-08-13, gopherstack-oxuf): a required-member sweep found 4 real gaps this service's 2026-08-07 A-grade audit missed entirely (grepping the then-current PARITY.md for UploadServerCertificate/GetSSHPublicKey/delegation all returned zero) -- UploadServerCertificate.PrivateKey, GetSSHPublicKey.Encoding, SetSecurityTokenServicePreferences.GlobalEndpointTokenVersion, and CreateDelegationRequest's Description/NotificationChannel/RequestorWorkflowId/SessionDuration/Permissions, the last also carrying a wrong wire shape (fabricated nested `<DelegationRequest>` instead of the real flat ConsoleDeepLink/DelegationRequestId). Also gave GetHumanReadableSummary its first-ever PARITY.md entry and real request/response shape, choosing an honest NOT_SUPPORTED state machine over fabricating the LLM summary text the real op produces. See the `ops:` entries above for each fix's reasoning and the SDK line numbers verified against the pinned iam@v1.58.1. `handleError`'s error-code switch was refactored to a data table (`iamErrorMappings`) mid-sweep purely to stay under the cyclop budget after adding 3 new error-code cases -- no behavior change.
- HTTP status codes: NoSuchEntity 404, EntityAlreadyExists/DeleteConflict/LimitExceeded 409 (fixed sweep <=3); default code ServiceFailure.
- Policy documents: stored as plain JSON in backend, percent-encoded ONLY at wire boundary via encodePolicyDocument().
- STS/assume-role cross-service linkage is out of services/iam scope (wired in cli.go).
- Sweep 6 (2026-08-07): consolidated comprehensiveBackend's private sync.Mutex onto the coarse b.mu (bd gopherstack-gjp/gopherstack-2sz3) and added Marker/MaxItems/Filter support to GetAccountAuthorizationDetails — see items_still_open for detail. RoleDetail.InstanceProfileList (the other item gjp originally flagged) was already fixed in an earlier pass (6bad2f9, 2026-07-16); confirmed still correct, no action needed.
- Sweep 5 (2026-07-24): full-surface invented-op audit (compared every routed dispatch-table key against the real aws-sdk-go-v2/service/iam v1.55.0 `api_op_*.go` file list — 176 real ops, all routed, 5 gopherstack-only fabrications found and deleted — see invented_ops_removed). Then field-diffed every Delete*/Update*(rename) op in the users/groups/roles/instance-profiles/policies family against the SDK doc comments' documented dependency-removal lists, finding and fixing 4 missing DeleteConflict checks (DeleteUser, DeleteRole, DeleteGroup, DeleteInstanceProfile) and 2 ghost-state leak classes (stale reverse policyAttachments index on rename; stale Handler-level tags on delete/rename of the 5 resource kinds tagged outside their backend model). Also fixed the comp() lazy-init data race (bd gopherstack-v9z0). Did not re-verify every family from scratch (see items_still_open) — this was a targeted correctness pass on the delete/rename lifecycle plus an invented-surface sweep, not a full field-by-field re-diff of every op.
- Sweep 4 (2026-07-11): re-audit found local drift since 71cd5441 was entirely commit ce30166a ("Parity sweep 3"), whose fixes were already reflected in this ledger (last_audit_commit had gone stale — the ledger was written *by* that commit but never bumped its own pointer). The only other change in range was the e51c0de9 dependency bump (aws-sdk-go-v2/service/iam v1.54.7 -> v1.55.0); diffed the vendored module trees and confirmed no API-surface change (CHANGELOG.md/generated.json/go_module_metadata.go only). Real fix made: RoleDetail.InstanceProfileList.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404,
masking Handler()'s already-typed ServiceFailure**: same shape as autoscaling's entry
(see that entry or gopherstack-3a8t for the full survey/rationale). `RouteMatcher` now
falls back to `service.MatchesUserAgentMarker(r.Header, "api/iam")` (verified against the
pinned `iam@v1.58.1/api_client.go:638` `AddSDKAgentKeyValue` call) only on the `ReadBody`
failure branch. No `ParseForm` migration was needed: `ExtractOperation`, `ExtractResource`,
and `Handler()` already used `httputils.ReadBody`+`url.ParseQuery` exclusively (never
`r.ParseForm()`), and `Handler()` already wrote a typed `ServiceFailure` (500) on a read
failure -- only `RouteMatcher` had the bug. Proof:
`TestHandler_OversizedBodySurfacesInternalFailure` in `handler_oversized_body_test.go`
drives a real IAM SDK client through `service.NewRegistry`/`service.NewServiceRouter`,
confirmed failing pre-fix with `UnknownError`; passes now with `ServiceFailure`.
`TestHandler_NormalSizedBodyStillRoutes` is the regression guard. Gates: `go build`,
`go vet`, `gofmt -l` (clean), `go test -race ./services/iam/...` (pass),
`golangci-lint run ./services/iam/...` (0 issues).

## gopherstack-wlo1 (2026-08-22): Handler()'s method-not-allowed branch was untyped -- and structurally unreachable via routing

`Handler()`'s own `if c.Request().Method != http.MethodPost { return
c.String(http.StatusMethodNotAllowed, "Method not allowed") }` guard
(handler.go) wrote a bare text/plain 405. IAM is AWS Query/XML
(`iam@v1.58.1` `awsAwsquery_` prefix), whose deserializer expects the
wrapped `<ErrorResponse><Error>` document; plain text doesn't decode, so a
real client would have seen a raw XML-unmarshal failure rather than a typed
API error.

UNLIKE sts's identical-looking branch (also fixed this pass), this one is
provably unreachable via any request a real client -- or even a corrupted
one -- can construct: `RouteMatcher` (handler.go) itself checks
`r.Method != http.MethodPost` and rejects non-POST requests before
`Handler()` is ever invoked, so no request that reaches `Handler()` through
`service.NewServiceRouter`/`RouteMatcher` can ever fail this check. The
same "coarse check equals fine check" deadlock as xray's analogous branch
(see that service's PARITY.md entry this pass).

Fixed defensively for consistency with the class, reusing this file's
existing `writeError(c, http.StatusMethodNotAllowed, "InvalidParameterValue",
"Method not allowed")` helper and the same `"InvalidParameterValue"` code
already used elsewhere in `Handler()` for malformed input -- not proven by
a real SDK client, since none can reach it.

## 2026-08-29: error-path sweep (failure-side wire shape) -- 9 wrong/unmodelled codes fixed

Campaign-wide hunt for the class distinct from the order-bug pass above:
what a client sees when a request *fails* -- HTTP status, AWS error code, and
whether the operation actually models that code, checked against each op's
own `awsAwsquery_deserializeOpError<Op>` switch in `deserializers.go`
(iam@v1.58.1, AWS Query/XML protocol), not the shared `types/errors.go` list.
All 176 ops' declared code sets extracted from the pinned SDK.

**Error path**: single global lookup table (`handler.go`'s `iamErrorMappings`,
`[]{err, code, status}`), matched by `errors.Is` in `handleError` -- same
shared-helper shape as s3/sts, so a wrong entry is service-wide, but a wrong
*call site* (right table entry, wrong sentinel chosen for that operation)
is scattered per-op and was the actual defect class found here.

**Root cause of every fix below**: `ErrInvalidAction` (wire code
`"InvalidAction"`) is correctly used exactly once, at `handler.go`'s
`dispatch()`, for a genuinely unrecognized `Action=` value -- the one case
that matches AWS Query protocol's real "InvalidAction" semantics (an
unregistered *operation name*, confirmed absent from all 176 per-op
switches since no well-behaved SDK client can ever trigger it against a
known operation). It had also been reused, incorrectly, as a catch-all for
unrelated validation and not-found failures *inside* several known,
well-formed operations -- where the operation's own switch models a
completely different code.

**Fixed (9 call sites, each cross-checked against its own op's declared
set)**:
- `UpdateAccessKey` (`access_keys.go`): invalid `Status` value now
  `ErrInvalidInput` (`InvalidInput`, modeled; was `InvalidAction`, not).
- `DeleteAccountAlias` (`account.go`): alias not found now the new
  `ErrAccountAliasNotFound` (`NoSuchEntity`, modeled; was `InvalidAction`).
- `CreateServiceLinkedRole` / `GetServiceLinkedRoleDeletionStatus`
  (`service_linked_roles.go`): empty `AWSServiceName` / `DeletionTaskId` now
  `ErrInvalidInput` (both ops model `InvalidInput`; `DeleteServiceLinkedRole`
  does not, so its own empty-`RoleName` `InvalidAction` case is left
  disclosed, not fixed -- no modeled alternative).
- `AddClientIDToOpenIDConnectProvider` (`providers.go`): empty `ClientID`
  now `ErrInvalidInput` (modeled).
- `EnableMFADevice` (`mfa.go`): device-not-found now the new
  `ErrMFADeviceNotFound` (`NoSuchEntity`, modeled), already-enabled now the
  new `ErrMFADeviceAlreadyEnabled` (`EntityAlreadyExists`, modeled) --
  `DeactivateMFADevice`'s device-not-found case shares the same fix
  (`ErrMFADeviceNotFound`, also modeled there); its "not currently enabled"
  case is left disclosed, no modeled fit in
  `{ConcurrentModification,EntityTemporarilyUnmodifiable,LimitExceeded,NoSuchEntity,ServiceFailure}`.
- `CreateVirtualMFADevice`/`CreateVirtualMFADeviceFull` (`mfa.go`): empty
  `VirtualMFADeviceName` now `ErrInvalidInput` (modeled).
- `SimulateCustomPolicy` (`policies.go`): empty `ActionNames` now
  `ErrInvalidInput` (modeled).
- `UploadServerCertificate`/`UploadSigningCertificate`
  (`server_certificates.go`/`signing_certificates.go`): both previously used
  `ErrMalformedPolicyDocument` (`MalformedPolicyDocument`) for empty
  `ServerCertificateName`/`CertificateBody` -- a code *neither* op models at
  all. Now `ErrInvalidInput` for the name (modeled on
  `UploadServerCertificate`) and `ErrMalformedCertificate` for the body
  (modeled on both).

**Reverse-direction bug fixed**: `RemoveClientIDFromOpenIDConnectProvider`
(`providers.go`) raised `ErrInvalidAction` when the client ID wasn't
registered on the provider. `api_op_RemoveClientIDFromOpenIDConnectProvider.go`'s
own doc comment: *"This operation is idempotent; it does not fail or return
an error if you try to remove a client ID that does not exist."* Now returns
success for that case (the provider-not-found case is untouched --
that failure mode isn't covered by the idempotency doc, and `NoSuchEntity`
is separately confirmed modeled on this op).

**Left disclosed, not fixed** (no modeled code exists for the condition,
so no replacement can be established from the SDK): `CreateServiceSpecificCredential`'s
empty `ServiceName` (models only `LimitExceeded`/`NoSuchEntity`/`NotSupportedService`);
`DeleteServiceLinkedRole`'s empty `RoleName`; `CreateAccountAlias`'s empty
alias (models only `ConcurrentModification`/`EntityAlreadyExists`/`LimitExceeded`/`ServiceFailure`);
`DeactivateMFADevice`'s "not currently enabled" state.

**Noted but out of scope** (different bug class -- a fabricated wire field,
not a wrong error code): `UpdateRole`'s handler (`handler_users.go`) rejects
a non-empty `Path` form value with `InvalidAction`, but the real
`UpdateRoleInput` (`api_op_UpdateRole.go`) has no `Path` member at all --
no real SDK client can ever send it, so this whole branch is only reachable
by a raw/non-SDK caller. Not touched this pass.

**Two stale tests found asserting the old wrong codes** (same pattern this
campaign has repeatedly found): `access_keys_test.go`'s `invalid_status`
case asserted `wantErrMsg: "InvalidAction"`; `mfa_test.go`'s
`TestEnableMFADevice_RejectsDoubleEnable` asserted `iam.ErrInvalidAction`.
Both corrected to assert the new, SDK-confirmed sentinels.

Proof: three new real-SDK-client tests in `errors_test.go`
(`TestDeleteAccountAlias_NotFound_NoSuchEntity`,
`TestEnableMFADevice_AlreadyEnabled_EntityAlreadyExists`,
`TestRemoveClientIDFromOpenIDConnectProvider_UnknownClientID_Idempotent`)
plus three for the certificate fixes
(`TestUploadServerCertificate_EmptyCertificateBody_MalformedCertificate`,
`TestUploadServerCertificate_EmptyName_InvalidInput`,
`TestUploadSigningCertificate_EmptyCertificateBody_MalformedCertificate`),
each asserting `errors.As` against the real typed SDK exception. All six
hand-confirmed failing against the pre-fix code (reverted the relevant
source lines, re-ran, restored) before the fix landed.

Gates: `go build`, `go vet ./...` (repo-wide -- clean except an unrelated
concurrently-edited `services/apigateway` package elsewhere in this shared
working tree), `go test -race -count=1 ./services/iam/...` (pass),
`golangci-lint run --fix ./services/iam/...` (0 issues).

## 2026-08-29: constraint-parameter sweep (a filter/sort/page limit silently not honoured)

Campaign-wide hunt for a third class, distinct from both sweeps above: a
request parameter that constrains the result set but isn't correctly
applied. Measured against the pinned SDK (`api_op_List*.go`) before fixing:
`ListUsers`/`ListRoles`/`ListGroups`/`ListInstanceProfiles` each declare
`Marker`/`MaxItems`/`PathPrefix`; `ListPolicies` additionally declares
`Scope`/`OnlyAttached`/`PolicyUsageFilter`. Did not re-audit the other
~170 ops this pass -- scoped to this coherent slice (the 5 PathPrefix-family
listings) after `handler_list_filters.go` turned up a live bug there.

**Found and fixed (chokepoint, 5 ops via one shared helper)**: `PathPrefix`
filtering ran *after* the backend's own `Marker`/`MaxItems` pagination
window had already been cut (`pageFromSortedNames` windows the raw,
unfiltered sorted-name list; `filterByPath` then filtered that window's
contents). Two bugs from this, both silent: (1) a page could come back
short of the requested `MaxItems` even when more matching items existed
past the current unfiltered window; (2) worse, every one of the 5 ops
hardcoded `IsTruncated: p.Next != "" && prefix == "/"` -- i.e. whenever a
non-default `PathPrefix` was actually filtering anything, `IsTruncated` was
forced `false` regardless of whether the backend had more data, so a real
client relying on `IsTruncated` (the documented contract) silently stopped
paging and never saw the remaining matches, even though `Marker` was still
populated on the response. Confirmed via `TestListUsers_PathPrefixTruncation`
(`list_filter_params_test.go`): 3 users, 2 matching a `PathPrefix`,
`MaxItems=1` so the match spans two backend windows -- failed against
unmodified code (returned only the first match, `IsTruncated=false`).
Fixed by adding `filteredPage` (`handler_list_filters.go`): when the
prefix is non-default it fetches the full unfiltered list once
(`fetchAllMaxItems = math.MaxInt32`), filters, then re-paginates the
*filtered* slice with `pkgs/page.New` so `Marker`/`IsTruncated` describe
the filtered result set. The default-prefix path is untouched (same
backend call as before, zero behavior change there). Applies identically
to `ListUsers`, `ListRoles`, `ListGroups`, `ListInstanceProfiles`, and
(via its own copy in `listPoliciesFilteredPage`) `ListPolicies` -- the
same wrong line, `p.Next != "" && prefix == "/"`, was duplicated 5 times
because no shared pagination-plus-filter helper existed before this fix;
now there is one (`filteredPage`) that the 4 simple listings share, plus
`listPoliciesFilteredPage` for `ListPolicies`' extra filters. Regression
coverage for the 3 siblings ListUsers doesn't directly test:
`TestListRolesGroupsInstanceProfiles_PathPrefix`.

**Found and fixed, `ListPolicies` only**: `OnlyAttached` and
`PolicyUsageFilter` were declared on `ListPoliciesInput` but never read
by the handler at all (class: never plumbed through) -- every call
returned every policy regardless of either parameter.
`OnlyAttached` now filters on `Policy.AttachmentCount > 0` (already
live-maintained by `addPolicyAttachmentLocked`/`removePolicyAttachmentLocked`,
no new state needed). `PolicyUsageFilter=PermissionsBoundary` now filters
on a new `PermissionsBoundaryARNs()` backend method (scans
`User.PermissionsBoundary`/`Role.PermissionsBoundary` across all users and
roles); `PolicyUsageFilter=PermissionsPolicy` excludes only policies used
*exclusively* as a boundary (a policy attached to a role/user AND also set
as some other principal's boundary still counts as a permissions policy --
the SDK doc comment doesn't state exclusivity explicitly, so this is a
documented judgment call, not an invented default). Proven by
`TestListPolicies_OnlyAttached` and `TestListPolicies_PolicyUsageFilter`
(`list_filter_params_test.go`), both failing against unmodified code
(returned every policy regardless of the filter).

**Checked and left as-is, `Scope`**: `ListPolicies`' pre-existing `Scope`
handling (`Local`/`AWS`/`All`) was already wired, just via a fragile
`strings.Contains(pol.Arn, ":aws:policy")` heuristic that happened to
always evaluate false (gopherstack never seeds or creates an AWS-managed
policy -- every `Policy` originates from `CreatePolicy`), making
`Scope=AWS` correctly-but-accidentally always empty. Replaced with an
explicit early return for `Scope=AWS` (documented as structural: there is
no AWS-managed-policy concept in this backend, so "no matches" is honest,
not a fabricated default) rather than leaving the coincidental string
match in place.

**Structural, not fixed**: real IAM's `Scope=AWS` would return the ~1000+
real AWS managed policies gopherstack does not model; disclosed above,
not a bug to fix without a modelling decision outside this pass's scope.

**PARITY.md accuracy note**: this file had no prior per-op entry claiming
`ListPolicies`'/`ListUsers`' filters were verified, so nothing here
corrects a previously-asserted-correct claim -- these were genuinely
unaudited for this bug class before now (the sweep-12/error-path sweeps
above covered sort-order and error-code selection respectively, not
filter/pagination honouring).

Gates: `go build ./...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/iam/...` (pass, including the 5 new
tests above), `golangci-lint run ./services/iam/...` (0 issues after
decomposing `listPoliciesFiltered` to stay under the `gocognit` budget and
routing the new `OnlyAttached` boolean compare through the existing
`formValueTrue` constant instead of a fresh `"true"` literal, per
`goconst`).

Not covered this pass (see items_still_open below): the remaining ~170
IAM ops were not re-audited for this constraint-parameter class. Notably
unexamined: `ListAttached{User,Role,Group}Policies`' `PathPrefix`,
`ListEntitiesForPolicy`'s `EntityFilter`/`PathPrefix`/`PolicyUsageFilter`,
`ListMFADevices`/`ListAccessKeys`/`ListSigningCertificates`/`ListSSHPublicKeys`/
`ListServiceSpecificCredentials` pagination-only parameters, and
`GetAccountAuthorizationDetails`' `Filter` (sweep 6 already added this one;
not re-verified this pass).

## 2026-08-30 -- gopherstack-uox6: value-semantics filter audit

Read this service against bd gopherstack-uox6's class ("a parameter that is read,
applied, and wrong" -- distinct from both the wire-shape sweeps above and the
2026-08-29 constraint-parameter sweep, which fixed WHETHER `PathPrefix` etc. were
applied at all; this pass asked whether the semantics of what IS applied are correct).

**Condition-operator evaluation (`conditions.go`)**, the richest matcher surface in
this service: all IAM condition operator families checked against the operator's own
documented meaning -- `StringEquals`/`StringLike`(wildcard `*`/`?`, both documented for
IAM policy grammar, unlike EventBridge's undocumented `?`)/`StringEqualsIgnoreCase`/
`Bool`/`Null`/`ArnEquals`+`ArnLike` (functionally identical per AWS docs, both
wildcarded)/`Numeric*`/`Date*` (`LessThanEquals`/`GreaterThanEquals` correctly include
the equality case) /`BinaryEquals`/`IfExists` suffix/`ForAllValues:`+`ForAnyValue:` set
qualifiers (vacuous-true / false-on-empty-set respectively, matching documented AWS
semantics) -- all correct. `evaluator.go`'s `wildcardMatch` (Action/Resource matching,
case-insensitive for Action, case-sensitive for Resource) is a real DP wildcard
matcher, not a substring stand-in, and both are documented AWS behaviour.

**`PathPrefix` (`handler_list_filters.go`, `policies.go`)**: filters on the *entity's
own* path throughout, including `listAttachedPoliciesFiltered` (resolves each
`AttachedPolicy` back to its owning `Policy.Path` via `GetPolicy`) and
`listEntitiesForPolicyFiltered` (resolves each user/group/role back to its own
`Path` via `userPath`/`groupPath`/`rolePath`) -- matches the SDK doc comment's
explicit "PathPrefix filters on the ENTITY's own path, not the policy's" already
recorded in this file's comments. No case where the policy's path was used in place
of the entity's.

**`GetAccountAuthorizationDetails`' `Filter` (`simulation.go`,
`authDetailsFilterSets`)**: all five `EntityType` enum members (`User`, `Group`,
`Role`, `LocalManagedPolicy`, `AWSManagedPolicy`) explicitly cased -- no
switch-without-default gap.

No bugs found; no code changes in this service this pass.

## 2026-08-31 per-item exact-case sweep (gopherstack-21my continuation)

Byte-for-byte item-level check against iam@v1.58.1 deserializers.go
(awsAwsquery_, confirmed by the `strings.EqualFold` match sites) for
GetAccountAuthorizationDetails, this service's richest item shape (four
distinct per-entity item types in one response: UserDetail, GroupDetail,
RoleDetail, ManagedPolicyDetail). Wrapper keys `UserDetailList`,
`GroupDetailList`, `RoleDetailList`, `Policies` all confirmed exact-case, each
`member`-wrapped (confirmed against `awsAwsquery_deserializeDocumentGroupDetailListType`
et al. -- no unwrapped-list-deserializer call site exists for any of the four
in the pinned SDK).

**BUG (fixed): `UserDetailXML` and `RoleDetailXML` (`models_simulation_types.go`)
omitted PermissionsBoundary and Tags entirely** -- absent, not wrong-named. The
real `UserDetail`/`RoleDetail` deserializers both read `PermissionsBoundary`
(nested `AttachedPermissionsBoundary{PermissionsBoundaryArn,
PermissionsBoundaryType}`) and `Tags>member{Key,Value}`, and this service's own
sibling ops -- `toUserXML` (GetUser/ListUsers) and `toRoleXML` (GetRole/ListRoles)
-- already emit both correctly from the same backing `User.PermissionsBoundary`/
`.Tags` and `Role.PermissionsBoundary`/`.Tags` fields (`RoleDetail`/`UserDetail`
embed `Role`/`User`, so the state was always reachable). Right entity count,
permanently blank PermissionsBoundary/Tags for every user and role in the
account-wide report regardless of what was actually set on them. `GroupDetail`
correctly has neither field on the real wire type (real IAM groups support
neither tags nor permissions boundaries) -- `GroupDetailXML`'s omission was
already correct, not a gap.

**BUG (fixed): `ManagedPolicyDetailXML` (`models_simulation_types.go`) omitted
DefaultVersionId, UpdateDate, AttachmentCount, and IsAttachable entirely** --
same sibling-trap shape, this time against `toPolicyXML` (GetPolicy/ListPolicies),
which already emits all four from the same backing `Policy` fields. Confirmed
this is worse than silent-empty: with the pre-existing `xml:"UpdateDate"` tag
(no `omitempty`) on a bare Go `string` field left at its zero value, `encoding/xml`
still emits `<UpdateDate></UpdateDate>`, and the real client's timestamp parser
hard-errors decoding it as an empty string against every real timestamp layout
it tries -- so any account with at least one managed policy failed the *entire*
`GetAccountAuthorizationDetails` call with a deserialization error, not just a
blank field.

Fixed both in `handler_account.go` (`toUserDetailXML`), `handler_roles.go`
(`toRoleDetailXML`), and `handler_policies.go` (`toManagedPolicyDetailXML`),
mirroring the exact conversion logic their singular/plural siblings already use
(including the `DefaultVersionId` `""`->`"v1"` and `UpdateDate` zero->CreateDate
fallbacks `toPolicyXML` already applies). Tests:
`TestGetAccountAuthorizationDetails_PermissionsBoundaryAndTags_RealClient` and
`TestGetAccountAuthorizationDetails_PolicyAttachmentFields_RealClient`
(`handler_account_reporting_test.go`), driven through the real aws-sdk-go-v2
client with distinguishable non-zero values (two users, two roles, an attached
policy). Both verified failing pre-fix by hand-revert -- the User/Role case
failed as a nil-pointer assertion, the Policy case failed one level worse, as a
full client-side deserialization error (`unable to parse time string ""`),
exactly the class this campaign has flagged as the more severe failure mode.

Also noted, not fixed (different bug class -- value semantics, not element
naming, so out of this issue's scope; recorded for a future
value-correctness pass): `Policy.IsAttachable` is never set anywhere in
`store.go`'s `CreatePolicy`, so it is always the Go zero value `false` for
every policy this backend creates -- affecting `GetPolicy`/`ListPolicies` too,
not just this op. Real AWS reports `IsAttachable=true` for essentially every
customer-managed policy (false is reserved for retired AWS-managed policies
this emulator doesn't model). This is a wrong-default-value bug, not an
absent/misnamed field, so it wasn't fixed under this pass's naming-focused scope.

NOT REACHED at item level this pass: the other ~15 List ops with nested item
shapes (ListRoles' `InstanceProfileList` nesting already covered incidentally
via `RoleDetail`; ListInstanceProfiles, ListServerCertificates,
ListSAMLProviders, ListOpenIDConnectProviders, ListVirtualMFADevices,
ListAccessKeys, ListSigningCertificates, ListSSHPublicKeys,
ListServiceSpecificCredentials, ListMFADevices, ListPolicies,
ListEntitiesForPolicy, GetOrganizationsAccessReport, and the delegation-request
family).

Gates: `go build ./services/iam/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/iam/...` (pass, including the 2 new
real-client tests above), `golangci-lint run ./services/iam/...` (0 issues).
Pre-existing `//nolint:lll // long XML element name` on
`GetAccountAuthorizationDetailsResponse.XMLName` (a struct this pass did not
otherwise touch) re-checked: `lll` is disabled repo-wide (superseded by
golines per `.golangci.yml`), so this directive is currently inert but
harmless -- left as-is, out of this pass's scope to clean up.

## 2026-08-31 per-item exact-case sweep, batch 2 (gopherstack-21my continuation)

Byte-for-byte item-level check against iam@v1.58.1 deserializers.go
(awsAwsquery_) for a subset of this issue's iam "not reached" list:
`ListInstanceProfiles` (incl. `ListInstanceProfilesForRole`/`GetInstanceProfile`/
`CreateInstanceProfile`, which all share one builder), `ListPolicies` (shares
`toPolicyXML` with `GetPolicy`/`CreatePolicy`, already fixed 2026-08-14),
`ListServerCertificates`, `ListAccessKeys`, `ListVirtualMFADevices`,
`ListSAMLProviders`.

**BUG (fixed): `InstanceProfileXML` (`models.go`), the shape shared by every
instance profile response (`Create`/`Get`/`List`/`ListForRole` all call the
same `toInstanceProfileXML`), omitted `Tags` entirely** -- the real
`InstanceProfile` deserializer reads it, and the tags ARE tracked: the same
`"ip:"`-prefixed key `TagInstanceProfile`/`ListInstanceProfileTags`/
`UntagInstanceProfile` already read and write
(`resourceTagDispatch("InstanceProfile", "ip:", "InstanceProfileName")`).
Unlike most bugs this campaign has found, this is **not** a Get-vs-List
disagreement -- every one of the four ops sharing this one builder was
equally wrong, so fixing the shared function fixed all four at once. Converted
`toInstanceProfileXML` from a free function to a `*Handler` method so it can
read `h.getTags`; updated all 7 call sites across `handler_instance_profiles.go`,
`handler_list_filters.go`, and `handler_roles.go` (mechanical rename, no logic
change at those sites). Test: `TestListInstanceProfiles_ItemShape_RealClient`
(`instance_profiles_test.go`), seeds two profiles with distinguishable tags,
asserts both round-trip through `ListInstanceProfiles`. Verified failing
pre-fix by hand-revert.

**BUG (fixed): `VirtualMFADeviceXML` (`models_mfa.go`), used by
`ListVirtualMFADevices` (`handler_mfa.go`), omitted `User` and `Tags`
entirely** -- both real `VirtualMFADevice` deserializer members. `User` (a
full nested `User` object) is backed by the same user-device link map
`GetMFADeviceOwner`/`EnableMFADevice`/`DeactivateMFADevice` already read and
write; `Tags` by the same `"mfa:"`-prefixed key
`TagMFADevice`/`ListMFADeviceTags` already use. `EnableDate` remains a genuine
gap -- `VirtualMFADevice` tracks device `Status` but not when it was enabled.
Fixed by resolving the owner via `h.Backend.GetMFADeviceOwner` +
`h.Backend.GetUser` and building a `UserXML` (the same builder `GetUser`/
`ListUsers` use) when present, and reading `h.getTags("mfa:"+serial)`. Test:
`TestListVirtualMFADevices_ItemShape_RealClient` (`mfa_test.go`), creates a
device, enables it for a tagged user, asserts both `User.UserName` and `Tags`
round-trip through `ListVirtualMFADevices`. Verified failing pre-fix by
hand-revert (`User` decoded nil).

**SEPARATE FINDING, not fixed (dead-code/registration-order issue, not a
wire-shape bug): `"ListMFADevices"` is registered by two different dispatch
tables (`iamMFALinkDispatch` via the `opListMFADevices` constant, and
`iamMFADeviceDispatch` via the literal string, which is the same value) --
`handler.go` merges `iamMFALinkDispatch` after `iamMFADeviceDispatch`
(`maps.Copy` order), so the second implementation always wins and the first is
unreachable. The two implementations are close enough in behavior
(`iamMFADeviceDispatch`'s falls back to an empty list on error and resolves
`UserName` per-device via `GetMFADeviceOwner`; `iamMFALinkDispatch`'s requires
a non-empty `UserName` input and echoes it back verbatim) that this has not
been observed to produce a wrong response in this pass's testing, but it is
worth a dedicated cleanup issue since dead code masquerading as live code is
exactly the kind of thing that misleads a future edit.

**RE-VERIFIED CLEAN, no changes needed:** `ListPolicies` (shares `toPolicyXML`
with `GetPolicy`/`CreatePolicy`, correct since the 2026-08-14 fix -- confirmed
by re-deriving against the pinned deserializer, not by trusting the prior
verdict), `ListServerCertificates` (`ServerCertificateMetadata`'s five
emitted fields all exact-case correct against
`awsAwsquery_deserializeDocumentServerCertificateMetadata`; `Expiration`
remains a genuine gap -- no certificate parsing, matching the pattern noted
for elbv2's `TrustStore.NumberOfCaCerts` earlier in this campaign),
`ListAccessKeys` (`AccessKeyMetadata`'s four real members all present and
correctly named), `ListSAMLProviders` (`SAMLProviderListEntry`'s three real
members all present and correctly named). Wrapping shape checked for every op
above: no call site of any unwrapped-list-deserializer variant exists for any
of them in the pinned SDK.

NOT REACHED at this layer: `ListOpenIDConnectProviders`, `ListSSHPublicKeys`,
`ListSigningCertificates`, `ListServiceSpecificCredentials`, `ListMFADevices`
(the non-virtual op, distinct from `ListVirtualMFADevices` above),
`ListEntitiesForPolicy`, `GetOrganizationsAccessReport`, and the
delegation-request family -- named so the next pass continues rather than
redoes.

Gates: `go build ./services/iam/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/iam/...` (pass, including both new
real-client tests above), `golangci-lint run ./services/iam/...` (0 issues
after `fieldalignment -fix ./services/iam/...` reordered
`CreateVirtualMFADeviceResponse`, whose pointer-field layout changed once
`VirtualMFADeviceXML` gained a `*UserXML` field). Pre-existing
`//nolint:revive` on `IAMError` (`models.go`, a struct this pass did not
otherwise touch) re-checked: still in use, `revive`'s stutter check would
otherwise fire on `iam.IAMError`.

## 2026-08-31 pass (gopherstack-21my continuation): remaining queued list ops

Swept the eight items this issue's queue named as not reached by the prior
pass, against `iam@v1.58.1` deserializers.go, byte-for-byte for case as well
as name.

**BUG (fixed): `ListEntitiesForPolicy`'s `PolicyEntityUser`/`PolicyEntityGroup`/
`PolicyEntityRole` (`models_policies.go`) never emitted `UserId`/`GroupId`/
`RoleId` at all** -- all three are real members of `types.PolicyUser`/
`PolicyGroup`/`PolicyRole` (`awsAwsquery_deserializeDocumentPolicyUser` /
`PolicyGroup` / `PolicyRole`), and all three are backed by state this backend
already has (`User.UserID`/`Group.GroupID`/`Role.RoleID`, all populated at
creation and already read correctly by `GetUser`/`GetGroup`/`GetRole`) -- so
every user/group/role in every `ListEntitiesForPolicy` response had the right
name and a permanently blank ID, right count, silently missing content.
Fixed by extending `filterEntityRows`'s per-entity lookup (previously only
resolving `Path`, for `PathPrefix` filtering) to also resolve the stable ID,
and threading it through `policyEntityRow` to the response builder. Test:
`TestListEntitiesForPolicy_ItemShape_RealClient`
(`list_filter_params_test.go`), attaches a policy to one user/group/role via
the real client and asserts each returned Id matches the id the corresponding
Create call returned. Verified failing pre-fix by hand-revert (`UserId`/
`GroupId`/`RoleId` decoded empty).

**RE-VERIFIED CLEAN, byte-for-byte case included:** `ListOpenIDConnectProviders`
(`OpenIDConnectProviderListEntry` genuinely has only `Arn` in the real SDK --
not a truncated shape), `ListSSHPublicKeys` (`SSHPublicKeyMetadata`'s four
real members all present and correctly cased in both the live dispatch entry
and its shadowed, documented-dead duplicate), `ListSigningCertificates`
(`SigningCertificate`'s five real members -- CertificateBody included, unlike
SSH keys' slimmer list type -- all present), `ListMFADevices` (the
non-virtual op; `MFADevice`'s three real members all present in the dispatch
entry that actually wins the two-registration collision noted in the prior
pass), the delegation-request family (`DelegationRequest`'s eleven emitted
members of twenty real ones are correctly named, and the nine omitted --
`ApproverId`, `ExpirationTime`, `OwnerId`, `PermissionPolicy`,
`RejectionReason`, `RequestorId`, `RequestorName`,
`RolePermissionRestrictionArns`, `UpdatedTime` -- are already documented in
`models_account.go` as gaps this backend has no state for; `Get`/
`ListDelegationRequests` share one builder, `toDelegationRequestXML`, so no
sibling disagreement is possible here).

**GAPS RECORDED, not fixed -- real per-AWS-docs fields this backend cannot
observe:**
- `ListServiceSpecificCredentials`'s `ServiceSpecificCredentialMetadataXML`
  is missing `ExpirationDate` and `ServiceCredentialAlias` (both real members
  of `types.ServiceSpecificCredentialMetadata`). The domain model
  (`ServiceSpecificCredential`, `models_credentials.go`) has no field for
  either -- these are Bedrock-API-key-only attributes this backend's
  credential model never tracked -- and `Create`/`ResetServiceSpecificCredential`
  share the identical two-field gap, so there is no sibling to disagree with.
- `GetOrganizationsAccessReport`'s `AccessDetails` is wire-typed `[]string`
  where the real member is `[]types.AccessDetail` (a six-field object:
  `EntityPath`, `LastAuthenticatedTime`, `Region`, `ServiceName`,
  `ServiceNamespace`, `TotalAuthenticatedEntities`). This is a real shape
  mismatch, but it is unobservable: this backend's access-report job never
  produces any access-detail records (`account.go`'s
  `GetOrganizationsAccessReport` only tracks job status/creation time), so
  the field is always the empty list either way -- matching the existing
  documented precedent for `PoliciesGrantingServiceAccess`
  (`models_account.go`) a few lines away. Not counted as a fix per this
  issue's restraint guidance: no legal input changes the outcome.

**METHOD NOTE:** ran the no-`*Unwrapped`-call-site check repo-wide against
`iam@v1.58.1`; unlike route53 (zero hits), iam has three:
`CertificationMapTypeUnwrapped`, `EvalDecisionDetailsTypeUnwrapped`,
`SummaryMapTypeUnwrapped` -- all query-protocol flattened *maps* (Simulate*
Policy's per-resource decision maps, `GetAccountSummary`'s `SummaryMap`), not
list-item collections, and none are in this pass's or the prior pass's
queue. Flagged for whoever next touches `GetAccountSummary` or
`SimulateCustomPolicy`/`SimulatePrincipalPolicy` rather than chased here.

Gates: `go build ./services/iam/... ./services/route53/...`, `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/iam/... ./services/route53/...`
(pass), `golangci-lint run ./services/iam/... ./services/route53/...` (0
issues, after switching `userPathAndID`/`groupPathAndID`/`rolePathAndID` from
named to bare returns -- `nonamedreturns` flagged the named-return form these
three helpers were first written with). No `nolint` directives exist in any
file this pass touched.

## 2026-08-31 unnamed-in-PARITY sweep (gopherstack-6flj/21my continuation)

Targeted the six `List*` operations whose names appeared nowhere in this
file before today: `ListAccountAliases`, `ListGroupPolicies`,
`ListOrganizationsFeatures`, `ListPolicyVersions`, `ListRolePolicies`,
`ListUserPolicies`. Confirmed protocol from the deserializer directly:
`iam@v1.58.1` is `awsAwsquery_` (XML query protocol) -- the smithy-go XML
decoder case-folds element names, so a case-only mismatch would decode
correctly today and be invisible to any round-trip test; watched for this
specifically and found none in this batch. All six read against their own
deserializer function in `deserializers.go` and, for the two with
structured items (`ListPolicyVersions`), the real `types.PolicyVersion`.

**All six clean, no bug found**:
- `ListAccountAliases`: wrapper `AccountAliases>member` matches
  `awsAwsquery_deserializeOpDocumentListAccountAliasesOutput`'s
  `"AccountAliases"` case exactly; `IsTruncated` present; `Marker` absent
  is consistent since this backend never truncates.
- `ListGroupPolicies` / `ListRolePolicies` / `ListUserPolicies`: all three
  share the same `PolicyNames>member` wrapper shape, all correct, all
  backed by real state (`h.Backend.List*Policies`).
- `ListPolicyVersions`: `Versions>member` wrapper correct;
  `PolicyVersionXML{VersionID, CreateDate, IsDefaultVersion}` covers every
  field `types.PolicyVersion` declares except `Document`, which real
  `ListPolicyVersions` documents as intentionally omitted ("The policy
  document is returned in the response to GetPolicyVersion and
  GetAccountAuthorizationDetails... It is not returned in the response to
  CreatePolicyVersion or ListPolicyVersions", `api_op_ListPolicyVersions.go`)
  -- so its absence here is correct AWS behavior, not a gap.
- `ListOrganizationsFeatures`: already a deliberately-empty stub
  (`models_account.go` comment already documents the wire shape was
  verified against the deserializer: `EnabledFeatures`/`OrganizationId`,
  not the previously-invented `OrganizationFeatures`/`RootId`). No backend
  state exists to populate it; correctly returns an empty list rather than
  fabricating one. Confirmed the wire shape is still accurate against
  `iam@v1.58.1` and left as-is.

No wrapper-key mismatches, no per-item field mismatches, no case-only
mismatches, and no hard-decode-error risks found in this batch's six
operations. This closes out the last of the previously-unswept-by-name
operations for iam under gopherstack-6flj's targeting; future passes on
iam should return to axes already covered rather than name-based gaps.

Gates: `go build ./services/iam/... ./services/dynamodb/...`, `go vet ./...`
(repo-wide, clean). No files under `services/iam/` were modified this pass
(read-only verification); no `go test`/`golangci-lint` re-run needed beyond
the repo-wide `go vet`.
