---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: detective
sdk_module: aws-sdk-go-v2/service/detective@v1.41.4   # version audited against
last_audit_commit: 73f9bede0bad60884fc2dadfb77a2c83ef55fd27
last_audit_date: 2026-08-20
overall: A            # A = genuine fixes found; B = already-accurate, proven op-by-op
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateGraph: {wire: ok, errors: ok, state: ok, persist: ok, note: idempotent-per-account matches AWS docs}
  DeleteGraph: {wire: ok, errors: ok, state: ok, persist: ok, note: "cleans investigations/datasources/orgConfigs, not just members/tags"}
  ListGraphs: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "already-invited accounts go to UnprocessedAccounts per AWS docs; MemberDetail now includes InvitationType and DatasourcePackageIngestStates (see notes)"}
  DeleteMembers: {wire: ok, errors: ok, state: ok, persist: ok}
  GetMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "MemberDetail now includes InvitationType and DatasourcePackageIngestStates"}
  ListMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "MemberDetail now includes InvitationType and DatasourcePackageIngestStates; NextToken already opaque base64"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  AcceptInvitation: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /invitation matches SDK method"}
  RejectInvitation: {wire: ok, errors: ok, state: ok, persist: ok}
  ListInvitations: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - NextToken is now an opaque base64 offset (encodePageToken/decodePageToken) instead of the raw next GraphArn; MemberDetail now includes InvitationType and DatasourcePackageIngestStates"}
  DisassociateMembership: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetGraphMemberDatasources: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetMembershipDatasources: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDatasourcePackages: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - NextToken is now an opaque base64 offset instead of the raw next package-name key"}
  UpdateDatasourcePackages: {wire: ok, errors: ok, state: ok, persist: ok, note: "always transitions to STARTED, no real ingest pipeline to fail - acceptable simplification; fixed this pass - DatasourcePackages entries outside the real 3-value enum (DETECTIVE_CORE, EKS_AUDIT, ASFF_SECURITYHUB_FINDING per botocore detective/2018-10-26 service-2.json shapes.DatasourcePackage) are now rejected with ValidationException instead of silently persisted"}
  StartMonitoringMember: {wire: ok, errors: ok, state: partial, persist: ok, note: "precondition status ACCEPTED_BUT_DISABLED is never reached elsewhere in the backend (AcceptInvitation goes straight to ENABLED), so this op can never succeed on a member reached only through normal API flow; see gaps"}
  GetInvestigation: {wire: ok, errors: ok, state: ok, persist: ok}
  ListIndicators: {wire: ok, errors: ok, state: ok, persist: ok, note: "real aws-sdk-go-v2 types.Indicator has IndicatorType + IndicatorDetail (a union of 8 type-specific sub-structs: FlaggedIpAddressDetail, ImpossibleTravelDetail, NewAsoDetail, NewGeolocationDetail, NewUserAgentDetail, RelatedFindingDetail, RelatedFindingGroupDetail, TTPsObservedDetail) and has NO Title member at all -- this emulator previously returned a gopherstack-invented free-text Title field instead of IndicatorDetail; deleted and replaced with the real union shape (interfaces.go IndicatorDetail + 8 sub-detail structs, handler_investigations.go indicatorDetailToJSON). Added the two previously-missing IndicatorType values (NEW_ASO, NEW_USER_AGENT) to builtInIndicators; all 8 real enum values remain valid/filterable via validIndicatorTypes, but see gopherstack-b6wo in gaps below -- only 4 of the 8 (TTP_OBSERVED, NEW_GEOLOCATION, NEW_ASO, NEW_USER_AGENT) are actually ever producible by builtInIndicators, correcting this note's prior 'all 8 producible' claim, which was already inaccurate when written since StartInvestigation has never assigned any Severity but INFORMATIONAL. An IndicatorType filter value outside the 8-value enum (ListIndicators documents ValidationException in its error set) is rejected instead of silently returning an empty Indicators list. Fixed 2026-08-20: types.TTPsObservedDetail has a real (non-deprecated) Technique member (aws-sdk-go-v2/service/detective@v1.41.4/types/types.go, deserializers.go's awsRestjson1_deserializeDocumentTTPsObservedDetail's \"Technique\" case) that this emulator's TTPsObservedDetail struct never carried and indicatorDetailToJSON never emitted -- a real client always saw a nil Technique on every TTP_OBSERVED indicator. Added Technique to interfaces.go's TTPsObservedDetail, populated it in builtInIndicators, and added the wire key in indicatorDetailToJSON; proven via wire_sdk_roundtrip_test.go's TestListIndicators_TTPsObservedDetail_Technique_SDKRoundTrip using the real detectivesdk client's typed field."}
  ListInvestigations: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - NextToken is now an opaque base64 offset instead of the raw next InvestigationId"}
  StartInvestigation: {wire: ok, errors: ok, state: ok, persist: ok, note: "EntityType is not a real input field (StartInvestigationInput has no EntityType member); derived server-side from EntityArn's role/ or user/ resource segment. ScopeStartTime/ScopeEndTime are required per SDK and validated as such."}
  UpdateInvestigationState: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeOrganizationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableOrganizationAdminAccount: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - AWS docs: 'Deletes the organization behavior graph.' Now deletes the graph(s) referenced by the current org admin(s) via the deleteGraphLocked helper shared with DeleteGraph (cascading members/investigations/tags/datasources/orgConfigs cleanup), then clears orgAdmins. Within this emulator's one-graph-per-account model, EnableOrganizationAdminAccount always designates the account's sole graph as the org graph, so deleting it on Disable is the faithful behavior."}
  EnableOrganizationAdminAccount: {wire: ok, errors: ok, state: ok, persist: ok, note: "auto-creates a behavior graph when the account has none, per AWS docs. Fixed this pass - now enforces AWS's singular Detective-administrator-account-per-organization/Region model (ListOrganizationAdminAccounts and the Administrator SDK type both describe it in the singular): a second Enable call replaces the existing orgAdmins entry instead of appending a duplicate, which previously let repeated Enable calls accumulate multiple conflicting Administrators in ListOrganizationAdminAccounts output."}
  ListOrganizationAdminAccounts: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - NextToken is now an opaque base64 offset instead of the raw next AccountId (now effectively unreachable in practice since EnableOrganizationAdminAccount enforces at most one admin, but kept consistent with the other list ops)"}
  UpdateOrganizationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
# Families audited as a group (when per-op is impractical):
families:
  route_matcher: {status: ok, note: "every REST path + HTTP method verified byte-for-byte against aws-sdk-go-v2 serializers.go opPath/request.Method for all 29 ops; matches exactly. handler_test.go exercises h.RouteMatcher()(c) and h.ExtractOperation(c) directly (not just h.Handler()) to prove the matcher itself, not just the dispatch switch, since unit tests calling h.Handler()(c) bypass RouteMatcher."}
  wire_timestamps: {status: ok, note: "smithytime.ParseDateTime/FormatDateTime confirms restjson1 Detective uses ISO8601 datetime strings (NOT epoch numbers) for CreatedTime/InvitedTime/UpdatedTime/DelegationTime/ScopeStartTime/ScopeEndTime; handler.go's \"2006-01-02T15:04:05.000Z\" format is a valid (always-3-decimal) RFC3339 the real client parses fine, vs. SDK's \"2006-01-02T15:04:05.999Z\" (trailing-zero-trimmed) output format - both are valid ISO8601, no bug"}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "StartMonitoringMember's precondition (member status ACCEPTED_BUT_DISABLED) is unreachable through normal API flow: AcceptInvitation transitions INVITED straight to ENABLED, mirroring the AWS happy path, but real Detective can also land a member in ACCEPTED_BUT_DISABLED (data-volume-too-high / volume-unknown edge cases per MemberDisabledReason) which this emulator does not model. Not fixed this pass: real AWS determines this state via internal GuardDuty volume telemetry with no documented client-controllable trigger, so modeling a way to reach it would mean inventing a control surface that does not exist in the real API rather than emulating one -- a larger, speculative feature, not a wire/state bug fix. Re-verified gopherstack-c902: ACCEPTED_BUT_DISABLED IS present in the pinned SDK's MemberStatus enum (types/enums.go, aws-sdk-go-v2/service/detective@v1.41.4, line 186), so this is not a wire gap -- the value exists in the model, it is just unreachable through any legitimate client action. Also re-verified the precondition itself is NOT missing: administrator.go's StartMonitoringMember already rejects any member whose status isn't ACCEPTED_BUT_DISABLED with ValidationException (see TestDetective_StartMonitoringMember's \"member not ACCEPTED_BUT_DISABLED returns 400\" case), so there was nothing left to fix here."
  - "MemberDetail still omits DisabledReason, VolumeUsageInBytes (deprecated), VolumeUsageUpdatedTime (deprecated), PercentOfGraphUtilization (deprecated), PercentOfGraphUtilizationUpdatedTime (deprecated), and VolumeUsageByDatasourcePackage. InvitationType and DatasourcePackageIngestStates were fixed this pass (see CreateMembers/GetMembers/ListMembers/ListInvitations notes). The remaining fields are volume/analytics telemetry this emulator does not model (no real data-ingest pipeline), and DisabledReason has no valid state to populate since ACCEPTED_BUT_DISABLED is unreachable (see the StartMonitoringMember gap above) -- all are optional fields real clients already treat as absent-safe, so omitting them is wire-legal, just incomplete. Low priority. Re-verified gopherstack-c902: DisabledReason and the volume metrics were deliberately split per the follow-up issue's instruction -- but the split does not change the verdict here. DisabledReason would be trivially serialisable IF the backend ever transitioned a member into ACCEPTED_BUT_DISABLED (storedMember already has a Status field to key off of), but grep confirms nothing in this codebase ever assigns memberStatusAcceptedDisabled to a member -- StartMonitoringMember only reads it as a precondition, never writes it. So there is no disabled-state instance anywhere in the backend for DisabledReason to be derived from; inventing a value would mean fabricating data with no backing state, which is worse than omitting the field. VolumeUsage*/PercentOfGraphUtilization genuinely need ingest-volume telemetry this emulator has no model for -- left absent rather than invented, matching this campaign's 'absent beats plausible-but-wrong' rule."
  - "2026-08-20 wrapper-key/nested-shape sweep: DatasourcePackageIngestDetail (ListDatasourcePackages) still omits LastIngestStateChange (map[state]TimestampForCollection per package, deserializers.go's awsRestjson1_deserializeDocumentDatasourcePackageIngestDetail 'LastIngestStateChange' case) -- this emulator tracks a single datasourceChangedAt timestamp per package/graph (used to build the sibling BatchGetGraphMemberDatasources/BatchGetMembershipDatasources DatasourcePackageIngestHistory shape via ingestHistoryLocked) but ListDatasourcePackages' handler never surfaces it as LastIngestStateChange. Genuine Layer-3 omission (member never emitted), not fixed -- out of scope for this pass's wrapper-key/nesting charter; the backing data (datasourceChangedAt) already exists so a future pass could wire it with the same shape ingestHistoryToJSON already produces elsewhere. Not previously recorded by gopherstack-c902."
  - "gopherstack-b6wo: Investigation Severity is, and can only ever be, INFORMATIONAL -- StartInvestigation's only write site (investigations.go, was line 186) never assigns any other value, mirroring the fact that real Detective computes Severity from ML/threat-intelligence analysis of the indicators found (aws-sdk-go-v2/service/detective@v1.41.4/types/types.go:234-236, InvestigationDetail.Severity doc comment: 'Severity based on the likelihood and impact of the indicators of compromise discovered in the investigation'; botocore detective/2018-10-26/service-2.json shapes.InvestigationDetail.members.Severity.documentation: same wording), which this emulator has no way to perform. Modelling gap, not a defect: INFORMATIONAL is the honest floor. Consequently only 4 of the 8 real IndicatorType values (TTP_OBSERVED, NEW_GEOLOCATION, NEW_ASO, NEW_USER_AGENT) are ever producible by builtInIndicators; FLAGGED_IP_ADDRESS/IMPOSSIBLE_TRAVEL/RELATED_FINDING/RELATED_FINDING_GROUP remain valid ListIndicators filter values (correct -- a real investigation can legitimately have zero indicators of a given type) but this emulator's synthetic generator can never produce them."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "Detective Organizations edge cases beyond the base Enable/Disable/List/Describe/Update surface (delegated-admin-account transfer, cross-region graph semantics) — out of scope for a single-region single-account emulator."
  - "UpdateOrganizationConfiguration's AutoEnable flag has no side effect: real AWS auto-enables Detective for new Organizations member accounts as they join the org. This emulator has no Organizations-service integration to source account-join events from, so AutoEnable is stored and returned correctly (DescribeOrganizationConfiguration) but never drives member auto-creation. Out of scope for a single-account emulator with no cross-service org simulation. Re-verified gopherstack-c902: services/organizations exists and other services (grafana, mgn) do reach it via a siblingServices/GetOrganizationsHandler cross_service.go pattern -- but only for synchronous reads (DescribeOrganizationalUnit, DescribeOrganization, ListDelegatedAdministrators, ListAccounts), never as an event source. Checked every other gopherstack service that models an AutoEnable-shaped org config (guardduty, inspector2, macie2, securityhub, all found via `grep -rl AutoEnable services`): every one of them stores and echoes AutoEnable identically, with zero side effect -- none has solved 'new account joins org' as a trigger. services/organizations' AcceptHandshake/InviteAccountToOrganization add an account to the org's own account list but publish no event or callback any sibling service subscribes to. This is a genuine cross-cutting gap (not a stale 'already solved elsewhere' claim like the codedeploy/EC2 case) -- AutoEnable is stored-and-echoed with no trigger to hook into anywhere in this codebase, which is the honest half of the stored-vs-ignored distinction, not the negligent half."
leaks: {status: clean, note: "DeleteGraph purges investigations/datasources/orgConfigs for the deleted graph ARN (not just members/tags). DisableOrganizationAdminAccount now reuses the same deleteGraphLocked cascade (see EnableOrganizationAdminAccount/DisableOrganizationAdminAccount notes above), so org-graph deletion via Disable is leak-free too. Verified via TestDeleteGraph_CleansUpDependentState and TestDisableOrganizationAdminAccount_DeletesGraph, both asserting the deleted ARN is absent from a post-delete Snapshot()/ListGraphs()."}
---

## Notes

Protocol: restjson1. All 29 CreateGraph..UpdateOrganizationConfiguration ops route
through a single `handleREST` switch keyed by `classifyPath(method, path)`; every
path/method pair was diffed byte-for-byte against
`aws-sdk-go-v2/service/detective@v1.39.1/serializers.go`'s
`awsRestjson1_serializeOpHttpBindings*Input` functions (opPath + request.Method) and
matches exactly, including the PUT-vs-POST split on `/invitation`
(AcceptInvitation=PUT, RejectInvitation lives at a different path
`/invitation/removal`=POST) and the GET/POST/DELETE split on `/tags/{ResourceArn}`.

### 2026-08-20 wrapper-key / nested-shape sweep

`last_audit_commit` provenance correction: the value recorded before this
pass (`40f059288a40c1d9b7956624bb288861e2e0651d`) dated to Jul 13 2026 by
`git show -s --format=%ad`, nearly a month before the `last_audit_date` of
2026-08-10 it sat next to — and `git log -- services/detective/PARITY.md`
shows the file's actual last edit was `d39bf33e4ef267a3b2c2dc9cae2fd5df5c78aeda`
(Aug 11 2026), the commit that produced everything currently recorded under
`ops:`/`gaps:`/the "Real bugs fixed this pass" list below. The recorded sha
was stale/wrong, not a real audit-base pointer. Corrected to the commit this
session actually started from (`73f9bede0bad60884fc2dadfb77a2c83ef55fd27`,
2026-08-20). No "FIXED" claim in the prior manifest failed re-derivation —
every op re-verified against the pinned SDK's live deserializer this pass
(see below) matched what `ops:` already recorded.

This pass re-read every op's live `awsRestjson1_deserializeOpDocument<Op>Output`
(all 29 ops checked; none hit the restjson1 single-structure-member dead-code
trap — every op's response body is decoded generically then dispatched
straight to that op's own `deserializeOpDocument<Op>Output`, confirmed by
reading each op's `HandleDeserialize` body) plus every nested-type
deserializer it references, against gopherstack's actual JSON keys emitted
in `handler_*.go`. Wrapper key, nesting level, and JSON type all matched on
every op — **zero Layer 1/2 wire-shape bugs found**. Enum-keyed maps
(`DatasourcePackageIngestStates`, `ListDatasourcePackages`' return map) key
on the real 3-value `DatasourcePackage` enum (`DETECTIVE_CORE`, `EKS_AUDIT`,
`ASFF_SECURITYHUB_FINDING`) and value on the real 3-value
`DatasourcePackageIngestState` enum (`STARTED`, `STOPPED`, `DISABLED`) —
`services/detective/datasource_packages.go`'s `validDatasourcePackages` and
`interfaces.go`'s ingest-state consts match both exactly. One genuine
Layer-3 gap (a real member never emitted at all) surfaced incidentally while
verifying `TTPsObservedDetail`'s field list and was fixed: see the
`ListIndicators` `ops:` entry above for the `Technique` field fix, proven by
`wire_sdk_roundtrip_test.go`'s real-SDK-client round trip.

Real bugs fixed in prior passes (see `ops:` above for detail): CreateMembers
UnprocessedAccounts reporting on re-invite; StartInvestigation trusting a
client-supplied (non-existent) `EntityType` field instead of deriving it from
`EntityArn`, plus missing `ScopeStartTime`/`ScopeEndTime` required-field
validation; EnableOrganizationAdminAccount never auto-creating a graph;
DeleteGraph leaking datasource/orgConfig/investigation state.

Real bugs fixed this pass (see `ops:` above for detail):

1. **`ListIndicators`/`Indicator` had a fabricated `Title` field with no
   basis in the real SDK.** `aws-sdk-go-v2/service/detective/types.Indicator`
   has exactly two members: `IndicatorType` and `IndicatorDetail` (a
   union-like struct with one of 8 type-specific sub-details populated:
   `FlaggedIpAddressDetail`, `ImpossibleTravelDetail`, `NewAsoDetail`,
   `NewGeolocationDetail`, `NewUserAgentDetail`, `RelatedFindingDetail`,
   `RelatedFindingGroupDetail`, `TTPsObservedDetail`) — there is no `Title`
   member anywhere in the shape. A real client parsing this emulator's
   `Title` string would silently drop it (deserializer ignores unknown
   keys) and get an empty `IndicatorDetail` on every indicator. Fixed by
   deleting the invented `Title` field and adding the real `IndicatorDetail`
   union (`interfaces.go`), wiring `builtInIndicators` (`investigations.go`)
   to populate the correct sub-detail per `IndicatorType`, and adding a
   `indicatorDetailToJSON` encoder (`handler_investigations.go`) with the
   exact wire field names byte-diffed against `deserializers.go`. Also added
   the two previously-missing `IndicatorType` enum values (`NEW_ASO`,
   `NEW_USER_AGENT`) so all 8 real values are producible/filterable.
2. **`MemberDetail` was missing `InvitationType` and
   `DatasourcePackageIngestStates`**, two real (non-deprecated)
   `MemberDetail` wire members. `InvitationType` is always `"INVITATION"` in
   this emulator (every member reaches a graph through the CreateMembers
   invite flow — there is no Organizations-auto-enable path to produce
   `"ORGANIZATION"`). `DatasourcePackageIngestStates` mirrors the graph-wide
   datasource ingest map, matching the simplification already used by
   `BatchGetGraphMemberDatasources`. Fixed in `models.go`
   (`toMemberDetail`), `interfaces.go`, `members.go`, and
   `handler_members.go`.
3. **`EnableOrganizationAdminAccount` accumulated duplicate `orgAdmins`
   entries** on repeated calls instead of replacing the existing
   designation, contradicting AWS's singular
   Detective-administrator-account-per-organization/Region model
   (`ListOrganizationAdminAccounts`/`Administrator` are both documented in
   the singular). Fixed in `administrator.go` to replace in place.
4. **`DisableOrganizationAdminAccount` did not delete the organization
   behavior graph**, contradicting AWS docs: "Removes the Detective
   administrator account in the current Region. Deletes the organization
   behavior graph." Fixed by extracting a `deleteGraphLocked` cascade helper
   (shared with `DeleteGraph`) in `graphs.go` and calling it for each
   admin's `GraphArn` before clearing `orgAdmins` in `administrator.go`.
5. **List pagination tokens were not uniformly opaque**: `ListInvitations`,
   `ListInvestigations`, `ListOrganizationAdminAccounts`, and
   `ListDatasourcePackages` returned the raw next item's identifier
   (GraphArn/InvestigationId/AccountId/package name) as `NextToken` instead
   of the opaque `base64(offset)` token every other Detective list op
   (`ListGraphs`/`ListMembers`/`ListIndicators`) already used. Wire-legal
   either way (AWS never guarantees token structure), but leaked internal
   identifiers to callers. Normalized all four to `encodePageToken`/
   `decodePageToken`.
6. **`ListIndicators` accepted any `IndicatorType` filter string**, including
   values outside the real 8-value enum (botocore
   `detective/2018-10-26/service-2.json` `shapes.IndicatorType`), silently
   returning an empty `Indicators` list (200 OK) instead of the
   `ValidationException` the op documents in its error set. Fixed in
   `investigations.go` (`validIndicatorTypes`, checked in `ListIndicators`
   before acquiring the lock).
7. **`UpdateDatasourcePackages` accepted any `DatasourcePackages` entry**,
   including values outside the real 3-value enum (`DETECTIVE_CORE`,
   `EKS_AUDIT`, `ASFF_SECURITYHUB_FINDING` per botocore
   `shapes.DatasourcePackage`), silently persisting an invented package name
   as `STARTED` instead of the documented `ValidationException`. Fixed in
   `datasource_packages.go` (`validDatasourcePackages`, checked before
   acquiring the lock).

"Looks-wrong-but-correct" traps for the next auditor:

- `AcceptInvitation` (and `AcceptInvitation`-adjacent flows) go straight from
  `INVITED` to `ENABLED`, never landing in `ACCEPTED_BUT_DISABLED`. This matches
  the AWS happy path (admission control passing); do not "fix" this into a
  detour through `ACCEPTED_BUT_DISABLED` without a concrete reason to model
  data-volume admission control.
- Timestamp format `"2006-01-02T15:04:05.000Z"` (always 3 decimals) vs. the SDK's
  own output format `"2006-01-02T15:04:05.999Z"` (trailing zeros trimmed) are both
  valid ISO8601/RFC3339 the real client parses identically — not a wire bug.
- `ErrGraphNotFound`/`ErrMemberNotFound`'s `Error()` text is literally the
  exception type string (`"ResourceNotFoundException"`), so the JSON response
  body's `message` field duplicates `__type`. Inelegant but not a wire-shape
  violation — AWS SDKs do not assert exact message text.
- `ListOrganizationAdminAccounts` pagination (`decodePageToken`/`encodePageToken`)
  is now effectively unreachable through the public API: since
  `EnableOrganizationAdminAccount` enforces at most one `orgAdmins` entry,
  `admins` never exceeds length 1, so `NextToken` can never be produced. Kept
  the opaque-token code path anyway for internal consistency with the other
  three list ops it was fixed alongside (`ListInvitations`,
  `ListInvestigations`, `ListDatasourcePackages`) — do not read the lack of a
  reachable pagination test for this one op as an oversight.
- `ErrAlreadyHasGraph` (`errors.go`) is dead/unused: `CreateGraph` is
  intentionally idempotent per AWS docs ("If the same account calls
  CreateGraph with the same administrator account, it always returns the
  same behavior graph ARN"), so nothing ever returns this error. Left as-is
  — it maps to a real `ConflictException` (not an invented error code), it's
  exported API surface, and removing it is out of scope for this pass.

**2026-08-30 (negative-continuation-token sweep)**: `store.go`'s `decodePageToken` accepted a
token that base64-decoded to a negative integer and returned it verbatim; every one of its 7
callers (`administrator.go`, `datasource_packages.go`, `graphs.go`, `members.go` x2,
`investigations.go` x2) only clamps the upper bound (`if start > len(x) { start = len(x) }`),
which does not catch a negative `start`, so `x[start:end]` panicked with `slice bounds out of
range [-5:]` given a token base64-decoding to `-5`. Fixed at the decode site: `decodePageToken`
now rejects a negative offset like any other malformed token, so all 7 callers inherit the fix.

Proof: `TestDecodePageToken_NegativeOffset` and `TestListGraphs_NegativeToken`
(`whitebox_test.go`) confirmed panicking pre-fix, pass now. Gates: `go build
./services/detective/...`, `go vet ./services/detective/...`, `go test -race -count=1
./services/detective/...`, `golangci-lint run ./services/detective/...` (0 issues). Work left
uncommitted per this pass's instructions.

### 2026-09-07: gopherstack-b6wo -- no defect, dead branches removed

Title: "investigation Severity is hardcoded INFORMATIONAL, so the medium and
high indicator branches are unreachable." Filed title-only, empty
description; both halves verified true, neither is a fixable defect.

**Claim 1 (Severity hardcoded INFORMATIONAL): TRUE.** Repo-wide grep for
every write site of an investigation's `Severity` field found exactly one:
`investigations.go`'s `StartInvestigation`, `Severity: severityInformational`
(was line 186). `UpdateInvestigationState` (investigations.go) mutates only
`inv.State`, never `inv.Severity`. No other function in the package assigns
`Severity`. `severityLow`/`severityMedium`/`severityHigh`/`severityCritical`
(`models.go`) were declared but, before this pass, referenced nowhere but the
two dead conditionals below.

**Claim 2 (medium/high indicator branches unreachable): TRUE**, and this is
the more interesting half. `investigations.go`'s `builtInIndicators` (before
this pass) gated two indicator-generation blocks on `inv.Severity`: `if
inv.Severity == severityMedium || ... == severityHigh || ... ==
severityCritical` (was line 93, adding `FLAGGED_IP_ADDRESS` +
`IMPOSSIBLE_TRAVEL`) and `if inv.Severity == severityHigh || ... ==
severityCritical` (was line 112, adding `RELATED_FINDING` +
`RELATED_FINDING_GROUP`). Since Severity is always `"INFORMATIONAL"` (Claim
1), neither condition can ever be true — both blocks were dead code, and
`ListIndicators` could never surface 4 of the real API's 8 `IndicatorType`
values, even though all 8 are accepted as valid filter values by
`validIndicatorTypes`.

**Verdict: modelling gap, not a defect**, and the causality in the title's
framing is backwards from the real API's. `aws-sdk-go-v2/service/
detective@v1.41.4/types/types.go:234-236` (`InvestigationDetail.Severity`
doc comment): "Severity based on the likelihood and impact of the indicators
of compromise discovered in the investigation." Confirmed verbatim in the
second oracle, botocore `detective/2018-10-26/service-2.json`
(`shapes.InvestigationDetail.members.Severity.documentation`): "The severity
assigned is based on the likelihood and impact of the indicators of
compromise discovered in the investigation." Real Detective computes
Severity *from* indicators via ML/threat-intelligence analysis over VPC Flow
Logs, CloudTrail, and GuardDuty — the reverse of what the removed code did
(deriving a richer indicator set *from* Severity). Since this emulator's
`builtInIndicators` is a synchronous, seeded-by-ID pure function with no real
analysis pipeline behind it (same "no real async/ML pipeline" class as the
amplify JobStatus/DomainStatus, networkmonitor MonitorState, and support
CaseDetails.Status issues closed the same day), there is no honest way to
compute a varying Severity, and inventing an ad hoc derivation rule (e.g.
keying off EntityType or scope length) would fabricate behavior with no
basis in the real API — this file's established "absent beats
plausible-but-wrong" rule (see the `MemberDetail`/`DisabledReason` gap
above) applies equally to Severity.

Given genuine unreachability, the dead branches were removed rather than
left in place (same precedent as networkmonitor's removal of
`monitorStatePending`/`probeStatePending` the same day): dead code that
looks like working severity-tiered logic is misleading on its own, distinct
from and worse than the underlying modelling gap. Removed both conditionals
from `builtInIndicators` (`investigations.go`) and the now-fully-unused
`severityLow`/`severityMedium`/`severityHigh`/`severityCritical` consts
(`models.go`), leaving `severityInformational` (still the sole value
written). `indicatorFlaggedIPAddress`/`indicatorImpossibleTravel`/
`indicatorRelatedFinding`/`indicatorRelatedFindingGroup` and the
`FlaggedIPAddressDetail`/`ImpossibleTravelDetail`/`RelatedFindingDetail`/
`RelatedFindingGroupDetail` wire structs and their `indicatorDetailToJSON`
encoder cases were left untouched — they are legitimate, order-independent
parts of the real 8-member `IndicatorDetail` union (`interfaces.go`) and
`validIndicatorTypes` filter set (`investigations.go`), not tied to Severity;
removing them would reduce genuine wire-shape completeness, and a filter for
an IndicatorType this emulator never emits correctly returns an empty list
rather than a `ValidationException` (a real investigation can legitimately
have zero indicators of a given type).

No regression test needed for the removal itself (zero observable behavior
change — the dead code never executed either before or after). Added a
pinning test instead, run against the pre-removal code first to confirm the
unreachability claim empirically rather than by grep alone:
`TestListIndicators_OnlySeverityFloorTypesProduced`
(`investigations_test.go`) starts an investigation, asserts `Severity ==
"INFORMATIONAL"`, then asserts `ListIndicators` never returns
`FLAGGED_IP_ADDRESS`/`IMPOSSIBLE_TRAVEL`/`RELATED_FINDING`/
`RELATED_FINDING_GROUP`. Passed both before and after the removal (`go test
-run TestListIndicators_OnlySeverityFloorTypesProduced -v`: `PASS` both
times) — it exists to guard against a future regression where someone
reintroduces a Severity-gated branch without also wiring a real way to reach
a non-floor Severity, not to pin a behavior change that happened here.

Also corrected a stale claim in this file's own `ListIndicators` `ops:` note
(written 2026-08-20): it said "all 8 real enum values are producible and
filterable" after adding `NEW_ASO`/`NEW_USER_AGENT` — true for the *filter*
side (`validIndicatorTypes`), never true for the *producible* side, since
StartInvestigation had already never assigned anything but INFORMATIONAL at
that point. Reworded to state only 4 of 8 are producible.

Gates: `go build ./services/detective/...` (clean), `go test -race
./services/detective/...` (`ok`), `golangci-lint run
./services/detective/...` (0 issues). Work left uncommitted per this pass's
instructions.
