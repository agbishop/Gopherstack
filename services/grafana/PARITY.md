---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: grafana
sdk_module: aws-sdk-go-v2/service/grafana@v1.38.4
last_audit_commit: e75a8cecd   # HEAD at this audit pass; diff from here forward
last_audit_date: 2026-08-20
# Grade A: this pass added the integration suite that is the only accepted parity proof
# (.claude/memories/parity-principles.md rule 3 -- test/integration/grafana_test.go, driving
# every operation through a real aws-sdk-go-v2 client against a live container) and closed
# every buildable gap the prior B-grade audit had left open: real per-account quota
# tracking (ServiceQuotaExceededException on CreateWorkspace), chaos-injectable *_FAILED/
# DEGRADED transitions, and genuine cross-service validation of WorkspaceRoleArn/
# VpcConfiguration/WorkspaceOrganizationalUnits/SSO permission grants against this
# emulator's own IAM/EC2/Organizations/SSO Admin/Identity Store backends. See "Notes" below
# for the mechanism and what's now confirmed still out of reach.
#
# Prior B-grade note, still accurate for the 25 operations' wire shapes: every one was read
# directly from serializers.go/deserializers.go (never assumed from the Go struct field names
# alone -- two real traps were only visible there: AssociateLicense's GrafanaToken is an HTTP
# header, not a body/query field, and ListVersions' workspaceId is the query param
# "workspace-id" with a hyphen, unlike every other operation's "workspaceId"). Real SDK
# round-trip tests (services/grafana/sdk_roundtrip_helper_test.go, following
# services/databrew's pattern) caught two further wire bugs before they shipped:
# AssociateLicense and DisassociateLicense's "not in a valid state" cases were initially
# modeled as ConflictException, but those two operations' own
# deserializeOpErrorAssociateLicense/DisassociateLicense functions do not list
# ConflictException among the exception shapes they recognize -- a real caller's
# errors.As(err, &types.ConflictException{}) would silently never match. Fixed: AssociateLicense
# now reports ValidationException for that case, and DisassociateLicense treats
# "no license to remove" as an idempotent no-op rather than inventing a wire-incompatible
# error. See "Errors" section below for the full per-operation exception-type table this was
# built from.
overall: A
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
# All 25 ops are now routed, modeled with real backend state, and persisted via
# InMemoryBackend.Snapshot/Restore (services/grafana/persistence.go).
ops:
  CreateWorkspace: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /workspaces; validates WorkspaceRoleArn/VpcConfiguration/WorkspaceOrganizationalUnits against IAM/EC2/Organizations (cross_service.go), enforces the real 5-workspace-per-account quota (ServiceQuotaExceededException), CREATING -> ACTIVE or a chaos-injected CREATION_FAILED after a 100ms simulated delay (workspaces.go)"}
  DescribeWorkspace: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /workspaces/{workspaceId}"}
  UpdateWorkspace: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /workspaces/{workspaceId}; same cross-service validation as CreateWorkspace, merges onto existing state, requires ACTIVE/DEGRADED, UPDATING -> ACTIVE or a chaos-injected UPDATE_FAILED/DEGRADED"}
  DeleteWorkspace: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /workspaces/{workspaceId}; cascades apiKeys/serviceAccounts/tokens/permissions synchronously (workspace_update.go); a chaos-injected fault reports DELETION_FAILED without deleting instead"}
  ListWorkspaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /workspaces, paginated via pkgs/page"}
  DescribeWorkspaceAuthentication: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /workspaces/{workspaceId}/authentication"}
  UpdateWorkspaceAuthentication: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /workspaces/{workspaceId}/authentication; validates IdpMetadata url-xor-xml and SAML-requires-samlConfiguration"}
  DescribeWorkspaceConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /workspaces/{workspaceId}/configuration; opaque JSON blob stored/returned verbatim"}
  UpdateWorkspaceConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /workspaces/{workspaceId}/configuration; grafanaVersion upgrade-only, validated against versions.go's static list; VERSION_UPDATING -> ACTIVE or a chaos-injected VERSION_UPDATE_FAILED/DEGRADED"}
  AssociateLicense: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /workspaces/{workspaceId}/licenses/{licenseType}; GrafanaToken read from Grafana-Token HEADER, not body -- see handler_license.go; UPGRADING -> ACTIVE or a chaos-injected UPGRADE_FAILED/DEGRADED"}
  DisassociateLicense: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /workspaces/{workspaceId}/licenses/{licenseType}; idempotent no-op when nothing to remove (no ConflictException on this op's wire)"}
  ListVersions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /versions; workspaceId query param is \"workspace-id\" (hyphenated), confirmed via serializers.go -- not \"workspaceId\""}
  ListPermissions: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /workspaces/{workspaceId}/permissions, paginated, filterable by groupId/userId/userType"}
  UpdatePermissions: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH /workspaces/{workspaceId}/permissions; real partial-failure batch -- a malformed instruction (empty users) or an ADD referencing an SSO_USER/SSO_GROUP ID absent from the account's IAM Identity Center identity store (cross_service.go) lands in Errors, valid instructions in the same batch still apply"}
  CreateWorkspaceApiKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /workspaces/{workspaceId}/apikeys; SecondsToLive validated 1..2592000 (30 days)"}
  DeleteWorkspaceApiKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /workspaces/{workspaceId}/apikeys/{keyName}"}
  CreateWorkspaceServiceAccount: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /workspaces/{workspaceId}/serviceaccounts; IsDisabled wire-typed as *string (\"true\"/\"false\"), not *bool -- preserved as-is, confirmed via types.go. gopherstack-1n1: enforces the doc-comment precondition 'You can only create service accounts for workspaces that are compatible with Grafana version 9 and above' (api_op_CreateWorkspaceServiceAccount.go), rejecting sub-9 workspaces (grafanaVersions includes 8.4) with ConflictException (versions.go's supportsServiceAccounts, recognized in this op's own deserializeOpErrorCreateWorkspaceServiceAccount) -- previously unenforced, no test exercised an explicit sub-9 GrafanaVersion. CreateWorkspaceServiceAccountToken states the identical precondition but needs no separate check: a token's ServiceAccountId can only resolve to an account created after this gate, and GrafanaVersion is upgrade-only (no downgrade path), so the condition is unreachable there."}
  DeleteWorkspaceServiceAccount: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /workspaces/{workspaceId}/serviceaccounts/{serviceAccountId}; cascades its tokens"}
  ListWorkspaceServiceAccounts: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /workspaces/{workspaceId}/serviceaccounts, paginated"}
  CreateWorkspaceServiceAccountToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST .../serviceaccounts/{id}/tokens; returns the plaintext key exactly once (ServiceAccountTokenSummaryWithKey), never re-exposed by List"}
  DeleteWorkspaceServiceAccountToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE .../serviceaccounts/{id}/tokens/{tokenId}"}
  ListWorkspaceServiceAccountTokens: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET .../serviceaccounts/{id}/tokens, paginated; summary shape has no Key field"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /tags/{resourceArn}; full percent-encoded ARN as one path segment, handled via rawPathSegments (s3tables-style RawPath + per-segment url.PathUnescape)"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /tags/{resourceArn}; TagKeys as repeated ?tagKeys= query param"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /tags/{resourceArn}"}
# Families audited as a group (when per-op is impractical):
families:
  route-matcher: {status: ok, note: "handler.go's routeRequest dispatch tree; RouteMatcher prefixes on /workspaces, /versions, /tags/; MatchPriority = PriorityPathVersioned"}
  filter_value_semantics: {status: ok, note: "2026-08-31 (gopherstack-uox6 value-semantics pass, CLEAN -- no bug found): covledger had no row for grafana; audited request-parameter semantics on all 9 List/Describe ops. Only ListPermissions has a real filter surface (groupId/userId/userType against ListPermissionsInput's own doc comment) -- verified correct: groupId requires UserType==SSO_GROUP AND ID match, userId requires UserType==SSO_USER AND ID match (so passing both is a structurally-impossible AND, matching the doc's 'you can specify only one'), userType is a direct SSO_USER/SSO_GROUP enum compare with wire-matching casing (types.UserTypeSsoUser/SsoGroup == \"SSO_USER\"/\"SSO_GROUP\"). Strengthened the existing test (TestUpdateAndListPermissions) with a new TestListPermissions_FiltersExcludeNonMatching seeding a second user AND a group so each filter has a wrong answer available to return -- the prior test's single seeded grant could not distinguish 'filtered correctly' from 'returned everything'; proved it can fail by temporarily short-circuiting the userId filter (git-diff-clean after restore), confirmed the failure, restored byte-identical. The five other List ops (ListWorkspaces, ListTagsForResource, ListVersions, ListWorkspaceServiceAccounts, ListWorkspaceServiceAccountTokens) declare no filter fields at all in the pinned SDK -- structurally no surface for this class. Checked the shared page.New pagination helper (5 callers) against each: none of ListWorkspacesInput/ListPermissionsInput/ListVersionsInput/ListWorkspaceServiceAccountsInput/ListWorkspaceServiceAccountTokensInput states a numeric MaxResults default anywhere -- confirmed on the module cache and, for ListWorkspaces, the live API reference page too (fetched once, carried the standing agent-toolkit-footer pattern, ignored) -- so grafanaDefaultPageSize=100 violates nothing documented; this differs from the narrowing-default-widened shape found elsewhere in this campaign because there is no default to widen. ListVersions' workspaceId-supplied branch (upgrade-only, strictly-greater-version) matches its own doc comment and the sibling UpdateWorkspaceConfigurationInput.GrafanaVersion wording ('Can only be used to upgrade... not downgrade')."}
gaps:
  - "StatusLicenseRemovalFailed (LICENSE_REMOVAL_FAILED) is never reached: DisassociateLicense is deliberately synchronous (see license.go's own doc comment on why it can't return a wire-accurate ConflictException), so there is no async transition for a chaos rule to intercept the way CreateWorkspace/UpdateWorkspace/AssociateLicense/UpdateWorkspaceConfiguration's are. Making it reachable would mean turning DisassociateLicense into an async op, a larger behavior change than this pass's gap-closing scope justifies."
  - "SSO user/group cross-service validation (validatePermissionUser in cross_service.go) is implemented and exercised by services/grafana's own unit tests, and by test/integration/grafana_test.go's Permissions subtest against the account's seeded default IAM Identity Center instance, but the integration suite does not additionally cover the case of a *second*, ambiguous SSO instance in the same account -- resolveIdentityStoreID picks the first with a non-empty IdentityStoreID, which is the correct behavior for the common (single-instance) case real AWS itself enforces, but is unverified for the multi-instance edge case."
structural_gaps:
  - "ListVersions' static version list (8.4/9.4/10.4 in store.go's grafanaVersions) is a reasonable stand-in, not the real AWS-supported set. Which Grafana versions Amazon Managed Grafana currently supports is operational data AWS changes over time out-of-band -- it is not encoded anywhere in the Go SDK module, and no implementation could derive the true current set from first principles; there is no data source for it to read. (bd: gopherstack-4spv)"
leaks: {status: clean, note: "Handler.Reset()/Backend.Reset() close every workspace's tags.Tags before clearing; InMemoryBackend.Close() stops the worker.Group backing every scheduled CREATING/UPDATING/etc. transition timer"}
---

## Implementation summary

All 25 operations are implemented with real backend state (no stubs): a genuine workspace
lifecycle state machine (CREATING/UPDATING/UPGRADING/VERSION_UPDATING all resolve to ACTIVE
after a simulated 100ms delay via `pkgs/worker`, mirroring `services/eks`'s
`scheduleClusterActivation` pattern), AWS SSO / SAML authentication sub-state, API keys and
service-account tokens with real expiry (`pkgs/awstime.Epoch` for the wire's epoch-seconds
timestamp fields), licence association/disassociation, a many-to-many permissions grant
table with real batch partial-failure semantics, and full tag support wired into
`resourcegroupstaggingapi` (`cli.go`'s `wireTaggingGrafana`).

**File layout**: `models.go` (stored-state types) / `wire.go` + `wire_convert.go` (JSON wire
shapes and their conversion to/from stored state) / `store.go` + `store_setup.go` (the
`InMemoryBackend`, one coarse `lockmetrics.RWMutex` guarding every collection since nearly
every operation reads-or-mutates workspace-scoped state) / `workspaces.go` +
`workspace_update.go` (CRUD + lifecycle) / `authentication.go` / `configuration.go` /
`license.go` / `versions.go` / `permissions.go` / `api_keys.go` / `service_accounts.go` /
`service_account_tokens.go` / `tags.go` (backend logic) / `handler.go` + one `handler_*.go`
per operation family (HTTP routing/dispatch) / `persistence.go` / `errors.go` / `consts.go`
/ `provider.go`.

## ARN format — settled and how it was verified

The manifest's prior audit correctly flagged that the resource-path segment of the ARN could
not be confirmed from the local SDK checkout alone (`ResourceArn` on
`TagResource`/`UntagResource`/`ListTagsForResource` is a bare `*string` with no `@resource`
pattern trait reproduced in the generated Go). This pass resolved it by reading
**terraform-provider-aws's own ARN-construction code**
(`internal/service/grafana/workspace.go`, fetched via `WebFetch` against
`raw.githubusercontent.com` since the AWS Service Authorization Reference page itself
returned no fetchable body): the resource is built as

```go
return c.RegionalARN(ctx, "grafana", "/workspaces/"+id)
```

confirming the full ARN shape is `arn:{partition}:grafana:{region}:{account}:/workspaces/{id}`
-- note the leading `/` baked into the resource segment itself (not a separate `resource/id`
or `resource:id` convention), matching what the prior audit had cited from AWS's general
documentation but could not verify from the SDK. `store.go`'s `WorkspaceARN` implements this
via `arn.Build("grafana", region, accountID, "/workspaces/"+id)`. Since the resource segment
itself contains a literal `/`, the real SDK client percent-encodes it as `%2F` when building
the `/tags/{resourceArn}` request URI -- `services/grafana/tags_test.go`'s
`TestTagResourceRoundTrip_ARNContainsSlash` exercises exactly this through a real
`aws-sdk-go-v2` client to prove `rawPathSegments`' `RawPath` + per-segment
`url.PathUnescape` handling (copied from `services/s3tables`'s proven fix) survives it.

The ARN's *service* segment (`grafana`) was already settled by the prior audit via the
classic `aws-sdk-go`'s `ServiceName`/`SigningName` constants and needed no re-verification.

## Errors — per-operation exception-type table

Verified by reading every operation's own `awsRestjson1_deserializeOpError<Op>` switch in
`deserializers.go` (not assumed from the shared `types/errors.go` declarations, which list
all seven possible shapes without saying which operations actually use which — see the
prior audit's own caution about this). Full table (operations with an unusual set called
out): every operation accepts `AccessDeniedException`/`InternalServerException`/
`ThrottlingException` (never triggered by this emulator — see gaps); most also accept
`ResourceNotFoundException` and `ValidationException`. **`ConflictException` is NOT
recognized by**: `AssociateLicense`, `DisassociateLicense`, `DescribeWorkspace`,
`DescribeWorkspaceConfiguration`, `ListPermissions`, `ListTagsForResource`, `ListVersions`,
`ListWorkspaces`, `TagResource`, `UntagResource`, `UpdatePermissions` — this emulator never
raises one from those operations (confirmed by re-reading every error path in this pass).
`ServiceQuotaExceededException` is recognized only by the five `Create*` operations plus
`CreateWorkspace` itself; `CreateWorkspace` now genuinely triggers it once an account holds
5 workspaces (Amazon Managed Grafana's published default "Number of workspaces" quota — see
`workspaces.go`'s `maxWorkspacesPerAccount`). `DescribeWorkspaceConfiguration`
uniquely does **not** accept `ValidationException` at all (it takes no body, so there is
nothing to validate) — this emulator's implementation never attempts to return one there.

## Cross-service validation — mechanism (this pass)

`cross_service.go` gives `InMemoryBackend` a `SetAppConfig(cfg any)` setter, called from
`provider.go`'s `Provider.Init` with `ctx.Config` (the `*CLI`). It can't resolve sibling
handlers *at* `Init` time: gopherstack constructs every service provider independently in one
pass (`cli.go`'s `initIndependentServices`) and only wires each provider's own CLI-struct
field afterward, once every provider has returned — grafana's `Init` runs inside that first
pass, before `iamHandler`/`ec2Handler`/etc. exist on the `*CLI`. Storing the `*CLI` pointer
(matched structurally via a local `siblingServices` interface, so no import of the top-level
package) and resolving `GetIAMHandler()` et al. *lazily*, on the first real request — well
after startup has finished — sidesteps the ordering problem without a second, cross-service-
aware init phase or any change to `cli.go`. The chaos fault store is the one exception: `cli.
faultStore` is constructed before `initializeServices` runs, so `chaosFaultStore()` could
resolve it eagerly too, but it shares the same lazy path for consistency.

`CreateWorkspace`/`UpdateWorkspace` validate `WorkspaceRoleArn` against `iam.StorageBackend.
GetRoleByArn`, `VpcConfiguration`'s subnet/security-group IDs against `ec2.Backend.
DescribeSubnets`/`DescribeSecurityGroups` (rejecting if the returned set is shorter than the
requested one), and `WorkspaceOrganizationalUnits` against `organizations.StorageBackend.
DescribeOrganizationalUnit` — all as `ValidationException`, since a caller-supplied reference
that doesn't resolve is that class of error across most AWS APIs surveyed for this pass, and
none of these four are actually validated against another service's exported surface anywhere
else in this codebase to confirm otherwise. `UpdatePermissions`' `ADD` instructions validate
`SSO_USER`/`SSO_GROUP` IDs the same way, but two hops deep: `ssoadmin.StorageBackend.
ListInstances` (always non-empty — `services/ssoadmin` pre-seeds a default instance to mirror
real AWS accounts) supplies the account's `IdentityStoreId`, then `identitystore.
InMemoryBackend.DescribeUser`/`DescribeGroup` resolves the grant's `User.Id` against it.

## Chaos-driven failure states — mechanism (this pass)

`pkgs/chaos`'s `Middleware` is wired generically into every service's request path
(`cli.go`'s `registry.Use(chaos.Middleware(faultStore))`) and already gave `AccessDeniedException`/
`ThrottlingException` a real trigger path with zero grafana-specific code — a chaos fault rule
targeting a real operation name (e.g. `CreateWorkspace`) short-circuits the HTTP request before
this handler ever runs. That mechanism can't reach the `*_FAILED`/`DEGRADED` statuses, though:
those are decided later, off a timer, with no request in flight for the middleware to
intercept. `chaos_transitions.go` calls `chaos.FaultStore.Match` directly from
`scheduleWorkspaceTransition`'s timer callback (and synchronously from `DeleteWorkspace`),
targeting the pseudo-operation name `WorkspaceTransition` — distinct from every real op name
this handler routes, so a rule scoped to it can never collide with the HTTP middleware's
per-request matching. A matched rule steers `CreateWorkspace`'s `CREATING` to
`CREATION_FAILED`, `UpdateWorkspace`'s `UPDATING` to `UPDATE_FAILED`, `AssociateLicense`'s
`UPGRADING` to `UPGRADE_FAILED`, and `UpdateWorkspaceConfiguration`'s `VERSION_UPDATING` to
`VERSION_UPDATE_FAILED` — or, if the rule's injected error code is literally `"DEGRADED"`, to
`DEGRADED` instead. `DeleteWorkspace` checks the same rule synchronously (it has no async
transition of its own to hook) and reports `DELETION_FAILED` without actually deleting.
`LICENSE_REMOVAL_FAILED` remains unreached — see `gaps:`.

## Deliberately simplified (honest, not hidden)

1. **Workspace IDs (`g-<10 hex chars>`) and numeric service-account/token IDs** are reasonable
   emulations, not confirmed byte-for-byte against a real workspace's ID format (the SDK
   types carry no pattern trait for `WorkspaceId`).
2. **`UpdatePermissions`'s partial-failure trigger** covers a zero-`Users` instruction and an
   `ADD` referencing an unresolvable `SSO_USER`/`SSO_GROUP` ID — the real API's full
   validation surface for this operation isn't documented in the Go SDK types alone, so this
   is a defensible, honest subset rather than a guess at the complete rule set.
3. **`LICENSE_REMOVAL_FAILED` is unreached** — see `gaps:` for why.

## Tests

**Unit** (`services/grafana/*_test.go`): `sdk_completeness_test.go` (empty exception list, all
25 ops), plus real-`aws-sdk-go-v2`-client round-trip tests for every operation family
(workspace lifecycle + validation + cascade-on-delete, authentication incl. SAML union
validation, configuration + version upgrade-only enforcement, license associate/disassociate
incl. the two wire-shape fixes described above, permissions incl. partial-failure batch
semantics, API keys, service accounts + tokens incl. cascade, and the ARN-with-embedded-slash
tag round trip) — following `services/databrew`'s `newRoundTripClient` pattern (a real SDK
client against an `httptest.Server` wired through the same `pkgs/service` registry/router used
in production), which is what actually caught the `AssociateLicense`/`DisassociateLicense`
`ConflictException` wire bugs described above; ad-hoc JSON assertions against `h.Handler()(c)`
directly would not have.

## Wrapper-key / nested-shape sweep (2026-08-20) — zero bugs found

Targeted re-audit for the specific bug class of a response member emitted under the
wrong wrapper key, wrong nesting level, wrong JSON type, or right-key-wrong-value
(including invented enum constants) — the class that has produced ~56 real bugs across
27 other services swept this session. Re-derived every finding from the pinned
`aws-sdk-go-v2/service/grafana@v1.38.4` source rather than trusting this file's prior
claims.

**False-positive-trap check (grafana is restjson1)**: confirmed the "dead flat-decode
path" trap from `gopherstack-cnhp` does **NOT** apply here — every one of the 24 ops'
`awsRestjson1_deserializeOpDocument<Op>Output` functions is referenced exactly twice in
`deserializers.go` (defined once, called once from that op's own `HandleDeserialize`),
i.e. genuinely live, except `TagResource`/`UntagResource`/`UpdateWorkspaceConfiguration`
(0 refs — correct, these three have no body member besides `ResultMetadata`). So every
wrapper key in `services/grafana/handler_*.go`'s `map[string]any{...}` envelopes is
load-bearing and was checked against its op's own live deserializer, not assumed.

**The three summary/full pairs** (all confirmed distinct wire structs matching their own
deserializer, field-for-field):
- `WorkspaceDescription` (28 fields, `deserializers.go` `awsRestjson1_deserializeDocumentWorkspaceDescription`) vs `WorkspaceSummary` (13 fields, `...WorkspaceSummary`) — `services/grafana/wire.go`'s `workspaceDescriptionWire`/`workspaceSummaryWire` match both sets exactly; the 15 Description-only fields (`accountAccessType`, `dataSources`, `degradedWorkspaceReason`, `freeTrialConsumed`, `freeTrialExpiration`, `ipAddressType`, `kmsKeyId`, `licenseExpiration`, `networkAccessControl`, `organizationalUnits`, `organizationRoleName`, `permissionType`, `stackSetName`, `vpcConfiguration`, `workspaceRoleArn`) are absent from `workspaceSummaryWire`, correctly.
- `AuthenticationDescription` (`awsSso`+`providers`+`saml`) vs `AuthenticationSummary` (`providers`+`samlConfigurationStatus`) — `authenticationDescriptionWire`/`authenticationSummaryWire` match exactly; no cross-contamination (Description never carries `samlConfigurationStatus`, Summary never carries `saml`/`awsSso`).
- `ServiceAccountTokenSummary` (`id`/`name`/`createdAt`/`expiresAt`/`lastUsedAt`, no key) vs `ServiceAccountTokenSummaryWithKey` (`id`/`key`/`name` only) — `serviceAccountTokenSummaryWire`/`serviceAccountTokenSummaryWithKeyWire` match exactly; `handler_service_account_tokens.go`'s `handleListWorkspaceServiceAccountTokens` never leaks `key`, and `handleCreateWorkspaceServiceAccountToken` is the only path that emits it.

**The `idpMetadata` union**: `types.IdpMetadata` (`SamlConfiguration.idpMetadata`) has exactly two discriminator keys, `url`/`xml` (`deserializers.go`'s `awsRestjson1_deserializeDocumentIdpMetadata`, `case "url"`/`case "xml"`). `services/grafana/wire.go`'s `idpMetadataWire{URL string 'json:"url,omitempty"'; XML string 'json:"xml,omitempty"'}` matches both keys, and `wire_convert.go`'s `toSamlConfigWire`/`fromSamlConfigWire` round-trip them; `authentication.go`'s `validateUpdateWorkspaceAuthenticationRequest` rejects both-set and neither-set, so the wire layer only ever serializes exactly one key — proven by `authentication_test.go`'s `TestUpdateWorkspaceAuthentication_SamlConfigured` (asserts the `*types.IdpMetadataMemberUrl` union member round-trips through a real SDK client) and `TestUpdateWorkspaceAuthentication_BothIdpMetadataMembers_Rejected`.

**HTTP method/path/header/query binding** (Layer 1): every op's `opPath`/`request.Method`
in `serializers.go`'s `awsRestjson1_serializeOp<Op>.HandleSerialize` was diffed against
`services/grafana/handler.go`'s `routeRequest`/`routeWorkspaces*` tree — all 24 match,
including the two documented traps (`AssociateLicense`'s `Grafana-Token` HTTP header,
not body — `serializers.go:` `encoder.SetHeader("Grafana-Token")` in
`awsRestjson1_serializeOpHttpBindingsAssociateLicenseInput`; `ListVersions`'s
`workspace-id` hyphenated query param — `encoder.SetQuery("workspace-id")` in
`awsRestjson1_serializeOpHttpBindingsListVersionsInput`).

**Enum values** (Layer 2e): every string constant in `models.go` (`StatusActive` ...
`StatusDegraded`, `AuthProviderAWSSSO`/`AuthProviderSAML`, `SamlStatusConfigured`/
`SamlStatusNotConfigured`, `LicenseEnterprise`/`LicenseEnterpriseFreeTrial`,
`PermissionTypeCustomerManaged`/`PermissionTypeServiceManaged`,
`AccountAccessTypeCurrentAccount`/`AccountAccessTypeOrganization`, `RoleAdmin`/
`RoleEditor`/`RoleViewer`, `UserTypeSSOUser`/`UserTypeSSOGroup`, `UpdateActionAdd`/
`UpdateActionRevoke`) diffed byte-for-byte against the SDK's `types/enums.go` `Values()`
lists — no invented constants, no case mismatches.

**Provenance check on this file's own prior claims**: `last_audit_commit` before this
pass was `3b90d4523`, dated `2026-08-06` (`git show -s --format=%ad`) — the **same day**
as the recorded `last_audit_date` (`2026-08-06`), not the days-to-weeks-earlier pattern
that flags a stale/copied manifest elsewhere. The commit that actually wrote that
frontmatter (`d39bf33e4`, 2026-08-11) branched from `3b90d4523` and merged 5 days later —
ordinary PR lag, not drift. One commit touched `services/grafana/` after that
(`69bbb940a`, 2026-08-15) but only added `handler_sdk_route_table_test.go` and one
`README.md` line — no behavior change, so no re-audit was owed and none was skipped.
`sdk_module`'s header (`v1.38.4`) is the only version string in this file (grepped) — no
stale prose disagreement. Both "FIXED" claims (AssociateLicense/DisassociateLicense not
recognizing `ConflictException`) re-derive cleanly against
`awsRestjson1_deserializeOpErrorAssociateLicense`/`...DisassociateLicense`'s switch
statements — both omit `ConflictException`, exactly as claimed. The existing
"`ConflictException` is NOT recognized by" list (11 ops) and the `ServiceQuotaExceededException`
claim were re-verified against every named op's own error switch — all 11 correct; the
"`ServiceQuotaExceededException` is recognized only by the five `Create*` operations"
phrasing is a wording slip in the existing prose (there are only 4 `Create*` operations
in this API — `CreateWorkspace`/`CreateWorkspaceApiKey`/`CreateWorkspaceServiceAccount`/
`CreateWorkspaceServiceAccountToken`, all 4 confirmed correct) — not a wire-accuracy bug,
left as-is rather than "fixed" since Layer 3 prose polish is out of this sweep's scope.

**Result: zero wrapper-key/nesting/type/value bugs found.** Every op's response envelope
key, every nested type's field set, and every enum value already matched the pinned SDK
exactly before this pass began.

**Integration** (`test/integration/grafana_test.go`, the parity proof per
`.claude/memories/parity-principles.md` rule 3 — a real `aws-sdk-go-v2` client against the
Dockerized binary, not the in-process router):
`TestIntegration_Grafana_WorkspaceLifecycle` drives all 25 operations sequentially against one
workspace (creation and its async transition to `ACTIVE`, describe/list/update, authentication
incl. a real SAML union round trip, configuration + version upgrade incl. a rejected
downgrade, licensing, permissions incl. a real IAM Identity Center user via the seeded default
SSO instance, API keys, service accounts + tokens, tagging, and deletion), asserting real
`smithy.APIError` codes (`ResourceNotFoundException`, `ValidationException`) alongside the
happy paths — not just non-nil checks. `TestIntegration_Grafana_ServiceQuota` fills an
isolated container's account to the real 5-workspace quota and asserts the 6th `CreateWorkspace`
returns `ServiceQuotaExceededException`. `TestIntegration_Grafana_CrossServiceValidation` (also
isolated) proves both directions for all three CreateWorkspace-time references: a fabricated
IAM role ARN / EC2 subnet+security-group pair / Organizations OU is rejected, and a role /
VPC+subnet+security-group / OU genuinely created via those services' own real SDK clients is
accepted. `TestIntegration_Grafana_ChaosWorkspaceTransitions` (isolated — chaos fault rules are
global mutable state) drives a `WorkspaceTransition`-scoped fault rule through
`CREATION_FAILED`, `DEGRADED`, and a synchronous `DELETION_FAILED` that leaves the workspace
undeleted.

## Value-semantics sweep (2026-08-31, gopherstack-uox6) — clean, no bug found

Targeted by an empty covledger row for `grafana` (no class recorded at all),
not by code shape. Checklist: is every documented filter/comparison field
read at all, against the operation's own key/casing/type, and what does its
absence mean.

`ListPermissions` is the only operation in this service with a real filter
grammar (`groupId`/`userId`/`userType`, `permissions.go:8-37`). All three were
checked against `ListPermissionsInput`'s own doc comment in the pinned
`aws-sdk-go-v2/service/grafana@v1.38.4` module — not a sibling type — and are
correct: `groupId` requires the stored entry be `UserTypeSSOGroup` *and* ID
match, `userId` requires `UserTypeSSOUser` *and* ID match (so a request
supplying both can never match anything, which is consistent with the doc's
"If you do this, you can specify only one userId or one groupId"), and
`userType` is a direct enum compare against wire-matching constants
(`"SSO_USER"`/`"SSO_GROUP"`, `models.go:71-72`, confirmed equal to
`types.UserTypeSsoUser`/`UserTypeSsoGroup` in `types/enums.go`). The wire
binding was independently re-verified against `serializers.go`'s
`awsRestjson1_serializeOpHttpBindingsListPermissionsInput` — `groupId`,
`userId`, `userType`, `maxResults`, `nextToken` are all query parameters, and
`handleListPermissions` (`handler_permissions.go:13`) reads them from
`r.URL.Query()` under the identical keys.

The existing `TestUpdateAndListPermissions` seeded exactly one permission
grant, so its `userId` filter assertion could not distinguish "filtered
correctly" from "returned everything" — the exact trap this campaign's
briefs warn about. Added `TestListPermissions_FiltersExcludeNonMatching`
(`permissions_test.go`), seeding two users and one group so every filter
(`userId`, `groupId`, `userType=SSO_GROUP`, `userType=SSO_USER`) has a
present-but-wrong record it must exclude. Confirmed it passes against
unmodified code, then temporarily short-circuited the `userId` filter
condition (`if false && userID != "" ...`) to confirm the test fails, then
restored the file — `git status --short services/grafana/permissions.go`
shows no diff.

The other five List operations (`ListWorkspaces`, `ListTagsForResource`,
`ListVersions`, `ListWorkspaceServiceAccounts`,
`ListWorkspaceServiceAccountTokens`) declare no filter fields in the pinned
SDK at all — checked directly against each `*Input` struct — so there is no
surface for a wrong-algorithm filter bug; recorded as a structural absence,
not assumed.

`page.New` (`pkgs/page`) is the shared pagination helper behind five of the
six List handlers with `grafanaDefaultPageSize = 100`. Per the shared-helper
lens (check each caller against its own doc, not the helper's default), none
of `ListWorkspacesInput.MaxResults`, `ListPermissionsInput.MaxResults`,
`ListVersionsInput.MaxResults`, `ListWorkspaceServiceAccountsInput.MaxResults`,
or `ListWorkspaceServiceAccountTokensInput.MaxResults` states a default
number anywhere in the Go doc comments — only a `1`–`100` valid range, which
the live `ListWorkspaces` API reference page (fetched once; carried the
standing "run `aws agent-toolkit search-skills`" footer this campaign has
flagged since pass 6, treated as inert data) confirmed independently. This is
a genuine structural absence, distinct from the narrowing-default-widened
shape found elsewhere in this campaign: there is no documented number for
`grafanaDefaultPageSize=100` to violate.

`ListVersions`' workspace-scoped branch (`upgradeVersionsFor`,
`versions.go:37`) returns every version strictly after the workspace's
current one — checked against both its own doc comment ("lists the available
upgrade versions") and the sibling
`UpdateWorkspaceConfigurationInput.GrafanaVersion` wording ("Can only be used
to upgrade... not downgrade"); correct, not fixed.

No `nolint` directives exist in any file touched this pass. Gates: `go
build`, `go vet` (repo-wide, clean), `go test -race -count=1`, `golangci-lint
run` all pass. No production code changed; `permissions_test.go` gained one
new test (assertions: +9 `require`, 0 dropped).

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `ApiKey`/`APIKey` acronym
casing gives it 2 op/handler pairs needing the ambiguous fold, both genuine
collisions between an exported `*InMemoryBackend` method and the real
unexported handler: `CreateWorkspaceApiKey`, `DeleteWorkspaceApiKey`.

Verified directly: ran the unpatched tool from `ef0eef041~1` five times and
diffed against the fixed tool at HEAD. `cmd/reqfieldscan` was byte-identical
across all 5 runs and HEAD -- zero damage. `cmd/reqfielddiff` was not: old
runs found 39 or 43 findings vs 39 at HEAD, with 4 fields flickering, all
only in old (misresolved) runs, never at HEAD: `CreateWorkspaceApiKey.{KeyName,
KeyRole, SecondsToLive, WorkspaceId}`. Read the source
(handler_api_keys.go:16-27): `createWorkspaceAPIKeyRequest` declares
`KeyName`/`KeyRole`/`SecondsToLive` (`json.Unmarshal`'d), all three passed to
`h.Backend.CreateWorkspaceAPIKey`; `WorkspaceId` comes from the URL path
segment. Confirmed genuine -- not a bug.

Verdict: zero real bugs, safe direction only.
