---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: cognitoidentity
sdk_module: aws-sdk-go-v2/service/cognitoidentity@v1.36.4
last_audit_commit: 81a1aabf0
last_audit_date: 2026-08-20
overall: A                # error-taxonomy field-diff vs deserializers.go found 3 real gaps, all fixed
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateIdentityPool: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed OpenIdConnectProviderARNs JSON key casing (prior pass); LimitExceededException deferred, see deferred[]"}
  DeleteIdentityPool: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades identities/roles/principalTags"}
  DescribeIdentityPool: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed OpenIdConnectProviderARNs JSON key casing (prior pass)"}
  ListIdentityPools: {wire: ok, errors: ok, state: ok, persist: ok, note: "name-cursor pagination verified"}
  UpdateIdentityPool: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed OpenIdConnectProviderARNs JSON key casing (prior pass); ConcurrentModificationException/LimitExceededException deferred. 2026-09-04: fixed DeveloperProviderName immutability -- see body note below"}
  GetId: {wire: ok, errors: ok, state: ok, persist: ok, note: "merges logins into existing identity per AWS semantics; ExternalServiceException/LimitExceededException deferred"}
  GetCredentialsForIdentity: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed Expiration epoch-seconds vs epoch-millis bug (prior pass); NEW this pass: InvalidIdentityPoolConfigurationException when the pool has no IAM role for the identity's auth state (real business-logic gap, not just an error-code omission -- GetCredentialsForIdentity previously handed out credentials for pools with zero role configuration)"}
  GetOpenIdToken: {wire: ok, errors: ok, state: ok, persist: n/a, note: "ExternalServiceException deferred"}
  SetIdentityPoolRoles: {wire: ok, errors: ok, state: ok, persist: ok, note: "partial-merge semantics (omitted keys preserved) — deliberately tested by prior Refinement2 pass, left as-is; ConcurrentModificationException deferred"}
  GetIdentityPoolRoles: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteIdentities: {wire: ok, errors: ok, state: ok, persist: ok, note: "best-effort silent skip of missing IDs matches AWS"}
  DescribeIdentity: {wire: ok, errors: ok, state: ok, persist: ok, note: "timestamps now routed through pkgs/awstime.Epoch (prior pass)"}
  GetOpenIdTokenForDeveloperIdentity: {wire: ok, errors: ok, state: ok, persist: ok, note: "PrincipalTags input field accepted by SDK but not consumed (see gaps). NEW this pass: an explicit IdentityId was previously validated for existence but its Logins were silently dropped instead of being linked (a disguised stub per parity principle #4) -- now actually links them, and rejects a developer-provider login already claimed by a different identity with DeveloperUserAlreadyRegisteredException"}
  GetPrincipalTagAttributeMap: {wire: ok, errors: ok, state: ok, persist: ok}
  ListIdentities: {wire: ok, errors: ok, state: ok, persist: ok, note: "timestamps now routed through pkgs/awstime.Epoch (prior pass)"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: n/a}
  LookupDeveloperIdentity: {wire: ok, errors: ok, state: ok, persist: n/a, note: "when both IdentityId and DeveloperUserIdentifier are supplied, they are cross-validated and a ResourceConflictException is returned on mismatch, per the operation's own doc comment (\"If you supply both, DeveloperUserIdentifier will be matched against IdentityId... Otherwise, a ResourceConflictException is thrown\"). 2026-08-23: fixed cross-pool identity leak -- IdentityId is now checked against IdentityPoolID like UnlinkDeveloperIdentity already does, see body note below"}
  MergeDeveloperIdentities: {wire: ok, errors: ok, state: ok, persist: ok}
  SetPrincipalTagAttributeMap: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UnlinkDeveloperIdentity: {wire: ok, errors: ok, state: ok, persist: ok}
  UnlinkIdentity: {wire: ok, errors: ok, state: ok, persist: ok, note: "ExternalServiceException deferred"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
families: {}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "IMPOSSIBLE (re-investigated gopherstack-tqdj): GetOpenIdTokenForDeveloperIdentity accepts a PrincipalTags request field (SDK-modeled, real doc comment: 'Use this operation to configure attribute mappings for custom providers.') but the backend never stores/applies it. Investigated two candidate real consumption points before concluding this: (1) GetCredentialsForIdentityInput (confirmed against api_op_GetCredentialsForIdentity.go) has NO Token/tag-related parameter at all -- only IdentityId/CustomRoleArn/Logins -- so PrincipalTags cannot flow through that op's wire surface no matter what gopherstack does internally; real AWS's actual use of PrincipalTags is to set STS session tags on the role-assumption Cognito performs *internally* on GetCredentialsForIdentity, which has no client-visible wire representation gopherstack could honestly populate without also faking IAM/STS session-tag enforcement this codebase doesn't model anywhere (see the separate 'session Policy/PolicyArns... not enforced' deferred item above). (2) Considered whether the OIDC token itself could carry the tags as a real https://aws.amazon.com/tags claim (mirroring services/sts's own GetWebIdentityToken, which does this) for a caller that hands the token to STS's AssumeRoleWithWebIdentity directly -- STS's WebIdentityToken parser (token_validation.go) is genuinely claim-driven and signature-verification-free, so this is *technically* wireable. Not implemented this pass: it would require replacing the current placeholder token format (a static JWT-shaped header + random payload + literal 'signature', see GetOpenIdToken/GetOpenIdTokenForDeveloperIdentity in credentials.go) with a real base64url JSON payload, is a materially larger change than an error-type fix, and no test or documented use case in this codebase currently chains a cognitoidentity-issued token into sts.AssumeRoleWithWebIdentity to exercise it -- speculative cross-service plumbing without a concrete consumer was judged too large/uncertain a change for this pass. Left as an honestly-documented gap, not fabricated."
  - "IMPOSSIBLE (re-investigated gopherstack-tqdj): SetIdentityPoolRoles/GetOpenIdTokenForDeveloperIdentity TokenDuration are accepted/validated (0-86400s range) but not enforced against issued token lifetime. Same root cause as the PrincipalTags item above: the returned token is an opaque synthetic string (credentials.go), not a real JWT with an exp claim, and no operation in this codebase currently re-validates staleness of a previously issued cognitoidentity OpenID token. Embedding a real TokenDuration-derived exp claim would require the same token-format rework discussed above, for the same currently-hypothetical consumer -- not implemented this pass for the same reason."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - HTTP status code choice for NotAuthorizedException (403 here) vs AWS's actual per-exception status (SDK error-type resolution is body-driven, not status-code-driven, so this doesn't break aws-sdk-go-v2 clients; only relevant to tooling that inspects raw HTTP status).
  - "ALREADY COVERED BY CHAOS (verified gopherstack-tqdj): ConcurrentModificationException (SetIdentityPoolRoles, UpdateIdentityPool per deserializers.go) is not emulated: there is no optimistic-concurrency/version token in this backend's resource model to make a genuine concurrent-write collision detectable, and fabricating one that never fires (or fires on arbitrary heuristics) would be worse than omitting it. Would need a real revision-counter field added to IdentityPool/IdentityRoles to do properly -- out of scope for an error-taxonomy pass. Concretely verified this pass: cognitoidentity.Handler implements ChaosServiceName() -> \"cognito-identity\" and ChaosOperations() -> h.GetSupportedOperations() (handler.go), and pkgs/chaos.Middleware is wired globally via registry.Use(chaos.Middleware(faultStore)) in cli.go, matching purely on the request's SigV4 service name + X-Amz-Target operation + region and injecting an arbitrary caller-specified FaultError{Code, StatusCode} without touching backend state -- a fault rule such as {\"service\":\"cognito-identity\",\"operation\":\"UpdateIdentityPool\",\"error\":{\"code\":\"ConcurrentModificationException\",\"statusCode\":400}} deterministically returns that exact typed error to a real client with zero backend code changes."
  - "ALREADY COVERED BY CHAOS (verified gopherstack-tqdj): TooManyRequestsException (every op) and LimitExceededException (CreateIdentityPool/GetId/UpdateIdentityPool per deserializers.go) are throttling/account-quota conditions; this in-memory emulator has no request-rate tracking and AWS's actual per-account pool/identity quotas are account-specific soft limits, not fixed constants -- inventing an arbitrary hard-coded threshold would be a fabricated business rule, not a verified one. Left unimplemented, consistent with how other gopherstack services treat throttling. Same chaos mechanism as ConcurrentModificationException above makes both reachable on demand with zero backend code changes."
  - "ALREADY COVERED BY CHAOS (verified gopherstack-tqdj): ExternalServiceException (GetCredentialsForIdentity, GetId, GetOpenIdToken, UnlinkIdentity per deserializers.go) is AWS's wrapper for a real external identity provider (Facebook/Google/a linked Cognito user pool) rejecting a token. This backend validates login tokens against its own stored state, not a real external IdP, so there is no authentic backend-state trigger condition for this exception here. Same chaos mechanism as above makes it reachable on demand with zero backend code changes."
leaks: {status: clean, note: "no goroutines/janitors in this service; single lockmetrics.RWMutex guards all store.Table access"}
---

## Notes

- Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: AWSCognitoIdentityService.<Op>`
  dispatch. Confirmed target prefix and `Content-Type: application/x-amz-json-1.1` against the
  real serializer output (`serializers.go`).

- **Bug fixed — Expiration wire format (epoch seconds, not milliseconds).**
  `GetCredentialsForIdentity`'s `Credentials.Expiration` was serialized via
  `creds.Expiration.UnixMilli()` into an `int64` field. The real deserializer
  (`aws-sdk-go-v2/service/cognitoidentity` `deserializers.go`, case `"Expiration"`) calls
  `smithytime.ParseEpochSeconds(f64)`, i.e. the wire value is a JSON number of **seconds**
  since epoch (fractional allowed), exactly matching `pkgs/awstime.Epoch`'s contract. A
  prior "accuracy" pass (`accuracy_test.go` Gap 7) got this backwards and had asserted the
  *wrong* (millisecond) behavior as correct — the test has been corrected to assert
  epoch-seconds and to bound the value against a ~1-hour-from-now expiry. `DescribeIdentity`
  and `ListIdentities`' `CreationDate`/`LastModifiedDate` were already numerically equivalent
  (hand-rolled `UnixMilli()/1000.0`) but have been switched to `awstime.Epoch` for consistency
  and to remove the now-redundant `millisPerSecond` local constant, per the pkgs-reuse rule.

- **Bug fixed — `OpenIdConnectProviderARNs` JSON key casing.** The real wire key (confirmed in
  both `serializers.go` and `deserializers.go`) is `OpenIdConnectProviderARNs` (lowercase `d`
  in `Id`). The handler emitted `OpenIDConnectProviderARNs` (uppercase `ID`) on
  `CreateIdentityPool`/`DescribeIdentityPool`/`UpdateIdentityPool` responses. Go's
  `encoding/json.Unmarshal` is case-insensitive, so *incoming* requests using either casing
  still decoded correctly (that direction was not actually broken), but `json.Marshal` is
  case-exact, so a real aws-sdk-go-v2 client's deserializer switch (`case
  "OpenIdConnectProviderARNs":`) would never match our old key and would silently drop the
  field from every response. Fixed the three JSON tags (Go field identifiers unchanged); the Go
  field name itself is a private implementation detail and does not need to match the wire key.

- `SetIdentityPoolRoles` uses partial-merge semantics: a key omitted from the request's `Roles`
  map (or a nil `RoleMappings`) leaves the existing stored value untouched rather than clearing
  it. This looked like a possible "should be full-replace, like most AWS `Set*` ops" bug, but it
  is deliberately covered by three dedicated tests from an earlier audit pass
  (`TestInMemoryBackend_Refinement2_SetIdentityPoolRoles_MergePreservesExistingRole`,
  `TestHandler_Refinement2_SetIdentityPoolRoles_PreservesExistingRole`,
  `TestAccuracy_SetIdentityPoolRoles_RoleMappingsNilPreservesExisting`) — left as-is this pass;
  flagging here so a future auditor with a way to verify against real AWS doesn't waste time
  re-discovering the same fork in the road.

- Sentinel-error → HTTP-status mapping: `ResourceNotFoundException`/`ResourceConflictException`/
  `InvalidParameterException` → 400, `NotAuthorizedException` → 403. Verified the real SDK's
  client-side error-type resolution (`deserializers.go`) only checks `response.StatusCode < 200
  || >= 300` and then dispatches purely on the body's error-type string — the exact status code
  used here doesn't affect aws-sdk-go-v2 client behavior either way, so this was left alone
  (deferred, not a proven bug).

- Persistence: `Handler.Snapshot`/`Restore` delegate to `InMemoryBackend.Snapshot`/`Restore`
  (persistence.go), which round-trip all four `store.Table` collections (pools, identities,
  roles, principalTags) through region-qualified DTOs. Verified present and wired correctly;
  not modified this pass.

- Region isolation: every resource is keyed by composite `"region|id"` (`regionKey` in
  backend.go) via `store.Table`/`store.Index`, with per-request region resolved from
  `X-Amz-Region`/SigV4 via `regionContextKey`. Verified consistent across all ops.

## 2026-08-22 — gopherstack-jodk: SetIdentityPoolRoles rejected a legal empty Roles map

Found via PR #2433's `terraform-tests (2)` CI shard, which reproduces against
the real terraform AWS provider: `tofu destroy` on an
`aws_cognito_identity_pool_roles_attachment` sends `SetIdentityPoolRoles`
with a non-nil, zero-length `Roles` map to clear the association, and
gopherstack rejected it with `InvalidParameterException: Roles must contain
at least one of authenticated or unauthenticated`
(`handler_identity_pool_roles.go:43`), so the destroy could never complete.
Verified against the pinned SDK's own client-side validator
(`cognitoidentity@v1.36.4/validators.go:929`,
`validateOpSetIdentityPoolRolesInput`): it checks only `v.Roles == nil`,
never its length, so a real client is free to send an empty-but-non-nil map.
Fixed by replacing the `len(in.Roles) == 0` rejection with `in.Roles == nil`.
The pre-existing partial-merge semantics in `identity_pool_roles.go`
(omitted keys preserved, tested by `TestInMemoryBackend_SetIdentityPoolRoles_MergePreservesExistingRole`)
were left untouched — out of this bug's scope.

Not a regression: `handler_identity_pool_roles.go` has no commits since this
branch's merge-base with `origin/main`. Proven with a real aws-sdk-go-v2
client test (`sdk_set_identity_pool_roles_empty_test.go`) that reproduces
the exact reported error verbatim against unfixed code and passes against
the fix. This is gopherstack-4ly2's over-validation class, caught on a
destroy path that a static validator-reading sweep does not reach.

## 2026-08-21 pass — required-output-member sweep (gopherstack-r80d batch 27)

Module confirmed as `aws-sdk-go-v2/service/cognitoidentity@v1.36.4` directly
(no `dirModuleOverride` entry; the sibling `cognitoidp` directory resolves to
the distinct `cognitoidentityprovider` module, not this one — checked
against `go.mod` and the module cache before starting, since batch 24 nearly
audited an already-settled near-name sibling for a different service).

`cmd/requiredoutputfields` flags 9 required output fields across 3 ops
(`CreateIdentityPool`/`DescribeIdentityPool`/`UpdateIdentityPool`, all
sharing `identityPoolOutput`): `AllowUnauthenticatedIdentities` (`bool`,
non-pointer, not provable), `IdentityPoolId`/`IdentityPoolName` (`*string`,
both provable). Independently reproduced via a fresh `go/parser` AST walk
(scratch tool, not committed) with zero disagreement against the CLI tool's
own output.

**Result: 0 bugs — clean.** All three required fields on
`identityPoolOutput` (`handler_identity_pools.go`) carry no `omitempty` tag,
so the wire key is emitted unconditionally regardless of value; the omission
this bug class targets cannot occur here regardless of reachability.

Looked one level deeper per the campaign's "look deeper" requirement: an AST
walk over `types/types.go` found exactly 3 domain structs anywhere in this
module carrying `This member is required.` members outside the 3 flagged
ops — `RoleMapping.Type` (enum, not provable), `RulesConfigurationType.Rules`
(`[]MappingRule`, provable), `MappingRule`'s own 4 members (all provable or
enum) — all reachable only through `GetIdentityPoolRoles`/
`SetIdentityPoolRoles` (0 required fields at the op level, hence invisible
to the flat scan). Read `handler_identity_pool_roles.go` end to end:
`roleMappingOutput.Type` and `rulesConfigurationOutput.Rules` both carry no
`omitempty` either, so the same "always emitted" conclusion holds one level
down too. Every other op's `*Output` struct (`GetCredentialsForIdentity`,
`GetId`, `DescribeIdentity`, `ListIdentities`, `GetOpenIdToken`,
`GetOpenIdTokenForDeveloperIdentity`, `LookupDeveloperIdentity`,
`MergeDeveloperIdentities`, `DeleteIdentities`, `ListTagsForResource`) was
confirmed to carry zero required members and reference no domain struct that
does (`Credentials`, `IdentityDescription`, `UnprocessedIdentityId` — all
declare zero required members in `types.go`).

`last_audit_commit` left unchanged (agents in this sweep cannot run git);
this pass found no bug to justify advancing it regardless.

## 2026-07-24 pass — error-taxonomy field-diff

Prior passes field-diffed wire *shapes* thoroughly but never field-diffed the *error
taxonomy* against `aws-sdk-go-v2/service/cognitoidentity`'s `deserializers.go`, which encodes
-- per operation, in its `awsAwsjson11_deserializeOpError<Op>` functions -- the exact set of
`strings.EqualFold("<ExceptionName>", errorCode)` cases the real client recognizes. Extracted
that table this pass (`awk` over `deserializers.go`) and diffed it against
`cognitoIdentitySentinelErrors` in `handler.go`, which only implemented 4 of the 11 modeled
exception types (`ResourceNotFoundException`/`ResourceConflictException`/
`InvalidParameterException`/`NotAuthorizedException`). Findings:

- **Bug fixed — generic-error fallback used the wrong wire type.** `resolveErrorType`'s
  catch-all returned Query/EC2-protocol-style `"InternalFailure"`, which does not match any
  case in *any* of cognitoidentity's 24 per-operation error switches (every one of them
  recognizes `"InternalErrorException"` instead, confirmed by grepping every
  `awsAwsjson11_deserializeOpError*` function). A real aws-sdk-go-v2 client hitting this path
  would fall through to an untyped smithy API error instead of a typed
  `*types.InternalErrorException`, breaking typed-exception-matching retry logic. Same bug
  class previously found and fixed in bedrockruntime (see that service's PARITY.md). Fixed:
  `resolveErrorType`'s fallback now returns `"InternalErrorException"`.

- **Bug fixed — `LookupDeveloperIdentity` silently ignored `DeveloperUserIdentifier` when
  `IdentityId` was also supplied.** The op's own doc comment in `api_op_LookupDeveloperIdentity.go`
  states: "Either IdentityID or DeveloperUserIdentifier must not be null. If you supply only
  one of these values, the other value will be searched... and returned... If you supply
  both, DeveloperUserIdentifier will be matched against IdentityID... Otherwise, a
  ResourceConflictException is thrown." The backend only ever branched on `identityID != ""`
  first and never even looked at `developerUserIdentifier` in that case -- so a caller
  supplying a mismatched pair got a silent (wrong) success instead of a conflict. Fixed by
  resolving both lookups independently and reconciling them (`reconcileLookupMatch`); added
  the new `ErrResourceConflict` sentinel (wire type `ResourceConflictException`, distinct
  from the pool-name-collision `ErrIdentityPoolAlreadyExists` which shares the same wire
  type per AWS, as multiple conditions can map to one exception type).

- **Bug fixed — `GetOpenIdTokenForDeveloperIdentity` dropped logins when `IdentityId` was
  explicit (disguised stub).** The op's doc comment says it "can also be used to... link new
  logins... to an existing... identity, by providing the existing IdentityId." The backend
  validated the identity existed but then discarded `logins` entirely instead of merging them
  -- looked like real logic (existence check + real state read) but silently no-opped the
  actual linking work, matching parity-principles.md's "disguised stub" pattern. Fixed with
  `linkDeveloperLogins`, which also implements the previously entirely-unimplemented
  `DeveloperUserAlreadyRegisteredException`: rejects linking a developer-provider login that's
  already registered to a *different* identity in the pool (checked via the pool's
  `DeveloperProviderName`, since `Logins` may also carry non-developer provider entries).

- **Bug fixed — `GetCredentialsForIdentity` never checked identity-pool role
  configuration.** Real AWS returns `InvalidIdentityPoolConfigurationException` when the pool
  has no IAM role for the identity's auth state; gopherstack happily minted synthetic
  credentials for pools with zero roles configured (or missing the specific auth/unauth role
  needed), which is a real, common, user-visible AWS error condition ("Invalid identity pool
  configuration. Check assigned IAM roles for this pool."). Fixed via `checkRoleConfigured`,
  called after login-token validation (so `NotAuthorizedException` still takes precedence for
  mismatched tokens, matching existing precedence). This is a real backend-state check
  (`b.rolesGet`), not a fabricated rule -- it required touching ~7 existing tests that
  previously called `GetCredentialsForIdentity` without ever configuring pool roles, which
  is not how a real AWS caller could ever have gotten credentials in the first place.

- **Deferred, with rationale recorded above:** `ConcurrentModificationException` (no
  optimistic-concurrency model to make authentic), `TooManyRequestsException`/
  `LimitExceededException` (no request-rate tracking; AWS's real limits are account-specific
  soft quotas, not constants safe to hard-code), `ExternalServiceException` (wraps a real
  external IdP's rejection; this backend has no real external IdP to fail). All three
  categories were deliberately *not* implemented with fabricated trigger conditions, per the
  no-stub/no-invented-business-rules principle -- an exception type with no real,
  state-driven trigger condition is worse than an honestly-documented gap.

## 2026-08-20 pass — wrapper-key / nested-shape sweep (clean)

Campaign-wide sweep for response members emitted under the wrong wrapper key, wrong
nesting level, wrong JSON type, or wrong enum value (the bug class found ~52 times across
22 other services this campaign, dominant pattern: a Go struct shared between a
request-side and response-side sibling leaking request-only fields). Verified every
op's own `awsAwsjson11_deserializeOpDocument<Op>Output`/`deserializeDocument<Type>` case
list in the pinned `aws-sdk-go-v2/service/cognitoidentity@v1.36.4` `deserializers.go`
against gopherstack's emitted JSON tags, field-by-field, and the enum value sets in
`types/enums.go`. Zero new bugs found.

- `GetCredentialsForIdentity`: `Credentials` deserializer (`deserializers.go:3341`) case
  list is exactly `AccessKeyId`, `Expiration` (epoch-seconds `json.Number`), `SecretKey`,
  `SessionToken`. gopherstack's `credentialsOutput` (`handler_credentials.go:16-20`) emits
  `AccessKeyId`/`SecretKey`/`SessionToken`/`Expiration` with `SecretKey` correctly sourced
  from the internal `SecretAccessKey` Go field via a `json:"SecretKey"` tag -- the
  STS-`SecretAccessKey`-vs-Cognito-`SecretKey` sibling trap this campaign keeps finding
  elsewhere does NOT reproduce here; already correct.
- `IdentityPool` (full) vs `IdentityPoolShortDescription`: distinct wire structs.
  `DescribeIdentityPoolOutput`'s own deserializer (`deserializers.go`, verified via
  `awsAwsjson11_deserializeOpDocumentDescribeIdentityPoolOutput`) case list --
  `AllowClassicFlow`, `AllowUnauthenticatedIdentities`, `CognitoIdentityProviders`,
  `DeveloperProviderName`, `IdentityPoolId`, `IdentityPoolName`, `IdentityPoolTags`,
  `OpenIdConnectProviderARNs`, `SamlProviderARNs`, `SupportedLoginProviders` -- matches
  gopherstack's `identityPoolOutput` (`handler_identity_pools.go`) field-for-field, same
  struct shared correctly across Create/Describe/Update (all three ops have an identical
  Output shape in the real SDK too, confirmed by reading each). `ListIdentityPools`' own
  `awsAwsjson11_deserializeDocumentIdentityPoolShortDescription` case list is only
  `IdentityPoolId`/`IdentityPoolName`; gopherstack's separate `identityPoolShortDescription`
  struct carries only those two fields -- no leakage of the full-pool's extra fields into
  the summary side (pattern (a), the dominant bug class this campaign, does not reproduce).
- `RoleMappings` nested tree, verified level by level against
  `awsAwsjson11_deserializeDocumentRoleMapping`/`RulesConfigurationType`/`MappingRule`:
  `RoleMappings` (map) -> `RoleMapping{Type, AmbiguousRoleResolution, RulesConfiguration}`
  -> `RulesConfigurationType{Rules}` -> `MappingRule{Claim, MatchType, RoleARN, Value}`.
  gopherstack's `roleMappingInput`/`roleMappingOutput` and
  `rulesConfigurationInput`/`Output` (`handler_identity_pool_roles.go`) match every key
  name and nesting level exactly. Enum values (`RoleMappingType`: `Token`/`Rules`;
  `AmbiguousRoleResolutionType`: `AuthenticatedRole`/`Deny`; `MappingRuleMatchType`:
  `Equals`/`Contains`/`StartsWith`/`NotEqual`, all from `types/enums.go`) are never
  fabricated by the backend -- `SetIdentityPoolRoles`/`GetIdentityPoolRoles` echo whatever
  value the caller supplied verbatim (`identity_pool_roles.go`'s `cloneRoleMappings`), so
  there is no server-side opportunity to invent an out-of-enum constant (the swf `Cause`
  bug class does not apply to a pure pass-through field).
- `IdentityDescription` (`ListIdentities`/`DescribeIdentity`): real case list
  `CreationDate`/`IdentityId`/`LastModifiedDate`/`Logins`, timestamps as epoch-seconds.
  gopherstack's `identityDescriptionOutput`/`describeIdentityOutput` match, both routed
  through `pkgs/awstime.Epoch`.
- Also diffed: `GetId`, `DeleteIdentities`/`UnprocessedIdentityId` (real `ErrorCode` enum
  is only `AccessDenied`/`InternalServerError` -- gopherstack's backend never actually
  populates the unprocessed list, so no fabricated value is possible there either),
  `GetOpenIdToken`, `GetOpenIdTokenForDeveloperIdentity`, `LookupDeveloperIdentity`,
  `MergeDeveloperIdentities`, `GetPrincipalTagAttributeMap`/`SetPrincipalTagAttributeMap`,
  `ListTagsForResource`/`TagResource`/`UntagResource`. All field-for-field clean.
- Confirmed protocol independently: `awsAwsjson11_*` prefix in `serializers.go`,
  `X-Amz-Target: AWSCognitoIdentityService.<Op>` header
  (`serializers.go:59` etc.), matching this file's existing protocol note. Confirmed the
  restjson single-structure-member false-positive trap does not apply (JSON-RPC always
  routes through `deserializeOpDocument<Op>Output`, verified present AND called -- 2
  occurrences -- for every op with a non-empty output; the 4 ops with a `0` count
  (`SetIdentityPoolRoles`, `UnlinkDeveloperIdentity`, `UnlinkIdentity`,
  `DeleteIdentityPool`) are legitimately void-output ops, not a missed wiring).
- `last_audit_commit` provenance check: prior value `2d47b51d4` is EC2's
  `RestoreImageFromRecycleBin` fix, dated `Wed Jul 29 22:13:36 2026`, matching the prior
  `last_audit_date: 2026-07-29` exactly -- no copy-paste drift, and NOT the `40f05928`
  sha seen clustered on other services this campaign. Updated to `81a1aabf0` (HEAD at the
  time of this pass) / `2026-08-20`.
- No existing test asserted a wrong key/nesting/type/value; none needed correction.
  `handler_credentials_test.go:192` already asserts the raw wire key `SecretKey` (not
  `SecretAccessKey`), and `handler_identity_pool_roles_test.go` already asserts real enum
  values (`"Token"`, `"AuthenticatedRole"`) end-to-end through the router.
- No fixes made this pass -> no new round-trip test added (nothing to prove via
  hand-revert). Gates: `go build`/`go vet`/`go fix -diff`/`gofmt -l` clean,
  `go test -race ./services/cognitoidentity/...` passes, `golangci-lint run
  ./services/cognitoidentity/...` reports 0 issues.

## 2026-08-23 — owner-scoping sweep: LookupDeveloperIdentity leaked identities across pools

`LookupDeveloperIdentity` requires `IdentityPoolId` and accepts an optional
`IdentityId`, but when `IdentityId` was supplied it was resolved via
`identityGet(region, identityID)` -- a region-wide lookup keyed only on the
identity ID, never checked against the given `IdentityPoolId`. Any caller
who knew (or brute-forced) another pool's identity ID could pass their own,
unrelated pool's ID and read that identity's `DeveloperUserIdentifierList`
(the backend-issued developer user identifiers linked to it) -- a
cross-principal information disclosure.

`UnlinkDeveloperIdentity`, ~130 lines below in the same file
(`identities.go`), already does this correctly:
`if identity.IdentityPoolID != poolID { return ...ErrIdentityPoolNotFound... }`.
`LookupDeveloperIdentity` was the one op in the family that skipped it.

Confirmed against the pinned SDK
(`aws-sdk-go-v2/service/cognitoidentity@v1.36.4/api_op_LookupDeveloperIdentity.go`):
`IdentityPoolId` is a required input member; `IdentityId` has no
independent scope of its own in the real API (there is no
`DeveloperProviderName` field on the real wire input either, despite
gopherstack's `lookupDeveloperIdentityInput` carrying one -- a separate,
pre-existing wire-shape mismatch not touched by this fix; real traffic
never sends it, so it doesn't change LookupDeveloperIdentity's behavior for
a genuine client).

Fixed by adding the same `identity.IdentityPoolID != poolID` check, same
error and message shape as `UnlinkDeveloperIdentity`
(`ErrIdentityPoolNotFound`, `"identity %q not found in pool %q"`).

Proof: `sdk_lookup_developer_identity_cross_pool_test.go` --
`TestLookupDeveloperIdentity_CrossPoolIdentityID_RealClient` -- creates two
pools with a real `aws-sdk-go-v2` client, links a developer identity to
pool A via `GetOpenIdTokenForDeveloperIdentity`, then calls
`LookupDeveloperIdentity` with pool B's ID and pool A's `IdentityId`.
Failed against unfixed code (`An error is expected but got nil`, plus a
custom message showing the leaked cross-pool identity/pool triple); passes
after the fix with `*types.ResourceNotFoundException`. Hand-reverted
`identities.go` via `cp` from `git show HEAD:...`, reran the test (failed,
confirming reproduction), restored the fix, `md5sum` confirmed byte-identical
to the pre-revert file. `go test ./services/cognitoidentity/...` and
`go test ./pkgs/persistence/...` both pass (no persisted struct changed).

Update `ops.LookupDeveloperIdentity.note` above: now also cross-validates
`IdentityId` against the supplied `IdentityPoolId`, returning
`ResourceNotFoundException` on a pool/identity mismatch, matching
`UnlinkDeveloperIdentity`'s existing behavior.

## 2026-09-04 pass — gopherstack-13d parity sweep

Audited for the standard campaign bug patterns (missing delete preconditions, ghost
rows after delete, inert config, discarded parameters, unreachable code, fabricated
values, stale caches, resource leaks), plus the sweep's specific lead: whether the
cognitoidp "user-pool side maps" ghost-row class (bd gopherstack-cq0z) reproduces here,
and whether identity pools referencing cognitoidp user pools go stale.

- **Ghost rows: does not reproduce.** All four resource collections (`pools`,
  `identities`, `roles`, `principalTags`, `store_setup.go`) are `store.Table`-backed,
  which `pkgs/pkgs-catalog.md` and `parity-principles.md` note is structurally immune to
  the hand-rolled-map ghost-row class. Grepped every non-test `map[string]` in the
  package (`credentials.go`, `identities.go`, `identity_pools.go`,
  `identity_pool_roles.go`, `tags.go`, `models.go`) -- none is a side table outside a
  `store.Table`; `pool.Tags` and `identity.Logins` are plain fields on the owning struct,
  deleted with their owner. `DeleteIdentityPool` (`identity_pools.go:78`) cascades
  `roles`, every identity in the pool, and every principal-tag mapping for the pool --
  confirmed correct, matches the existing `note`.
- **Cross-service cognitoidp linkage: does not exist, so nothing to go stale.**
  `CognitoIdentityProviders`/`IdentityProviders` (`ProviderName`/`ClientId`/
  `ServerSideTokenCheck`) is stored and echoed verbatim (`identity_pools.go`,
  `handler_identity_pools.go`) but never dereferenced against `services/cognitoidp`
  state anywhere in either package (confirmed via
  `grep -rn cognitoidentity services/cognitoidp` and
  `grep -rn cognitoidp services/cognitoidentity`, both empty outside this note). Login
  token validation against a real external/user-pool IdP is the already-documented
  `ExternalServiceException` gap (deferred above, `GetId`/`GetCredentialsForIdentity`/
  `GetOpenIdToken`/`UnlinkIdentity`) -- there is no code path here that reads a
  cognitoidp user pool's existence, so a deleted user pool cannot leave a ghost
  reference; the field is inert pass-through data end to end, same as the already-known
  `PrincipalTags`/`TokenDuration` IMPOSSIBLE items above.
- **Bug fixed -- `UpdateIdentityPool` let `DeveloperProviderName` be silently changed
  after being set.** `api_op_CreateIdentityPool.go:74`: "Once you have set a developer
  provider name, you cannot change it. Please take care in setting this parameter."
  `identity_pools.go:224` (before fix) unconditionally overwrote
  `pool.DeveloperProviderName` whenever the caller supplied any non-empty value on
  Update, with no check for whether the pool already had one -- so a second
  `UpdateIdentityPool` call with a different value silently violated the documented
  invariant instead of being a no-op. Fixed by additionally requiring
  `pool.DeveloperProviderName == ""` before accepting the new value, so a value can
  still be set for the first time via Update (nothing in either op's doc restricts
  first-set to Create only), but an already-set value can never change -- matching "you
  cannot change it" literally rather than inventing a specific rejection error code the
  SDK doesn't name for this case. Proof:
  `TestInMemoryBackend_UpdateIdentityPool_DeveloperProviderNameImmutable`
  (`identity_pools_test.go`) creates a pool with `DeveloperProviderName:
  "developer.myapp.com"`, calls `UpdateIdentityPool` with `"developer.other.com"`, and
  asserts the value is still `"developer.myapp.com"` both from the Update return value
  and from a follow-up `DescribeIdentityPool`. Hand-reverted the guard in
  `identity_pools.go` (confirmed the substitution matched exactly once via a Python
  `assert count==1`), reran: failed with `expected: "developer.myapp.com", actual:
  "developer.other.com"` (both assertions), confirming real reproduction; restored the
  fix and `md5sum`-confirmed the restored file byte-identical to the pre-revert version.
- **Not fixed / no bug found:** `GetIdentityPoolRoles` (`identity_pool_roles.go:48`)
  returns `cp := *roles` -- a shallow struct copy that still aliases the backend's live
  `RoleMappings` map with the caller's returned value, unlike `clonePool`/`cloneIdentity`
  which deep-clone every mutable field. Traced every current call site
  (`handler_identity_pool_roles.go`'s `handleGetIdentityPoolRoles`, and every test): none
  mutates the returned `RoleMappings` map, and `SetIdentityPoolRoles` always replaces the
  field with a fresh clone (`existing.RoleMappings = cloneRoleMappings(...)`,
  `identity_pool_roles.go:41`) rather than mutating map contents in place, so this
  aliasing is not observable through any AWS-wire behavior today. This is a Go-API
  internal-hygiene inconsistency, not a spec-verifiable AWS parity gap (the SDK has
  nothing to say about gopherstack's internal aliasing) -- left unfixed per "fix only
  what the SDK unambiguously specifies," noted here so a future pass doesn't rediscover
  it from scratch. Filing as a low-priority follow-up is reasonable but out of scope for
  this pass.

Gates run: `GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/cognitoidentity/...`
(pass), `GOTOOLCHAIN=go1.26.6 golangci-lint run ./services/cognitoidentity/...` (`0
issues.`), and the campaign's required dependent-package check
`GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/cloudformation/...
./services/cognitoidp/... ./services/sts/...` (all pass). `last_audit_commit` left at
`81a1aabf0` per this repo's convention of only advancing it alongside a settlement of
the *entire* file's audit state; the fix is dated in this section instead.

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `Id`/`ID` (`OpenId`/`OpenID`) acronym casing
gives it 3 op/handler pairs needing the ambiguous fold, 3 of them
genuine collisions between an exported backend method and the real
unexported handler: `GetId`, `GetOpenIdToken`, `GetOpenIdTokenForDeveloperIdentity`.

Verified directly rather than assumed: ran the unpatched tool from
`ef0eef041~1` five times and diffed against the fixed tool at HEAD, for
both `cmd/reqfieldscan` and `cmd/reqfielddiff`. Both were byte-identical
across all 5 old runs and HEAD (23 SDK operations compared) -- the
determinism defect never flipped a finding here, because the resolution
that actually mattered (this package's dispatch-table union) already
carried the correct field set regardless of which fold candidate won.

Verdict: confirmed zero damage, not merely predicted.
