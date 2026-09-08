---
service: cognitoidp
sdk_module: aws-sdk-go-v2/service/cognitoidentityprovider@v1.67.4
last_audit_commit:                                # unknown: pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-08-08
# 2026-08-30: cursor-population sweep (does every List/Describe response struct that DECLARES a
# NextToken/PaginationToken actually SET one before the collection can exceed a page?). Enumerated
# all 16 SDK ops whose Input/Output declare a continuation token. This service's dispatch table
# layers multiple op-maps per family (groupsOpsA/B, identityProvidersOpsA/B/C,
# resourceServersOpsA/B, userPoolClientsOpsA/B/C, ...) with later maps.Copy calls in handler.go
# overwriting earlier ones on key collision -- for every op below, verified which registration
# actually wins before auditing its handler (the "duplicate wire-key" risk this file's own history
# already flags for cognitoidp). 5 genuine bugs found and fixed, 3 of them (ListUserImportJobs,
# ListResourceServers, ListUserPoolClients) previously known and explicitly left `deferred` (see
# below) because the wire structs didn't even declare the field, not just leave it unpopulated --
# all three now do. Also fixed: ListIdentityProviders (field WAS already declared -- the
# `identity_providers` row below claimed "no gaps found" from a field diff that checked item
# shape, not pagination) and AdminListGroupsForUser (field never declared). All 5 fixed via
# pkgs/page.New at the handler layer (the winning handler in each duplicate-registration case),
# reusing each backend's existing deterministic sort. 9 ops confirmed already correct: AdminListDevices,
# AdminListUserAuthEvents, ListDevices, ListGroups (via handleListGroupsFull -> ListGroupsPage),
# ListTerms, ListUserPools, ListUsers, ListUsersInGroup (via handleListUsersInGroupFull ->
# ListUsersInGroupPage), ListWebAuthnCredentials.
#
# CORRECTION 2026-08-29 (pagination-arithmetic sweep): "confirmed already correct" above was
# checked for cursor-population/wire-shape only, not pagination arithmetic. 7 of these 9 --
# AdminListDevices, ListDevices, ListGroups, ListUsersInGroup, ListWebAuthnCredentials, ListUsers,
# ListUserPools -- turned out to have a genuine Class B (infinite loop on a stale cursor) bug in
# their own hand-rolled equality-scan cursor. None of them use pkgs/page. Fixed this pass; see the
# dated pagination-arithmetic section near the end of this file. ListTerms (real pkgs/page.New
# user) and AdminListUserAuthEvents (real bug too, but its authEvents store is never populated by
# any code path in this emulator, so it was unreachable in practice -- fixed anyway) are covered
# there too. 2 left unfixed as provably bounded:
# ListUserPoolClientSecrets (real AWS's documented 2-active-secrets limit, enforced here as
# maxExtraClientSecrets) and ListUserPoolReplicas (this backend enforces "at most one [replica] is
# allowed per user directory", matching real Cognito's current one-secondary-region limit --
# user_pool_replicas.go:68-71).
overall: A                # 2026-08-08 (gopherstack-kxow): restored from B to A -- terms/, the
                       # sole reason for the prior B (its entire wire model was invented and
                       # unreachable by any real SDK client), is now a full, field-diffed
                       # redesign against the real 5-op family and its two enums; see
                       # families.terms and the CLOSED gap entry below. No other family
                       # regressed; the remaining `deferred` items (risk_config LastModifiedDate,
                       # ListUserImportJobs/ListResourceServers pagination, domains
                       # Routing/Version, devices' extra DeviceStatus field) are the same
                       # small, structural/low-severity gaps that were already present under
                       # the prior A grade and were never the reason for the B downgrade.
                       #
                       # --- prior (2026-08-08, gopherstack-n7gh follow-up) history ---
                       # downgraded from A to B
                       # this pass -- terms/ (below) is a real, non-structural gap: its entire
                       # wire model is fictional, and no genuine aws-sdk-go-v2 client can
                       # successfully call CreateTerms against this backend today (client-side
                       # request validation itself rejects the call before it's even sent).
                       # Every other family this pass touched came back clean or was fixed;
                       # this is the one confirmed real op family a real client cannot use.
                       # Completed the rest of
                       # gopherstack-n7gh's stated scope. UserMigration_ForgotPassword trigger
                       # source implemented (user_migration.go/lambda_triggers.go); domain
                       # AWSAccountId/ManagedLoginVersion/S3Bucket now populated (domains.go).
                       # Op-by-op re-walk of user_import_jobs/devices/webauthn/
                       # managed_login_branding/risk_config/terms/log_delivery plus a full
                       # field diff of identity_providers/resource_servers found and fixed real
                       # bugs: webauthn's response used the wrong wire key ("FriendlyName"
                       # instead of "FriendlyCredentialName" -- silently unreadable by any real
                       # SDK client) and dropped the required AuthenticatorTransports field;
                       # managed_login_branding completely discarded Settings/Assets/
                       # UseCognitoProvidedValues -- the entire payload of the feature -- on
                       # Create/Update; SetLogDeliveryConfiguration was a disguised stub that
                       # hardcoded nil regardless of the client's LogConfigurations; CreateUserImportJob
                       # silently dropped CloudWatchLogsRoleArn (a required field) and
                       # PasswordHashingAlgorithm. All fixed this pass -- see gaps below for
                       # detail and see deferred below for what was found but NOT fixed:
                       # terms/ is built on an entirely fictional wire model (not just missing
                       # fields) and needs a full redesign, not a bounded fix.
                       # SRP-6a itself was already completed earlier the same day in commit
                       # 041c16c75 (see the CLOSED gap entry below) -- this pass did not touch it.
                       #
                       # --- prior (2026-07-25, parity-4) history, kept for context ---
                       # 7 new SDK ops (CreateUserPoolReplica/ListUserPoolReplicas/UpdateUserPoolReplica/DeleteUserPoolReplica/AdminGetUserAuthFactors/GetProvisionedLimit/UpdateProvisionedLimit) implemented for real against a bumped SDK, closing TestSDKCompleteness -- all field-diffed against the installed SDK's types/serializers/deserializers, all backed by real state (no notImplemented additions). Two explicit, documented modeling assumptions (replica's initial Status; provisioned limits' account-level-max ceiling) -- see families.user_pool_replicas/provisioned_limits below. Everything from the 2026-07-23 pass (CUSTOM_AUTH state machine, UserMigration/PreAuthentication/PostAuthentication triggers, PreventUserExistenceErrors on ConfirmSignUp/ConfirmForgotPassword, DescribeUserPoolDomain CustomDomainConfig, dead-code deletion) carries forward unchanged, not re-walked this pass. 0 golangci-lint issues, 0 banned nolints, race-clean
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
ops:
  InitiateAuth: {wire: ok, errors: ok, state: ok, persist: ok, note: "USER_PASSWORD_AUTH/ADMIN_USER_PASSWORD_AUTH/CUSTOM_AUTH/REFRESH_TOKEN_AUTH all real; PreventUserExistenceErrors masking (prior pass, gopherstack-2sp); PreTokenGeneration trigger fires on token issuance (prior pass, gopherstack-8fw); PreAuthentication/PostAuthentication/UserMigration triggers fire (prior pass); CUSTOM_AUTH a real Lambda-driven state machine (prior pass). THIS PASS (gopherstack-n7gh): USER_SRP_AUTH is now real SRP-6a (see srp.go) -- previously it required AuthParameters[PASSWORD] directly (a real SRP client never sends this, so every real-client USER_SRP_AUTH call had always failed with NotAuthorizedException). Routes to InitiateAuthSRP given AuthParameters[SRP_A]."}
  AdminInitiateAuth: {wire: ok, errors: ok, state: ok, persist: ok, note: "never masks UserNotFoundException, matching AWS (admin API); PreTokenGeneration trigger fires (prior pass); PreAuthentication/PostAuthentication/UserMigration/CUSTOM_AUTH real (prior pass). THIS PASS (gopherstack-n7gh): ADMIN_USER_SRP_AUTH is now real SRP-6a via AdminInitiateAuthSRP -- previously this flow name was not even in authenticate()'s accepted-flow switch (only \"USER_SRP_AUTH\" was), so a real client's AdminInitiateAuth SRP attempt got InvalidUserPoolConfigurationException outright, a second bug on top of the plaintext-password one."}
  RespondToAuthChallenge: {wire: ok, errors: ok, state: ok, persist: ok, note: "SOFTWARE_TOKEN_MFA real RFC6238 TOTP; SMS_MFA/EMAIL_OTP require the generated one-time code; NEW_PASSWORD_REQUIRED real; PreTokenGeneration trigger fires on token issuance; CUSTOM_CHALLENGE handled for real (prior pass). gopherstack-n7gh: PASSWORD_VERIFIER now verifies a real SRP-6a zero-knowledge password-claim signature (PASSWORD_CLAIM_SECRET_BLOCK/PASSWORD_CLAIM_SIGNATURE/TIMESTAMP against server-held (A,b,B,v) session state) instead of unconditionally issuing tokens for any session token that merely existed. Also fixed: success now runs the same FORCE_CHANGE_PASSWORD/MFA gate USER_PASSWORD_AUTH runs (postCredentialCheckLocked) -- previously RespondToSRPChallenge always issued tokens directly, bypassing NEW_PASSWORD_REQUIRED/MFA entirely for any SRP login. CLOSED 2026-08-22 (gopherstack-1b07): MFA_SETUP is now a real, reachable challenge. mfaChallengeType (mfa.go) previously defaulted every MFA-required user with no PreferredMfaSetting straight to SOFTWARE_TOKEN_MFA, including a user who had never called AssociateSoftwareToken -- a dead end, since RespondToMFAChallenge requires a TOTP secret that was never associated. It now returns MFA_SETUP (InitiateAuth doc: 'For users who are required to setup an MFA factor before they can sign in') unless the user already has a verified software token (user.TOTPVerified) or an explicit preference. RespondToAuthChallenge/AdminRespondToAuthChallenge handle ChallengeName=MFA_SETUP via new backend method RespondToMFASetupChallenge, which requires the session's user to have TOTPVerified (set by VerifySoftwareToken), records SOFTWARE_TOKEN_MFA as the user's MFA preference, and issues tokens -- completing InitiateAuth's documented 'use the session returned by VerifySoftwareToken as an input to RespondToAuthChallenge ... with challenge name MFA_SETUP to complete sign-in.' NOT modeled: the MFAS_CAN_SETUP challenge-parameter value InitiateAuth's doc says accompanies MFA_SETUP (ChallengeParameters is not populated for any of this backend's non-SRP challenges, a pre-existing gap this pass did not extend to fix) and any session single-use/rotation semantics across AssociateSoftwareToken -> VerifySoftwareToken -> RespondToAuthChallenge -- the SDK's doc prose never states whether each op consumes/rotates the session, so this backend echoes the same session token unchanged through all three calls (only the final RespondToAuthChallenge call actually deletes it) rather than inventing rotation behavior."}
  AdminRespondToAuthChallenge: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fixes as RespondToAuthChallenge (shared backend method), including CUSTOM_CHALLENGE and PASSWORD_VERIFIER real SRP verification (gopherstack-n7gh) and MFA_SETUP (gopherstack-1b07, this pass)"}
  AssociateSoftwareToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "CLOSED 2026-08-22 (gopherstack-1b07): now also accepts Session as documented alternate to AccessToken, resolving the user via the MFA_SETUP challenge session InitiateAuth/AdminInitiateAuth issues instead of always requiring findUserByAccessTokenLocked. See RespondToAuthChallenge note for the full flow."}
  VerifySoftwareToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "now verifies a real RFC 6238 TOTP code against the associated secret (was: any 6 digits accepted) — gopherstack-2sp. CLOSED 2026-08-22 (gopherstack-1b07): now also accepts Session as documented alternate to AccessToken (see AssociateSoftwareToken/RespondToAuthChallenge)."}
  SetUserMFAPreference: {wire: ok, errors: ok, state: ok, persist: ok, note: "SetUserMFAPreferenceInput has NO Session field in the real SDK -- AccessToken is `This member is required` (api_op_SetUserMFAPreference.go). Earlier notes on this op family (gopherstack-1b07's own filing, and the PARITY entry it came from) incorrectly claimed a documented Session alternate here; corrected 2026-08-22. This op was never actually affected by the MFA_SETUP Session gap."}
  AdminSetUserMFAPreference: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateUserPool: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeUserPool: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateUserPool: {wire: ok, errors: ok, state: ok, persist: ok, note: "PasswordPolicy, MfaConfiguration, LambdaConfig(stored only, see gaps), AccountRecoverySetting, DeletionProtection all settable"}
  ListUserPools: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteUserPool: {wire: ok, errors: ok, state: ok, persist: ok, note: "now refuses to delete when DeletionProtection=ACTIVE (was: silently deleted regardless) — gopherstack-2sp. FIXED (gopherstack-tq5q): also now refuses to delete a pool that still owns a domain (InvalidParameterException, matching real AWS's documented error for this case — github.com/hashicorp/terraform-provider-aws#16479) instead of silently orphaning the domain row. domainsKeyFn is the bare, pool-independent domain string (store_setup.go), so a deleted pool's domain used to survive with no owning pool and no cleanup path (DeleteUserPoolDomain required the pool to still exist) — an unrecoverable lockout on that domain name. See DeleteUserPoolDomain note for the companion recovery path for domains already orphaned by data predating this fix. Also: the user cascade now calls the same deleteUserStateLocked helper AdminDeleteUser/DeleteUser use (users.go) instead of repeating its cleanup list by hand — the devices/authEvents repeat (gopherstack-cq0z) had already drifted once by missing groupMembers/webauthnCredentials (gopherstack-ljak); a shared helper closes that drift class instead of adding one more line to a list."}
  CreateUserPoolClient: {wire: ok, errors: ok, state: ok, persist: ok, note: "PreventUserExistenceErrors field added this pass (was entirely unimplemented); OAuth flows/scopes/callback URLs/token validity units/secret generation all real"}
  UpdateUserPoolClient: {wire: ok, errors: ok, state: ok, persist: ok, note: "PreventUserExistenceErrors now updatable"}
  DescribeUserPoolClient: {wire: ok, errors: ok, state: ok, persist: ok}
  ListUserPoolClients: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteUserPoolClient: {wire: ok, errors: ok, state: ok, persist: ok}
  AddUserPoolClientSecret: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-h910): output was a flat ClientSecret string; real AddUserPoolClientSecretOutput nests ClientSecretDescriptor{ClientSecretId, ClientSecretValue, ClientSecretCreateDate}. UserPoolClient.ClientSecret was a single string with no ClientSecretId, so DeleteUserPoolClientSecret's required ClientSecretId was unwireable -- added a separate ClientSecretId-keyed ExtraClientSecrets set (capped at 1, i.e. 2 active secrets total including the original), LimitExceededException past the cap"}
  DeleteUserPoolClientSecret: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-h910): dropped the required ClientSecretId entirely (decoded a dead SecretHash field that doesn't exist on the real API and was never even read). Now requires ClientSecretId, ResourceNotFoundException for an unknown id, removes only the matching entry from ExtraClientSecrets"}
  ListUserPoolClientSecrets: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed alongside gopherstack-h910: output was a fabricated flat Secrets []string; real ListUserPoolClientSecretsOutput is ClientSecrets []ClientSecretDescriptorType (Id + CreateDate only, value never revealed). Also fixed a false comment claiming AWS allows at most one active secret -- the real API documents up to 2"}
  SignUp: {wire: ok, errors: ok, state: ok, persist: ok, note: "password policy enforced, real confirm code generated; PreSignUp trigger now fires and applies autoConfirmUser/autoVerifyEmail/autoVerifyPhone, CustomMessage trigger now fires (this pass, gopherstack-8fw)"}
  ConfirmSignUp: {wire: ok, errors: ok, state: ok, persist: ok, note: "expiring codes, CodeMismatchException/ExpiredCodeException; PostConfirmation trigger fires fire-and-observe -- invocation errors surface but do not roll back confirmation, matching AWS; PreventUserExistenceErrors=ENABLED now masks an unknown username behind CodeMismatchException, the same error a real-but-wrong-code account produces (this pass, closes remainder of gopherstack-aib)"}
  AdminConfirmSignUp: {wire: ok, errors: ok, state: ok, persist: ok, note: "PostConfirmation trigger now fires (this pass), same source/semantics as ConfirmSignUp"}
  ResendConfirmationCode: {wire: ok, errors: ok, state: ok, persist: ok, note: "PreventUserExistenceErrors=ENABLED now masks unknown-user UserNotFoundException as a fabricated success (prior pass, closes gopherstack-aib); CustomMessage trigger now fires (this pass)"}
  AdminCreateUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "PreSignUp trigger now fires (source PreSignUp_AdminCreateUser); only autoVerifyEmail/autoVerifyPhone applied, autoConfirmUser has no target state for admin-created users (this pass). FIXED 2026-08-22 (gopherstack-zquj): adminUserJSON's attribute list was tagged json:\"UserAttributes\", but AdminCreateUserOutput.User is a UserType whose own member is \"Attributes\" -- every real client's User.Attributes decoded nil. See Notes below."}
  AdminSetUserPassword: {wire: ok, errors: ok, state: ok, persist: ok}
  AdminGetUser: {wire: ok, errors: ok, state: ok, persist: ok}
  AdminDeleteUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-ljak): now also clears groupMembers[poolID][*][username] and webauthnCredentials[userStateKey] via the shared deleteUserStateLocked helper (was: left both behind). Usernames are caller-chosen and the pool persists, so recreating a deleted username used to inherit group membership it was never granted -- observable via ListUsersInGroup and the cognito:groups claim (userGroupsLocked) -- plus stale WebAuthn credentials (observable via AdminGetUserAuthFactors' WEB_AUTHN entry)."}
  AdminResetUserPassword: {wire: ok, errors: ok, state: ok, persist: ok}
  AdminUpdateUserAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  AdminDeleteUserAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  AdminDisableUser: {wire: ok, errors: ok, state: ok, persist: ok}
  AdminEnableUser: {wire: ok, errors: ok, state: ok, persist: ok}
  AdminUserGlobalSignOut: {wire: ok, errors: ok, state: ok, persist: ok, note: "revokes refresh tokens + stamps tokenRevokedBefore so already-issued access tokens are rejected too"}
  GlobalSignOut: {wire: ok, errors: ok, state: ok, persist: ok, note: "same revocation mechanism as AdminUserGlobalSignOut"}
  RevokeToken: {wire: ok, errors: ok, state: ok, persist: ok}
  ListUsers: {wire: ok, errors: ok, state: fixed, persist: ok, note: "CORRECTION 2026-08-29 (pagination-arithmetic sweep): the 'pkgs/page-style pagination' note above was wrong -- handleListUsers hand-rolls its own equality-scan cursor inline (handler_users.go), does not call pkgs/page at all, and had a Class B infinite-loop bug on a stale cursor. See the pagination-arithmetic section below."}
  ListUsersInGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-22 (gopherstack-zquj): same adminUserJSON \"UserAttributes\"-vs-\"Attributes\" bug as AdminCreateUser (this type backs both ops' item shape). See Notes below."}
  ForgotPassword: {wire: ok, errors: ok, state: ok, persist: ok, note: "PreventUserExistenceErrors=ENABLED masks unknown-user UserNotFoundException as a fabricated success (prior pass, closes gopherstack-aib); CustomMessage trigger now fires (prior pass, gopherstack-8fw). THIS PASS (gopherstack-n7gh follow-up): an unknown username now also tries the UserMigration_ForgotPassword Lambda trigger (user_migration.go's tryUserMigrationForgotPassword) before falling back to PreventUserExistenceErrors masking / UserNotFoundException, matching the documented 'user migration during forgot-password flow' trigger source. Per AWS docs, no password is sent in this event (request.password is omitted entirely, not sent empty) since the user has none yet."}
  ConfirmForgotPassword: {wire: ok, errors: ok, state: ok, persist: ok, note: "PreventUserExistenceErrors=ENABLED now masks an unknown username behind CodeMismatchException, same rationale as ConfirmSignUp (this pass, closes remainder of gopherstack-aib)"}
  ChangePassword: {wire: ok, errors: ok, state: ok, persist: ok}
  GetUser: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-ljak, same shape as AdminDeleteUser): now also clears groupMembers/webauthnCredentials via the shared deleteUserStateLocked helper"}
  DeleteUserAttributes/AdminDeleteUserAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  VerifyUserAttribute/GetUserAttributeVerificationCode: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateGroup/DeleteGroup/GetGroup/ListGroups/UpdateGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "precedence respected"}
  AdminAddUserToGroup/AdminRemoveUserFromGroup/AdminListGroupsForUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "cognito:groups claim reflected in ID/access tokens"}
  CreateResourceServer/DescribeResourceServer/ListResourceServers/UpdateResourceServer/DeleteResourceServer: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateIdentityProvider/DescribeIdentityProvider/ListIdentityProviders/UpdateIdentityProvider/DeleteIdentityProvider/GetIdentityProviderByIdentifier: {wire: ok, errors: ok, state: ok, persist: ok, note: "audited at a family level, not re-walked line by line this pass — unchanged since prior sweep"}
  TagResource/UntagResource/ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "pkgs/tags"}
  GetSigningCertificate: {wire: ok, errors: ok, state: ok, persist: ok, note: "deterministic self-signed X.509 wrapping the pool's real RSA key"}
  GetUserPoolMfaConfig/SetUserPoolMfaConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  jwks_well_known: {wire: ok, errors: ok, state: ok, persist: ok, note: "RS256, real RSA-2048 per pool, JWKS + GetSigningCertificate both derive from the same key"}
  AdminGetUserAuthFactors: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-4, new SDK op. Field-diffed AdminGetUserAuthFactorsOutput against the SDK: Username/ConfiguredUserAuthFactors/PreferredMfaSetting/UserMFASettingList all present. Factors are derived from real user state, not fabricated: PASSWORD from user.PasswordHash != \"\"; SMS_OTP from UserMFASettingList containing SMS_MFA or any legacy MFAOptions[].DeliveryMedium == SMS; SOFTWARE_TOKEN from user.TOTPVerified or SOFTWARE_TOKEN_MFA in UserMFASettingList; WEB_AUTHN from a non-empty webauthnCredentials entry for the user. Shares its PASSWORD/SMS_OTP/WEB_AUTHN derivation with the existing GetUserAuthFactors via a new commonAuthFactorSetLocked helper (users.go) -- GetUserAuthFactors' own behavior/output is unchanged, only the shared plumbing was extracted. FIXED 2026-08-23 (manifest-harvest pass, bd: none filed): the SOFTWARE_TOKEN derivation was moved into commonAuthFactorSetLocked itself, so GetUserAuthFactors now derives it too -- both GetUserAuthFactorsOutput and AdminGetUserAuthFactorsOutput share the exact same types.AuthFactorType enum (PASSWORD/EMAIL_OTP/SMS_OTP/WEB_AUTHN/SOFTWARE_TOKEN, cognitoidentityprovider@v1.67.4 types/enums.go:184-192) so there was no reason for the self-service op to omit a factor the admin op derives from the same user record. This was real state the backend already tracked (user.TOTPVerified / SOFTWARE_TOKEN_MFA in UserMFASettingList) and never surfaced through GetUserAuthFactors -- the item this note itself previously deferred as items_still_open. Proven via new TestInMemoryBackend_GetUserAuthFactors_SoftwareToken (users_test.go), hand-reverted to confirm it fails against the pre-fix code (asserts SOFTWARE_TOKEN present, pre-fix returns only PASSWORD), restored, md5sum byte-identical. EMAIL_OTP remains unmodeled by both ops -- this backend has no EmailMfaSettings state anywhere (verified: zero matches for EmailMfaSettings/EmailMFASettings in services/cognitoidp), a genuine modelling gap, not a bug -- not added."}
  user_import_jobs: {status: fixed, note: "FIXED 2026-08-21 (gopherstack-muzq): StartUserImportJob correctly stamps InProgress, and StopUserImportJob correctly reaches Stopped -- but the self-completion path (a real import job finishes on its own once its CSV is processed, per UserImportJobStatusType's InProgress->Succeeded/Failed transitions, cognitoidp@v1.67.4 types/enums.go) did not exist: nothing but an explicit client Stop ever wrote to Status again. TestUserImportJob_CRUD only ever asserted InProgress right after Start then moved straight to Stop, so a machine that never self-advances was indistinguishable from a correct one. Confirmed no other advancing path anywhere in the package. Reused the package's own existing Janitor (janitor.go, a worker.Group ticker that already sweeps expired refresh tokens/MFA sessions) rather than inventing new infrastructure: added AdvanceUserImportJobStatuses(minAge) (user_import.go), mirroring bedrock's AdvanceCustomizationJobStatuses(minAge) shape, wired into Janitor.SweepOnce. New test TestUserImportJob_SelfCompletesToSucceeded (user_import_test.go) drives the janitor directly and asserts DescribeUserImportJob eventually reports Succeeded with no Stop call. Hand-reverted user_import.go+janitor.go to git show HEAD, confirmed the new test fails (Condition never satisfied, status stuck InProgress), restored, md5sum byte-identical. op-by-op re-walk THIS PASS (gopherstack-n7gh follow-up): field-diffed userImportJobType against types.UserImportJobType and found CreateUserImportJobInput's required CloudWatchLogsRoleArn and optional PasswordHashingAlgorithm were accepted by no input field at all (silently dropped -- class a) -- fixed, now stored and echoed. Also added CreationDate/StartDate/CompletionDate (CreatedAt was already tracked internally but never echoed; StartedAt/CompletedAt added, set by StartUserImportJob/StopUserImportJob), PreSignedUrl (fabricated the same way domains.go fabricates CloudFrontDistribution/S3Bucket -- an AWS-internal value no caller can validate), and FailedUsers/ImportedUsers/SkippedUsers=0 (honest: this backend has no real CSV-processing pipeline, so zero imported/failed/skipped is literally true, not fabricated). FIXED (2026-08-30, cursor sweep): the deferred pagination gap noted below is closed -- listUserImportJobsInput/Output now declare PaginationToken/MaxResults (real AWS field names, not NextToken -- confirmed from api_op_ListUserImportJobs.go) and handleListUserImportJobs pages via pkgs/page.New. Proven via TestListUserImportJobs_Pagination + hand-revert."}
  devices: {status: ok, note: "op-by-op re-walk THIS PASS: field-diffed deviceType against types.DeviceType and confirmed absence carefully -- the real DeviceType has exactly 5 fields (DeviceAttributes/DeviceCreateDate/DeviceKey/DeviceLastAuthenticatedDate/DeviceLastModifiedDate) and NO DeviceStatus field at all; device remembered status is write-only via AdminUpdateDeviceStatus/UpdateDeviceStatus's DeviceRememberedStatus and is never readable back through Get/List/AdminGet/AdminList in real Cognito. This backend's deviceType.DeviceStatus is therefore an EXTRA fabricated field not on the real wire -- flagged, NOT fixed: several existing tests (devices_test.go) assert on it, a real AWS SDK JSON client harmlessly ignores unknown response keys, and removing it would only lose test-observable state for a purely cosmetic gain. Documented as a trap below rather than silently left as-is. Evidence, checked 2026-08-13 against aws-sdk-go-v2/service/cognitoidentityprovider@v1.67.4 types/types.go:677-698: struct DeviceType has exactly DeviceAttributes/DeviceCreateDate/DeviceKey/DeviceLastAuthenticatedDate/DeviceLastModifiedDate, no DeviceStatus member; the awsAwsjson11_deserializeDocumentDeviceType default case (deserializers.go) discards unrecognized keys, confirming the extra field is additive-only, not a wire break. Re-derive by diffing that struct against whatever cognitoidentityprovider version go.mod pins next -- do not assume this verdict survives an SDK bump unchecked."}
  webauthn: {status: ok, note: "op-by-op re-walk THIS PASS found and fixed two real bugs. (1) The response wire key was wrong: this backend emitted \"FriendlyName\", but the real WebAuthnCredentialDescription's JSON key (confirmed in deserializers.go) is \"FriendlyCredentialName\" -- meaning no real aws-sdk-go-v2 client could ever read this field back; a classic wrong-shape bug parity-principles.md warns about, caught only by reading the actual struct/deserializer, not the handler's own output. (2) AuthenticatorTransports, a REQUIRED field on WebAuthnCredentialDescription, was entirely absent; it is honestly derivable from the client-submitted Credential blob's response.transports (a real WebAuthn PublicKeyCredential.toJSON() field), which was already being accepted but never read (class a) -- now extracted and threaded through CompleteWebAuthnRegistration/ListWebAuthnCredentials. UPDATE 2026-08-21 (gopherstack-r80d batch 19): required Credentials array was still tagged omitempty on ListWebAuthnCredentialsOutput, dropping the key for a user with zero registered credentials despite the handler already building a non-nil empty slice -- fixed, see Notes below."}
  managed_login_branding: {status: ok, note: "op-by-op re-walk THIS PASS found the largest gap in this sweep: Settings (the branding style JSON), Assets (the array of logo/background image files), and UseCognitoProvidedValues -- literally the entire payload of the 'managed login branding' feature -- were accepted by no input field at all on Create/Update and never echoed on any read (class a, not a minor omission). Fixed: stored as the raw client-supplied documents, un-transformed, the same pattern UserPool.LambdaConfig already uses for its own arbitrary-shaped config, since Settings is an AWS Document type (arbitrary JSON) this backend has no reason to model field-by-field. Also fixed CreationDate/LastModifiedDate (CreatedAt/LastModifiedAt were already tracked internally but never echoed -- class b, bounded)."}
  risk_config: {status: ok, note: "op-by-op re-walk THIS PASS: the live path (SetRiskConfigurationFull/DescribeRiskConfigurationFull, wired via securityConfigOpsB overriding securityConfigOpsA -- same domainsOpsA/B shadowing pattern as domains.go) is a real, fully typed implementation already field-diffed clean against RiskConfigurationType/AccountTakeoverRiskConfigurationType/CompromisedCredentialsRiskConfigurationType/RiskExceptionConfigurationType in a prior pass. Confirmed the securityConfigOpsA SetRiskConfiguration/DescribeRiskConfiguration handlers that hardcode nil are DEAD code (shadowed, never dispatched), not a live bug -- verified by reading handler.go's maps.Copy ordering, not assumed. CLOSED 2026-08-29 (bd gopherstack-6flj/21my continuation): RiskConfigurationType.LastModifiedDate is now tracked -- see the gaps entry above for detail. UPDATE 2026-08-21 (gopherstack-r80d batch 19): NotifyConfigurationType.SourceArn (required whenever NotifyConfiguration is present) was tagged omitempty and dropped when a real client sent an explicit empty-string SourceArn (the real SDK's client-side validator only null-checks the pointer, not its content) -- fixed. AccountTakeoverActionType.Notify's omitempty (drops a real `false`) also fixed but not counted as a bug, since no real client round trip can distinguish an omitted key from an explicit false. AccountTakeoverRiskConfigurationType.Actions/CompromisedCredentialsRiskConfigurationType.Actions confirmed structurally unreachable-empty: the real SDK's own client-side validators reject a nil Actions before the request is ever sent. See Notes below."}
  domains: {status: ok, note: "CreateUserPoolDomain/DescribeUserPoolDomain/DeleteUserPoolDomain/UpdateUserPoolDomain — field-diffed DomainDescriptionType against the SDK: DescribeUserPoolDomain was missing CustomDomainConfig entirely (prior pass) — fixed then. FIXED (gopherstack-tq5q): DeleteUserPool now refuses to delete a pool with an attached domain (see DeleteUserPool note) instead of orphaning it, but domain rows orphaned by data that predates that fix would otherwise have no cleanup path at all -- CreateUserPoolDomain would then fail forever against that name. DeleteUserPoolDomain's pool-existence guard is now relaxed for exactly that case: a domain whose recorded UserPoolID no longer resolves to a live pool can still be deleted when the caller supplies that same (now-nonexistent) UserPoolID, so the name can be reused; an arbitrary nonexistent pool ID cannot claim someone else's orphaned domain (guard still checks d.UserPoolID == the requested userPoolID when the pool itself doesn't exist). THIS PASS (gopherstack-n7gh follow-up): AWSAccountId/ManagedLoginVersion/S3Bucket now populated. AWSAccountId echoes the backend's own accountID (same source ARN-building already uses, e.g. arn.Build calls in user_pools.go) rather than pkgs/awsmeta, since nothing in this service's dispatch path ever calls awsmeta.Set -- reading awsmeta.Account(ctx) here would have always silently resolved to its hardcoded default, not real per-backend state. ManagedLoginVersion is a real request field on CreateUserPoolDomain/UpdateUserPoolDomainInput AND a real response field (verified in both api_op_*.go files) that was accepted by neither our create nor update input struct at all (class a) -- fixed, defaults to 1 (hosted UI classic) when unset at creation, an explicit undocumented-default assumption (AWS doesn't state the default in godoc), left unchanged on update when omitted. S3Bucket is fabricated the same way CloudFrontDistribution already was (an AWS-internal bucket name, informational-only, not independently verifiable by any client). Routing/Version (also real DomainDescriptionType fields) remain unpopulated -- multi-region domain routing and app-version reporting this backend has no model for; tracked as items_still_open, not silently dropped."}
  terms: {status: ok, note: "CLOSED 2026-08-08 (gopherstack-kxow): full redesign around the real wire model, field-diffed against api_op_CreateTerms.go/api_op_DeleteTerms.go/api_op_DescribeTerms.go/api_op_ListTerms.go/api_op_UpdateTerms.go and types.TermsType/TermsDescriptionType/TermsEnforcementType/TermsSourceType -- the complete op family the SDK defines; no GetTerms exists (confirmed by directory listing, not assumed). CreateTerms now requires ClientId/Enforcement/TermsName/TermsSource/UserPoolId and accepts Links (map[string]string); Enforcement/TermsSource are validated against their real single-value enums (NONE/LINK, 'reserved for future use' per the SDK godoc). Storage rescoped: terms is now a store.Table[Terms] keyed by a server-generated TermsID (uuid, matching AWS's opaque TermsId) with a byPool secondary index for ListTerms, replacing the old table keyed directly by UserPoolID (which could hold only one bare {UserPoolID,Text} record per pool, structurally incompatible with the real multi-document-per-client model). CreateTerms validates ClientId belongs to UserPoolId (ResourceNotFoundException) and rejects a duplicate ClientId+TermsName pair (TermsExistsException, a real error code on CreateTerms/UpdateTerms per deserializers.go). Describe/Update/Delete take TermsId+UserPoolId and 404 if TermsId doesn't belong to that pool. ListTerms now paginates for real (pkgs/page, MaxResults/NextToken) where the old op ignored both. List output uses TermsDescriptionType (TermsId/TermsName/Enforcement/CreationDate/LastModifiedDate only -- no ClientId/Links/TermsSource/UserPoolId, confirmed by reading the full struct, not assumed from TermsType). cognitoidpSnapshotVersion deliberately NOT bumped despite the Terms DTO shape/key change: Restore discards the ENTIRE snapshot on a version mismatch (persistence.go), so bumping would lose every pool/user/password-hash/MFA-setting on upgrade to protect one table that cannot hold real pre-redesign data anyway (CreateTerms was unreachable by any real SDK client before this fix). Restore instead handles terms separately via restoreTermsLocked: it decodes defensively and drops any row that doesn't carry a real TermsID (a v1 pre-redesign {UserPoolID,Text} row decodes with TermsID empty and is filtered out), while every other table restores normally. Covered by TestInMemoryBackend_RestoreDropsPreRedesignTerms (splices an old-shape terms payload into an otherwise-real snapshot and asserts pools/users survive while terms comes back empty). New tests in terms_test.go drive real required-field JSON through the handler (the exact thing the old bug hid behind) and were verified to fail against the pre-fix code in a worktree before the fix landed. UPDATE 2026-08-21 (gopherstack-r80d batch 19): required TermsType.Links was still tagged omitempty and dropped whenever Links was omitted on Create (a real, reachable state) -- fixed, see Notes below."}
  log_delivery: {status: ok, note: "op-by-op re-walk THIS PASS found SetLogDeliveryConfiguration was a disguised stub (parity-principles.md rule 4): handleSetLogDeliveryConfiguration called Backend.SetLogDeliveryConfiguration(in.UserPoolID, nil) UNCONDITIONALLY -- the client's LogConfigurations payload (a required field) was never read at all, and the input struct didn't even declare it, so Set was a no-op regardless of what was sent, and Get always echoed back whatever Set never stored. Fixed: LogConfigurations is now accepted (stored/echoed as the raw client-supplied array, same un-transformed-map pattern as LambdaConfig/managed_login_branding's Settings, given the nested CloudWatchLogsConfigurationType/FirehoseConfigurationType/S3ConfigurationType/EventSourceName/LogLevel enum tree) and wrapped in the real LogDeliveryConfigurationType shape ({UserPoolId, LogConfigurations})."}
  identity_providers: {status: ok, note: "FULL field diff THIS PASS (not just spot-checked): identityProviderJSON/identityProviderSummaryJSON (the live 'Full'/accurate wire path, wired the same domainsOpsA/B-shadowing way as domains.go) match types.IdentityProviderType and types.ProviderDescription field-for-field -- AttributeMapping/CreationDate/IdpIdentifiers/LastModifiedDate/ProviderDetails/ProviderName/ProviderType/UserPoolId all present with correct field names and epoch-seconds timestamps. No gaps found in item shape. CORRECTION (2026-08-30, cursor sweep): 'no gaps found' above was itself a false-clean -- it was a field diff of item shape, not pagination. listIdentityProvidersFullOutput already declared NextToken (unlike resource_servers/user_import_jobs above, which didn't even declare the field) but handleListIdentityProvidersFull never populated it, silently returning every provider on one page regardless of MaxResults. FIXED via pkgs/page.New. Proven via TestListIdentityProviders_Pagination + hand-revert."}
  resource_servers: {status: ok, note: "FULL field diff THIS PASS: resourceServerAccurateType matches types.ResourceServerType exactly (Identifier/Name/Scopes/UserPoolId, no timestamp fields on the real type either). No gaps found in item shape -- but see cursor-sweep fix below for what a field diff of item shape alone misses. UPDATE 2026-08-21 (gopherstack-r80d batch 19): ResourceServerScopeType.ScopeName/.ScopeDescription (both required *string per scope) were tagged omitempty and dropped when a real client sent an explicit empty-string value (the real SDK's client-side validator only null-checks the pointer, not its content) -- fixed. See Notes below. FIXED (2026-08-30, cursor sweep): the deferred pagination gap noted below is closed -- listResourceServersAccurateInput/Output (the handler that actually wins registration, resourceServersOpsB over resourceServersOpsA) now declare NextToken and handleListResourceServersAccurate pages via pkgs/page.New. Proven via TestListResourceServers_Pagination + hand-revert."}
  user_pool_replicas: {status: ok, note: "parity-4, new family (multi-Region replication / MRR): CreateUserPoolReplica/ListUserPoolReplicas/UpdateUserPoolReplica/DeleteUserPoolReplica. UserPoolReplicaType field-diffed against the SDK (RegionName/Role/Status/UserPoolArn); the X-Amz-Target names and CreateUserPoolReplicaOutput/DeleteUserPoolReplicaOutput/UpdateUserPoolReplicaOutput/ListUserPoolReplicasOutput field names (all singular 'UserPoolReplica' except the List op's plural 'UserPoolReplicas') were confirmed against deserializers.go, not assumed from the (looser) dev-guide prose, which shows a JSON example using a 'Replica' key that does NOT match the real wire field -- a live trap for a future auditor who trusts the docs example over the SDK. CreateUserPoolReplica validates the pool exists (ResourceNotFoundException) and rejects a replica Region equal to the primary pool's own Region (InvalidParameterException) -- both real, documented AWS behaviors. It also enforces the real documented constraint 'You can have at most one secondary replica in an additional Region per user directory' by rejecting a second CreateUserPoolReplica call for the same pool regardless of region (InvalidParameterException) -- this is NOT an invented restriction, it is quoted verbatim from the Cognito multi-Region-replication developer guide. New replicas start Status=INACTIVE per that same guide ('New secondary user pools start in the INACTIVE state'); note the guide's own JSON example elsewhere shows an initial 'PENDING_CREATE' status that is not even a member of the SDK's ReplicaStatusType enum (CREATING/ACTIVE/INACTIVE/DELETING) -- INACTIVE was chosen as the only real, both-documented-and-enum-valid option; this is a explicit, documented assumption, not a fabrication, but flagged for the next auditor to re-verify against a live pool if ever possible. DeleteUserPoolReplica returns the replica with Status transitioned to DELETING (mirroring AWS's documented async deletion) before removing it. UserPoolTags on Create are stored under the replica's own ARN via the existing resourceTags/ListTagsForResource mechanism (real state, not dropped). Persisted via a new userPoolReplicas store.Table (composite poolID:region key, byPool index), round-tripped through Snapshot/Restore, covered by TestInMemoryBackend_SnapshotRestore's full_state_round_trip case."}
  provisioned_limits: {status: ok, note: "parity-4, new family: GetProvisionedLimit/UpdateProvisionedLimit. Confirmed ACCOUNT-LEVEL (not per-user-pool) by fetching the live Cognito quotas developer guide this pass: 'Provisioned limits are account-level resources. They apply to the aggregate rate of all requests from all user pools in one AWS Region in your AWS account' -- this backend models exactly one account+Region so GetProvisionedLimit/UpdateProvisionedLimit take no UserPoolId and do no pool-existence check, which is correct, not an oversight. LimitDefinitionType/LimitType field-diffed against the SDK (LimitClass/Attributes, FreeLimitValue/ProvisionedLimitValue/LimitDefinition). The 18 API_CATEGORY default (free) RPS values in provisioned_limits.go's category table (UserAuthentication=120, UserCreation=50, UserFederation=25, UserAccountRecovery=30, UserRead=120, UserUpdate=25, UserToken=120, UserResourceRead=50, UserResourceUpdate=25, UserList=30, UserPoolRead=15, UserPoolUpdate=15, UserPoolResourceRead=20, UserPoolResourceUpdate=15, UserPoolClientRead=15, UserPoolClientUpdate=15, ClientAuthentication=150, LimitManagement=1) and their Adjustable:Yes/No flags are the real, live-fetched values from 'Amazon Cognito user pools API operation categories and request rate quotas' -- not invented. UpdateProvisionedLimit rejects non-adjustable categories (InvalidParameterException, matching 'Only adjustable quota categories support provisioning') and rejects a negative RequestedLimitValue. One explicit, documented assumption: AWS's real two-tier model has a Service-Quotas-granted 'account-level max limit' above the provisioned limit, but that ceiling is account-specific (granted by AWS Support) with no universal published number -- this backend models an adjustable category's account-level max as 10x its documented default RPS (accountMaxMultiplier in provisioned_limits.go) and enforces it with ServiceQuotaExceededException, the real exception name AWS uses for this condition. Persisted via a new flat provisionedLimits map[string]int32 (Category -> current value), round-tripped through Snapshot/Restore."}
gaps:
  - "CLOSED 2026-08-22 (gopherstack-xasq): SchemaAttribute wrote StringAttributeMinLength/MaxLength (int64) and NumberAttributeMinValue/MaxValue (float64) as flat top-level fields; the real SchemaAttributeType nests these as string-valued sub-objects (StringAttributeConstraints{MinLength,MaxLength}/NumberAttributeConstraints{MinValue,MaxValue} -- confirmed via awsAwsjson11_deserializeDocumentSchemaAttributeType/...NumberAttributeConstraintsType/...StringAttributeConstraintsType, cognitoidentityprovider@v1.67.4 deserializers.go:25283/24065/25745, and the matching serializers.go:9250/8993/9437 on the request side). Fixed by replacing the four flat fields with two new nested, string-valued struct types (numberAttributeConstraintsJSON, stringAttributeConstraintsJSON). AddCustomAttributesInput.CustomAttributes is the one genuine request-side use of SchemaAttribute (serializers.go:9696-9713) -- confirmed broken pre-fix: constraints a caller set there were silently dropped, since the real client never sends the flat keys this backend read. CreateUserPool/DescribeUserPool responses (userPoolDataAccurate.SchemaAttributes) were the other affected surface. Two corrections to this bug's original scope: UpdateUserPool is NOT affected -- UpdateUserPoolOutput carries no UserPool/schema data at all in the real API (api_op_UpdateUserPool.go), and UpdateUserPoolInput has no Schema field either (schema is immutable after creation in real Cognito, confirmed by its absence from serializers.go's UpdateUserPoolInput serializer). ListUserPools is also not really affected by this specific defect -- UserPoolDescriptionType (its real response element) has no SchemaAttributes field at all (deserializers.go's case list for it: CreationDate/Id/LambdaConfig/LastModifiedDate/Name/ReplicaRegions/Status only), so gopherstack's userPoolData.SchemaAttributes there was already a harmless extra key ignored by any real client regardless of shape, not a min/max-dropping bug; left as-is since an unknown extra key costs nothing and no test depends on removing it. cognitoidpSnapshotVersion bumped 1 -> 2 (structural retype of a persisted field, not additive -- confirmed via pkgs/persistence's TestSnapshotVersionGuard, golden refreshed with -update). Proven via a real-SDK-client round trip (schema_attribute_constraints_test.go) confirmed to fail against the pre-fix flat shape (StringAttributeConstraints/NumberAttributeConstraints decoded nil) and pass after; a pre-existing test (TestInMemoryBackend_RestoreDropsPreRedesignTerms) hardcoded a literal snapshot version=1 and had to be updated to stop hardcoding it, since it happened to equal the then-current version rather than actually testing anything about that number."
  - "CLOSED 2026-08-08 (gopherstack-kxow): terms/ was built on an entirely invented wire model (createTermsInput={UserPoolId}, termsType={DefaultTermsAndConditions}) that no real aws-sdk-go-v2 client could ever reach -- CreateTerms's own client-side validation middleware rejects the request before it is even sent, since ClientId/Enforcement/TermsName/TermsSource are all required and none existed here. Full redesign: real input/output modeled for all 5 real ops (Create/Delete/Describe/List/Update -- there is no GetTerms), storage rescoped to a TermsID-keyed store.Table with a byPool index (was: one record per UserPoolID), TermsEnforcementType/TermsSourceType enums validated, TermsExistsException added, ListTerms pagination implemented for real. See families.terms above for full detail and SDK citations."
  - "CLOSED 2026-08-07 (gopherstack-n7gh, was gopherstack-p8i): USER_SRP_AUTH/ADMIN_USER_SRP_AUTH now implement real SRP-6a (services/cognitoidp/srp.go). The algorithm (3072-bit RFC 5054 N, g=2, k=H(PAD(N)|PAD(g)), x=H(PAD(salt)|H(poolName|username|\":\"|password)), server B=(k*v+g^b) mod N, server S=(A*v^u)^b mod N, HKDF-SHA256 with the \"Caldera Derived Key\" info string truncated to 16 bytes, HMAC-SHA256 password-claim signature over poolName|username|secretBlock|timestamp) was verified field-by-field against amazon-cognito-identity-js's AuthenticationHelper.js/CognitoUser.js/DateHelper.js -- the reference client implementation AWS itself publishes -- not reconstructed from memory. Locked in by an INDEPENDENTLY-written second implementation of the same client-side math in test code (srp_client_test.go, package cognitoidp_test, cannot see srp.go's unexported symbols) that performs a real handshake against the server and must derive the identical signature; see srp_test.go for the round-trip, FORCE_CHANGE_PASSWORD-after-SRP, tampered-signature-rejected, plaintext-InitiateAuth-rejected, and persistence-survival regression tests. Every password-setting call site (SignUp, SignUpWithValidation, ConfirmForgotPassword, ChangePassword, AdminSetUserPassword(Full), AdminCreateUser(WithPolicy/Full), RespondToNewPasswordRequired, UserMigration) now derives and stores a matching SRPSalt/SRPVerifier via the new hashAndSRP helper, and both fields survive Snapshot/Restore (persistence.go)."
  - "CLOSED 2026-08-08 (gopherstack-n7gh follow-up): UserMigration_ForgotPassword trigger source and domain AWSAccountId/ManagedLoginVersion/S3Bucket, the two items explicitly named but not reached in the SRP-6a pass -- see families.ForgotPassword and families.domains above for detail."
  - "CLOSED 2026-08-08 (gopherstack-n7gh follow-up): op-by-op re-walk of user_import_jobs/devices/webauthn/managed_login_branding/risk_config/terms/log_delivery plus a full field diff of identity_providers/resource_servers, the remaining named scope item. Found and fixed 4 real bugs beyond the headline items: webauthn's wrong wire key (FriendlyName vs FriendlyCredentialName) and missing required AuthenticatorTransports; managed_login_branding's Settings/Assets/UseCognitoProvidedValues completely discarded; SetLogDeliveryConfiguration's disguised-nil-stub; CreateUserImportJob's dropped CloudWatchLogsRoleArn/PasswordHashingAlgorithm. See families above for each. terms/ was found to be built on a fictional wire model entirely and needs a full redesign -- explicitly NOT fixed this pass, see deferred below."
deferred:
  - "devices' deviceType.DeviceStatus is an extra field NOT present on the real DeviceType wire shape (verified by reading the complete SDK struct: only DeviceAttributes/DeviceCreateDate/DeviceKey/DeviceLastAuthenticatedDate/DeviceLastModifiedDate exist; device remembered status is write-only in real Cognito, never returned by any Get/List device op). Not removed: several existing tests assert on it and no real client breaks from an extra unknown JSON key, so removing it purely for spec purity would cost test-observable state for no functional gain. Flagged for whoever next touches devices.go so it isn't mistaken for a verified-real field. Evidence: aws-sdk-go-v2/service/cognitoidentityprovider@v1.67.4, types/types.go:677-698, checked 2026-08-13 -- see families.devices above for the full citation including the deserializer default-case confirmation. This entry records a verdict as of that version; re-check the same struct before trusting it against a newer SDK pin."
  - "CLOSED 2026-08-29 (bd gopherstack-6flj/21my continuation): risk_config's RiskConfigurationType.LastModifiedDate is now tracked -- TypedRiskConfiguration gained a LastModifiedAt field, stamped by SetTypedRiskConfiguration on every SetRiskConfiguration call and echoed by both DescribeRiskConfiguration and SetRiskConfigurationOutput via toRiskConfigJSON. See TestSetRiskConfiguration_LastModifiedDatePopulated (wire_field_fixes_test.go) for the real-SDK-client round trip."
  - "NEW (found 2026-08-29, NOT fixed -- out of scope for a bounded wire-field pass): InitiateAuthInput.AuthFlow's real, documented \"USER_AUTH\" value (choice-based authentication -- types/enums.go AuthFlowType, api_op_InitiateAuth.go) is entirely unimplemented. precheckAuthLocked's AuthFlow allow-list (auth.go) only accepts USER_PASSWORD_AUTH/ADMIN_USER_PASSWORD_AUTH/ADMIN_NO_SRP_AUTH/USER_SRP_AUTH/ADMIN_USER_SRP_AUTH/CUSTOM_AUTH; a real SDK client sending AuthFlow=USER_AUTH gets a clean, honest ErrInvalidUserPoolConfig rejection rather than a silent misbehavior (verified by reading precheckAuthLocked directly -- not a wire bug, a missing feature), but no InitiateAuth call using USER_AUTH, PREFERRED_CHALLENGE, SELECT_CHALLENGE, or the AvailableChallenges response member can ever succeed here. Grep confirms zero references to USER_AUTH/AvailableChallenges/SELECT_CHALLENGE/PREFERRED_CHALLENGE anywhere in this package outside the SDK import. This is a structural gap on the scale of the pre-redesign terms/ finding (a whole real, reachable feature missing, not a field-level defect) -- flagged for a dedicated future pass rather than attempted here."
  - "CLOSED 2026-08-30 (cursor sweep): pagination is now implemented on ListUserImportJobs and ListResourceServers (see their own entries above), plus ListUserPoolClients, ListIdentityProviders, and AdminListGroupsForUser, which the same sweep found had the identical gap but were not yet named here."
  - "domains: Routing and Version, two more real DomainDescriptionType fields (multi-region failover routing config; app version string), remain unpopulated -- this backend has no multi-region-domain-routing model and no meaningful 'app version' to report. Left absent rather than fabricated, per the same standard as terms/ above, just far smaller in scope."
  - "MFA_SETUP's ChallengeParameters carries no MFAS_CAN_SETUP value (InitiateAuth doc: 'The MFA types activated for the user pool will be listed in the challenge parameters MFAS_CAN_SETUP value') -- this backend does not populate ChallengeParameters for any non-SRP challenge (SOFTWARE_TOKEN_MFA/SMS_MFA/EMAIL_OTP/NEW_PASSWORD_REQUIRED/MFA_SETUP all return an empty map), a pre-existing gap gopherstack-1b07 (2026-08-22) did not extend to fix. Also undetermined: the SDK's doc prose never states whether AssociateSoftwareToken/VerifySoftwareToken/RespondToAuthChallenge rotate or single-use the MFA_SETUP session between calls, so this backend echoes the same session token unchanged across all three (only RespondToAuthChallenge deletes it) rather than inventing rotation semantics."
leaks: {status: clean, note: "janitor.go sweeps expired refresh tokens/mfa sessions/confirm codes/attr verification codes on a bounded interval (WithJanitor); ctx cancellation observed via StartWorker. This pass added custom_auth.go (CUSTOM_AUTH state machine) and user_migration.go (UserMigration trigger), both of which reuse the existing mfaSessions map/EvictExpiredMFASessions sweep for their session state -- no new maps, goroutines, or tickers introduced. All new backend methods (tryUserMigration, applyPostMigrationFinalStatus, startCustomAuth, customAuthRound, defineAuthChallenge, createAuthChallenge, verifyCustomAuthChallenge, preAuthenticationCheck, postAuthenticationNotify) are plain functions that assume the caller already holds b.mu (documented per-function), never call b.mu.Lock/RLock themselves -- verified no double-lock/deadlock paths and confirmed via `go test -race` (full suite, 233s, clean). De-stub hygiene: the ~15-op handler.go/handler_auth.go/handler_user_pools.go/handler_user_pool_clients.go/handler_users.go dead-code shadowing flagged as deferred in the prior sweep is now fully deleted (dead handlers + their now-orphaned model types removed across 4 files + models_auth.go/models_user_pools.go/models_user_pool_clients.go/models_users.go), closing that item; golangci-lint (0 issues) confirms nothing is newly unused. FIXED (gopherstack-cq0z, 2026-09-06): DeleteUserPool's user cascade deletes users directly (b.users.Delete) instead of calling AdminDeleteUser, so it did not inherit AdminDeleteUser's own devices/authEvents cleanup for each user -- the cascade-variant of the ghost-row class, where a parent delete bypasses the single-resource delete path holding the fix. Now clears devices[userStateKey]/authEvents[userStateKey] per user in the same cascade loop. Pool-level side maps (riskConfigurations, logDeliveryConfigs, poolMfaConfigs, and the pool's own resourceTags entry) are NOT cleared by DeleteUserPool either and remain open findings, not addressed this pass. See TestDeleteUserPool_ClearsUserDeviceState."}
---

## Notes

### 2026-08-30 (dispatch-duplicate sweep: is the winner correct, not just which one wins)

The 2026-08-22 (`gopherstack-zquj`) keycheck pass hand-resolved all 27 ops registered twice in
`dispatchTable()` and field-diffed each winning handler's item *shape* against the SDK. This pass
asked the stricter question that entry itself flagged as narrower than a full audit for the four
`List*` ops: for every one of the 27 pairs, does the *shadowed loser* actually contain a stub
that would silently start serving traffic if a future edit ever swapped its `maps.Copy` call
after the winner's, and is the currently-winning registration provably the one still wired.

Re-derived `dispatchTable()`'s real `maps.Copy` order from `handler.go` directly (did not trust
line-number ordering in any prior note) and read both handlers in every pair. Result: all 27
winners are already correct -- no live bug found, consistent with the 2026-08-22 field-diff.
Three of the losers are the exact stubs a prior survey named ahead of time
(`handleAssociateSoftwareToken`: hardcoded RFC 6238 example secret; `handleGetUserAttributeVerificationCode`:
hardcoded `user@example.com`/`EMAIL` regardless of the real user; `handleDescribeRiskConfiguration`:
calls the backend and discards the result, returning an empty type unconditionally). A fourth,
not previously named, is the same class: `handleVerifyUserAttribute` calls
`Backend.VerifyUserAttribute`, itself a documented no-op ("the mock does not send verification
codes so all attributes are considered already verified. Returns success for any code.") --
already shadowed by the real `VerifyUserAttributeWithCode` path (`attributesOpsC`, later in the
`maps.Copy` chain), so not reachable, but worth naming since nothing had verified *why* the
1b07/zquj passes' "fixed for hygiene" note didn't mean "deleted" -- it didn't; the dead handler
bodies were still present and un-audited on this question going into this pass.

The four `List*` ops closed by the 2026-08-30 cursor-population sweep and the 2026-08-29
pagination-arithmetic sweep (`ListGroups`, `ListUsersInGroup`, `ListIdentityProviders`,
`ListResourceServers`) were re-checked on this pass's question too: all four winners
(`handleListGroupsFull`, `handleListUsersInGroupFull`, `handleListIdentityProvidersFull`,
`handleListResourceServersAccurate`) are the ones actually wired, confirmed by `maps.Copy` order,
not just by pagination behavior.

Deleted all 27 shadowed loser handlers (dead code, unreachable via any real client, confirmed by
`grep` for direct test references before removal) and their now-orphaned wire-only input/output
types, across `handler_mfa.go`, `handler_groups.go`, `handler_identity_providers.go`,
`handler_resource_servers.go`, `handler_domains.go`, `handler_security_config.go`,
`handler_branding.go`, `handler_attributes.go` and their `models_*.go` siblings.
`resourceServersOpsA()` and `attributesOpsB()` are now-empty and were deleted along with their
`maps.Copy` call in `dispatchTable()` (the other 25 pairs' surviving groups still register at
least one non-duplicate op, so their `*OpsA/B` functions and `maps.Copy` calls stay). Backend
methods the deleted handlers called into (`InMemoryBackend.VerifyUserAttribute`,
`SetRiskConfiguration`/`DescribeRiskConfiguration` raw-map variants, `GetUICustomization`/
`SetUICustomization`) were left alone: they're exported, still exercised directly by
`persistence_test.go`/`attributes_management_test.go`, and `SetRiskConfiguration`/
`DescribeRiskConfiguration`'s backing map is still read/written by snapshot persistence --
deleting them was out of this pass's scope (dispatch-table duplicates only, not backend cleanup).

Added `TestVerifySoftwareToken_WrongCode_Rejected` (`mfa_test.go`, drives the real typed SDK
client) -- the only one of the 27 pairs without an existing test that would fail if the shadowed
`handleVerifySoftwareToken` stub (unconditional `Status: "SUCCESS"`) ever won the dispatch race.
Strengthened `TestIdentityProvider_GetByIdentifier` to assert `AttributeMapping`/`IdpIdentifiers`/
`CreationDate` (fields the shadowed non-Full `handleGetIdentityProviderByIdentifier` never
populated) so it also pins wiring, not just success. Every other pair already had an existing
test that would fail against its shadowed loser (verified by reading each test's assertions
against what the loser actually returns, not by re-deriving from scratch) -- see the bd issue for
the full per-pair table.

`go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` all clean for
`./services/cognitoidp/...` after the deletions (0 lint issues; the only findings during this
pass were `goimports` trailing-blank-line diffs in the three files where a whole trailing type
block was removed, fixed by `gofmt -w` on those three files only).

### 2026-08-29 (error-path sweep: what a typed client sees on failure)

Extracted all 129 `awsAwsjson11_deserializeOpError<Op>` switches from
cognitoidentityprovider@v1.67.4's deserializers.go and cross-referenced every backend call site
that raises a sentinel error against its own op's modeled set. `resolveErrorType`'s shared
`cognitoSentinelErrors` table was correct; every bug was the sentinel chosen at a call site.

**Method note — this service has a real trap for static analysis.** Many ops have two
registrations under the same wire action name: an older stub/simple handler (e.g.
`handleCreateResourceServer`, a pure echo with no backend call at all) registered in an early
`*OpsA()` group, and a later `*OpsB()`/`*OpsC()` "Accurate" handler that calls the real backend
method, registered later in `dispatchTable()`'s `maps.Copy` sequence and silently winning
(`registerStubOpsIfAbsent`-style precedent, parity-principles.md item 2). A first analysis pass
that doesn't resolve `maps.Copy` order — or that misses `opXxx` constant map keys (vs quoted
string literals) — traces the *dead* stub instead of the live handler. This produced two false
starts this pass: `CreateIdentityProvider`'s live handler (`CreateIdentityProviderFull`) already
correctly used `ErrDuplicateProvider`, and `VerifyUserAttribute`'s live handler
(`VerifyUserAttributeWithCode`) already correctly used `ErrInvalidParameter` — both were flagged
by an initial naive trace of the dead `CreateIdentityProvider`/`VerifyUserAttribute` methods,
which I also fixed for hygiene (harmless — unreachable via any real client) but which were never
the actual bug. Re-derived the dispatch table by simulating `maps.Copy` in
`dispatchTable()`'s real order before trusting any cross-reference.

Confirmed bugs fixed (real `aws-sdk-go-v2/service/cognitoidentityprovider` client,
`errors.As` against the SDK's own typed exception, in `error_path_sweep_test.go`):

- **Fabricated code, `CreateUserPool`**: rejected a duplicate pool name with wire code
  `UserPoolAlreadyExistsException` — not a real AWS Cognito error (absent from
  `types/errors.go` entirely) and not even correct behavior: AWS Cognito does not enforce
  unique pool names (`CreateUserPool`'s own deserializer models no "already exists" exception at
  all). Removed the duplicate-name rejection entirely (a second pool with the same name now
  succeeds with a distinct ID) and deleted the now-dead `poolNameExists` helper. An existing
  test (`user_pools_test.go`) and an HTTP-level test (`user_pools_config_test.go`) both asserted
  the fabricated-reject behavior as correct and were corrected.
- **Fabricated code, `AdminCreateUser`**: rejected a duplicate username with wire code
  `UserAlreadyExistsException` — also absent from the entire SDK. `AdminCreateUser`'s own
  deserializer models `UsernameExistsException` (already used correctly by `SignUp`). Repointed
  all three call sites (including two dead legacy methods, for hygiene) to the existing
  `ErrUsernameExists` sentinel; fixed one existing test asserting the fabricated code.
- **Wrong code, `AdminGetDevice`/`AdminListDevices`**: raised `UserNotFoundException` for a
  missing user, but both ops' own deserializers model `ResourceNotFoundException` — unlike
  `AdminGetUser` and similar ops, which do model `UserNotFoundException`. Repointed to
  `ErrDeviceNotFound` (same wire code, already correct for the sibling device-not-found check
  two lines below).
- **Wrong code, `AddCustomAttributes`/`SetUserMFAPreference`/`AdminSetUserMFAPreference`**: all
  three raised `InvalidUserPoolConfigurationException` for a semantic validation failure (bad
  custom-attribute name; preferred MFA not in the enabled list), but none of the three ops model
  that code — only `InvalidParameterException`, which they all model. `InvalidUserPoolConfigurationException`
  is genuinely correct elsewhere in this file (`InitiateAuth`/`AdminInitiateAuth`, which do model
  it, for unsupported/misconfigured auth flows) — confirmed each call site against its own
  op's deserializer rather than assuming the sentinel was wrong everywhere.
- **Wrong code, `CreateUserPoolDomain`**: raised `GroupExistsException` (`CreateGroup`'s own
  sentinel, `ErrAlreadyExists`) for a duplicate domain; the op has no dedicated "already exists"
  exception, so repointed to `ErrInvalidParameter` (which it does model, and which real Cognito
  domains-must-be-globally-unique behavior plausibly maps to as a bad-value rejection).
- **Wrong code, `RevokeToken`**: raised `NotAuthorizedException` for a token issued to a
  different client, but its own deserializer models `UnauthorizedException` — a distinct,
  newer type ("the request isn't authorized... invalid access token") — not the generic
  `NotAuthorizedException` most other ops use. Added `ErrTokenUnauthorized`.
- **Wrong code, `AssociateSoftwareToken`**: raised `UserNotFoundException` when a session's
  bound user no longer exists (deleted after the session was issued); its deserializer doesn't
  model that, only `NotAuthorizedException` (consistent with the surrounding stale-session
  checks in the same function). Note: `VerifySoftwareToken` shares this exact code path and
  *does* model `UserNotFoundException` — but also models `NotAuthorizedException`, so this is a
  correct choice for both, just less specific than ideal for `VerifySoftwareToken`. Not covered
  by a new integration test (constructing a stale-session/deleted-user state requires internal
  fixture manipulation disproportionate to this one-line fix); verified by code inspection
  against both ops' deserializers.

**Left, not fixed**: `CreateResourceServer` raises `GroupExistsException` for a duplicate
(userPoolID, identifier) pair; the op's deserializer models no "already exists" exception at
all, but unlike the ssm Delete-idempotent findings in the same campaign pass, there's no doc
comment or established sibling convention indicating whether real AWS upserts, silently ignores,
or does something else entirely for this case — left rather than guessed, per this campaign's
restraint principle.

**Also observed, not part of this bug class**: `handler_mfa.go`'s `mfaOpsB()` registers a
`wrapAccuracy(h.handleAdminSetUserMFASetting)` handler under the dispatch key
`"AdminSetUserMFASetting"` — not a real AWS Cognito action name (the real op is
`AdminSetUserMFAPreference`, already correctly registered via `opAdminSetUserMFAPreference` in
the same map). This extra entry is dead code — no real client can ever send that action name —
left as-is (harmless, out of this pass's error-class scope).

### What this pass fixed (2026-08-22, gopherstack-1b07)

Closed the structural gap gopherstack-zquj filed as gopherstack-1b07 (see
the now-updated `deferred`/`ops` entries above): `AssociateSoftwareToken`
and `VerifySoftwareToken` document `Session` as an alternate to
`AccessToken` for the real MFA_SETUP continuation flow -- confirmed by
reading each op's actual input struct and doc prose in
`cognitoidentityprovider@v1.67.4`:

- `api_op_AssociateSoftwareToken.go`: `AccessToken *string` ("You can
  provide either an access token or a session ID in the request") and
  `Session *string` ("the session ID from a successful sign-in"), both
  optional on the input; output carries `Session *string` too.
- `api_op_VerifySoftwareToken.go`: same `AccessToken`/`Session` pair, doc
  states plainly "The request takes an access token or a session string,
  but not both." Output's `Session` doc: "This session ID satisfies an
  MFA_SETUP challenge. Supply the session ID in your challenge response."
- `api_op_InitiateAuth.go` (`ChallengeName` doc, `types.ChallengeNameType`
  enum value `MFA_SETUP` in `types/enums.go:267`): "For users who are
  required to setup an MFA factor before they can sign in ... To set up
  time-based one-time password (TOTP) MFA, use the session returned in
  this challenge from InitiateAuth or AdminInitiateAuth as an input to
  AssociateSoftwareToken. Then, use the session returned by
  VerifySoftwareToken as an input to RespondToAuthChallenge or
  AdminRespondToAuthChallenge with challenge name MFA_SETUP to complete
  sign-in." This is the full round trip the fix had to make reachable, not
  just the two ops the issue named.

**One correction to the filing this issue came from**: it (and the
gopherstack-zquj note before it) also named `SetUserMFAPreference` as
having this same documented `Session` alternate. It does not --
`SetUserMFAPreferenceInput` in `api_op_SetUserMFAPreference.go` has
`AccessToken *string` marked `This member is required` and no `Session`
field at all. gopherstack's own `setUserMFAPreferenceAccurateInput`
(models_mfa.go) already had no `Session` field either, so there was
nothing broken there to fix -- confirmed by reading the full struct, not
assumed from the pattern of the other two ops.

**What was missing end to end, not just the two named ops**: before this
fix, nothing in `InitiateAuth`/`AdminInitiateAuth` ever issued a session
usable for this flow at all. `mfaChallengeType` (mfa.go) unconditionally
defaulted any MFA-required user with no `PreferredMfaSetting` to
`SOFTWARE_TOKEN_MFA` -- including a user who had never called
`AssociateSoftwareToken`, for whom that challenge is an unconditional dead
end (`RespondToMFAChallenge` requires a TOTP secret that was never
associated). So even AccessToken-based `AssociateSoftwareToken` could not
have rescued that user: the session that was issued named the wrong
challenge, and no session at all was of type `MFA_SETUP`.

Fixed by:

1. `mfaChallengeType` now returns `MFA_SETUP` for a user with no
   `PreferredMfaSetting` who has not verified a software token
   (`user.TOTPVerified == false`), and only falls back to
   `SOFTWARE_TOKEN_MFA` once a token has actually been verified -- this
   preserves the existing regression test
   (`TestHandler_RespondToAuthChallenge_SMSMFAFlow`, which enrolls TOTP via
   AccessToken *before* turning MFA on, so `TOTPVerified` is already true
   by the time `InitiateAuth` runs and the SOFTWARE_TOKEN_MFA challenge is
   still correctly issued for that case).
2. `resolveMFASetupSubjectLocked` (mfa.go): resolves the acting user from
   either `accessToken` or `session`, matching VerifySoftwareToken's "either
   ... or ... but not both" contract. The session path requires the
   `mfaSessionEntry.ChallengeType` to be `MFA_SETUP` (not any other pending
   challenge type) and not expired, exactly like the existing
   `RespondToMFAChallenge`/`RespondToSRPChallenge`/
   `RespondToNewPasswordRequired` session lookups it sits alongside.
3. `AssociateSoftwareToken`/`VerifySoftwareToken` signatures both grew a
   `session` parameter (and `VerifySoftwareToken` grew a `string` first
   return, `AssociateSoftwareToken` a second `string` return) to thread the
   resolved/echoed session through to the wire's already-correctly-tagged
   `Session` output field. All in-package call sites (handler_mfa.go,
   mfa_test.go, totp_test.go, persistence_test.go, users_test.go) updated;
   confirmed via `go build ./...` (no callers outside `services/cognitoidp`
   exist for either method).
4. `RespondToMFASetupChallenge` (mfa.go), a new backend method, completes
   the flow: it requires the session to be `MFA_SETUP` and its user to have
   `TOTPVerified == true` (set by step 3's `VerifySoftwareToken`), records
   `SOFTWARE_TOKEN_MFA` as the user's MFA preference via the existing
   `applyMFAPreferenceLocked`, deletes the session, and issues tokens.
   Wired into `RespondToAuthChallenge`/`AdminRespondToAuthChallenge` for
   `ChallengeName == "MFA_SETUP"`.

**Not modeled, disclosed rather than invented**: real Cognito's exact
session single-use/rotation semantics across the
`AssociateSoftwareToken -> VerifySoftwareToken -> RespondToAuthChallenge`
round trip are not stated anywhere in the pinned SDK's doc prose (only that
each op's output carries *a* session to use next). This backend echoes the
same session token unchanged across all three calls -- only the final
`RespondToAuthChallenge` call deletes it from `b.mfaSessions` -- rather
than fabricating rotation behavior the SDK doesn't document. Also not
modeled: the `MFAS_CAN_SETUP` value `InitiateAuth`'s doc says accompanies
an `MFA_SETUP` challenge in `ChallengeParameters` -- this backend does not
populate `ChallengeParameters` for any non-SRP challenge at all (a
pre-existing gap predating this fix, not extended here). Both are recorded
under `deferred` above.

**Proof**: `mfa_setup_session_test.go` adds
`TestMFASetupSession_CompletesSignInWithoutAccessToken`, which drives the
full documented round trip through a real `aws-sdk-go-v2` client
(`InitiateAuth` -> `AssociateSoftwareToken(Session)` ->
`VerifySoftwareToken(Session)` -> `RespondToAuthChallenge(MFA_SETUP)`) with
no `AccessToken` anywhere, and
`TestAssociateAndVerifySoftwareToken_AccessTokenPath`, a same-shape
regression guard proving the pre-existing `AccessToken` path is unchanged.
Hand-reverted `mfa.go`/`handler_mfa.go`/`handler_auth.go` (plus the
pre-existing test files' call sites, needed only so the package still
compiled against the old 2-arg/1-return signatures) to `git show HEAD:...`
and re-ran: `TestMFASetupSession_CompletesSignInWithoutAccessToken` failed
at the very first real assertion --

```
mfa_setup_session_test.go:74:
    Error: Not equal:
      expected: "MFA_SETUP"
      actual  : "SOFTWARE_TOKEN_MFA"
```

-- confirming the pre-fix backend never issues an `MFA_SETUP` challenge at
all (the deeper of the two bugs this fix closes), while
`TestAssociateAndVerifySoftwareToken_AccessTokenPath` still passed
unfixed, confirming the AccessToken path was never broken. Restored all 7
files from the saved fixed copies and confirmed `md5sum` byte-identical
before continuing.

Snapshot version: left at 2 (`cognitoidpSnapshotVersion`, persistence.go).
This fix adds no new persisted field and changes no existing one --
`mfaSessionEntry.ChallengeType` is an existing `string` field that simply
takes on one more value (`"MFA_SETUP"`) it already had the type to hold,
and `User.TOTPVerified`/`PreferredMfaSetting` (both pre-existing) are read,
not restructured. Confirmed no `pkgs/persistence` golden regen was
needed: `go test ./pkgs/persistence/...` passes unchanged.

### What this pass fixed (2026-08-22, gopherstack-xasq)

Fixed the structural gap gopherstack-zquj filed (see below): `SchemaAttribute`
wrote `StringAttributeMinLength`/`StringAttributeMaxLength` (int64) and
`NumberAttributeMinValue`/`NumberAttributeMaxValue` (float64) as flat
top-level fields. The real `SchemaAttributeType` nests these one level
deeper as string-valued sub-objects, confirmed against
`cognitoidentityprovider@v1.67.4`:

- `awsAwsjson11_deserializeDocumentSchemaAttributeType` (deserializers.go:25283)
  switches on `"NumberAttributeConstraints"`/`"StringAttributeConstraints"`,
  not flat keys.
- `awsAwsjson11_deserializeDocumentNumberAttributeConstraintsType`
  (deserializers.go:24065) and `...StringAttributeConstraintsType`
  (deserializers.go:25745) both decode `MinValue`/`MaxValue` and
  `MinLength`/`MaxLength` as `*string`, not a number -- a wire-type
  mismatch, not just a wrong key: a real client would reject the old
  flattened int64/float64 values outright, a harder failure than a missing
  field.
- The request-side serializers (serializers.go:9250, 8993, 9437) mirror the
  same nested, string-valued shape, confirming both directions.

Replaced the four flat fields with two new nested types
(`numberAttributeConstraintsJSON`, `stringAttributeConstraintsJSON`), both
string-valued, referenced via `*T` pointers so an attribute with no
constraints omits the whole sub-object (matching the real serializer's
`if v.NumberAttributeConstraints != nil` gate).

**Request side was genuinely broken, not just the response.**
`AddCustomAttributesInput.CustomAttributes` is the one place `SchemaAttribute`
is decoded from a client request (serializers.go:9696 confirms the real
client sends `CustomAttributes` as a list of the same nested
`SchemaAttributeType`). Before this fix, any constraints a caller set via
`AddCustomAttributes` were silently dropped: the real client never sends the
flattened top-level keys this backend read, so `StringAttributeConstraints`/
`NumberAttributeConstraints` always decoded as the Go zero value. Confirmed
neither direction was a no-op stub returning success while doing nothing
(the attribute itself, and every other field, bound correctly) -- only the
constraints sub-object was affected.

**Two corrections to the original bug report's scope**, both verified against
the SDK rather than assumed:

- `UpdateUserPool` is not actually affected. `UpdateUserPoolOutput` carries no
  `UserPool`/schema data in the real API at all (api_op_UpdateUserPool.go),
  and `UpdateUserPoolInput` has no `Schema` field either -- schema is
  immutable after pool creation in real Cognito (confirmed by its absence
  from serializers.go's `UpdateUserPoolInput` serializer, unlike
  `CreateUserPoolInput` which does serialize `Schema`).
- `ListUserPools` is not really affected by this defect either.
  `UserPoolDescriptionType` (its real response element) has no
  `SchemaAttributes` field at all -- deserializers.go's full case list for it
  is `CreationDate`/`Id`/`LambdaConfig`/`LastModifiedDate`/`Name`/
  `ReplicaRegions`/`Status`. gopherstack's `userPoolData.SchemaAttributes` on
  `ListUserPools` was already a harmless extra key any real client ignores
  regardless of shape, not a min/max-dropping bug. Left in place (an unknown
  extra JSON key costs nothing and no test depends on removing it); worth a
  follow-up cleanup issue, not fixed here.

`CreateUserPool`'s request also never bound `Schema` at all in this
backend -- `createUserPoolWithOptsInput` has no `Schema` field, so an initial
schema on pool creation is silently dropped regardless of this fix. That is
a missing-field gap, not a flattening defect (there is no `SchemaAttribute`
value to be mis-shaped in the first place), so it is out of this bug's
scope; custom attributes can only be added post-creation via
`AddCustomAttributes` today.

**Snapshot version bumped 1 -> 2.** `SchemaAttribute` is used unchanged as
the persisted DTO for `UserPool.CustomAttributes`
(`userPoolSnapshot.CustomAttributes`), so this is a structural retype of a
persisted field (fields removed, not purely added) -- confirmed by
`pkgs/persistence`'s `TestSnapshotVersionGuard`, which fails the bump
otherwise. Golden refreshed via `-update`; the diff touched only
`cognitoidp`'s entry (field list + version), confirmed via `git diff`
before accepting it.

**Proof**: `schema_attribute_constraints_test.go` drives `AddCustomAttributes`
then `DescribeUserPool` through a real `aws-sdk-go-v2` client and asserts
`StringAttributeConstraints`/`NumberAttributeConstraints` decode non-nil with
the correct values. Hand-reverted `models_attributes.go` to the pre-fix
flat shape (`cp` to scratchpad, restored via `git show HEAD:...`) and
confirmed the test fails there (`StringAttributeConstraints decoded nil`),
then restored the fix and confirmed the file was byte-for-byte identical to
the pre-revert version (`md5sum`).

**One pre-existing test needed updating, not because it defended the wrong
shape, but as collateral from the version bump**:
`TestInMemoryBackend_RestoreDropsPreRedesignTerms` hardcoded
`raw["version"] = float64(1)` to simulate a legacy snapshot -- that literal
happened to equal `cognitoidpSnapshotVersion` at the time the test was
written, not because the test cared about the number "1" specifically. With
the bump to 2, the hardcoded "1" made `Restore` treat the whole fixture as
an incompatible version and discard everything (not just terms), failing the
test. Fixed by no longer overwriting `raw["version"]` at all -- it already
holds whatever version `b.Snapshot` just stamped, which is what the test
actually needs. No test in the repo asserted on the old flattened keys
(`StringAttributeMinLength` et al. never appeared in any test body), so no
test was found ratifying the defect itself.

Gates run: `go build ./...`, `go vet ./...`, `gofmt -l` (clean),
`go test -race ./services/cognitoidp/...` (113s, clean),
`go test ./pkgs/persistence/...` (clean), `golangci-lint run` on
`services/cognitoidp/...` and `pkgs/persistence/...` (0 issues, including a
`fieldalignment` finding on the reordered `SchemaAttribute` struct, fixed by
hand rather than running `fieldalignment -fix`/`golangci-lint --fix`
package-wide), `make build-check` (clean). No `//nolint` added.

### What this pass fixed (2026-08-22, gopherstack-zquj: keycheck ambiguous-binding sweep)

Re-ran `cmd/keycheck` (awsAwsjson11_, cognitoidentityprovider@v1.67.4 --
confirmed via `dirModuleOverride["cognitoidp"] = "cognitoidentityprovider"`
in `cmd/structfielddiff/main.go:23` and `go.mod:27`; NOT `cognitoidentity`,
a separate, already-settled sibling module/directory) fresh rather than
inheriting the prior session's "102 ops checked, 304 mismatched keys"
figure. Reproduced 102/304 exactly pre-fix (103 ops resolvable), confirming
it was current, not stale.

**Real bug found and fixed** (real-SDK-client round trip, hand-reverted
against `git show HEAD:services/cognitoidp/models_users.go`, confirmed
failing, restored, md5sum byte-identical):
`adminUserJSON.UserAttributes` (`models_users.go`) was tagged
`json:"UserAttributes,omitempty"`. This type backs `AdminCreateUserOutput.
User` and `ListUsersInGroupOutput.Users[]`, both of which are `UserType`
(`cognitoidentityprovider@v1.67.4`) -- and `UserType`'s own attribute-list
member is `Attributes`, not `UserAttributes`
(`awsAwsjson11_deserializeDocumentUserType`, `deserializers.go` case
`"Attributes"`; confirmed `UserType` has no `UserAttributes` member at all,
`types/types.go:3152-3155`). `GetUserOutput`/`AdminGetUserOutput` legitimately
use `UserAttributes` as their own distinct member name
(`deserializers.go`, `deserializeOpDocumentGetUserOutput`/
`deserializeOpDocumentAdminGetUserOutput`) -- this is not a blanket rename,
only `adminUserJSON` was wrong. Every real client's `User.Attributes` (on
AdminCreateUser) and every item's `.Attributes` (on ListUsersInGroup)
decoded nil regardless of what attributes were supplied, for every pool.
Both ops were previously graded `wire: ok` in this file -- missed because
`ListUsersInGroup`'s winning handler could not be checked at all under the
stale ambiguous-binding framing (below), and `AdminCreateUser`'s dropped key
was buried among ~19 lambda-trigger-envelope false positives on the same op
(also below). Fixed: tag changed to `json:"Attributes,omitempty"` (Go field
name `UserAttributes` left alone -- a pure tag fix, no call-site changes).
New tests `TestAdminCreateUser_UserAttributesKey_RealSDKClient`/
`TestListUsersInGroup_AttributesKey_RealSDKClient`
(`wire_field_fixes_test.go`) assert on the typed SDK client's
`User.Attributes`/`Users[].Attributes`, not a raw body -- both fail against
unfixed code (`Attributes` empty) and pass fixed. No snapshot version bump:
`adminUserJSON` is wire-only, never persisted.

**Error envelope**: cognitoidp's error path
(`handleError`/`cognitoSentinelErrors`, `handler.go`) always writes the
shared awsjson1.1 `{"__type": ..., "message": ...}` envelope via
`service.JSONErrorResponse` -- confirmed this matches what
`awsAwsjson11_deserializeError*` actually parses (unlike s3control's
unwrapped-XML-for-every-op finding elsewhere this campaign); no envelope bug
found.

**Two structural findings, filed rather than fixed** (per this pass's
"do not restructure" constraint):

- **gopherstack-xasq**: `SchemaAttribute` (shared by CreateUserPool/
  DescribeUserPool/UpdateUserPool/ListUserPools) flattens
  `StringAttributeMinLength`/`MaxLength` and `NumberAttributeMinValue`/
  `MaxValue` onto the top level as int64/float64, where the real
  `SchemaAttributeType` nests them as string-valued
  `StringAttributeConstraints{MinLength,MaxLength}`/
  `NumberAttributeConstraints{MinValue,MaxValue}` sub-objects
  (`deserializers.go` case `"StringAttributeConstraints"`/
  `"NumberAttributeConstraints"` in
  `awsAwsjson11_deserializeDocumentSchemaAttributeType`, each sub-type's own
  `MinLength`/`MaxLength`/`MinValue`/`MaxValue` cases). Every real client's
  schema attribute constraints decode unset. This is what keycheck reported
  as 4 mismatched keys each on CreateUserPool/DescribeUserPool.
- **gopherstack-1b07** (CLOSED 2026-08-22): `AssociateSoftwareToken`/
  `VerifySoftwareToken`'s documented `Session` alternate identifier to
  `AccessToken` (the real MFA_SETUP-challenge-continuation flow, used when
  the caller has no access token yet) was wired on the wire side
  (`associateSoftwareTokenAccurateOutput.Session`/
  `verifySoftwareTokenAccurateOutput.Session` correctly tagged) but the
  backend (`mfa.go`) only ever resolved a user via
  `findUserByAccessTokenLocked` -- there was no session-based lookup at all,
  so a real client using the documented Session-only flow could not complete
  MFA setup. This was the "documented alternate identifier is the broken
  one" pattern named in this pass's brief. Fixed by adding
  `resolveMFASetupSubjectLocked` (mfa.go), a real MFA_SETUP challenge path in
  `mfaChallengeType`, and `RespondToMFASetupChallenge` to complete sign-in --
  see the RespondToAuthChallenge/AssociateSoftwareToken/VerifySoftwareToken
  entries above for the full fix and what remains undetermined
  (MFAS_CAN_SETUP, session rotation). **Correction to this finding's own
  original scope**: `SetUserMFAPreference` does NOT have a documented
  `Session` alternate at all -- `SetUserMFAPreferenceInput.AccessToken` is
  `This member is required` in the real SDK, with no `Session` field
  anywhere on that input (`api_op_SetUserMFAPreference.go`). Naming it
  alongside AssociateSoftwareToken/VerifySoftwareToken was wrong in the
  original filing (and in gopherstack-zquj's note before it); gopherstack's
  own `setUserMFAPreferenceAccurateInput` already had no `Session` field
  either, so there was nothing to fix there.

**Two new `cmd/keycheck` blind-spot refinements found, documented in
gopherstack-ck9f rather than in `cmd/keycheck/main.go` directly** (another
agent had that file locked for restjson1 path-dispatch work this session):

1. A refinement of blind spot #2: cognitoidp's auth ops that fire a Lambda
   trigger (SignUp, ConfirmSignUp, AdminConfirmSignUp, AdminCreateUser,
   InitiateAuth, AdminInitiateAuth, RespondToAuthChallenge,
   AdminRespondToAuthChallenge, ForgotPassword, ResendConfirmationCode,
   GetTokensFromRefreshToken) reach a shared Lambda-trigger-invocation
   helper (`lambda_triggers.go`) whose event/response envelope keys
   (`version`, `triggerSource`, `region`, `userPoolId`, `userName`,
   `callerContext`, `request`, `response`, `clientMetadata`,
   `userAttributes`, per-trigger fields like `challengeName`/`session`/
   `autoConfirmUser`/`emailMessage`/etc.) get attributed to the op being
   checked. This false-positive class accounts for roughly 250 of the
   original 304 mismatched keys (~85%) -- confirmed by tracing every large
   mismatch list on these ops back to `lambda_triggers.go`'s real,
   documented Cognito Lambda trigger event shape, not the op's own
   response. A related sub-case: several ops build an internal
   `map[string]string` of user attributes (`attrs["sub"]`,
   `attrs["custom:temporaryPassword"]`, `attrs["phone_number_verified"]`,
   `attrs["device_name"]`) later converted via `sortedAttributeList` into a
   `[]AttributeType{Name,Value}` list -- the map's keys become attribute
   `Name` *values*, never JSON keys.
2. A refinement of blind spot #6: cognitoidp keeps, for many op families, a
   legacy `handle<Op>` (bound in an earlier "OpsA" map) alongside a hardened
   `handle<Op>Accurate`/`handle<Op>Full` (bound in a later "OpsB"/"OpsC"
   map), and `dispatchTable()` merges every Ops-map via sequential
   `maps.Copy` calls -- the LAST-copied map always wins deterministically
   (Go's `maps.Copy` overwrites on collision), unlike sqs's genuinely
   ambiguous dual-protocol handlers. keycheck reported all 27 such ops as
   `AmbiguousOps`/ERROR (masked entirely under the stale "42 unresolved"
   framing). Hand-resolved all 27 via `dispatchTable()`'s literal call
   order (`handler.go:337-379`) and fully checked the winning handler for
   each: `mfaOpsB` (AssociateSoftwareToken, VerifySoftwareToken,
   SetUserMFAPreference, AdminSetUserMFAPreference -- found the
   gopherstack-1b07 gap above), `groupsOpsB` (CreateGroup, GetGroup,
   ListGroups, UpdateGroup, ListUsersInGroup -- found the `adminUserJSON`
   bug above, field-diffed `GroupType` otherwise clean),
   `identityProvidersOpsC` (all 5 ops field-diffed clean against
   `IdentityProviderType`/`ProviderDescription`), `resourceServersOpsB`
   (all 5 field-diffed clean against `ResourceServerType`/
   `ResourceServerScopeType`), `domainsOpsB` (clean against
   `CreateUserPoolDomainOutput`/`UpdateUserPoolDomainOutput`, `Routing`
   already a known deferred gap above), `securityConfigOpsB` (clean against
   the full `RiskConfigurationType` tree via keycheck's own `-dump-type`,
   `LastModifiedDate` already a known deferred gap above),
   `brandingOpsB` (clean against `UICustomizationType`, `CSSVersion` a new,
   minor missing-optional-field note -- not filed, same low-severity class
   as the existing deferred items), `attributesOpsC` (clean against
   `CodeDeliveryDetailsType`). `AdminSetUserMFASetting` (the one remaining
   unresolved op, not ambiguous) is not a current
   `cognitoidentityprovider@v1.67.4` operation at all -- no
   `api_op_AdminSetUserMFASetting.go` exists in the pinned SDK, confirmed by
   directory listing -- so it is legacy/unreachable-by-any-real-client
   code, not a wire bug to fix.

Post-fix re-run: 102 ops checked, 303 mismatched keys (the one real fix
removed `AdminCreateUser`'s `UserAttributes` mismatch;
`ListUsersInGroup`'s companion fix isn't counted in that figure since it
was one of the 27 previously-ambiguous ops, never part of the original 304).
The remaining 303 are, by the triage above, false positives: this is a
substantially-checked, substantially-clean service whose real defect count
this pass is 1 (plus the 2 filed structural gaps), not 304.

All gates green for `services/cognitoidp` (`go build ./...`, `go vet`,
`gofmt -l`, `go test -race ./services/cognitoidp/...`, `go test
./pkgs/persistence/...`, `golangci-lint run`, `make build-check`); no
`//nolint` added.

### What this pass fixed (2026-08-21, gopherstack-r80d batch 19: required OUTPUT member cut)

Instrument validated before use per the brief: two independent domain-struct
walks of `types.go` (a character-level brace matcher and a `go/parser`
AST pass) agreed exactly -- 93 structs, 29 carrying >=1 required member, 64
required fields summed; a third check (raw `grep -c "This member is
required."`) also returned 64. `cmd/requiredoutputfields`'s flat per-op scan
reports 27 required fields across 25 ops-with-required (129 ops total) --
the domain-struct total (64, and higher still counting every
wrapper-key-hidden op) is larger for the same "one wrapper key hides the
domain struct's own required members" reason established by
pinpoint/bedrockagent/accessanalyzer: `CreateTerms`/`DescribeTerms`/
`UpdateTerms` don't appear in the flat 25-op list at all (their `Terms`
field isn't itself required) yet each nests `types.TermsType`, which alone
carries 9 required members.

4 bugs found and fixed, all proven via real `aws-sdk-go-v2/service/
cognitoidentityprovider` client round trips
(`wire_output_required_r80d_test.go`), hand-reverted/confirmed-failing/
restored, md5sum-verified byte-identical:

1. `termsType.Links` (types/types.go:2225 area, `TermsType.Links` required) --
   `CreateTermsInput.Links` is optional, so a real client can create Terms
   with none; `toTermsType` (handler_terms.go) passed the nil map straight
   through and the wire tag was `omitempty`, dropping the key entirely for
   `CreateTerms`/`DescribeTerms`/`UpdateTerms`. Fixed by defaulting to
   `map[string]string{}` and removing `omitempty` -- required-but-empty
   means present-and-empty, not absent, per this cut's established
   convention.
2. `listWebAuthnCredentialsOutput.Credentials` (`ListWebAuthnCredentialsOutput.
   Credentials` required) -- the handler already built a non-nil empty
   slice for a user with zero registered passkeys, but the wire tag's
   `omitempty` still dropped the key (Go's `omitempty` triggers on `len==0`
   regardless of nilness). Fixed by removing `omitempty`.
3. `resourceServerScopeType.ScopeName`/`.ScopeDescription`
   (`ResourceServerScopeType.ScopeName`/`.ScopeDescription`, both `*string`
   and required) -- the real SDK's own client-side validator
   (`validateResourceServerScopeType`, validators.go:3613) only null-checks
   the pointer, not its content, so a real client can send an explicit
   empty-string scope name/description; both were tagged `omitempty` and
   vanished when empty. Fixed by removing `omitempty` on both.
4. `notifyConfigJSON.SourceArn` (`NotifyConfigurationType.SourceArn`, `*string`,
   required whenever `NotifyConfiguration` is present) -- same shape as #3:
   `validateNotifyConfigurationType` (validators.go:3483) only null-checks
   the pointer. Fixed by removing `omitempty`.

Fixed but NOT counted (unprovable via any real SDK client round trip):
`accountTakeoverActionTypeJSON.Notify` (`AccountTakeoverActionType.Notify`,
required `bool`) was tagged `omitempty`, which drops the key exactly when
`Notify == false` -- a real, reachable state (e.g. "don't notify" on a
low-risk assessment). Fixed by removing `omitempty` to match AWS's real
wire contract, but excluded from the tally: an omitted key and an explicit
`false` both decode to `false` in a real client, so no test can observe the
difference -- the amplify batch-14 `Branch.Stage` class, reapplied here.

Disclosed, not fixed (missing-feature gaps, not dropped-required-field
bugs -- the underlying feature is entirely unmodeled, not merely
under-surfaced):

- `EventFeedbackType.Provider` (required, `*string`) has no backing struct
  field on `AuthEvent`/`authEventFeedbackType` at all. This looked at first
  like the dominant "no struct field" bug class (matching bedrockagent's
  `FlowVersion.ExecutionRoleArn`), but tracing the data path further shows
  it can never be reached by a real client: `b.authEvents` (auth_events.go)
  is only ever *read*, never written, by any op -- this emulator does not
  hook sign-in flows to synthesize adaptive-auth risk events, so
  `AdminUpdateAuthEventFeedback`/`UpdateAuthEventFeedback` can never find an
  event to attach feedback to (`ErrAuthEventNotFound` for any real EventId),
  and `AdminListUserAuthEvents` always returns an empty list to a real
  client. Same missing-feature class as swf's undelivered `*EventAttributes`
  types (batch 17) -- tracked as a general risk-event-synthesis gap, not
  this cut's bug.
- `UserPoolAddOnsType.AdvancedSecurityMode`, `UsernameConfigurationType.
  CaseSensitive`, and `RefreshTokenRotationType.Feature` are all real,
  required-when-present sub-objects of `UserPoolType`/`UserPoolClientType`
  that this backend does not model at all (zero references to
  `AdvancedSecurityMode`/`CaseSensitive`/`RefreshTokenRotation` anywhere in
  the package outside this note) -- CreateUserPool/CreateUserPoolClient
  never accept or store these settings, so the wrapper is never populated
  in the first place. Same missing-feature class.
- `UserPoolType.LambdaConfig`/`AccountRecoverySetting` (and therefore every
  required member nested under `CustomEmailLambdaVersionConfigType`,
  `CustomSMSLambdaVersionConfigType`, `PreTokenGenerationVersionConfigType`,
  `InboundFederationLambdaType`, `RecoveryOptionType`) and
  `ManagedLoginBranding.Assets` (`AssetType.Category`/`ColorMode`/
  `Extension`) are stored and echoed as opaque `map[string]any`/
  `[]map[string]any` -- genuinely inapplicable to this bug class, not
  unaudited, since whatever a real client sends comes back byte-for-byte
  (the accessanalyzer `AnalyzerConfiguration`/opaque-union precedent).

Ruled out clean (read end to end, no bug): `GetProvisionedLimit`/
`UpdateProvisionedLimit` (`LimitType`/`LimitDefinitionType`, already
field-diffed in a prior pass and confirmed still correct -- non-pointer
fields with no `omitempty` gaps); `GetUser`/`AdminGetUser`/`AdminGetUserAuthFactors`/
`GetUserAuthFactors` (`UserAttributes`/`Username` always non-empty --
`userAttrsWithSub` guarantees at least a `sub` attribute, `Username` is the
caller's own); `IdentityProviderType`/`ResourceServerType`/
`UICustomizationType`/`DeviceType` (each declares zero required members of
its own per the AST walk, and the wrapper pointer is always non-nil on
success); `DeviceType.DeviceAttributes[].Name` (element names always come
from real map keys via the shared `sortedAttributeList` helper, never
empty); `AccountTakeoverRiskConfigurationType.Actions`/
`CompromisedCredentialsRiskConfigurationType.Actions` (both required
whenever their parent is present, but the real SDK's own validators
(validators.go:3144, :3253) reject a nil `Actions` client-side before the
request is ever sent -- structurally unreachable, the apprunner
`SourceCodeVersion` class); `customDomainConfigJSON.CertificateArn`
(handler_domains.go already only builds the wrapper when
`d.CertificateArn != ""`, so the two can never disagree); `ContextDataType`
(input-only type, confirmed via SDK grep -- never appears in any Output
struct, out of scope for this cut).

Not reached this batch: a full field-by-field audit of `UserPoolType`/
`UserPoolClientType`'s untyped (`map[string]any`) sub-config surface beyond
confirming it's opaque pass-through; `AssetType`'s branding family beyond
confirming it's opaque. Both would need new feature modeling, not a
required-output-field fix, so they were scoped out rather than audited
further.

All gates green for `services/cognitoidp` (`go build`, `go vet`, `gofmt -l`,
`go test -race`, `golangci-lint run`: 0 issues, 0 banned nolints, 0 new
nolints). Repo-wide `go build ./...`, `go vet ./...`, `go vet -tags e2e
./...`, `go vet -tags integration ./...` all clean. `services/sagemaker/`
untouched (concurrent-agent exclusion, `git status` checked before and
after -- its in-flight conversion still shows uncommitted changes at this
commit). `services/_REQUIRED_OUTPUT_CANDIDATES.md` updated: cognitoidp moved
from the ranked table into "Already examined".

### What this pass fixed (2026-08-13, gopherstack-h910)

`DeleteUserPoolClientSecret` dropped the required `ClientSecretId` -- the request struct
instead decoded a `SecretHash` field that does not exist on the real API at all and was
never even read anywhere in the handler, a dead field left over from an earlier design.
Investigating why turned up the real cause: `UserPoolClient` modeled a client's secret as
a single `ClientSecret` string, so there was no `ClientSecretId`-keyed value for
`DeleteUserPoolClientSecret` to identify. Real AWS supports up to 2 active secrets per app
client for zero-downtime rotation (`AddUserPoolClientSecret`'s own doc comment), each
identified by a real `ClientSecretId` -- a feature this emulator could not represent at
all under the old model.

Fixed by adding a second, ClientSecretId-keyed secret slot (`ExtraClientSecrets`, capped
at 1 to match the real 2-active-secrets-total limit) alongside the original
`ClientSecret` field, which is left untouched: real `types.UserPoolClientType` (the
`DescribeUserPoolClient`/`CreateUserPoolClient` response shape) has no `ClientSecretId`
field for the original secret either, so this emulator does not fabricate one for it --
it stays reachable only the way it always was, and is not visible to
List/DeleteUserPoolClientSecret. `AddUserPoolClientSecret`'s and
`ListUserPoolClientSecrets`' response shapes were also wrong (a flat `ClientSecret`
string and a flat `Secrets []string` respectively, neither of which exist on the real
API -- both are really `ClientSecretDescriptorType`/`[]ClientSecretDescriptorType` with
`ClientSecretId`/`ClientSecretCreateDate`, and `ClientSecretValue` only ever populated by
`AddUserPoolClientSecret`, never by `List`). Also fixed a false comment on
`ListUserPoolClientSecrets` claiming "AWS allows at most one active secret" -- the real
API's own doc comment says up to 2.

### What this pass fixed (2026-08-08, gopherstack-kxow)

Closed the sole gap that had dropped this service's grade from A to B: `terms/`'s
entirely invented wire model, found (but deliberately left unfixed, as a full
redesign) during the `gopherstack-n7gh` follow-up pass below. See
`families.terms` above for the full field-diff and citations; summary:

1. **Enumerated the real op family from the SDK, not from gopherstack's existing
   handlers** (the thing under suspicion): `ls` on
   `aws-sdk-go-v2/service/cognitoidentityprovider@1.67.4`'s api_op directory for
   `*Terms*.go` turns up exactly 5 files -- `CreateTerms`, `DeleteTerms`,
   `DescribeTerms`, `ListTerms`, `UpdateTerms`. There is no `GetTerms`.
2. **Modeled the real input/output for all 5 ops** plus both enums
   (`TermsEnforcementType`/`TermsSourceType`, each currently a single valid
   value, `NONE`/`LINK`, per the SDK's own "reserved for future use" godoc).
   `CreateTerms` now requires `ClientId`/`Enforcement`/`TermsName`/
   `TermsSource`/`UserPoolId` and accepts `Links` (`map[string]string`) --
   previously none of these existed and the op was unreachable by any real SDK
   client (its own request-validation middleware refuses to send the request at
   all). `ListTerms` returns `TermsDescriptionType` (`TermsId`/`TermsName`/
   `Enforcement`/`CreationDate`/`LastModifiedDate` only -- confirmed by reading
   the full struct, not assumed from the larger `TermsType`).
3. **Rescoped storage around `ClientId`**: `terms` is now a `store.Table[Terms]`
   keyed by a server-generated `TermsID` (uuid, matching AWS's opaque `TermsId`)
   with a `byPool` secondary index for `ListTerms`, replacing the old table
   keyed directly by `UserPoolID` (which structurally could hold only one
   record per pool, incompatible with the real per-client, multi-document
   model). `CreateTerms` validates the `ClientId` belongs to the `UserPoolId`
   (`ResourceNotFoundException`) and rejects a duplicate `ClientId`+`TermsName`
   pair (`TermsExistsException`, a real error code confirmed in
   `deserializers.go`'s `CreateTerms`/`UpdateTerms` error case lists).
4. **No invented endpoint needed deleting**: every op gopherstack already routed
   under this family (`CreateTerms`/`DeleteTerms`/`DescribeTerms`/`ListTerms`/
   `UpdateTerms`) has a real AWS counterpart; the fix was to correct their wire
   shape and storage, not to remove a fabricated path.
5. **`cognitoidpSnapshotVersion` deliberately left at 1, not bumped**: an
   earlier version of this fix bumped it to 2 since the `Terms` DTO shape and
   table key both changed (was `{UserPoolID, Text}` keyed by `UserPoolID`, now
   `{TermsID, UserPoolID, ClientID, TermsName, Enforcement, TermsSource, Links,
   CreatedAt, LastModifiedAt}` keyed by `TermsID`) -- but `Restore` discards
   the entire snapshot, every table, on any version mismatch, not just the
   changed one. That trade is backwards: it would lose every real pool, user,
   password hash, and MFA setting on upgrade in order to protect a `terms`
   table that cannot contain real pre-redesign data anyway (`CreateTerms` was
   unreachable by any genuine SDK client before this fix). Reverted the bump;
   `restoreTermsLocked` instead decodes the `terms` table on its own, outside
   the shared `dtoReg.RestoreAll` path, and drops any entry that doesn't carry
   a non-empty `TermsID` (a pre-redesign row decodes cleanly into the new
   shape but with `TermsID` empty, since that field didn't exist yet) --
   losing nothing, since no real row can look like that, while every other
   table restores unaffected.
6. **Pre-fix verification**: the new `terms_test.go` (real required-field JSON
   driven through the handler, exactly what the old bug hid behind) was run
   against the pre-fix code in a `git worktree` before the fix landed. 5 of 6
   new test functions failed -- the old handler accepted `CreateTerms` missing
   every required field and `DescribeTerms`/`UpdateTerms`/`DeleteTerms`
   referencing a nonexistent `TermsId`, always returning `200 OK`, confirming
   the "unreachable, not merely wrong" diagnosis concretely.

### What this pass fixed (2026-08-08, gopherstack-n7gh follow-up)

Completed the remainder of `gopherstack-n7gh`'s stated scope (SRP-6a itself was
already done earlier the same day, commit `041c16c75`). Full detail and SDK
citations are in the `ops`/`families` entries above (`ForgotPassword`, `domains`,
`user_import_jobs`, `devices`, `webauthn`, `managed_login_branding`, `risk_config`,
`terms`, `log_delivery`, `identity_providers`, `resource_servers`); summary:

1. **`UserMigration_ForgotPassword`**: `ForgotPassword` on an unknown username now
   tries the `UserMigration` Lambda trigger (correct trigger source, no plaintext
   password in the event per AWS docs) before falling back to
   `PreventUserExistenceErrors` masking / `UserNotFoundException`.
2. **Domain `AWSAccountId`/`ManagedLoginVersion`/`S3Bucket`**: all three added to
   `DescribeUserPoolDomain`; `ManagedLoginVersion` is also now a real accepted
   request field on `CreateUserPoolDomain`/`UpdateUserPoolDomain` (previously
   silently dropped).
3. **webauthn had a genuine wrong-wire-shape bug**: the response used
   `"FriendlyName"` where real Cognito uses `"FriendlyCredentialName"` (verified
   against `deserializers.go`, not assumed) -- no real SDK client could ever have
   read this field. Fixed, plus added the required `AuthenticatorTransports` field
   (extracted from the client's already-accepted-but-unread `Credential.response.
   transports`).
4. **`managed_login_branding` discarded its entire payload**: `Settings`, `Assets`,
   and `UseCognitoProvidedValues` -- not minor fields, the actual branding
   configuration -- were accepted by no input field and never echoed on
   Create/Describe/Update. Fixed as raw pass-through storage, the same pattern
   `UserPool.LambdaConfig` already uses for its own arbitrary-shaped config.
5. **`SetLogDeliveryConfiguration` was a disguised stub**: it called
   `SetLogDeliveryConfiguration(poolID, nil)` unconditionally, ignoring the
   client's `LogConfigurations` regardless of what was sent -- a real bug of
   exactly the kind `parity-principles.md` warns about ("a 'real-looking' op may
   be a disguised stub"). Fixed.
6. **`CreateUserImportJob` dropped its required `CloudWatchLogsRoleArn`** (and
   optional `PasswordHashingAlgorithm`) entirely; also added
   `CreationDate`/`StartDate`/`CompletionDate`/`PreSignedUrl`/`FailedUsers`/
   `ImportedUsers`/`SkippedUsers` to the response (the first three were already
   tracked internally and simply never echoed; the counts are honestly `0` since
   this backend has no real CSV-processing pipeline).
7. **`terms/` was found to be built on an entirely fictional wire model** -- not a
   missing-field gap but a different data model altogether (see `deferred` above
   for detail). Explicitly NOT fixed: a full redesign, not a bounded change, and
   the same principle that stopped a rushed SRP-6a attempt in an earlier pass
   applies here -- a half-correct terms/ implementation would be worse than the
   current honestly-limited one. CLOSED 2026-08-08 by `gopherstack-kxow`, see the
   section above.

All new/changed wire fields were verified against
`aws-sdk-go-v2/service/cognitoidentityprovider@1.67.4` in the module cache
(types/types.go, the per-op `api_op_*.go` files, and `deserializers.go` for exact
JSON key names), not against this package's own prior output.

### What this pass fixed (2026-07-25, parity-4)

The Go SDK module was bumped (`aws-sdk-go-v2/service/cognitoidentityprovider`
1.59.1 -> 1.67.0), which shipped 7 new operations `TestSDKCompleteness` did not
yet know about: `CreateUserPoolReplica`, `ListUserPoolReplicas`,
`UpdateUserPoolReplica`, `DeleteUserPoolReplica` (multi-Region replication),
`AdminGetUserAuthFactors`, `GetProvisionedLimit`, `UpdateProvisionedLimit`. All
7 are implemented for real (routing, backend state, request parsing, response
wire shape, error codes, Snapshot/Restore) -- none were added to
`notImplemented`. See `families.user_pool_replicas`, `families.provisioned_limits`,
and `ops.AdminGetUserAuthFactors` above for the full field-diff and derivation
detail; summary:

1. **User pool replicas** (`user_pool_replicas.go`, `models_user_pool_replicas.go`,
   `handler_user_pool_replicas.go`): a new `userPoolReplicas` `store.Table`
   (composite `poolID:region` key, `byPool` index) backs multi-Region
   replication. `CreateUserPoolReplica` enforces two real, documented AWS
   constraints: the replica Region must differ from the primary pool's own
   Region, and a user pool may have at most one secondary replica ("at most
   one secondary replica in an additional Region per user directory"). New
   replicas start `INACTIVE`, matching the developer guide's prose (the same
   guide's JSON example elsewhere shows a `PENDING_CREATE` status that isn't
   even a valid `ReplicaStatusType` enum member -- a documentation
   inconsistency resolved in favor of the real SDK enum).

2. **`AdminGetUserAuthFactors`** (`users.go`, `models_users.go`,
   `handler_users.go`): derives `ConfiguredUserAuthFactors` entirely from
   existing user/MFA state -- `PasswordHash`, `UserMFASettingList`,
   `MFAOptions[].DeliveryMedium`, `TOTPVerified`, and the `webauthnCredentials`
   map -- never a fixed/fabricated list. Shares its PASSWORD/SMS_OTP/WEB_AUTHN
   logic with the pre-existing `GetUserAuthFactors` via an extracted
   `commonAuthFactorSetLocked` helper; `GetUserAuthFactors`'s own behavior is
   unchanged.

3. **Provisioned limits** (`provisioned_limits.go`, `models_provisioned_limits.go`,
   `handler_provisioned_limits.go`): confirmed account-level (not per-user-pool)
   against the live Cognito quotas guide, so these two ops take no
   `UserPoolId`. The 18 API_CATEGORY default RPS values and their
   adjustable/not-adjustable flags are the real, live-fetched AWS defaults, not
   invented. The one place this pass had to invent a number
   (`accountMaxMultiplier`, since AWS's real per-account Service-Quotas max is
   granted individually and unpublished) is called out explicitly in both the
   code comment and `families.provisioned_limits` above.

### What the 2026-07-23 pass fixed

1. **CUSTOM_AUTH did not exist as a flow at all (highest-severity finding this pass).**
   `InitiateAuth`/`AdminInitiateAuth` with `AuthFlow: "CUSTOM_AUTH"` unconditionally
   returned `InvalidUserPoolConfigurationException: unsupported auth flow "CUSTOM_AUTH"`
   — not a disguised stub, just entirely unrouted. Implemented the real Lambda-driven
   state machine (`custom_auth.go`): `DefineAuthChallenge` decides issue-tokens /
   fail / present-a-challenge each round from the accumulated session history;
   `CreateAuthChallenge` builds public (client-visible) and private (server-only)
   challenge parameters; `VerifyAuthChallengeResponse` judges the answer. A wrong
   answer does **not** auto-fail the attempt — exactly like AWS, the Lambda alone
   decides via the next `DefineAuthChallenge` call, so "fail after N wrong answers"
   policies work. `RespondToAuthChallenge`/`AdminRespondToAuthChallenge` gained a
   `CUSTOM_CHALLENGE` case, and `ChallengeParameters` is now populated end-to-end
   (was always `{}` on the wire; `AuthResult`/`authOutput` gained the field).
   Verified against `aws-lambda-go/events` (`CognitoEventUserPoolsDefineAuthChallenge/
   CreateAuthChallenge/VerifyAuthChallenge`) and the real SDK's
   `RespondToAuthChallengeOutput.ChallengeParameters` field. 7 new tests in
   `custom_auth_test.go` cover single-round issue/fail, multi-round retry, the
   "DefineAuthChallenge not configured" error, ExplicitAuthFlows restriction, and the
   Admin path.

2. **UserMigration and PreAuthentication/PostAuthentication triggers were stored but
   never invoked**, closing the remainder of `gopherstack-8fw`. `UserMigration` now
   fires (`user_migration.go`) when `USER_PASSWORD_AUTH`/`ADMIN_USER_PASSWORD_AUTH`/
   `ADMIN_NO_SRP_AUTH` names an unknown username and the pool has the trigger
   configured: a Lambda response with `userAttributes` creates and authenticates a new
   user in one round trip (matching AWS: "migrate a user from an external system on
   first sign-in"); a response with no attributes, or no trigger configured, falls
   back to the pre-existing "unknown user" handling (including
   `PreventUserExistenceErrors` masking) exactly as before. `FinalUserStatus:
   "RESET_REQUIRED"` is honored per AWS's documented semantics: *this* migrating
   sign-in still succeeds with tokens, but the account is left in
   `FORCE_CHANGE_PASSWORD` so the *next* sign-in requires a password reset (see
   "Traps" below for the residual uncertainty on this specific timing).
   `PreAuthentication`/`PostAuthentication` now fire around `authenticate()`/
   `issueTokensLocked()` respectively; a Lambda that throws fails the sign-in attempt
   (`UserLambdaValidationException`) before (PreAuthentication) or after
   (PostAuthentication) credentials are checked. `PostAuthentication` does not
   re-fire on `REFRESH_TOKEN_AUTH`, matching AWS. UserMigration is scoped to
   `InitiateAuth`/`AdminInitiateAuth` only this pass — `ForgotPassword`'s
   `UserMigration_ForgotPassword` trigger source is a documented `items_still_open`
   item, not implemented.

3. **PreventUserExistenceErrors=ENABLED did not mask `ConfirmSignUp`/
   `ConfirmForgotPassword`**, closing the remainder of `gopherstack-aib` (InitiateAuth/
   ForgotPassword/ResendConfirmationCode were already closed in prior passes). An
   unknown username on either op now returns `CodeMismatchException` — the same error
   a real account with a wrong code produces — instead of `UserNotFoundException`,
   closing the last username-enumeration vector in the auth surface. New tests in
   `prevent_user_existence_test.go`; one pre-existing test's assertion
   (`Test_ForgotPassword_PreventUserExistenceErrors/ENABLED_masks_as_a_fabricated_success`)
   was updated in place since it was asserting the now-fixed gap's old (wrong)
   behavior.

4. **`DescribeUserPoolDomain` never returned `CustomDomainConfig`.** Field-diffed
   `DomainDescriptionType` against the real SDK this pass (see `families.domains`) and
   found a custom domain's ACM `CertificateArn` was tracked internally
   (`UserPoolDomain.CertificateArn`) but never echoed back on Describe — a real fidelity
   gap for anything that reads it back to detect drift (e.g. the Terraform AWS
   provider). Fixed; `AWSAccountId`/`ManagedLoginVersion`/`S3Bucket`/`Version` remain
   unpopulated (not tracked by this backend's domain model) and are recorded as
   `items_still_open`, not silently dropped.

5. **De-stub hygiene: deleted the ~15-op handler.go dead-code shadowing** flagged as a
   deferred cleanup candidate in the prior sweep. `handler_auth.go`,
   `handler_user_pools.go`, `handler_user_pool_clients.go`, and `handler_users.go` each
   registered a non-accurate handler for an op *and* a `*Accurate`/`*WithOpts`/`*Full`
   twin under the same op name later in `dispatchTable()`'s `maps.Copy` chain, so the
   accurate version always won and the first was unreachable dead code (confirmed via
   `grep` for direct test references — none existed). Deleted the dead handler funcs,
   their now-orphaned model-only input/output types (in `models_auth.go`,
   `models_user_pools.go`, `models_user_pool_clients.go`, `models_users.go`), the
   `authOpsB()`/shadowed-entry helper functions, and the corresponding
   `dispatchTable()` wiring. `golangci-lint`'s `unused` check (0 issues) confirms
   nothing is newly orphaned.

### What prior passes fixed

1. **MFA challenge codes were a disguised stub (highest-severity finding).**
   `RespondToMFAChallenge` / `VerifySoftwareToken` validated only that the supplied code was
   6 ASCII digits and then always succeeded — SOFTWARE_TOKEN_MFA, SMS_MFA, and EMAIL_OTP were
   all bypassable with any random 6-digit string, regardless of the secret returned by
   `AssociateSoftwareToken`. Fixed with a real RFC 6238 TOTP implementation
   (`totp.go`: HMAC-SHA1, 30s step, ±1 step clock-skew tolerance, RFC 4226 dynamic
   truncation) verified against the **official RFC 4226 Appendix D and RFC 6238 Appendix B
   test vectors** (two independently published vector sets that cross-check each other —
   RFC 4226 counter=1 and RFC 6238 T=59 both yield `287082`). SMS_MFA/EMAIL_OTP now require
   the one-time code generated by `newMFASession` (mirroring the existing
   ForgotPassword/ConfirmSignUp confirmation-code pattern) instead of accepting anything.
   A wrong code no longer consumes the MFA session, so the caller can retry until it
   expires — matching real Cognito.

2. **PreventUserExistenceErrors was completely unimplemented** on `UserPoolClient` (not
   stored, not exposed on Create/Update/Describe, not enforced anywhere) despite being
   explicitly part of the client model. Added the field (default `"LEGACY"`, matching AWS),
   wired it through Create/UpdateUserPoolClient input/output, and enforced it in the
   non-admin `InitiateAuth` path: `ENABLED` now masks an unknown username behind the exact
   same `NotAuthorizedException` text a wrong password produces (proven in tests: the two
   error strings are asserted equal). `AdminInitiateAuth` intentionally never masks — AWS
   only applies this to the non-admin, unauthenticated API.

3. **DeletionProtection was stored but never enforced.** `DeleteUserPool` deleted pools
   unconditionally even when `DeletionProtection: "ACTIVE"`. Now returns
   `InvalidParameterException` with AWS's documented remediation message, and the pool can
   still be deleted after `UpdateUserPool` flips it back to `"INACTIVE"`.

4. **(2026-07-12 re-audit) `ForgotPassword`/`ResendConfirmationCode` did not honor
   `PreventUserExistenceErrors` (previously flagged gap `gopherstack-aib`).** Both ops
   unconditionally returned `UserNotFoundException` for an unknown username regardless of
   the app client's setting, letting a caller enumerate valid usernames even with
   `PreventUserExistenceErrors=ENABLED` — the exact vulnerability that setting exists to
   close, and the same masking `InitiateAuth` already got in a prior sweep. Fixed in
   `backend.go`'s `ForgotPassword`/`ResendConfirmationCode`: when the client masks
   existence errors, an unknown username now returns a fabricated success (same
   `CodeDeliveryDetails` shape a real account gets) instead of erroring, and the fabricated
   code is never stored so it can't actually be redeemed. New table-driven tests in
   `prevent_user_existence_test.go` (`Test_ForgotPassword_PreventUserExistenceErrors`,
   `Test_ResendConfirmationCode_PreventUserExistenceErrors`) lock in both the masked and
   unmasked (LEGACY) behavior.

### Traps for the next auditor

- **`domains.go`, `security_config.go`, and `identity_providers.go` still use the
  `opsA()`/`opsB()` legacy-plus-accurate split** the 2026-07-23 pass deleted from
  `handler_auth.go`/`handler_user_pools.go`/`handler_user_pool_clients.go`/
  `handler_users.go` (see the dispatch-shadowing trap below) -- this is a DIFFERENT,
  still-live pattern in these 3 files: `opsA()` registers a legacy/incomplete handler
  under an op name, `opsB()` registers the real "Full"/"Accurate" handler under the
  same name via a matching `op<Name>` constant, and `dispatchTable()`'s `maps.Copy(table,
  h.XOpsA()); ...; maps.Copy(table, h.XOpsB())` ordering makes `opsB()` win every time.
  This is intentional (not dead code to delete) for `DeleteUserPoolDomain`/
  `DescribeUserPoolDomain` (which stay on `opsA()`, no `opsB()` override exists for
  them) but means `CreateUserPoolDomain`/`UpdateUserPoolDomain`,
  `SetRiskConfiguration`/`DescribeRiskConfiguration`, and the identity-provider
  Create/Update ops are ALL served by handlers that don't textually appear next to
  their `"OpName": service.WrapOp(...)` map entry in `opsA()` -- reading only `opsA()`
  gives a false picture of live behavior for those specific ops. Verified this
  distinction by reading `handler.go`'s `maps.Copy` call order directly for each
  family this pass, not assumed from file layout.
- **The Cognito multi-Region-replication *developer guide*'s JSON examples do not match
  the real wire shape.** Its `CreateUserPoolReplica`/`UpdateUserPoolReplica` examples show
  the response wrapped under a `"Replica"` key and an initial `"Status": "PENDING_CREATE"`.
  Neither is real: `deserializers.go` confirms the actual field is singular
  `"UserPoolReplica"` (plural `"UserPoolReplicas"` only on `ListUserPoolReplicas`), and
  `PENDING_CREATE` is not a member of `types.ReplicaStatusType` at all (the real enum is
  `CREATING`/`ACTIVE`/`INACTIVE`/`DELETING`). This implementation trusts the generated
  SDK code over the prose/examples wherever they disagree — do the same if you revisit this
  family, and don't "fix" the wire shape back to match the docs.
- **USER_SRP_AUTH/ADMIN_USER_SRP_AUTH now implement real SRP-6a** (2026-08-07,
  gopherstack-n7gh) — see `srp.go` and the closed gap above for the algorithm and
  verification method. `InitiateAuth`/`AdminInitiateAuth` route these two flow names to
  `InitiateAuthSRP`/`AdminInitiateAuthSRP` (given `AuthParameters["SRP_A"]`) instead of
  `authenticate()`, which now explicitly rejects them (`ErrInvalidUserPoolConfig`) if
  ever called directly with a plaintext password — a caller bug, not a valid path, since
  a real SRP client never sends one.
- **`RespondToMFAChallenge` is now challenge-type-aware** (`verifyMFAChallengeCode` in
  backend.go): SOFTWARE_TOKEN_MFA verifies against `user.TOTPSecret` via `verifyTOTPCode`;
  SMS_MFA/EMAIL_OTP verify against `mfaSessionEntry.Code`. If you add a new MFA challenge
  type, it must be added to this switch explicitly — the `default` case now denies rather
  than silently accepting (previously everything funneled through one format-only check).
- **`GenerateTOTPCode` is exported** specifically so integration/SDK-driven tests can
  compute the code a real authenticator app would produce for a secret returned by
  `AssociateSoftwareToken`, without needing a TOTP library dependency.
- Protocol is `json-1.1` (`X-Amz-Target: AWSCognitoIdentityProviderService.<Op>`), not
  XML — confirmed via `handler.go`'s `service.HandleTarget` wiring; no XML wrapper traps
  apply to this service the way they do to EC2/S3-family query/XML services.
- Token timestamps (`iat`/`exp`/`auth_time`/`UserCreateDate`/etc.) are epoch-seconds JSON
  numbers throughout — already correct, no `awstime.Epoch` gap found.
- **The dead-code `dispatchTable()` shadowing (dating back to an old `accuracy_handler.go`
  split) is now fully deleted (2026-07-23 pass).** Every op in `dispatchTable()` has
  exactly one live registration now; there is no more "read `handler_auth.go` and get a
  false read on behavior because a same-named `*Accurate` twin actually wins" trap. If a
  future op needs both a legacy and an accurate variant again, register only the live one
  under the bare op-name key and delete the other immediately — do not let both linger.
- **CUSTOM_AUTH's wire `ChallengeName` is always the literal `"CUSTOM_CHALLENGE"`
  string**, regardless of what name `DefineAuthChallenge`'s Lambda response used for its
  own bookkeeping (`mfaSessionEntry.CustomAuthChallengeName`, which only ever appears in
  the `session` history passed *back* to `DefineAuthChallenge`/`CreateAuthChallenge`, never
  on the wire to the client). Do not conflate the two when reading `custom_auth.go` —
  `challengeCustomChallenge` (`"CUSTOM_CHALLENGE"`) is the fixed wire constant;
  `CustomAuthChallengeName` is Lambda-internal bookkeeping only.
- **UserMigration's `FinalUserStatus: "RESET_REQUIRED"` timing is a good-faith reading of
  AWS's documented wording** ("the user must change their password *during the next
  sign-in attempt*"), not verified against a live Cognito pool: this implementation lets
  the *migrating* attempt itself succeed with tokens (the plaintext password was already
  validated by the Lambda for this one attempt) and only gates *subsequent* attempts
  behind `FORCE_CHANGE_PASSWORD`/`NEW_PASSWORD_REQUIRED`. If a real Cognito trace ever
  contradicts this ordering, `applyPostMigrationFinalStatus` in `user_migration.go` is the
  single place to fix it — it deliberately runs *after* `authenticate()` has already used
  the freshly-migrated user's `CONFIRMED` status for this attempt.
- **UserMigration only fires for `InitiateAuth`/`AdminInitiateAuth`, not `ForgotPassword`.**
  AWS also defines a `UserMigration_ForgotPassword` trigger source for migrating a user who
  tries to reset a password they never had in Cognito; this backend does not implement that
  path (`ForgotPassword` on an unknown username still just returns/masks
  `UserNotFoundException` as before). Tracked as `items_still_open`.
- **Ordering between UserMigration and PreAuthentication for a migrating sign-in is an
  implementation choice, not a verified AWS behavior.** `InitiateAuth`/`AdminInitiateAuth`
  run `tryUserMigration` first (to obtain a `*User` to authenticate at all), then call
  `authenticate()`, which fires `PreAuthentication` unconditionally -- so PreAuthentication
  does fire for a freshly-migrated user, just after migration rather than before. Real
  Cognito's exact ordering between these two triggers on a migrating request was not
  verified against a live pool.

## 2026-08-29 (pagination-arithmetic sweep, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

Scope: arithmetic inside every hand-rolled pagination helper in this service (not wire-shape
cursor *population*, already covered by the "cursor-population sweep" comment above this
file's schema header -- that sweep and this one found different bug classes in overlapping
ops; see the CORRECTION notes left in place of its wrong claims).

**Census.** 8 hand-rolled paginators in this package, none importing `pkgs/page`:
`paginateDevicesLocked` (`devices.go`, shared by `ListDevices` and `AdminListDevices`),
`ListGroupsPage` (`groups.go`, `ListGroups`), `ListUsersInGroupPage` (`groups.go`,
`ListUsersInGroup`), `paginateAuthEventsLocked` (`auth_events.go`,
`AdminListUserAuthEvents`), `ListWebAuthnCredentials`'s own inline cursor (`webauthn.go`),
and two handler-inline cursors that live in the handler function itself rather than a
named helper: `handleListUsers` (`handler_users.go`, `ListUsers`) and
`handleListUserPools` (`handler_user_pools.go`, `ListUserPools`). `ListTerms` is the one
List op in this service that genuinely does use `pkgs/page.New` (`terms.go`) and was
already correct. Every other List/Describe op in this service either has no pagination at
all or goes through one of the wins-the-registration handlers the cursor-population sweep
already covers.

**Bug (Class B: infinite loop, cursor matched by equality) x8.** All eight helpers above
shared the identical bug: search `all`/`ids` for the item named by the token by equality,
and on a miss (the item was deleted, or never existed) leave `start`/`startIdx` at its zero
value instead of the collection length. A client resuming with a token naming a
since-deleted device/group/group-member/credential/user/pool got page one again, forever.
Fixed identically at each site: a miss now sets `start = len(all)` (the "default a miss to
empty" pattern, as in glacier) instead of leaving it at `0`. None of the eight can express
the bug anymore.

`AdminListUserAuthEvents`/`paginateAuthEventsLocked` is a genuine instance of the same bug,
but currently unreachable in practice: no code path in this emulator (no sign-in hook, no
janitor sweep) ever writes into `b.authEvents`, so the collection is always empty and the
scan-miss branch never has anything to search regardless. Fixed anyway for correctness
under any future caller; tested via a new `SeedAuthEventForTest` `export_test.go` helper
that seeds the otherwise-unreachable store directly.

**Testing.** New `pagination_arithmetic_test.go` covers all eight helpers, each via the
operation that calls it (backend-level for the five that expose an exported backend method;
real `aws-sdk-go-v2` typed client for the two handler-inline ones, since there's no backend
method to call directly for those). Boundary walk (N=7, page=3, full concatenation checked)
plus a stale-cursor case (a token for an item that never existed, or a real deletion via the
service's own delete op where one exists) for every site; exact-division/single-page/empty
checks added where the setup cost was low. All stale-cursor subtests were confirmed to fail
against the unmodified code before the fix (Class B: another non-empty cursor came back
instead of terminating), then pass after it. Existing pagination tests this sweep found
(`TestListUsers_Pagination` in `handler_users_lifecycle_test.go`,
`TestListUserPools_Pagination` in `user_pools_config_test.go`,
`TestAdminListGroupsForUser_Pagination`/`TestGroup_ListGroups_Pagination`/
`TestGroup_ListUsersInGroup_Pagination`) already did real boundary walks with
no-duplicates checks -- good tests -- but none of them presented a stale cursor, which is
why this class of bug survived them; new `*_StaleCursor` tests close that gap without
duplicating the existing boundary-walk coverage.

**Reachable-handler check (per the shadowed-registration risk this file already tracks).**
Verified each of `ListDevices`, `AdminListDevices`, `ListGroups`, `ListUsersInGroup`,
`ListWebAuthnCredentials`, `ListUsers`, `ListUserPools`, `AdminListUserAuthEvents` is
registered exactly once across every `*OpsA/B/C` map `maps.Copy`'d into `handler.go`'s
dispatch table -- none of the eight fixed here are among the operations with a duplicate
registration, so the handler read during this audit is the one that actually serves
traffic for all eight.

**Gates:** `go build`, `go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/cognitoidp/...` (pass, including every pre-existing pagination test), `golangci-lint
run ./services/cognitoidp/...` (0 issues, confirmed by removing the new test files and
re-running rather than assuming pre-existing-file status).

**2026-08-30 (unstable-pagination-order sweep, wrapper-key-sweep branch)**: `ListUserPools`
(`user_pools.go`) sorted only by `Name` before `handleListUserPools`'s (`handler_user_pools.go`)
`NextToken`-based pagination. `CreateUserPool` has no "already exists" exception -- real AWS
Cognito does not enforce unique pool names, and this codebase already has a test documenting that
(`TestInMemoryBackend_CreateUserPool`'s `duplicate_name` case, `user_pools_test.go`) -- so `Name`
alone is not a unique sort key, and the underlying `b.pools.All()` read is also an unspecified-order
map walk. Two same-named pools could swap relative order between the call that produced a page's
`NextToken` and the call that resumed from it, dropping or duplicating a pool at the boundary, even
though the `NextToken` itself (pool ID, via `handleListUserPools`'s `p.ID == in.NextToken` scan) is
unique -- the same "unique cursor, tie-prone sort" shape the campaign brief documents for elbv2 and
ssoadmin. Not a duplicate-rejection case (unlike waf's activated rules): duplicate pool names are
legitimate on real AWS, matching route53 hosted zones, so the fix is a tiebreak, not a Create-path
rejection. Fixed by tiebreaking the sort on `ID` (unique) when `Name` compares equal.

This is a different bug from the four *List* operations with a shadowed dispatch-table
registration this file already documents finding safe (`ListGroups`/`ListUsersInGroup` via
`groupsOpsA`+`groupsOpsB`, `ListIdentityProviders` via `identityProvidersOpsB`+`OpsC`,
`ListResourceServers` via `resourceServersOpsA`+`OpsB`) -- re-verified this pass: in every one of
those four, the *winning* registration (the later `maps.Copy` in `dispatchTable()`, always the
`Full`/`Accurate`-suffixed handler) reads via a `store.Index.Get` filtered to one user pool and
sorts by a field that is unique within that pool once filtered (`GroupName`, `Username`,
`ProviderName`, `Identifier`) -- already safe, unchanged. Note for the next pass: this file's
"registers four operation names twice" framing undercounts -- a broader sweep this pass found at
least 19 more operation names (mostly Create/Update/Describe/Get/Set, not List) registered under
both a literal string and an `opX` constant across separate `OpsA`/`OpsB`/`OpsC` functions, all
following the same later-registration-wins `Full`/`Accurate` pattern; not re-audited here since none
are List/paginated operations relevant to this sweep's scope.

Every other paginated `List*` site in this service was audited and confirmed already safe:
`ListGroupsPage`/`ListUsersInGroupPage` (`groups.go`, backing the two *Full* handlers above),
`ListIdentityProviders` (`identity_providers.go`), `ListResourceServers` (`resource_servers.go`),
`ListUsers`/`ListUsersFiltered` (`users.go`), `ListTerms` (`terms.go`), `paginateDevicesLocked`
(`devices.go`), `ListWebAuthnCredentials` (`webauthn.go`) all filter to one pool/user (via
`store.Index.Get` or a per-key inner map) before sorting by a field unique within that filtered set,
or (devices/webauthn) sort by a field that is itself the inner map's own key.
`paginateAuthEventsLocked` (`auth_events.go`) sorts by `CreatedAt` with an explicit `EventID`
tiebreak already in place -- confirmed correct, unchanged, and a good precedent that made the
`ListUserPools` gap stand out by contrast. `ListUserPoolReplicas` (`user_pool_replicas.go`) carries
a doc comment establishing it never has more than one item to page over in practice (one replica per
region, no cross-region duplication path) -- trusted per this campaign's guidance to trust a comment
that gives a correct reason, not re-litigated.

Proof: `TestListUserPools_PaginationOrderIsReproducible` (`pagination_arithmetic_test.go`) creates
16 user pools all sharing one `PoolName`, walks them with `MaxResults=3` across `NextToken`-resumed
pages (real SDK client), and asserts the concatenation reproduces the set exactly with no
drops/duplicates, looped 30 times; failed reliably against the unfixed code, passes after the
`ID` tiebreak. Existing `TestListUserPools_Pagination` (`user_pools_config_test.go`) and
`TestListUserPools_Pagination_StaleCursor` (same file as the new test) both use distinct pool names
throughout (`pool-00`..`pool-04`, `listpools-stale-000`..`002`) and so could not have caught this;
`TestListUserPools_Pagination` additionally dedups by `Name` in its own assertion, which would have
masked an ID-level duplicate even had one occurred.

Gates: `go build ./services/cognitoidp/...`, `go vet ./services/cognitoidp/...`,
`go test -race -count=1 ./services/cognitoidp/...` (pass), `golangci-lint run
./services/cognitoidp/...` (0 issues). Work left uncommitted per this pass's instructions.

**2026-08-30 (gopherstack-r3pr fabricated-error-code audit, no code change)**:
`cmd/errcodeaudit` reports zero findings for this service — no invented error-code
literal detected. Given this package's history of shadowed duplicate op
registrations (27 removed in an earlier pass), independently checked whether any
of the 39 `*Ops[A-Z]?()` group functions feeding `dispatchTable()`
(`maps.Copy`, which silently lets a later group win) register the same op name
twice, including via `op*` constants a literal grep would miss. Built each group
map directly off a zero-value `*Handler` and diffed the 39 key sets against each
other (temporary diagnostic, not committed): 130 distinct op names, zero
collisions.

**2026-08-30 (gopherstack WrapOp-blind-spot re-scan, `cmd/reqfieldscan`)**:
`cmd/reqfieldscan` (added `aa4ec0ad2`) reported only 81/130 (62%) of this
service's dispatch table resolved, with the other 49 ops "unresolved" -- an
implausible number per that tool's own "treat low coverage as a measurement
bug" guidance, hand-confirmed as exactly that: this service defines a local
generic wrapper `wrapAccuracy[I,O](fn) service.JSONOpFunc { return
service.WrapOp(fn) }` (`handler.go:484`), so the map-literal call site the
tool's literal `sel.Sel.Name == "WrapOp"` check looks for is never present
for the 49 ops registered via `wrapAccuracy(...)` -- confirmed 1:1 (49
`wrapAccuracy(h.*)` call sites, 49 unresolved ops). A second, separate gap:
many of this service's handlers are named `handle<Op>Full`/`handle<Op
>Accurate`/`handle<Op>WithOpts` rather than exactly `handle<Op>`, which the
tool's own naming-convention resolver doesn't try. A scratch-only patched
copy of the tool (not committed; both gaps are specific to this service's
conventions, not upstream-worthy per the tool's own disclosed-blind-spot
policy) resolved all 130/130 and surfaced 6 flagged fields the unpatched
tool's 3-of-130 partial run could not have reached. Hand-verified each:

- **`CreateUserPool.MfaConfiguration` -- real bug, fixed.** Every other
  writer of `pool.MfaConfiguration` (`SetUserPoolMfaConfig`,
  `UpdateUserPoolWithOpts`) wires it through; `CreateUserPoolWithOpts`'s own
  `UserPoolOptions` struct had no field to carry it at all, so a pool
  created with `MfaConfiguration: "ON"` silently came back `OFF` until a
  separate `SetUserPoolMfaConfig`/`UpdateUserPool` call. Fixed by adding
  `MfaConfiguration` to `UserPoolOptions` and wiring
  `handleCreateUserPoolWithOpts`'s `opts` literal and
  `CreateUserPoolWithOpts`'s pool literal to it (`UpdateUserPoolWithOpts`
  already takes `mfaConfiguration` as an explicit positional param and does
  not read `opts.MfaConfiguration` -- left as is, no double-write path).
  Proof: `TestHandler_CreateUserPool_MfaConfiguration`
  (`user_pools_test.go`), confirmed failing (asserted "ON", got "OFF")
  against the unfixed code.
- **`AdminDisableProviderForUser.User` -- verified, not a bug.**
  `AdminDisableProviderForUser`'s own doc comment states this backend does
  not track federated identity provider links at all, and validates only
  that the pool exists (matching real AWS's behavior for an unknown provider
  link) -- reading `User` would have nothing to act on. Comment correctly
  explains the gap; not fixed.
- **`ConfirmDevice.DeviceSecretVerifierConfig` -- verified, structural, not
  fixed.** This SRP verifier config exists to support a later
  `DEVICE_SRP_AUTH` re-authentication flow; grepped the whole service for
  `DEVICE_SRP_AUTH` and found no such `AuthFlow` recognized anywhere
  (`InitiateAuth`/`AdminInitiateAuth` only handle `USER_SRP_AUTH`,
  `REFRESH_TOKEN_AUTH`, `ADMIN_USER_SRP_AUTH`), and the `Device` model has no
  field to store a verifier/salt in even if it were read. A whole
  unimplemented auth flow, not a narrow dropped-field fix.
- **`AdminRespondToAuthChallenge.UserPoolID` -- real gap, left at a layer
  boundary, not fixed.** Every challenge-response backend method this
  handler calls (`RespondToMFAChallenge`, `RespondToNewPasswordRequired`,
  `RespondToSRPChallenge`, `RespondToMFASetupChallenge`,
  `RespondToCustomAuthChallenge`) takes only `clientID`/`session`, never
  `userPoolID` -- contrast `AdminInitiateAuth`, whose sibling backend calls
  (`AdminInitiateAuthSRP`, `AdminInitiateAuth`) do take and use it to scope
  the user lookup. No pool-ownership validation exists anywhere in this
  package (grepped for a `ClientBelongsToPool`-shaped helper: none), so a
  caller presenting a `Session`/`ClientId` from one pool while claiming a
  different `UserPoolId` is not rejected. A correct fix means adding
  `userPoolID` to (and validating it in) five backend method signatures --
  crosses the handler/backend layer boundary, reported rather than fixed.
- **`VerifySoftwareToken.FriendlyDeviceName` -- verified, not fixed.** Real
  AWS treats this as a display-only label with no modeled behavioral effect
  either (it does not gate MFA behavior in the real API), and no device
  model in this service has a field to hold it. Lowest-priority of the five.
- **`ListUserPoolReplicas.NextToken` -- real minor gap, not fixed.** The
  handler returns every replica in one page regardless of `NextToken`/
  `MaxResults` and never issues a `NextToken` of its own. Low real-world
  impact -- `user_pool_replicas.go`'s own doc comment (trusted per this
  campaign's "a comment that gives a correct reason" guidance, and already
  cited by this file's own List-pagination sweep above) establishes at most
  one replica per region with no cross-region duplication path, so there is
  rarely more than a handful of items to page over in practice -- but the
  field is still genuinely wired on the wire and genuinely ignored.

**Re-derived collision/group count (previously recorded: 130 ops / 39 groups
/ zero collisions).** Re-ran the same style of check this pass (AST-walked
every `map[string]service.JSONOpFunc{...}` composite literal, resolving keys
through both string literals and `opX`-style string constants): **130
distinct operations across 41 registration groups, zero collisions** --
confirmed as of this pass; group count has drifted up to 41 (this file's own
count is exactly the kind of number that goes stale, as its own record notes
elsewhere) but the load-bearing claim, zero collisions, still holds.

Gates: `go build ./services/cognitoidp/...`, `go build ./...` (repo-wide,
clean), `go vet ./services/cognitoidp/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/cognitoidp/...` (pass),
`golangci-lint run ./services/cognitoidp/...` (0 issues). Work left
uncommitted per this pass's instructions.
