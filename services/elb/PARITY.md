---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: elb
sdk_module: aws-sdk-go-v2/service/elasticloadbalancing@v1.36.4   # version audited against
last_audit_commit: 9249d4561                      # HEAD when this audit began (working tree, pre-commit)
last_audit_date: 2026-08-20
overall: A            # A = genuine fixes found; B = already-accurate, proven op-by-op
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateLoadBalancer: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed InvalidScheme/UnsupportedProtocol/TooManyLoadBalancers/DuplicateTagKeys/TooManyTags error codes (were generic ValidationError); parity-3: inline HTTPS/SSL Listeners.member.N.SSLCertificateId now runs the same ARN-format check as SetLoadBalancerListenerSSLCertificate (was accepted unchecked at creation time, format-checked only on later Set calls); gopherstack-6851: inline SSLCertificateId now also existence-checked against the real ACM/IAM backends when cli.go's wireELBCrossService has wired a CertificateResolver in (nil resolver, e.g. isolated unit tests, stays permissive)"}
  DeleteLoadBalancer: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed missing DeleteLoadBalancerResult wrapper (real SDK GetElement failed)"}
  DescribeLoadBalancers: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-3: fixed missing LoadBalancerDescription.Policies field -- was entirely absent from the response struct, so every real client saw an always-empty Policies regardless of what stickiness/other policies existed; now populates AppCookieStickinessPolicies/LBCookieStickinessPolicies/OtherPolicies from the LB's policy set"}
  CreateLoadBalancerListeners: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed UnsupportedProtocol error code via shared parseOneListener; parity-3: fixed classic-listeners limit-exceeded error code (was ValidationError, real op's typed-error switch only has InvalidConfigurationRequest/CertificateNotFound/DuplicateListener/LoadBalancerNotFound/UnsupportedProtocol); inline SSLCertificateId now format-validated (see CreateLoadBalancer note); gopherstack-6851: now also existence-checked via the same CertificateResolver as CreateLoadBalancer/SetLoadBalancerListenerSSLCertificate"}
  DeleteLoadBalancerListeners: {wire: ok, errors: ok, state: ok, persist: ok}
  RegisterInstancesWithLoadBalancer: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-3: deleted invented classic-registered-instances (1000) hard-reject -- real op's typed-error switch only recognizes InvalidInstance/LoadBalancerNotFound, no typed exception exists for exceeding this DescribeAccountLimits-advertised limit, so enforcing it rejected requests a real AWS client would have had accepted; gopherstack-5c3m: now raises InvalidInstance for an instance id that doesn't resolve against the real EC2 backend, via a new elb.EC2Resolver.InstanceExists method (existing EC2Resolver, previously SecurityGroupExists/SubnetExists only); nil resolver stays permissive; cli.go's wireELBCrossService wiring is OUTSTANDING (cli.go was out of scope for this pass) -- elbEC2ResolverAdapter needs an InstanceExists method added before the repo-wide build passes again"}
  DeregisterInstancesFromLoadBalancer: {wire: ok, errors: ok, state: ok, persist: ok}
  ConfigureHealthCheck: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyLoadBalancerAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeLoadBalancerAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  AddTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed TooManyTags error code (was ValidationError); added missing DuplicateTagKeys same-request validation"}
  DescribeTags: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveTags: {wire: ok, errors: ok, state: ok, persist: ok}
  ApplySecurityGroupsToLoadBalancer: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-6851: now raises InvalidSecurityGroup for a security group that doesn't resolve against the real EC2 backend, via cli.go's wireELBCrossService (elb.EC2Resolver); nil resolver (e.g. isolated unit tests) stays permissive, matching this package's existing cross-service-resolver convention (services/directconnect/networkmanager)"}
  AttachLoadBalancerToSubnets: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-6851: now raises SubnetNotFound for a subnet that doesn't resolve against the real EC2 backend, via the same EC2Resolver as ApplySecurityGroupsToLoadBalancer; nil resolver stays permissive"}
  DetachLoadBalancerFromSubnets: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableAvailabilityZonesForLoadBalancer: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableAvailabilityZonesForLoadBalancer: {wire: ok, errors: ok, state: ok, persist: ok}
  SetLoadBalancerListenerSSLCertificate: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed missing Result wrapper; gopherstack-6851: CertificateNotFound now raised for an SSLCertificateId that resolves to neither a real ACM nor a real IAM server certificate, via cli.go's wireELBCrossService (elb.CertificateResolver checks both, since AWS accepts either -- aws-sdk-go-v2/service/elasticloadbalancing@v1.36.4 types/errors.go:36-39); nil resolver stays permissive"}
  SetLoadBalancerPoliciesOfListener: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed missing Result wrapper"}
  SetLoadBalancerPoliciesForBackendServer: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed missing Result wrapper"}
  CreateAppCookieStickinessPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed missing Result wrapper"}
  CreateLBCookieStickinessPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed missing Result wrapper"}
  CreateLoadBalancerPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed missing Result wrapper; fixed PolicyTypeNotFound error code (was generic ValidationError); added missing PublicKeyPolicyType to allowlist; TooManyPolicies not enforced (gap, see below); gopherstack-ogvw: PolicyAttributes.member.N.AttributeName is now checked against the given PolicyTypeName's builtinPolicyTypes() schema, raising InvalidConfigurationRequest for a name the type doesn't declare (previously any attribute name was silently accepted and stored); Cardinality (ONE/ZERO_OR_ONE/ZERO_OR_MORE/ONE_OR_MORE) is deliberately NOT enforced -- see validatePolicyAttributes doc comment in policies.go"}
  DeleteLoadBalancerPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed missing Result wrapper; parity-3: fixed policy-still-in-use error code (was ValidationError, real op's typed-error switch only has InvalidConfigurationRequest/LoadBalancerNotFound -- a ValidationError code would not deserialize into InvalidConfigurationRequestException, so errors.As would silently fail to match on a real client). Proven by Test_SDKRoundTrip_DeleteLoadBalancerPolicyInUse_IsTyped"}
  DescribeAccountLimits: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED this pass (gopherstack-uhsb): Marker/PageSize were parsed nowhere -- the (fixed, 3-row) limit catalog was always returned in full with no NextMarker, regardless of what a client asked for. Now paginates for real via the same opaque-offset Marker scheme (encodePageMarker/decodePageMarker) DescribeLoadBalancers already uses. Low real-world impact (the catalog only ever has 3 rows, per the official quota table -- see the gaps entry above), but the fix is cheap and the prior behavior was a genuine, if rarely-observable, divergence: a client requesting PageSize=1 got all 3 rows back with a 200 instead of 1 row plus a NextMarker."}
  DescribeInstanceHealth: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeLoadBalancerPolicies: {wire: ok, errors: ok, state: ok, persist: ok, note: "no-LoadBalancerName sample-policy fallback verified correct vs AWS docs, not a bug"}
  DescribeLoadBalancerPolicyTypes: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed wrong PolicyTypeNotFound error code (was reusing PolicyNotFound, the policy-instance sentinel)"}
# Families audited as a group (when per-op is impractical):
families:
  snapshot_restore: {status: ok, note: "Handler-level Snapshot/Restore delegation (persistence.go) verified intact; backend.Snapshot/Restore round-trip all LB + policy state incl. tags; version-guarded (v4) against incompatible older snapshots"}
  route_matcher: {status: ok, note: "single query/xml POST matcher (Version=2012-06-01 form field) confirmed reachable for all 29 dispatch-table ops; TestSDKCompleteness passes with empty notImplemented list"}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - gopherstack-6851 FOLLOW-UP addressed this pass: ApplySecurityGroupsToLoadBalancer/AttachLoadBalancerToSubnets now validate SecurityGroups/Subnets against the real EC2 backend (elb.EC2Resolver, wired by cli.go's wireELBCrossService), and CreateLoadBalancer/CreateLoadBalancerListeners/SetLoadBalancerListenerSSLCertificate now validate SSLCertificateId against the real ACM and IAM backends (elb.CertificateResolver, same wiring call). CreateLoadBalancer's own SecurityGroups/Subnets fields (as opposed to Apply/Attach) are NOT existence-checked -- out of scope for this pass, tracked separately if ever needed.
  - CreateLoadBalancerPolicy has no TooManyPolicies limit (AWS models TooManyPoliciesException for this op per the SDK's op-specific error switch, but no default per-LB policy count limit is documented anywhere gopherstack could source a correct number from; fabricating one risked being wrong, so left unenforced rather than guessed). Re-verified gopherstack-6851 2026-08-10: the official quota table at docs.aws.amazon.com/elasticloadbalancing/latest/classic/elb-limits.html lists exactly three Classic ELB quotas (Load Balancers per Region: 20, Listeners per Classic Load Balancer: 100, Registered Instances per Classic Load Balancer: 1,000) and no policies-per-load-balancer quota -- confirmed absent, not just unfound, so still deliberately left unenforced.
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - none — full op-by-op pass completed this round (parity-3: re-verified after the Go-refactoring-2 file split; all backend.go/handler.go logic re-read post-split and re-diffed against the SDK)
leaks: {status: clean, note: "Reset()/Snapshot()/Restore() all close+recreate tags.Tags registries correctly (no Prometheus label leak); DeleteLoadBalancer cascade-deletes policies via policiesByLB index with a cloned slice before delete to avoid corrupting the in-progress scan"}
---

## Notes

### 2026-08-29 (list-filter-params sweep: parameters declared and never honoured)

Measured all 6 collection-returning operations (`DescribeAccountLimits`,
`DescribeInstanceHealth`, `DescribeLoadBalancerPolicies`,
`DescribeLoadBalancerPolicyTypes`, `DescribeLoadBalancers`, `DescribeTags`; excluded
`DescribeLoadBalancerAttributes`, which returns a single struct, not a collection) and
every constraining parameter each declares in its own `api_op_<Op>.go` Input struct.
Genuinely clean this pass: every declared filter (`LoadBalancerNames`, `Instances`,
`PolicyNames`, `PolicyTypeNames`) and both pagination params (`Marker`/`PageSize` on
`DescribeAccountLimits`/`DescribeLoadBalancers`) were already read and correctly applied,
including truncation and cursor round-trip. No fixes made this pass; no code changed.

Protocol: query/xml (single POST, `Action=` form param, `Version=2012-06-01`). Root
namespace `http://elasticloadbalancing.amazonaws.com/doc/2012-06-01/`.

### Bug class found this pass: missing `<XxxResult>` wrapper elements (8 ops)

`DeleteLoadBalancer`, `SetLoadBalancerListenerSSLCertificate`,
`SetLoadBalancerPoliciesOfListener`, `SetLoadBalancerPoliciesForBackendServer`,
`CreateAppCookieStickinessPolicy`, `CreateLBCookieStickinessPolicy`,
`CreateLoadBalancerPolicy`, and `DeleteLoadBalancerPolicy` all have void (no-payload)
results in the real API, but the real deserializer still unconditionally calls
`NodeDecoder.GetElement("<Op>Result")` before returning success — see
`deserializers.go`'s per-op `awsAwsquery_deserializeOp*` functions, each of which does
`t, err = decoder.GetElement("<Op>Result")` even when there's nothing to decode inside
it. `GetElement` returns `"<name> node not found"` if the element is absent
(`smithy-go@.../encoding/xml/xml_decoder.go:82`). Before this fix, gopherstack's
response structs for these 8 ops omitted the `Result` field/tag entirely, so **every
real aws-sdk-go-v2 call to any of these 8 operations failed client-side with a
deserialization error**, even though the operation succeeded server-side and mutated
state correctly. This is the same bug class documented in rds/neptune/docdb parity
sweeps ("required `XxxResult` wrapper elements"). Fixed by adding an empty
`type <op>Result struct{}` with the correct `xml:"<Op>Result"` tag to each response
struct, matching the pattern already used correctly by
`createLoadBalancerListenersResponse` / `addTagsResponse` / etc.

**Trap for the next auditor**: don't assume a void-result op's response struct is
correct just because it "looks empty and harmless" — check it actually declares a
`Result` field with the right `xml:"<Op>Result"` tag. An empty response struct with
only `XMLName`/`Xmlns`/`ResponseMetadata` fields is *always* wrong for this SDK's
query/xml protocol, because `GetElement` is unconditional in the generated
deserializer regardless of whether the shape has members.

### Error-code parity, verified directly against the SDK's per-op error switch tables

`deserializers.go` has an `awsAwsquery_deserializeOpError<OpName>` function per
operation with a `switch { case strings.EqualFold("<Code>", errorCode): ... }` table
listing exactly which typed exceptions that op can produce. This is the ground truth
for which wire `<Code>` string is expected — cross-referencing every backend/handler
sentinel against these tables (not just `types/errors.go`'s existence list) surfaced:

- `TooManyLoadBalancers` (not `ValidationError`) for `CreateLoadBalancer`'s 20-LB limit
  (`AccessPointNotFoundException`'s sibling `TooManyAccessPointsException`).
- `TooManyTags` (not `ValidationError`) for `AddTags`' / `CreateLoadBalancer`'s 10-tag
  limit.
- `DuplicateTagKeys` — was not validated at all; a same-request duplicate tag key
  silently overwrote instead of erroring. Added to `AddTags` (and transitively to
  `CreateLoadBalancer`'s inline `Tags`, since it calls `Backend.AddTags`).
- `InvalidScheme` (not `ValidationError`) for `CreateLoadBalancer`'s Scheme validation.
- `UnsupportedProtocol` (not `ValidationError`) for a listener's `Protocol` value
  outside HTTP/HTTPS/TCP/SSL.
- `PolicyTypeNotFound` (not `PolicyNotFound`) for an unknown `PolicyTypeName` in both
  `CreateLoadBalancerPolicy` and `DescribeLoadBalancerPolicyTypes` —
  `PolicyTypeNotFoundException` and `PolicyNotFoundException` are two *distinct* typed
  exceptions in the SDK (policy type vs. policy instance); gopherstack was reusing the
  policy-instance one for both.
- `PublicKeyPolicyType` was missing from the `CreateLoadBalancerPolicy` policy-type
  allowlist even though it's a real, built-in Classic ELB policy type (used for
  back-end server authentication, listed by `DescribeLoadBalancerPolicyTypes`) — every
  real attempt to create one was wrongly rejected as "unknown policy type".

All six fixes are proven by `handler_sdk_roundtrip_test.go`, which drives the real
`aws-sdk-go-v2/service/elasticloadbalancing` client against an `httptest` server and
asserts `errors.As` into the exact typed exception struct (e.g.
`*types.TooManyAccessPointsException`), not just an HTTP status code — this is the only
way to prove the wire `<Code>` string round-trips correctly through the real
deserializer's error-type dispatch.

### Confirmed correct (not bugs, don't re-flag)

- `DescribeLoadBalancerPolicies` with no `LoadBalancerName` returning the 4 built-in
  `ELBSample-*`/`ELBSecurityPolicy-*` sample policies (not all policies across the
  account) is genuine documented AWS behavior, not a stub.
- `CreatedTime` is emitted as `time.RFC3339`; the real deserializer parses it with
  `smithytime.ParseDateTime`, which accepts RFC3339 with optional fractional seconds —
  compatible.
- `elb.http.desyncmitigationmode` as an `AdditionalAttributes` key on
  `LoadBalancerAttributes` is a real Classic-ELB attribute (AWS added desync
  mitigation mode to Classic ELB, not just ALB) — not a fabricated attribute.
- `<member>` list-wrapper convention (`LoadBalancerDescriptions`, `Instances`,
  `ListenerDescriptions`, `Tags`, etc.) matches
  `awsAwsquery_deserializeDocument*` list decoders throughout; no over-nesting found
  outside the 8 missing-Result-wrapper ops above.
- Snapshot/Restore Handler-level delegation (`persistence.go`) is intact: `Handler`
  type-asserts its `StorageBackend` to a `snapshotter`/`restorer` interface and
  delegates to `InMemoryBackend.Snapshot`/`Restore`, exactly mirroring the
  `services/securityhub` pattern referenced in its doc comment.

## parity-3 pass (2026-07-24)

Since the 2026-07-12 audit, `services/elb/` went through a full file-split refactor
(`backend.go`/`handler.go` → one file per resource family: `load_balancers.go`,
`listeners.go`, `instances.go`, `policies.go`, `attributes.go`, etc., plus matching
`handler_*.go` files) with no intended behavior change. This pass re-read every
production file post-split and re-diffed each op's wire shape and error codes against
`deserializers.go`'s per-op typed-error switch tables (the same ground-truth method the
2026-07-12 pass established), rather than trusting the refactor was behavior-preserving.
It found four real, in-scope bugs the split either introduced or left unaudited:

1. **`DescribeLoadBalancers`' `LoadBalancerDescription.Policies` field was entirely
   missing from `xmlLoadBalancerDescription`.** Every real client's `DescribeLoadBalancers`
   call saw an always-empty `Policies` (`AppCookieStickinessPolicies`/
   `LBCookieStickinessPolicies`/`OtherPolicies` all nil), regardless of how many
   policies actually existed on the LB. This doesn't fail deserialization (the
   query/xml decoder tolerates missing optional elements, unlike the `<XxxResult>`
   wrapper bug class from the prior pass), so it would never surface as a client error
   — only as silently-wrong data. Fixed by adding `toXMLPolicies` (routes each policy by
   `PolicyTypeName` into the three sub-lists, mirroring `types.Policies`) and threading
   each LB's policies (fetched via the existing `DescribeLoadBalancerPolicies` backend
   method) through to `toXMLLoadBalancer`. Proven by
   `Test_SDKRoundTrip_LoadBalancerPolicies_WireShape`, which creates one policy of each
   kind and asserts the real SDK client's typed `Policies` struct via
   `DescribeLoadBalancers`.

2. **Two ops used the generic `ValidationError` code where the real op's typed-error
   switch requires `InvalidConfigurationRequest`**: `DeleteLoadBalancerPolicy`'s
   policy-still-in-use rejection, and `CreateLoadBalancerListeners`' classic-listeners
   (100) limit-exceeded rejection. Per `deserializers.go`,
   `awsAwsquery_deserializeOpErrorDeleteLoadBalancerPolicy` and
   `awsAwsquery_deserializeOpErrorCreateLoadBalancerListeners` only recognize
   `InvalidConfigurationRequest`/`LoadBalancerNotFound` (plus `CertificateNotFound`/
   `DuplicateListener`/`UnsupportedProtocol` for the latter) — `ValidationError` isn't
   in either switch, so on a real client `errors.As` against the typed exception would
   silently fail to match even though the HTTP status and generic error string looked
   right. Both now use the existing `ErrInvalidConfiguration` sentinel. The dead
   `ErrValidation` sentinel (a byte-for-byte duplicate of `ErrInvalidParameter`'s
   `"ValidationError"` code, used only by these two call sites) was deleted along with
   them. Proven by `Test_SDKRoundTrip_DeleteLoadBalancerPolicyInUse_IsTyped` (typed
   `InvalidConfigurationRequestException` via `errors.As`) and
   `TestAccountLimitMaxListeners` (asserts the `InvalidConfigurationRequest` code
   string in the response body).

3. **`RegisterInstancesWithLoadBalancer` hard-rejected registration past 1000
   instances with an invented error.** `DescribeAccountLimits` correctly advertises
   `classic-registered-instances: 1000` as an account limit (that part is real AWS
   behavior), but the real `RegisterInstancesWithLoadBalancer` op has no typed
   exception for exceeding it — `awsAwsquery_deserializeOpErrorRegisterInstancesWithLoadBalancer`
   only recognizes `InvalidInstance`/`LoadBalancerNotFound`. A real AWS account can
   register past the soft limit (it's advisory, enforced by different means, if at
   all, not by this API rejecting the call); gopherstack's hard 1000-instance cap
   would incorrectly fail requests a real client would have had succeed. Deleted per
   the "delete invented errors not in the real SDK" rule — no replacement behavior was
   substituted since none is documented.

4. **Inline `SSLCertificateId` on `CreateLoadBalancer`/`CreateLoadBalancerListeners`
   skipped the ARN-format check** (`validateCertificateID`, the same regex-based
   `arn:aws:(acm|iam):` check `SetLoadBalancerListenerSSLCertificate` already ran).
   Both code paths share `parseOneListener`, but only the required-non-empty check ran
   there — the format check was applied only when the cert was set *after* creation,
   letting a malformed cert ARN through at LB/listener creation time while rejecting
   the identical string via `SetLoadBalancerListenerSSLCertificate`. Now both paths
   validate identically. This is a same-service, in-scope consistency fix, distinct
   from the pre-existing cross-service `CertificateNotFound` gap noted above (which
   remains a gap: neither path can verify the cert ARN actually *exists* in ACM/IAM,
   only that it's shaped like one).

All four are covered by new/extended tests: `Test_SDKRoundTrip_LoadBalancerPolicies_WireShape`,
`Test_SDKRoundTrip_DeleteLoadBalancerPolicyInUse_IsTyped`, the `malformed_cert_arn_rejected`
case in `TestDuplicateListenerCreateListeners`, and
`TestCreateLoadBalancerRejectsMalformedInlineCertARN`.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404,
masking Handler()'s already-typed InternalFailure**: same shape as autoscaling's entry
(see that entry or gopherstack-3a8t for the full survey/rationale). `RouteMatcher` now
falls back to `service.MatchesUserAgentMarker(r.Header, "api/elasticloadbalancing")`
(verified against the pinned `elasticloadbalancing@v1.36.4/api_client.go:638`
`AddSDKAgentKeyValue` call) only on the `ReadBody` failure branch. Migrated
`ExtractOperation`/`ExtractResource`/`Handler()` off `r.ParseForm()` onto
`httputils.ReadBody`+`url.ParseQuery`, per the docdb/neptune precedent (gopherstack-bahs).
Proof: `TestHandler_OversizedBodySurfacesInternalFailure` in `handler_oversized_body_test.go`
drives a real ELB (Classic) SDK client through `service.NewRegistry`/`service.NewServiceRouter`,
confirmed failing pre-fix with `UnknownError`; passes now with `InternalFailure`.
`TestHandler_NormalSizedBodyStillRoutes` is the regression guard. Gates: `go build`,
`go vet`, `gofmt -l` (clean), `go test -race ./services/elb/...` (pass),
`golangci-lint run ./services/elb/...` (0 issues).
## elb (this session, 2026-08-20)

Wrapper-key / nested-shape wire-parity sweep, targeting AWS bug shapes (a)-(e) from the
campaign brief: member generalized/missing across sibling types, wrong nesting level
under the right key, wrong type, case near-misses (N/A for this query/XML service, see
below), right key/wrong value, and request-only fields leaking into responses.

**Result: zero new bugs.** All 29 ops, their envelope `<Xxx*Result>` element names,
every `<member>` list wrapper, and every nested attribute-bag shape were re-verified
directly against `aws-sdk-go-v2/service/elasticloadbalancing@v1.36.4`'s
`deserializers.go` (both the per-op `HandleDeserialize`/`GetElement("<Op>Result")`
functions and the per-shape `awsAwsquery_deserializeDocument*` functions) rather than
trusting the prior pass's PARITY.md notes. This confirms, independently, everything the
2026-07-24 (`parity-3`) pass already recorded — the extremely thorough prior sweeps
(missing `Result` wrappers, missing `Policies` field, error-code fixes) left nothing
in the wrapper-key/nesting space to find.

- **Ops swept**: all 29 — `AddTags`, `ApplySecurityGroupsToLoadBalancer`,
  `AttachLoadBalancerToSubnets`, `ConfigureHealthCheck`,
  `CreateAppCookieStickinessPolicy`, `CreateLBCookieStickinessPolicy`,
  `CreateLoadBalancer`, `CreateLoadBalancerListeners`, `CreateLoadBalancerPolicy`,
  `DeleteLoadBalancer`, `DeleteLoadBalancerListeners`, `DeleteLoadBalancerPolicy`,
  `DeregisterInstancesFromLoadBalancer`, `DescribeAccountLimits`,
  `DescribeInstanceHealth`, `DescribeLoadBalancerAttributes`,
  `DescribeLoadBalancerPolicies`, `DescribeLoadBalancerPolicyTypes`,
  `DescribeLoadBalancers`, `DescribeTags`, `DetachLoadBalancerFromSubnets`,
  `DisableAvailabilityZonesForLoadBalancer`, `EnableAvailabilityZonesForLoadBalancer`,
  `ModifyLoadBalancerAttributes`, `RegisterInstancesWithLoadBalancer`, `RemoveTags`,
  `SetLoadBalancerListenerSSLCertificate`, `SetLoadBalancerPoliciesForBackendServer`,
  `SetLoadBalancerPoliciesOfListener` — matches `GetSupportedOperations()` exactly
  (29/29) and matches `ls api_op_*.go` in the pinned SDK module exactly (29/29).
- **Protocol**: confirmed `awsAwsquery_*` (AWS Query/XML), from both the generated
  function-name prefix in `deserializers.go` and `RouteMatcher`'s own
  `application/x-www-form-urlencoded` + `Version=2012-06-01` check
  (`services/elb/handler.go`). `services/_PROTOCOLS.md` was not consulted per the
  brief's instruction not to trust it; the live deserializer prefix is authoritative
  and was read directly.
- **Required-member grep**: ran `grep -n "This member is required"` over the pinned
  SDK's `types/types.go`. Hits: `AccessLog.Enabled`, `ConnectionDraining.Enabled`,
  `ConnectionSettings.IdleTimeout`, `CrossZoneLoadBalancing.Enabled`,
  `HealthCheck.{HealthyThreshold,Interval,Target,Timeout,UnhealthyThreshold}`,
  `Listener.{InstancePort,LoadBalancerPort,Protocol}`, `Tag.Key`. Every one of these
  has a corresponding `xml:"..."` field with **no `omitempty`** in gopherstack's
  wire structs (`xmlBoolAttribute.Enabled`, `xmlConnectionDraining.Enabled`,
  `xmlConnectionSettings.IdleTimeout`, `xmlHealthCheck.*`, `xmlListener.*` sans
  `SSLCertificateID`, `xmlTag.Key`) — confirmed always present on the wire, including
  the zero-value case (e.g. an LB with no `ConfigureHealthCheck` call yet still emits
  a full `<HealthCheck>` element with zero-valued fields rather than omitting it).
- **`<member>` wrapping**: verified per list, not once for the service, by reading
  each list's own `awsAwsquery_deserializeDocument<Name>` function:
  `AvailabilityZones`, `BackendServerDescriptions`, `Instances`,
  `ListenerDescriptions`, `SecurityGroups`, `Subnets`, `AdditionalAttributes`,
  `TagDescriptions`/`Tags`, `AppCookieStickinessPolicies`/
  `LBCookieStickinessPolicies`/`OtherPolicies`, `PolicyAttributeDescriptions`,
  `PolicyAttributeTypeDescriptions`, `PolicyDescriptions`,
  `PolicyTypeDescriptions`, `InstanceStates`, `Limits` — all 15+ distinct lists use
  `case strings.EqualFold("member", t.Name.Local)`. All match gopherstack's
  `xmlXxxList{Members []Xxx \`xml:"member"\`}` convention throughout
  `handler_*.go`. Correct.
- **`PolicyAttributeDescription` vs `PolicyAttributeTypeDescription`**: confirmed
  distinct structs in the pinned SDK (`types/types.go:452-497`) — the former is
  `{AttributeName, AttributeValue}` (used by `DescribeLoadBalancerPolicies`'
  `PolicyDescription.PolicyAttributeDescriptions`), the latter is `{AttributeName,
  AttributeType, Cardinality, DefaultValue, Description}` (used by
  `DescribeLoadBalancerPolicyTypes`' `PolicyTypeDescription.PolicyAttributeTypeDescriptions`).
  `services/elb/handler_policies.go` models them as two separate wire structs
  (`xmlPolicyAttributeDescription` vs `xmlPolicyAttributeTypeDescription`) feeding two
  separate list types, each populated from its own backend model
  (`PolicyAttribute`/`models.go` vs `PolicyAttributeTypeDescription`/`models.go`) with
  no cross-contamination. No pattern-(a) generalization found.
- **Envelope names + ResponseMetadata**: verified the live deserializer path per op —
  `HandleDeserialize` calls `smithyxml.FetchRootElement` (does **not** validate the
  root element's name against `<Op>Response>`, confirming the brief's "check which
  deserializer actually runs before reporting an envelope-name finding" — a root-tag
  mismatch would be a false positive here, symmetric to the cloudfront finding) and
  then unconditionally `decoder.GetElement("<Op>Result")` — this **is** load-bearing:
  every response struct in `services/elb/handler_*.go` was checked to declare its
  `Result` field with the exact matching `xml:"<Op>Result"` tag (this is what the
  2026-07-24 pass's 8-op "missing Result wrapper" bug class targeted; re-verified
  clean this pass, none regressed). Separately: `aws-sdk-go-v2`'s
  `RequestIDRetriever` middleware (pinned core `v1.43.4`,
  `aws/middleware/request_id_retriever.go`) reads the request ID from the
  **`X-Amzn-Requestid`/`X-Amz-RequestId` HTTP headers**, not from the XML body's
  `<ResponseMetadata><RequestId>` — so for this SDK version, the body-level
  `ResponseMetadata` element is not actually consumed client-side for successful
  responses (only relevant for the AWS-documented wire shape / other SDKs, e.g.
  boto3). gopherstack still emits `<ResponseMetadata><RequestId>` on every response,
  which is correct AWS wire behavior and harmless either way — noted here since the
  brief specifically calls out verifying ResponseMetadata's presence.
- **Case-sensitive near-misses**: none reported, per the brief — `strings.EqualFold`
  element matching throughout `deserializers.go` (confirmed on every
  `awsAwsquery_deserializeDocument*` case statement read this pass) makes XML tag
  casing non-load-bearing for this protocol.
- **Existing tests**: no wrong-key/wrong-nesting test assertions found. All response
  assertions in `services/elb/*_test.go` go through typed Go structs (compile-time
  field-name checking) or the real SDK client (`sdk_roundtrip_test.go`), not raw XML
  string matching — grepped for `Contains(t, ..., "<...")`-style raw-XML assertions
  and found none. Nothing to correct.
- **Families CLEAN**: `LoadBalancerDescription` (all 20 fields, incl.
  `SourceSecurityGroup`, `CanonicalHostedZoneName{,ID}`, `Scheme`, `VPCId`),
  `DescribeLoadBalancerAttributes`/`ModifyLoadBalancerAttributes`'s nested attribute
  bag (`AccessLog`/`ConnectionDraining`/`ConnectionSettings`/
  `CrossZoneLoadBalancing`/`AdditionalAttributes` all sub-elements, not flattened —
  the shape-(b) risk the brief called out did not materialize),
  `DescribeInstanceHealth`, `DescribePolicies`/`DescribePolicyTypes`,
  `DescribeTags`/`AddTags`/`RemoveTags`, `DescribeAccountLimits`, and the
  Register/Deregister/Enable/Disable/Attach/Detach/Apply filtered-list-return family
  (each confirmed returning its own correctly-named/typed list, not a sibling's).
- **Gaps disclosed, not fixed** (pre-existing, out of scope for a wire-shape sweep,
  not newly found this pass): the SDK models several typed exceptions gopherstack has
  no sentinel for at all (`InvalidSubnetException`/`"InvalidSubnet"` — distinct from
  the modeled `SubnetNotFoundException`/`"SubnetNotFound"` — plus
  `DependencyThrottleException`, `LoadBalancerAttributeNotFoundException`,
  `OperationNotPermittedException`, `TooManyPoliciesException`); these are
  error-code-completeness gaps (Layer 3), not wrapper-key/nesting bugs, so left
  untouched per this sweep's scope.
- **Provenance verdict**: `last_audit_commit: c9c03908` vs `last_audit_date:
  2026-07-24` — `git show -s --format=%ad c9c03908` returns `2026-07-24`, an exact
  match (self-consistent, no drift). Stamp refreshed this pass to
  `9249d4561` / `2026-08-20` (current HEAD / today).
- **SDK-version check**: `go.mod` pins
  `github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing v1.36.4`, matching the
  `sdk_module` header already recorded — no version drift since the last pass.
- **Brief vs. pinned SDK disagreements**: none. Every wire-shape claim in the sweep
  brief (field names, nesting, `<member>` wrapping, the
  `PolicyAttributeDescription`/`PolicyAttributeTypeDescription` distinction) was
  verified true against `elasticloadbalancing@v1.36.4`.
- **Gates**: `go build ./services/elb/...` clean; `go vet ./services/elb/...` clean;
  `go fix -diff ./services/elb/...` empty; `gofmt -l services/elb/` empty;
  `go test -race ./services/elb/...` → `ok` (1.2s); `golangci-lint run
  ./services/elb/...` → `0 issues`; no `nolint:cyclop|gocyclo|gocognit|funlen` in
  `services/elb/`; `git status --short` shows no file under `services/elb/` touched
  this session (all changes limited to this `PARITY.md`).

### 2026-08-31 (response-element-naming re-verification, gopherstack-uox6 trigger)

Triggered by the rds `DBParameterGroups` bug (`e2a4d084a`, see gopherstack-uox6): a
response list whose per-item XML wrapper was named after the *status type* instead of
the name the pinned deserializer's list decoder actually matches, so the list decoded
empty for every real client despite looking correct on skim. Checked whether this
service's own wrapper-key/nested-shape sweep (recorded above, 2026-08-20, `53664f525`)
already covers this axis or only checked top-level `<Xxx*Result>` keys.

**It already covers it.** That sweep explicitly re-verified "every `<member>` list
wrapper[,] and every nested attribute-bag shape... directly against
`aws-sdk-go-v2/service/elasticloadbalancing@v1.36.4`'s `deserializers.go` (both the
per-op `HandleDeserialize`/`GetElement("<Op>Result")` functions and the per-shape
`awsAwsquery_deserializeDocument*` functions)" and found zero bugs. This pass
independently re-spot-checked the rds bug's exact shape -- a list nested inside a
larger response struct, checking the wrapping element name its own list decoder
matches on -- for the `Policies` family (`AppCookieStickinessPolicies`,
`LBCookieStickinessPolicies`, `OtherPolicies`/`PolicyNames`,
deserializers.go:4666/6106/7550): all three list decoders match
`strings.EqualFold("member", t.Name.Local)`, and `handler_load_balancers.go`'s
`xmlAppCookieStickinessPolicyList`/`xmlLBCookieStickinessPolicyList`/
`xmlStringValueList` all emit `xml:"member"` per item -- correct. No status-shaped
list (a list of `*Status` structs wrapped under a non-`member` name, the rds bug's
specific shape) exists anywhere in this service's deserializers -- confirmed by
`grep -n "func awsAwsquery_deserializeDocument.*StatusList\b"` against
`deserializers.go`, zero matches. **Zero new bugs found; nothing changed in this
service.** `go build`, `go vet` (repo-wide, clean), `go test -race
./services/elb/...` all pass on the unmodified tree. No AWS documentation was
fetched this pass.

### 2026-09-06 (gopherstack-5c3m, gopherstack-ogvw -- title-only issues, re-derived)

Both issues were filed with a title and no description; scope and fix were re-derived
from the pinned SDK before any code was written.

**gopherstack-5c3m** (`RegisterInstancesWithLoadBalancer` instance-existence check):
confirmed no existence check existed against an EC2 instance id -- only format
(`i-[a-f0-9]{8,17}`) was checked. `elb.EC2Resolver` already existed
(`services/elb/crossservice.go`) with `SecurityGroupExists`/`SubnetExists`, wired by
cli.go's `wireELBCrossService`/`elbEC2ResolverAdapter` for `ApplySecurityGroupsToLoadBalancer`/
`AttachLoadBalancerToSubnets` -- the title's claim of "no hook wired" was right for
instances specifically but the interface/setter/wiring convention was already
established, so this extends it rather than inventing a new mechanism. Error extraction
(`awk "/deserializeOpErrorRegisterInstancesWithLoadBalancer\(/,/^}/" deserializers.go`):
`RegisterInstancesWithLoadBalancerResult`, `UnknownError`, `InvalidInstance`,
`LoadBalancerNotFound` -- confirms `InvalidInstance` (already an existing sentinel,
`ErrInvalidInstance`, used elsewhere for `DescribeInstanceHealth`'s "not registered"
case) is a real typed error for this op. Added `InstanceExists(id string) bool` to
`elb.EC2Resolver`; `RegisterInstancesWithLoadBalancer` now calls it per instance (nil
resolver skips the check entirely -- proven by `TestRegisterInstancesWithLoadBalancer_EC2Resolver/no_resolver_wired_accepts_any_id`).
**cli.go wiring is OUTSTANDING**: `cli.go` was out of scope for this pass, and
extending `EC2Resolver` with a new method means `cli.go`'s `elbEC2ResolverAdapter`
(which implements only `SecurityGroupExists`/`SubnetExists`) no longer satisfies the
interface -- `go build ./services/elb/...` is unaffected (elb doesn't import cli.go),
but a repo-wide `go build ./...` will fail until `elbEC2ResolverAdapter` gains an
`InstanceExists(id string) bool` method (e.g. `len(a.backend.DescribeInstances([]string{id}, ""))
> 0`, mirroring its existing `SecurityGroupExists`/`SubnetExists`) and `wireELBCrossService`
is otherwise unchanged (`SetEC2Resolver` already carries it through).

**gopherstack-ogvw** (`CreateLoadBalancerPolicy` attribute-schema validation): the
policy-type attribute schema IS fully modeled -- `builtinPolicyTypes()` (`policies.go`)
already carries `PolicyAttributeTypeDescription{AttributeName, AttributeType,
Cardinality, DefaultValue, Description}` per type, and `handler_policies.go` already
validates `PolicyTypeName` itself against `knownPolicyTypes` before calling the
backend. What was missing: nothing checked that a submitted `PolicyAttributes.member.N.AttributeName`
was actually declared by that policy type -- any name/value pair was accepted and
stored verbatim. Error extraction
(`awk "/deserializeOpErrorCreateLoadBalancerPolicy\(/,/^}/" deserializers.go`):
`CreateLoadBalancerPolicyResult`, `UnknownError`, `DuplicatePolicyName`,
`InvalidConfigurationRequest`, `LoadBalancerNotFound`, `PolicyTypeNotFound`,
`TooManyPolicies` -- `InvalidConfigurationRequestException`'s doc comment
(`types/errors.go:197`, verbatim: "The requested configuration change is not valid.")
is the natural fit and is the sentinel this codebase already uses for
`CreateLoadBalancerPolicy`-adjacent misconfiguration (`ErrInvalidConfiguration`). Added
`validatePolicyAttributes` (`policies.go`), called from `CreateLoadBalancerPolicy`
before the load balancer is touched: rejects any attribute name not in the given
type's schema with `ErrInvalidConfiguration`. `PolicyAttributeTypeDescription.Cardinality`'s
doc (`types/types.go:479-490`, verbatim: "ONE(1) : Single value required",
"ZERO_OR_ONE(0..1) : Up to one value is allowed", "ZERO_OR_MORE(0..*) : Optional.
Multiple values are allowed", "ONE_OR_MORE(1..*0) : Required. Multiple values are
allowed") pins a precise meaning, but literal enforcement was deliberately NOT
implemented: `ProxyProtocolPolicyType`'s sole attribute (`ProxyProtocol`) has
`Cardinality: "ONE"` plus a `DefaultValue`, and this package's own pre-existing tests
(e.g. `TestCreateLoadBalancerPolicy/duplicate_policy_returns_conflict`'s setup,
`sdk_roundtrip_test.go`'s `rt-bad-pol`/describe-policy-types tests) create that policy
type while supplying zero `PolicyAttributes` and expect success -- a literal "single
value required" reading would break those and contradicts this backend's established
default-substitution behavior for `ONE`-cardinality attributes with a `DefaultValue`.
Name-membership validation is the genuinely well-founded fix; cardinality enforcement
is left as a documented non-fix (see `validatePolicyAttributes`' doc comment).

Regression tests: `TestRegisterInstancesWithLoadBalancer_EC2Resolver` (3 subtests,
`crossservice_test.go`) and `TestCreateLoadBalancerPolicy/unknown_attribute_name_rejected`
(`policies_test.go`). Both proven to fail against the unmodified backend (reverted the
production diff, kept the tests, ran them: `unknown_instance_rejected` and
`unknown_attribute_name_rejected` each failed with `expected: 400, actual: 200`;
restored the production diff afterward). Gates: `go test -race ./services/elb/...` ok;
`golangci-lint run services/elb/...` 0 issues; `go test ./pkgs/persistence/ -run
TestSnapshotVersionGuard` pass (read-only, unrelated to this change -- no field was
added to any persisted struct).

### 2026-09-07 (gopherstack-5gfl -- errtargetaudit class A findings, first triage of this block)

This service had no prior error-envelope/errtargetaudit entry; genuinely untriaged.
`GOTOOLCHAIN=go1.26.6 go run ./cmd/errtargetaudit`: `operations with SDK ground truth:
29, resolved: 29, with an emission found: 24` (full coverage, no warning) -- `class A
findings (3)`, grouped as `2 finding(s): code=LoadBalancerNotFound ... ops=[CreateLoadBalancer
DeleteLoadBalancer]` and `1 finding(s): code=PolicyNotFound ... ops=[DeleteLoadBalancerPolicy]`.

Verified every finding against `deserializers.go`'s per-op typed-error switch
(`awk "/deserializeOpError<Op>\(/,/^}/" deserializers.go | grep -oE '"[A-Za-z0-9]+"'`,
pinned `elasticloadbalancing@v1.36.4`) rather than trusting the tool's classification.
Protocol confirmed query/xml as in prior passes; the extraction pattern needed no
adjustment -- `services/elb/handler.go`'s `writeError`/`elbErrorResponse` emits a real
`<ErrorResponse><Error><Code>...` envelope, which is exactly what
`awsxml.GetErrorResponseComponents` (called from every `deserializeOpError<Op>`) reads
into `errorComponents.Code` before the switch runs, so the raw extraction is
ground truth, not a false negative risk.

- **`CreateLoadBalancer` / `LoadBalancerNotFound` (`services/elb/tags.go:57`) --
  false positive, class 4 (guard cannot fire: resource created moments earlier in the
  same request).** `CreateLoadBalancer`'s own declared set (`CertificateNotFound`,
  `DuplicateLoadBalancerName`, `DuplicateTagKeys`, `InvalidConfigurationRequest`,
  `InvalidScheme`, `InvalidSecurityGroup`, `InvalidSubnet`, `OperationNotPermitted`,
  `SubnetNotFound`, `TooManyLoadBalancers`, `TooManyTags`, `UnsupportedProtocol`) does
  not include `LoadBalancerNotFound` -- the sentinel reference the tool found is
  `AddTags`'s own (legitimate) not-found guard, reached one hop away via
  `handler_load_balancers.go:96`'s `h.Backend.AddTags(ctx, []string{name}, initialTags)`,
  which `handleCreateLoadBalancer` calls immediately after `h.Backend.CreateLoadBalancer`
  returns successfully to apply AWS's documented inline-`Tags` convenience. The backend
  uses one coarse `*lockmetrics.RWMutex` per call (`store.go`), so there is a lock-free
  window between the two calls, but the load balancer just-created under that same name
  cannot itself have vanished except via a concurrent `DeleteLoadBalancer` racing that
  exact window -- not a code path any real single client request can hit, and not a
  scenario real AWS's own atomic create+tag semantics can produce either (which is
  exactly why the real op's error switch has no such code at all). No fix applied.
- **`DeleteLoadBalancer` / `LoadBalancerNotFound` (`services/elb/load_balancers.go:341`)
  -- real bug, FIXED.** Declared set is `UnknownError` only -- no not-found code
  whatsoever. The pinned SDK's `api_op_DeleteLoadBalancer.go` doc comment settles the
  correct remedy directly: "If the load balancer does not exist or has already been
  deleted, the call to DeleteLoadBalancer still succeeds." `DeleteLoadBalancer` now
  returns `nil` for an unknown name instead of `ErrLoadBalancerNotFound`, matching this
  documented idempotent-delete behavior. Regression tests corrected (both previously
  pinned the wrong 400 with no disclaiming note, exactly the trap flagged for this
  campaign): `TestDeleteLoadBalancer`'s `delete_not_found` subtest (renamed
  `delete_not_found_is_idempotent_success`) and `TestDeleteLoadBalancerNotFoundReturns404`
  (renamed `TestDeleteLoadBalancerNotFoundIsIdempotent`), both now asserting
  `http.StatusOK`. Neutered (reverted the one-line production fix, kept the tests,
  confirmed both fail with `expected: 200, actual: 400`; restored the fix afterward).
- **`DeleteLoadBalancerPolicy` / `PolicyNotFound` (`services/elb/policies.go:308-318`) --
  real mismatch, no safe remedy, landmine comment added.** Declared set is
  `InvalidConfigurationRequest`/`LoadBalancerNotFound` only (confirmed against both
  `deserializers.go` and the live AWS API reference page for this op, which lists only
  those two under Errors) -- `PolicyNotFound` is a real exception in this SDK
  (`types.PolicyNotFoundException`), just not one this specific op's model declares, so
  a real client would get an untyped `smithy.GenericAPIError` instead. Unlike
  `DeleteLoadBalancer`, no AWS documentation states this op is idempotent for a missing
  policy, so -- per this campaign's rule that a declared-set mismatch proves the code
  wrong but not that any particular remedy is right -- no behavior change was made.
  Added a landmine comment at the call site instead of guessing. The pre-existing
  `TestPolicyNotFoundReturns400` pinned this without any such note; added one rather than
  leaving it to survive a future pass as apparently-intentional.

**elbv2 shared-helper check**: `services/elbv2` has its own independent
`AddTags`/`DeleteLoadBalancer`/`DeleteLoadBalancerPolicy` (different signatures,
different error model -- ALB/NLB, not Classic ELB). `grep` for cross-package references
in both directions found only two comments in `services/elb/persistence.go` and
`store_setup.go` citing `elbv2` as a prior *pattern* precedent -- no shared code, no
shared helpers. Nothing in `services/elbv2/` was touched.

Gates: `go build ./services/elb/...` clean; `go test -race -count=1 ./services/elb/...`
ok (1.6s); `golangci-lint run services/elb/...` 0 issues; no field added to any
persisted struct, so the `pkgs/persistence` guard was not run. Re-ran the tool after the
fix: `resolved: 29, with an emission found: 23`, `class A findings (2)` -- the two
dismissed findings above (`CreateLoadBalancer`/`LoadBalancerNotFound`,
`DeleteLoadBalancerPolicy`/`PolicyNotFound`); `DeleteLoadBalancer`'s finding is gone.

## 2026-09-07 gopherstack-39ip: verified the negative claim directly, no remedy found

Re-derived `DeleteLoadBalancerPolicy`'s declared set from
`elasticloadbalancing@v1.36.4/deserializers.go`'s
`awsAwsquery_deserializeOpErrorDeleteLoadBalancerPolicy`: exactly `InvalidConfigurationRequest`
(`InvalidConfigurationRequestException`, "The requested configuration change is not
valid.") and `LoadBalancerNotFound` (`AccessPointNotFoundException`, "The specified load
balancer does not exist."). Cross-checked against the live `API_DeleteLoadBalancerPolicy.html`
page: same two codes, same doc text, nothing else.

The prior pass's central claim -- no equivalent to `DeleteLoadBalancer`'s idempotent-success
sentence exists for this op -- checked personally this pass rather than inherited: fetched
both the pinned SDK's doc comment (`api_op_DeleteLoadBalancerPolicy.go`, just "Deletes the
specified policy from the specified load balancer. This policy must not be enabled for any
listeners.") and the live API reference page in full. Neither carries any sentence about
behavior for a missing load balancer or policy. Claim confirmed negative.

Also confirmed `PolicyNotFoundException` ("One or more of the specified policies do not
exist.", `types/errors.go`) is a semantically perfect match for this guard's condition but,
per the extraction above, is not in this op's declared set -- same right-code-wrong-op shape
as memorydb's `SnapshotNotFoundFault` (gopherstack-2i0c). Neither declared code's own doc text
fits a missing policy: `LoadBalancerNotFound` names the wrong resource, and
`InvalidConfigurationRequest`'s doc is a generic stretch. No fix applied; sharpened the
landmine comment (policies.go:308-318) with this direct verification.

`TestPolicyNotFoundReturns400` left unchanged -- still pins the current (unendorsed)
`PolicyNotFound` behavior, correctly, since nothing changed.

Gates: `GOTOOLCHAIN=go1.26.6 go test -race ./services/elb/...` ok;
`GOTOOLCHAIN=go1.26.6 golangci-lint run ./services/elb/...` 0 issues. Re-ran
`cmd/errtargetaudit`: same finding, same line, confirming no emission change.
