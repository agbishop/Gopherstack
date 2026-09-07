---
service: shield
sdk_module: aws-sdk-go-v2/service/shield@v1.37.4
last_audit_commit: 2d47b51d4
last_audit_date: 2026-07-29
overall: A            # all documented gaps from the prior sweep closed; one invented op deleted
ops:
  CreateSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSubscriptionState: {wire: ok, errors: ok, state: ok, persist: n/a}
  CreateProtection: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this sweep -- now enforces subscriptionMaxProtections=1000 and subscriptionMaxProtectionsPerType=100 via checkProtectionQuotas, returning LimitsExceededException (ErrLimitExceeded) when exceeded, matching the limits CreateProtection itself reports via DescribeSubscription"}
  DescribeProtection: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteProtection: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this sweep -- now cascade-deletes the protection's alarConfigs row (was an orphaned-row leak; ApplicationLayerAutomaticResponseConfiguration is a field of the real Protection object, not an independent resource)"}
  ListProtections: {wire: ok, errors: ok, state: ok, persist: ok, note: "InclusionFilters + offset pagination verified against InclusionProtectionFilters"}
  AssociateHealthCheck: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateHealthCheck: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "resolves both Shield protection ARN and resource ARN; resolveShieldProtectionARN partition prefix fixed this sweep, see below"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateDRTLogBucket: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this sweep -- now requires AssociateDRTRole to have been called first (NoAssociatedRoleException/ErrNoAssociatedRole) and enforces the documented 10-bucket cap (LimitsExceededException/ErrLimitExceeded), matching real AWS SRT-authorization prerequisite behavior"}
  DisassociateDRTLogBucket: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateDRTRole: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateDRTRole: {wire: ok, errors: ok, state: ok, persist: ok, note: "idempotent per AWS"}
  DescribeDRTAccess: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this sweep -- RoleArn key is now omitted from the response when unset instead of emitting an empty string, matching the real *string (nil-omitted) field"}
  AssociateProactiveEngagementDetails: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateEmergencyContactSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEmergencyContactSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableProactiveEngagement: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableProactiveEngagement: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateProtectionGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this sweep -- now enforces subscriptionMaxProtectionGroups=100 and the ARBITRARY-pattern subscriptionMaxMembersPerGroup=10000 quota, returning LimitsExceededException (ErrLimitExceeded) when exceeded"}
  DescribeProtectionGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ListProtectionGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateProtectionGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this sweep -- now enforces the same ARBITRARY-pattern subscriptionMaxMembersPerGroup=10000 quota as CreateProtectionGroup"}
  DeleteProtectionGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAttacks: {wire: ok, errors: ok, state: ok, persist: ok, note: "TimeRange {FromInclusive/ToExclusive} shape verified against types.TimeRange"}
  DescribeAttack: {wire: ok, errors: ok, state: ok, persist: ok, note: "AttackProperties/SubResources (optional AWS fields) never populated -- acceptable, see gaps"}
  DescribeAttackStatistics: {wire: ok, errors: ok, state: ok, persist: n/a, note: "top-level TimeRange/DataItems, no wrapper -- matches DescribeAttackStatisticsOutput"}
  EnableApplicationLayerAutomaticResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableApplicationLayerAutomaticResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApplicationLayerAutomaticResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  ListResourcesInProtectionGroup: {wire: ok, errors: ok, state: ok, persist: n/a, note: "derives ALL/BY_RESOURCE_TYPE membership live from the protections table"}
deleted_invented_ops:
  - GetAttackVectorDefinitionVersion: not present anywhere in aws-sdk-go-v2/service/shield@v1.34.20
    (no api_op_GetAttackVectorDefinitionVersion.go, no client method, no mention in types.go/
    enums.go). This was a gopherstack-invented operation with a fabricated
    AttackVectorDefinitionVersion response field. Deleted this sweep: removed from
    GetSupportedOperations, its dispatch case, its handler func + constant in
    handler_attacks.go, and its two dedicated tests in handler_attacks_test.go. TestHandler_OpsLen
    updated 37 -> 36. sdk_completeness_test.go (sdkcheck.CheckCompleteness) does not flag extra
    ops, only missing ones, so this was silently accepted by the completeness gate before deletion.
families:
  protection: {status: ok, note: "CreateProtection/DescribeProtection/DeleteProtection/ListProtections verified against types.Protection -- no CreationTime/Tags fields on the real shape; gopherstack emits an extra CreationTime, harmless (unknown-field-tolerant awsjson1.1 deserializer)"}
  protectionGroup: {status: ok, note: "same extra-CreationTime note as protection; ProtectionGroup has no CreationTime in real SDK either"}
  subscription: {status: ok, note: "TimeCommitmentInSeconds bug (prior sweep) still holds; Limits/SubscriptionLimits nested shapes verified field-by-field against types.SubscriptionLimits/ProtectionLimits/ProtectionGroupLimits -- gopherstack's extra 'MaxProtections' key on ProtectionLimits is fabricated (not a real field) but harmless since it's additive"}
  attack: {status: ok, note: "ListAttacks/DescribeAttack/DescribeAttackStatistics verified against types.AttackSummary/AttackDetail/AttackStatisticsDataItem/AttackVolume"}
  tags: {status: ok, note: "resolveTaggableProtection accepts both Shield protection ARN and resource ARN, matching TagResourceInput.ResourceARN semantics; partition-prefix bug fixed this sweep"}
  drtAccessAndEngagement: {status: ok, note: "AssociateDRTRole/LogBucket (now enforces role-before-bucket prerequisite + 10-bucket cap), proactive engagement state machine (DISABLED->PENDING->ENABLED) verified"}
  alar: {status: ok, note: "ApplicationLayerAutomaticResponseConfiguration nested in Protection response only when set, matching optional-field AWS behavior; cascade-delete-on-DeleteProtection fixed this sweep"}
  quotas: {status: ok, note: "fixed this sweep -- CreateProtection/CreateProtectionGroup/UpdateProtectionGroup/AssociateDRTLogBucket now enforce every quota they themselves report via DescribeSubscription or that real AWS documents (subscriptionMaxProtections, subscriptionMaxProtectionsPerType, subscriptionMaxProtectionGroups, subscriptionMaxMembersPerGroup, 10-bucket DRT log bucket cap), returning LimitsExceededException (new ErrLimitExceeded sentinel) via handler.go's classifyShieldError"}
gaps:
  - "IMPOSSIBLE (re-confirmed gopherstack-kp7b): DescribeAttack/ListAttacks never populate AttackDetail.AttackProperties or AttackDetail.SubResources (both optional AWS fields); simulated/internal attacks only carry AttackVectors/AttackCounters/Mitigations. This is NOT a chaos-coverable gap (chaos only injects error responses, not fabricated success-payload data) and was re-examined against types.AttackProperty/types.Contributor/types.SubResourceSummary in the vendored SDK this pass: AttackProperty.TopContributors is a list of Contributor{Name, Value int64} -- e.g. a source-country name with a traffic-volume count -- and SubResourceSummary.Counters is a list of SummarizedCounter (Average/Max/Median/Sum/N, real statistical aggregates). gopherstack has no real network traffic for a simulated attack to report on, so populating either field would mean inventing plausible-looking contributor names and traffic counts with zero grounding -- exactly the 'invented metrics/counts' this project's honesty rules forbid, not a smaller version of a real feature. Left honestly absent (the real field is optional and simply omitted when Shield has nothing to report, which is what a synthetic attack's true state is). DescribeAttack/ListAttacks remain fully AWS-shape-correct for every field they DO populate."
  - "IMPOSSIBLE (re-confirmed gopherstack-kp7b): LockedSubscriptionException (subscription's first-year AutoRenew lock, changeable only in the last 30 days of the commitment) is not modeled -- UpdateSubscription always allows changing AutoRenew. Deliberately NOT implemented: gopherstack subscriptions are always \"fresh\" (no historical passage of time), so enforcing the real 335-day lock would make UpdateSubscription permanently fail for every subscription in the emulator, which is worse for testability than the current permissive behavior. Documented gap, not a wire bug. (Not chaos-relevant either way: a caller that specifically wants to exercise this __type can already do so via chaos fault injection on UpdateSubscription, same as the three items below.)"
deferred:
  - "ALREADY COVERED BY CHAOS (verified gopherstack-kp7b): OptimisticLockException (concurrent-modification detection via a resource version/etag) is not modeled anywhere -- CreateProtectionGroup/UpdateProtectionGroup/DeleteProtectionGroup/AssociateDRTLogBucket/DisassociateDRTLogBucket/UpdateEmergencyContactSettings all declare it in their real error catalogs but gopherstack's coarse per-backend lock (lockmetrics.RWMutex) makes every mutation atomic, so the race window OptimisticLockException exists to protect against never occurs in this emulator's backend state. Concretely verified this pass: shield.Handler implements ChaosServiceName() -> \"shield\" and ChaosOperations() -> h.GetSupportedOperations() (handler.go), and pkgs/chaos.Middleware is wired globally via registry.Use(chaos.Middleware(faultStore)) in cli.go -- it matches purely on the request's SigV4 service name + X-Amz-Target operation + region and injects an arbitrary caller-specified FaultError{Code, StatusCode} without touching backend state, so a fault rule such as {\"service\":\"shield\",\"operation\":\"UpdateProtectionGroup\",\"error\":{\"code\":\"OptimisticLockException\",\"statusCode\":400}} deterministically returns that exact typed error to a real client with zero backend code changes."
  - "ALREADY COVERED BY CHAOS (verified gopherstack-kp7b): AccessDeniedException / AccessDeniedForDependencyException are never returned -- gopherstack does not model IAM permission checks for any service, Shield included, so there is no backend-state condition to trigger either from. Consistent with the rest of the codebase; not a Shield-specific gap. Same chaos mechanism as OptimisticLockException above makes both reachable on demand for a caller that wants to test its own error-handling path, with zero backend code changes."
  - "ALREADY COVERED BY CHAOS (verified gopherstack-kp7b): InvalidResourceException (thrown by real AWS when a ResourceArn is a well-formed ARN for a supported type but the underlying resource doesn't exist / isn't accessible) is not distinguished from InvalidParameterException (used for malformed/unsupported-type ARNs) because gopherstack has no cross-service resource-existence oracle to check against. Would require wiring Shield's CreateProtection to query other services' backends (elbv2/cloudfront/route53/ec2/globalaccelerator) for resource existence -- that kind of cross-service backend reference is set up at CLI init time (cli.go), out of bounds for this pass (see applicationautoscaling's PARITY.md for the same cli.go-wiring constraint on a different service). Same chaos mechanism as above makes InvalidResourceException reachable on demand in the meantime."
leaks: {status: clean, note: "no goroutines/janitors in this service; all state lives in store.Table-backed maps guarded by lockmetrics.RWMutex; Snapshot/Restore round-trip verified (persistence_test.go). Fixed this sweep: DeleteProtection previously left an orphaned alarConfigs row keyed by the deleted protection's ResourceARN -- a later CreateProtection for the same ResourceARN would incorrectly inherit the stale ALAR config from the deleted protection. Now cascade-cleaned; regression test in protections_test.go (TestInMemoryBackend_DeleteProtectionCascadeCleansALARConfig)."}
---

## Notes

- 2026-08-22, gopherstack-r80d batch 30 (required-output-member audit):
  shield (6 required output fields / 36 ops, 5 ops-with-required per a fresh
  `cmd/requiredoutputfields` run, cross-checked against an independent
  brace-depth awk walk of `shield@v1.37.4`'s `api_op_*.go` files -- both
  agreed exactly at 6). Read all 5 flagged ops end to end against their
  handlers: `DescribeAttackStatistics` (`DataItems`, `TimeRange`),
  `DescribeProtectionGroup` (`ProtectionGroup`), `GetSubscriptionState`
  (`SubscriptionState`), `ListProtectionGroups` (`ProtectionGroups`),
  `ListResourcesInProtectionGroup` (`ResourceArns`). All 5 handlers build
  their response as a `map[string]any`/`json.Marshal` literal
  (`handler_attacks.go`, `handler_protection_groups.go`,
  `handler_subscription.go`), so the protocol question this campaign asks
  per service comes back the same way as ssoadmin/mediatailor this batch:
  the tag-rule doesn't apply, every required key is written unconditionally.
  Followed two wrapped types below the flat scan: `TimeRange`
  (types.go:644-653) has zero required members of its own (confirmed against
  the SDK type directly); `AttackStatisticsDataItem` (types.go:106-119) has a
  required `AttackCount` one level below `DataItems` -- gopherstack's own
  `AttackStatisticsItem` (models.go:169-172) tags it `json:"AttackCount"`
  with no `omitempty`, and `DescribeAttackStatistics` (attacks.go:214-266)
  already guarantees a non-empty `DataItems` slice (seeding one
  `{AttackCount: 0}` item when no attacks exist in the trailing year), so the
  same "required-but-inapplicable means present-and-empty, not absent"
  convention this campaign established elsewhere is already correctly
  applied here. Also followed `ProtectionGroup`'s own 4 required members
  (types.go:374-424, `Aggregation`/`Members`/`Pattern`/`ProtectionGroupId`) --
  already documented above in this same Notes section's
  `types.ProtectionGroup` struct citation -- `protectionGroupToMap`
  (handler_protection_groups.go:220-236) writes all 4 unconditionally.
  Result: 0 bugs. No code changes.
- Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: AWSShield_20160616.<Op>`.
  Route matcher checked against the real SDK's `awsAwsjson11_serializeOp*` files -- target
  prefix is exactly `AWSShield_20160616.` (verified via `serializers.go`
  `resolveAuthSchemeOptions`/build-target constant strings across every `api_op_*.go`).
- Real `aws-sdk-go-v2` JSON deserializers silently ignore unrecognized response keys
  (`default: _, _ = key, value` in every `awsAwsjson11_deserializeDocument*` case, confirmed
  at the end of `awsAwsjson11_deserializeDocumentProtection` in `deserializers.go`). This
  means gopherstack emitting *extra* fields the real API doesn't have is additive-only, not
  a wire break -- but that verdict must be re-derivable, not taken on trust. Evidence, checked
  2026-08-13 against `aws-sdk-go-v2/service/shield@v1.37.4`: `types.Protection` (types.go:343-369)
  has exactly `ApplicationLayerAutomaticResponseConfiguration`/`HealthCheckIds`/`Id`/`Name`/
  `ProtectionArn`/`ResourceArn`, no `CreationTime`; `types.ProtectionGroup` (types.go:374-424)
  has exactly `Aggregation`/`Members`/`Pattern`/`ProtectionGroupId`/`ProtectionGroupArn`/
  `ResourceType`, no `CreationTime` either; `types.ProtectionLimits` (types.go:469-477) has
  exactly one member, `ProtectedResourceTypeLimits`, no `MaxProtections`. Re-check this by
  diffing those three struct definitions against the pinned version above -- if a future SDK
  bump adds any of these fields for real, gopherstack's existing extra key becomes a
  same-named coincidence to re-verify, not an automatic pass. Only *missing* or *misnamed*
  fields the SDK actively reads are real bugs (the `TimeCommitmentInSeconds` bug fixed a
  prior sweep was the latter kind).
- Real `types.Subscription.TimeCommitmentInSeconds` is `int64` seconds. gopherstack's
  internal `Subscription.TimeCommitmentInDays` field/JSON tag intentionally kept as *days*
  (readable business value, 365) -- the seconds conversion happens only at serialization
  time in `handler.go` (`secondsPerDay` constant). No persistence/snapshot format change
  was needed since the internal DTO shape didn't change, only the wire response.
  `subscriptionCommitmentDays` in backend.go remains the source of truth in days.
  If this internal field is ever renamed, remember `shieldSnapshotVersion` must be bumped
  per the doc comment in persistence.go.
- `floatSeconds()` in handler.go duplicates `pkgs/awstime.Epoch()` byte-for-byte (both
  compute `float64(t.UnixNano()) / 1e9` with a nanosecond-precision divide, though
  `awstime.Epoch` additionally special-cases the zero `time.Time` to return exactly `0`).
  Not fixed this sweep (out of the "real bug" budget -- purely a reuse/style item, not a
  parity bug since Shield's own timestamps are never zero-valued in practice), but a future
  pass should switch handler.go to `awstime.Epoch` and delete `floatSeconds`/`nanosPerSecond`.
- Error mapping (`classifyShieldError` in handler.go, called from `handleError`) is now a
  data-driven ordered rule table (`shieldErrorRules`) instead of a hardcoded switch, so new
  sentinel -> wire-code mappings can be added without growing handleError's own complexity.
  Fixed this sweep: (1) `ErrSubscriptionRequired` wraps `awserr.ErrConflict` internally (for
  backward-compatible `errors.Is` matching) but was being serialized as
  `ResourceAlreadyExistsException` instead of the real `InvalidOperationException` -- it is
  now listed ahead of the generic `awserr.ErrConflict` rule so the specific mapping wins;
  (2) `errInvalidPaginationToken` fell through to the `default` case and was serialized as a
  500 `InternalErrorException` instead of the real 400 `InvalidPaginationTokenException` --
  this was compounded by a second bug in all three list handlers (handleListProtections,
  handleListProtectionGroups, handleListAttacks in handler_protections.go/
  handler_protection_groups.go/handler_attacks.go), which re-wrapped `decodeOffsetToken`'s
  error as `fmt.Errorf("%w: %s", errInvalidRequest, err.Error())` -- using `%s` (a plain
  string) for the original error discarded the `errInvalidPaginationToken` sentinel from the
  chain entirely, so even after fixing handleError's rule table the specific __type could
  never surface. Fixed by wrapping with `%w` instead (`fmt.Errorf("invalid NextToken: %w",
  err)`), which properly chains errInvalidPaginationToken so errors.Is finds it;
  (3) added `ErrLimitExceeded` -> `LimitsExceededException` and `ErrNoAssociatedRole` ->
  `NoAssociatedRoleException`, both newly raised by the quota/DRT-prerequisite fixes above.
  Regression tests: errors_test.go (TestHandler_ErrorWireType_*).
  All of gopherstack's error responses now use only real Shield `__type` values:
  `ResourceNotFoundException`, `ResourceAlreadyExistsException`, `InvalidParameterException`,
  `InvalidOperationException`, `InvalidPaginationTokenException`, `LimitsExceededException`,
  `NoAssociatedRoleException`, `InternalErrorException`.
- `protectionARN`/`protectionGroupARN` (protections.go/protection_groups.go) previously called
  `arn.Build("shield", "", accountID, resource)` with a hardcoded empty region string, which
  made `arn.PartitionForRegion("")` always resolve to `"aws"` regardless of the backend's actual
  configured region -- so a GovCloud/China/ISO backend would still mint `arn:aws:shield::...`
  protection ARNs instead of `arn:aws-us-gov:shield::...` etc. `arn.Build`'s only
  region-omitted-but-partition-correct special case is `service=="iam"`; rather than adding a
  second special case to the shared `pkgs/arn` package (which every other service also
  depends on), both functions now build the ARN string directly with
  `arn.PartitionForRegion(region)`. `resolveShieldProtectionARN` (tags.go) was updated in
  lockstep -- it now derives the expected `arn:{partition}:shield::` prefix from `b.region`
  instead of hardcoding `arn:aws:shield::`. Regression test:
  `TestInMemoryBackend_TagResourceByShieldARNGovCloudPartition` (tags_test.go).

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 3 confirmed bugs

- `DescribeProtection`/`ListProtections`: {wire: fixed} -- protectionToMap
  emitted "CreationTime"; types.Protection has no such member
  (ApplicationLayerAutomaticResponseConfiguration/HealthCheckIds/Id/Name/
  ProtectionArn/ResourceArn only).
- `DescribeProtectionGroup`/`ListProtectionGroups`: {wire: fixed} --
  protectionGroupToMap emitted "CreationTime"; types.ProtectionGroup has no
  such member.
- `DescribeSubscription`: {wire: fixed} -- ProtectionLimits carried a
  fabricated "MaxProtections" key; types.ProtectionLimits declares only
  ProtectedResourceTypeLimits.

All 3 proven via real `aws-sdk-go-v2/service/shield` client round trips or
raw-body assertion (handler_protections_test.go strengthened in place,
wire_field_fixes_y1zn_test.go new), hand-reverted/confirmed-failing/
restored/`md5sum`-verified byte-identical.

## gopherstack-zquj (2026-08-22): type-checked hand-written-key sweep, clean

zquj's premise: shield's map-literal responses (41 `map[string](any|
interface{}|string)` literal openings across the non-test package, per
`grep -rEo 'map\[string\](any|interface\{\}|string)\{'`) are immune to
`omitempty`/struct-tag bugs but have no compiler, struct-tag scanner, or
raw-body test checking their hand-typed key strings against anything real
-- exactly the shape y1zn (above) had already found 3 bugs in a day
earlier. Built a type-checked scanner (`keycheck`, not committed into this
repo this pass -- see the note below) rather than grepping, per zquj's
explicit instruction: it parses `shield@v1.37.4`'s `deserializers.go` AST to build
each op's real wire-key tree from the `awsAwsjson11_deserializeOpDocument*`
/ `awsAwsjson11_deserializeDocument*` case-switch lists (the deserializer
is the authority, not the Go field name), then parses the service's own
handler `.go` files for every string-keyed map literal and `X["key"]=`
assignment reachable from each op's dispatch handler, and diffs.

**Instrument validated against gopherstack-r80d batch 32's scheduler
casing bug (commit `8469dcdd9`) before trusting it**: fed the scanner's
SDK-side parser `scheduler@v1.20.4`'s deserializers.go and confirmed it
extracts `NetworkConfiguration`'s real case key as lowercase-first
`"awsvpcConfiguration"` (deserializers.go:2723-2748) and
`CapacityProviderStrategyItem`'s as `"capacityProvider"`/`"base"`/
`"weight"` (deserializers.go:2254-2287) -- not the pre-fix capitalized Go
field names -- so a hand-written `"AwsvpcConfiguration"`/
`"CapacityProvider"` would correctly diff as not-in-tree. Handler-side
const-key resolution, nested-map traversal, and `X["key"]=` tracking were
separately validated against a synthetic fixture with a known-planted
typo'd key, which the scanner caught with zero false positives on its
correctly-keyed siblings (including a const-resolved key and a
nested-through-two-levels `IndexExpr` assignment).

**Two scanner false-positive classes found and fixed before trusting a
clean result**: (1) `map[string]struct{}`/`map[string]bool` string-keyed
literals are validation sets (`validAggregations`/`validPatterns` in
protection_groups.go), not wire output -- excluded by value type; (2) the
shared error-envelope keys `__type`/`message` are written by every op's
error path via a common helper, which a call-graph BFS that can't
distinguish success from error paths would otherwise attribute to every
op's *success* shape -- excluded as protocol-reserved.

**Result: 36 real ops resolved (37 dispatched minus the internal
`__SimulateAttack` test-only op, which has no real SDK output type), 89
tracked written keys, 0 keys outside the op's real reachable wire shape.**
Spot-verified two of the largest-surface ops by hand against the raw
deserializer rather than trusting the tool alone: `DescribeSubscription`'s
8 written keys match `Subscription`'s case list exactly
(deserializers.go:7048-7159, called from
`awsAwsjson11_deserializeOpDocumentDescribeSubscriptionOutput` at
deserializers.go:8291); the y1zn fixes above already removed this
service's only known instances of this bug class, so a clean result here
is corroboration, not a surprise.

**Known blind spots, disclosed rather than hidden**: the scanner checks
"does this key exist anywhere in the op's reachable shape," not "at the
right nesting level" -- a key misplaced one level off from a same-named
sibling field elsewhere in the same op's tree would not be caught (no such
case found here on manual spot-check, see above). Two `X["key"]=` sites
were skipped as dynamic (a `map[string]any`-named local in one function
colliding, by name only, with an unrelated `map[string]struct{}` in
`sliceToSet`, handler.go:414-425 -- harmless here since the skipped value
was never a candidate wire key, but a real limitation of name-based rather
than scope-based variable tracking).

Did not commit the scanner into `cmd/` this pass -- a second agent was
editing this working tree concurrently under `services/`, and a new
whole-repo build unit is exactly the kind of change that shouldn't land
mid-session next to someone else's in-flight edits. Its source is
preserved (not in this repo) for promotion; see gopherstack-zquj's
followup issue for reusing it the way `cmd/opcensus` is reused, rather
than re-deriving the SDK-deserializer-AST approach per sweep.

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors

`Handler()`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.
shield is awsjson1.1 (confirmed from `shield@v1.37.4` deserializers.go's
`awsAwsjson11_deserializeOpError*` prefix); plain text doesn't decode
through `restjson.GetErrorInfo`, so a real client got `*json.SyntaxError`,
not even `UnknownError`.

Fixed by routing the ReadBody error through this handler's own
`handleError(c, err)`: `classifyShieldError` checks for
`json.SyntaxError`/`json.UnmarshalTypeError` first (neither matches a
`*http.MaxBytesError`/read error) then `shieldErrorRules()` sentinels, so
it falls through to the pre-existing default (`"InternalErrorException"`,
500) -- modeled at `shield@v1.37.4` `types/errors.go:80`.

CONFIRMED (documented "left untyped"/deliberate-gap decisions distinct
from error-typing, already on file for this service, not touched by this
fix): `LockedSubscriptionException` is deliberately not modeled
(gopherstack-kp7b, this file's Notes) since gopherstack subscriptions have
no historical passage of time; `OptimisticLockException` is deliberately
not modeled since the coarse per-backend lock makes every mutation atomic
(same note, also chaos-injectable). Neither is an error-dispatch gap like
the ReadBody-failure path this fix addresses.

Proven with a real `aws-sdk-go-v2/service/shield` client's
`CreateProtection`, whose `Name` field alone exceeds
`httputils.MaxRequestBodyBytes` (16 MiB).
`TestHandler_OversizedBodySurfacesInternalErrorException`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalErrorException"`; confirmed it fails pre-fix with
`*json.SyntaxError` (hand-reverted, byte-identical restore after).
- **2026-08-19 wrapper-key/nested-shape re-audit (independent, this session):** protocol
  re-confirmed as awsjson1.1 from `serializers.go` (`X-Amz-Target:
  AWSShield_20160616.<Op>`) and `HandleDeserialize` bodies decode via
  `deserializeOpDocument<Op>Output` for every op (36/36) -- no dead-helper trap here
  (unlike restjson's optional-flattening case, awsjson1.1 output structs always decode
  through their own OpDocument helper). Verified field-by-field against the pinned
  `shield@v1.37.4` `types/types.go` + `deserializers.go` case lists, all the way down:
  `Subscription`/`SubscriptionLimits`/`ProtectionLimits`/`ProtectionGroupLimits`/
  `ProtectionGroupPatternTypeLimits`/`ProtectionGroupArbitraryPatternLimits`/`Limit`
  (subscription.go/handler_subscription.go), `AttackDetail`/`AttackSummary`/
  `SummarizedCounter`/`Mitigation`/`AttackVectorDescription`/`AttackStatisticsDataItem`/
  `AttackVolume`/`AttackVolumeStatistics`/`TimeRange` (attacks.go/handler_attacks.go),
  `Protection`/`ProtectionGroup`/`ApplicationLayerAutomaticResponseConfiguration`/
  `ResponseAction`/`BlockAction`/`CountAction` (handler_protections.go/
  handler_protection_groups.go/handler_application_layer_automatic_response.go),
  `EmergencyContact` (handler_emergency_contacts.go), `DescribeDRTAccessOutput`
  (handler_drt.go). Confirmed the `TagResource`/`ListTagsForResource`/`UntagResource`
  family correctly uses the ALL-CAPS `ResourceARN` wire key (`api_op_TagResource.go`,
  `serializeOpDocumentTagResourceInput`) where every other op in this service uses
  `ResourceArn` -- gopherstack already has this right (handler_tags.go:12,35,60). Zero
  new bugs found; every existing PARITY.md "ok" verdict in the areas re-checked above
  reproduced from the live deserializer, not taken on trust. Gates: `go build`, `go vet`,
  `go fix -diff` (empty), `gofmt -l` (empty), `go test -race` (ok, 1.1s),
  `golangci-lint run` (0 issues) all clean, no code changes made this session.
- **2026-08-31, gopherstack-uox6 (value-semantics sweep, first pass on this service for
  this class)**: the wire-shape audits above (this file's `ops`/`families` grades) check
  that fields exist, are read, and round-trip -- a separate axis from whether a filter's
  documented VALUE semantics are honored once read. `cmd/covledger -service shield`
  reported no rows going into this pass; no contradicting evidence found in git log or
  this file's prior notes. Checked all 12 List/Describe ops' filter and pagination
  parameters against their own SDK doc comments (`aws-sdk-go-v2/service/shield@v1.37.4`).
  Three real bugs found and fixed:
  - `ListAttacks`'s `EndTime.ToExclusive` boundary was inclusive in code
    (`attacks.go`'s `ListAttacks`: `ts > endTime` kept `ts == endTime`) where its own
    field name says exclusive (`types.TimeRange.ToExclusive`, "Unix time in seconds") --
    an attack starting exactly at the boundary was wrongly included. `FromInclusive`'s
    boundary was already correct. Fixed to `ts >= endTime`.
  - `ListProtections`/`ListProtectionGroups`/`ListAttacks` all documented "The default
    setting is 20" for an omitted `MaxResults` (`api_op_List*.go` doc comments,
    identical wording on all four Shield Advanced list ops including
    `ListResourcesInProtectionGroup` below), but `clampMaxResults`'s `v <= 0` branch
    returned the handler's internal page-size CAP (1000/1000/10000) instead -- a client
    that omitted `MaxResults` got up to 50x-500x more items per page than real AWS, in
    one page instead of paginated. Fixed: `clampMaxResults` now returns the new
    `defaultListPageSize = 20` constant when `MaxResults` is omitted, and only clamps an
    explicitly-supplied value to the existing per-op cap.
  - `ListResourcesInProtectionGroup` implemented NO pagination at all --
    `MaxResults`/`NextToken` weren't even parsed from the request, every member ARN was
    always returned in one response, ignoring the same documented default-20 behavior as
    its three siblings. Fixed: added the same offset-token pagination pattern used by
    `ListProtections`/`ListProtectionGroups`/`ListAttacks`.

  `ListProtections`'s `InclusionFilters` (`ProtectionNames`/`ResourceArns`/
  `ResourceTypes`) and `ListProtectionGroups`'s `InclusionFilters`
  (`ProtectionGroupIds`/`Patterns`/`ResourceTypes`/`Aggregations`) combining logic was
  checked against the SDK's "exactly match all of the filter criteria that you provide"
  wording and is correct: AND across filter categories, OR within a category's value
  list; unrecognized `ResourceTypes` values correctly reject rather than match-all
  (`resourceARNMatchesType`'s switch has no default-true case). `MaxResults` upper-bound
  caps (1000/1000/10000) are gopherstack-internal choices, not contradicted by any
  documented maximum (the SDK doc comments state only the default, no ceiling) --
  left unchanged.

## gopherstack-g2l5 (2026-09-07): per-op declared-error-catalog mismatches, 3 confirmed bugs

`cmd/errtargetaudit` flagged 4 class A findings: gopherstack emitting a wire `__type` that the
real op's own error catalog (`deserializers.go`'s `deserializeOpError<Op>` case list, confirmed
per-op below) never declares. Root cause in all 4: `classifyShieldError`'s `shieldErrorRules()`
is one global sentinel -> code table shared by every op, but several sentinels are raised by
handlers whose real declared catalog differs from the code the shared rule assigns.

- `CreateProtectionGroup` (`protection_groups.go`, subscription-required check) and `TagResource`
  (`tags.go`, subscription-required check) both raised `ErrSubscriptionRequired`, which
  `shieldErrorRules()` maps to `InvalidOperationException` -- correct for the other 7 ops that
  raise it (`CreateProtection`, `AssociateDRTLogBucket`, `AssociateDRTRole`,
  `AssociateProactiveEngagementDetails`, `EnableProactiveEngagement`,
  `DisableProactiveEngagement`, `EnableApplicationLayerAutomaticResponse`, all confirmed to
  declare `InvalidOperationException`), but `CreateProtectionGroup`/`TagResource`'s own catalogs
  declare no `InvalidOperationException` at all. Both catalogs do declare
  `ResourceNotFoundException`, which is also the real Shield code for "no subscription" per
  `DescribeSubscription`'s own catalog and gopherstack's existing `ErrSubscriptionNotFound`
  sentinel (`subscription.go`) -- switched both sites to raise `ErrSubscriptionNotFound` instead.
- `UpdateProtectionGroup`'s ARBITRARY-pattern member-cap check (`protection_groups.go`) raised
  `ErrLimitExceeded` (-> `LimitsExceededException`), correct for the identical check in
  `CreateProtectionGroup` (whose catalog does declare it) but not for `UpdateProtectionGroup`,
  whose catalog has no `LimitsExceededException` entry. Switched to `ErrValidation` (->
  `InvalidParameterException`, which the catalog does declare); the wire response carries only a
  message string (no structured `Type`/`Limit` fields), so no response-shape change was needed.
- `ListAttacks`'s `NextToken` decode error (`handler_attacks.go`) chained `decodeOffsetToken`'s
  shared `errInvalidPaginationToken` sentinel (-> `InvalidPaginationTokenException`), correct for
  the pagination helper's other 3 callers (`ListProtections`/`ListProtectionGroups`/
  `ListResourcesInProtectionGroup`, all confirmed to declare it) but `ListAttacks`'s own catalog
  has no `InvalidPaginationTokenException` entry (only `InvalidOperationException`/
  `InvalidParameterException`). Re-classified via `errInvalidRequest` (->
  `InvalidParameterException`) instead of forwarding the original sentinel.

All 4 sites verified against the pinned `shield@v1.37.4` `deserializers.go`
(`awk '/deserializeOpError<Op>\(/,/^}/'` extraction, raw output below) rather than guessed:

```
CreateProtectionGroup: UnknownError InternalErrorException InvalidParameterException
  LimitsExceededException OptimisticLockException ResourceAlreadyExistsException
  ResourceNotFoundException
TagResource: UnknownError InternalErrorException InvalidParameterException
  InvalidResourceException ResourceNotFoundException
UpdateProtectionGroup: UnknownError InternalErrorException InvalidParameterException
  OptimisticLockException ResourceNotFoundException
ListAttacks: UnknownError InternalErrorException InvalidOperationException
  InvalidParameterException
```

Regression tests (`errors_test.go`): `TestHandler_ErrorWireType_CreateProtectionGroupSubscriptionRequired`,
`TestHandler_ErrorWireType_TagResourceSubscriptionRequired`,
`TestHandler_ErrorWireType_UpdateProtectionGroupMembersLimit`,
`TestHandler_ErrorWireType_ListAttacksInvalidPaginationToken` -- each drives the real HTTP handler,
asserts the correct `__type`, asserts the previously-wrong `__type` is absent, and (for the two
mutating ops) asserts the target resource was not created/mutated by the rejected request.
`quotas_test.go`'s pre-existing `TestInMemoryBackend_UpdateProtectionGroupArbitraryMembersQuota`
previously asserted `errors.Is(err, shield.ErrLimitExceeded)` with no note that this pinned the
now-fixed wrong code -- updated to assert `ErrValidation`/absence of `ErrLimitExceeded` and that
the group's members are left unmutated. All 4 fixed lines hand-reverted one at a time, confirmed
each still compiles and its own regression test fails pre-fix, then restored.

**Re-running `cmd/errtargetaudit` after the fix**: 3 of the 4 findings are gone. The
`ListAttacks` finding still appears verbatim (`handler.go:450`/`455`, `declared correctly by:
[ListProtectionGroups ListProtections ListResourcesInProtectionGroup]`) -- this is the tool's
documented one-hop-callee-trace limitation, not a live bug: it still sees `ListAttacks` calling
`decodeOffsetToken`, which contains the `errInvalidPaginationToken` literal, and cannot tell that
the fixed call site checks `err != nil` but no longer forwards that specific sentinel into the
returned error (it wraps `errInvalidRequest` instead). Confirmed by
`TestHandler_ErrorWireType_ListAttacksInvalidPaginationToken`, which passes against the real HTTP
handler and asserts the wire type is `InvalidParameterException`, not
`InvalidPaginationTokenException`. Coverage after the fix: 36 ops resolved, 11/36 (31%) with an
emission found (down from 13/36 pre-fix, since 3 of the fixed sites now emit via the same
generic already-attributed codes as other ops rather than a distinct sentinel reference).

Gates: `go test -race -count=1 ./services/shield/...` ok (1.5s); `golangci-lint run
services/shield/...` 0 issues.
