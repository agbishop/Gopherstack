---
service: mediastore
sdk_module: aws-sdk-go-v2/service/mediastore@v1.32.4
last_audit_commit: 67b92e0b9
last_audit_date: 2026-08-20
overall: A            # all three prior gaps genuinely closed in code this pass, with tests
                      # 2026-08-29: errcodeaudit ERROR-path sweep. 2 confident findings, both
                      # verified NOT live bugs. handler.go:167's "BadRequestException" fires only
                      # when X-Amz-Target is missing/malformed -- unreachable by any real SDK
                      # client (which always sets it correctly), so this is a dispatch-level
                      # routing-fallback false positive, same class as the tool's already-suppressed
                      # "matches no operation" cases, just triggered by a header-prefix guard
                      # instead of an op-string switch default. writeBackendError's generic
                      # awserr.ErrNotFound fallback (-> fabricated "ResourceNotFoundException", not
                      # a real mediastore type) is dead code: both mediastore sentinels wrapping
                      # ErrNotFound/ErrAlreadyExists not already caught by an earlier specific case
                      # (ContainerNotFoundException/PolicyNotFoundException/
                      # CorsPolicyNotFoundException/ContainerInUseException) don't exist -- every
                      # currently-defined sentinel is caught before reaching it. Left unchanged, no
                      # replacement code invented (mediastore has no generic not-found type either).
ops:
  CreateContainer: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeContainer: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteContainer: {wire: ok, errors: ok, state: ok, persist: ok}
  ListContainers: {wire: ok, errors: ok, state: ok, persist: ok, note: "HMAC-signed opaque NextToken via pkgs/page; sorted by Name"}
  PutContainerPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetContainerPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteContainerPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "PolicyNotFoundException when unset -- confirmed against real AWS API reference (not idempotent-success)"}
  PutCorsPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetCorsPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteCorsPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutLifecyclePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetLifecyclePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteLifecyclePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutMetricPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetMetricPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMetricPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  StartAccessLogging: {wire: ok, errors: ok, state: ok, persist: ok}
  StopAccessLogging: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  Container: {status: ok, note: "CreateContainer/DescribeContainer/DeleteContainer/ListContainers verified end-to-end against the real aws-sdk-go-v2 client over an httptest server (not just unit tests) -- wire shapes, epoch CreationTime, ARN/Endpoint format, error deserialization all round-trip cleanly."}
  ContainerPolicy: {status: ok, note: "Put/Get/Delete round-trip the raw policy JSON string verbatim; Delete returns PolicyNotFoundException when unset, matching AWS."}
  CorsPolicy: {status: ok, note: "Put validates AllowedOrigins/AllowedHeaders non-empty per rule; Get/Delete round-trip full rule set including AllowedMethods/ExposeHeaders/MaxAgeSeconds."}
  LifecyclePolicy: {status: ok, note: "Put/Get/Delete round-trip the raw JSON string verbatim."}
  MetricPolicy: {status: ok, note: "Put validates ContainerLevelMetrics enum and >5-rule limit; Get/Delete round-trip full policy including MetricPolicyRules."}
  Tags: {status: ok, note: "Tag/Untag/ListTagsForResource keyed by ARN via containerNameFromARN; tags also settable at CreateContainer time."}
gaps:
  - "gopherstack-apg3 (2026-09-07, audited, NOT fixed -- structural): DeleteContainer does not
    require the container to be empty. Real AWS's doc comment (aws-sdk-go-v2/service/mediastore
    @v1.32.4 api_op_DeleteContainer.go:10-12, byte-identical in botocore's mediastore/2017-09-01/
    service-2.json.gz operations.DeleteContainer.documentation) states \"Before you make a
    DeleteContainer request, delete any objects in the container or in any folders in the
    container. You can delete only empty containers,\" but DeleteContainer's own modeled error
    set (deserializers.go:201-251's awsAwsjson11_deserializeOpErrorDeleteContainer switch;
    identical in botocore's operations.DeleteContainer.errors) is only ContainerInUseException,
    ContainerNotFoundException, InternalServerError -- no distinct \"container not empty\"
    exception exists to enforce against. ContainerInUseException's own doc (types/errors.go:10-11:
    \"The container that you specified in the request already exists or is being updated\")
    already maps 1:1 to gopherstack's existing create-time already-exists/being-updated case
    (handler.go:643-645), not emptiness. STRUCTURAL, not fixable without new cross-service
    architecture: services/mediastoredata (the object data plane) is a fully independent
    provider -- its Init (mediastoredata/provider.go:17-30) builds its own NewInMemoryBackend
    with no handle to mediastore, matching pkgs/service.AppContext (service.go:179-185), which
    carries only Config/JanitorCtx/Logger/PortAlloc/JanitorTimeout, no cross-service registry --
    the same isolation shape as appconfig/appconfigdata. It goes deeper than isolation, though:
    mediastoredata's object store (InMemoryBackend.states map[string]*store.Table[Object],
    mediastoredata/store.go:38-42) is keyed ONLY by region (getRegion, store.go:15-27), with no
    container dimension at all -- its handler resolves only the SigV4 region per request
    (requestContext, handler.go:118-126), never a container identifier. Even a hypothetical
    shared backend could not answer \"is this container empty\" today: mediastoredata would
    first need a container key added to its storage model, on top of the missing cross-service
    handle. Left unenforced rather than fabricated. Adjacent items checked per the same issue:
    error type for a missing container is correct (ContainerNotFoundException, handler.go:625-631,
    already pinned by a terraform-provider-aws-waiter comment); container-name format validation
    (validateContainerName, containers.go:19-26) runs only on CreateContainer, not Delete or any
    other container-name-consuming op -- long-standing and uniform across this file, and the real
    SDK's client-side validator (validators.go:33-51) only checks required-ness, not the
    pattern/max shape traits, so there is no clear SDK-side evidence this diverges from real AWS;
    left as-is, unverified rather than asserted as a bug. DeleteContainer skips any Status
    precondition (containers.go:107-143): deleting a CREATING container is a tested, deliberate
    choice (supersedes the pending creating->active transition -- containers.go:125-129's comment,
    pinned by containers_test.go's TestInMemoryBackend_ContainerActivationDelay); a second
    DeleteContainer call on an already-DELETING container is UNTESTED and its real-AWS behavior
    (idempotent success vs. ContainerInUseException) could not be verified from the SDK/botocore
    model -- flagged as unverified, not fixed."
deferred: []
leaks: {status: clean, note: "No goroutines, timers, or janitors in this service; InMemoryBackend is a single lockmetrics.RWMutex over per-region store.Table maps. The new container-lifecycle simulation (activationDelay/containerTransitions, see gap-closure note below) does NOT add a goroutine -- transitions are advanced lazily on read/mutate (advanceContainerStates), matching services/redshift's clusterTransitions pattern minus its optional background reconciler goroutine, which was deliberately not added here since lazy advancement alone is sufficient for every caller (SDK waiters always call Describe/List in a loop) and keeps this service goroutine-free."}
---

## Notes

- 2026-08-22, gopherstack-r80d batch 31 (required-output-member audit):
  mediastore (6 required output fields / 21 ops, 6 ops-with-required per a
  fresh `cmd/requiredoutputfields` run, cross-checked against an independent
  standalone `go/ast` walk of `mediastore@v1.32.4`'s `api_op_*.go` files --
  both agreed exactly at 6). Module resolved directly: directory `mediastore`
  == SDK module `aws-sdk-go-v2/service/mediastore@v1.32.4` per `go.mod`, with
  no `dirModuleOverride` entry -- verified this service's own source imports
  `aws-sdk-go-v2/service/mediastore`, not `mediastoredata` (a separate
  directory/module for the data-plane API, settled independently, not
  touched by this batch).

  6 flagged ops: `CreateContainer`/`ListContainers` wrap `types.Container`
  (types.go:12-48), which declares **zero** required members of its own
  (confirmed against the SDK type directly, not assumed from the shape) --
  gopherstack's `createContainerResponse`/`listContainersResponse`
  (models.go:177-190) tag their `Container`/`Containers` wrapper key with no
  `omitempty` either way, so the only real requirement (the wrapper key
  itself) is always satisfied. `GetContainerPolicy` (`Policy *string`),
  `GetLifecyclePolicy` (`LifecyclePolicy *string`), and `GetMetricPolicy`
  (`*types.MetricPolicy`, itself requiring `ContainerLevelMetrics`,
  types.go:106-122) all use non-`omitempty` string/struct wire fields
  (models.go:193-210) and are only ever returned once a real value exists --
  `GetContainerPolicy`/`GetLifecyclePolicy`/`GetMetricPolicy`
  (containers.go:216-232, lifecycle_policy.go:29-44, metric_policy.go:76-92)
  each return `ErrPolicyNotFound`/`ErrLifecyclePolicyNotFound`/
  `ErrMetricPolicyNotFound` when unset rather than an empty success value,
  and the corresponding `Put*` handlers reject an empty/invalid value before
  storage (`PutMetricPolicy`, metric_policy.go:44-57, rejects any
  `ContainerLevelMetrics` other than `ENABLED`/`DISABLED`; `PutContainerPolicy`/
  `PutLifecyclePolicy` reject non-valid-JSON, which excludes the empty
  string). `GetCorsPolicy` wraps `[]types.CorsRule` (types.go:52-95, 2
  required members per rule -- `AllowedHeaders`/`AllowedOrigins`, both
  `[]string`) -- `PutCorsPolicy` (cors_policy.go:16-19) rejects any rule with
  an empty `AllowedOrigins`/`AllowedHeaders` before storage, and `GetCorsPolicy`
  errors `ErrCorsPolicyNotFound` when no policy is set. Followed
  `MetricPolicyRule` (types.go:131-145, 2 required members --
  `ObjectGroup`/`ObjectGroupName`) one level below `MetricPolicy.MetricPolicyRules`:
  `validateMetricPolicyRule` (metric_policy.go:29-41) rejects an empty/invalid
  value for either before storage. Result: 0 bugs. No code changes.
- Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: MediaStore_20170901.<Op>` --
  confirmed byte-for-byte against `aws-sdk-go-v2/service/mediastore@v1.29.23`'s
  `serializers.go` header-setting calls for every operation.
- Verified end-to-end against the **real aws-sdk-go-v2 client** (not just hand-rolled unit
  tests) driving an `httptest.Server` wrapping `Handler.Handler()`: CreateContainer (incl.
  Tags at create time), DescribeContainer, paginated ListContainers (MaxResults=2 across 3
  pages with real NextToken round-trip), Put/Get for ContainerPolicy/CorsPolicy/
  LifecyclePolicy/MetricPolicy, StartAccessLogging/StopAccessLogging, TagResource/
  ListTagsForResource/UntagResource, and the full error-code surface (ValidationException on
  bad name, ContainerInUseException on duplicate create, ContainerNotFoundException via
  `errors.As` into `*types.ContainerNotFoundException`, PolicyNotFoundException on
  double-delete, ValidationException on >5 metric rules). All wire shapes decoded cleanly
  through the SDK's generated deserializers with no manual workarounds required.
- `Container.CreationTime` is epoch-seconds (`float64` via `.Unix()`), matching the SDK
  deserializer's `smithytime.ParseEpochSeconds`. Do not "fix" this to RFC3339 -- that would
  break the real client.
- Error code mapping (`Handler.writeBackendError`) is exhaustive and exact:
  `ContainerNotFoundException` (404), `PolicyNotFoundException` (404, covers
  container/lifecycle/metric policy not-found), `CorsPolicyNotFoundException` (404, CORS
  gets its own type -- do not conflate with `PolicyNotFoundException`),
  `ContainerInUseException` (409, container-already-exists), `ValidationException` (400).
  All confirmed present in `types/errors.go` of the real SDK.
- **Bug found and fixed this pass**: six validation sentinel errors
  (`ErrInvalidContainerName`, `ErrInvalidPolicy`, `ErrCorsRuleInvalid`,
  `ErrInvalidMetricPolicy`, `ErrTooManyMetricRules`, `ErrEmptyTagKey`) had `"ValidationException: "`
  hand-baked into the *message* text, duplicating the `__type` field that
  `writeBackendError`/`JSONErrorResponse` already sends separately. Over the wire this
  produced doubled text like `ValidationException: ValidationException: container name must
  be 1-255 characters...` (confirmed via a real-SDK probe -- `smithy.OperationError`'s
  `Error()` formats as `api error <Type>: <message>`, so the type name appeared twice).
  Fixed by stripping the redundant prefix from all six error strings in `backend.go`; no
  test asserted the old (buggy) exact text, so no call sites needed updating. This is a
  message-content-only fix -- `__type` and HTTP status were already correct, so no client
  behavior keyed off `__type` or status code changes.
- `copyContainer` does a **shallow** pointer-slice copy for `CorsPolicy` (comment explains
  why: rule pointers are only ever replaced wholesale by `PutCorsPolicy`, never mutated
  in-place, and `GetCorsPolicy` only ever hands callers a fresh `[]CorsRule` **value** copy,
  never the pointers) -- this looks like a copy-safety bug at first glance but is not; do
  not "fix" by deep-copying without re-checking that invariant still holds.
- Container names are unique **per region only** (see `store_setup.go`); `containers` is a
  `map[string]*store.Table[Container]` keyed by region, intentionally not registered on a
  `*store.Registry` since the region set is only known at runtime. `persistence.go` snapshots/
  restores each region's table directly.
- `paginationSecret` (HMAC key for `ListContainers` NextToken) is deliberately **not**
  persisted -- regenerated fresh per process start, matching AppConfig/AppConfigData. A
  NextToken issued before a restore will fail its HMAC check afterward; this is an accepted,
  pre-existing limitation shared with sibling services, not a new gap.
- **2026-07-24 re-audit (parity-3 sweep)**: independently re-field-diffed every op against a
  freshly-fetched `aws-sdk-go-v2/service/mediastore@v1.29.23` (`types/types.go`,
  `types/errors.go`, `types/enums.go`, `validators.go`, `deserializers.go`, every
  `api_op_*.go`) rather than trusting the prior audit's conclusions at face value. Confirmed
  byte-for-byte: `Container`/`CorsRule`/`MetricPolicy`/`MetricPolicyRule`/`Tag` field sets and
  types, all five modeled exceptions (`ContainerInUseException`, `ContainerNotFoundException`,
  `CorsPolicyNotFoundException`, `InternalServerError`, `LimitExceededException`,
  `PolicyNotFoundException` -- six, not five) with correct HTTP status/fault mapping,
  `ContainerStatus`/`ContainerLevelMetrics`/`MethodName` enum values, and the epoch-seconds
  `CreationTime` deserializer (`smithytime.ParseEpochSeconds`). Also confirmed
  `DescribeContainerInput.ContainerName` and `ListContainersInput`/`MaxResults` carry no
  client-side `validateOp*Input` middleware in the real SDK (no generated validator exists for
  those two ops) -- gopherstack's server-side `ContainerName` non-empty check on
  `DescribeContainer` is therefore a defensible server-side guard rather than a
  shape-mismatch; left as-is since no real client can produce a request that would surface a
  behavioral difference. No new gaps found; no regressions; no invented ops/fields. Ran the
  full self-gate suite (`go build ./services/mediastore/...`, `go test -race`, `go vet`,
  `gofmt -l`, `golangci-lint run`, banned-nolint grep, `git diff --stat go.mod go.sum`) -- all
  clean/empty. `go build ./...` (full tree) fails, but only in `services/networkmonitor`
  (`buildNestedProbes` undefined), a concurrent session's uncommitted in-progress edit
  unrelated to and untouched by this pass -- `services/mediastore` itself was not the cause
  and was not modified to route around it.

## Re-audit 2026-07-24 (gap closure -- parity-3 phase 2)

All three previously-dismissed gaps were genuinely fixed this pass (not just
re-argued as low-value), after independently confirming the real published
constraints in the MediaStore botocore API model
(`models/apis/mediastore/2017-09-01/api-2.json` in `aws-sdk-go@v1.55.5`'s
module cache, which carries the `max`/`min`/`pattern` shape traits that the
Go v2 SDK's generated `validators.go` does NOT enforce client-side -- v2's
generated validators only check required-ness, not length/pattern/count, for
this API):

1. **MetricPolicyRule.ObjectGroup/ObjectGroupName limits** -- the model shows
   `ObjectGroup {max: 900, min: 1, pattern: "/?(?:[A-Za-z0-9_=:\.\-\~\*]+/){0,10}(?:[A-Za-z0-9_=:\.\-\~\*]+)?/?"}`
   and `ObjectGroupName {max: 30, min: 1, pattern: "[a-zA-Z0-9_]+"}`. Both are
   now enforced server-side in `metric_policy.go`'s new
   `validateMetricPolicyRule` (regexes `objectGroupRE`/`objectGroupNameRE`),
   called from `PutMetricPolicy` for every rule, returning the new
   `ValidationException`-mapped `ErrObjectGroupInvalid`/
   `ErrObjectGroupNameInvalid` sentinels (wired into
   `Handler.writeBackendError`). The "unreachable via SDK client" framing in
   the prior gap was true only for the *Go v2* SDK's client-side check (which
   never existed for these fields) but wrong as a reason to skip server-side
   enforcement: any raw-HTTP caller, other-language SDK, or future SDK
   version can send an out-of-bounds value, and the real service rejects it.
   Tested in `handler_metric_policy_test.go`'s existing
   `TestHandler_PutMetricPolicy_Validation` table (new cases:
   `object_group_too_long`, `object_group_exactly_max_length_allowed`,
   `object_group_invalid_characters`, `object_group_name_too_long`,
   `object_group_name_exactly_max_length_allowed`,
   `object_group_name_invalid_characters`).
2. **CorsPolicy rule-count limit** -- the model shows `CorsPolicy {type: list,
   member: CorsRule, max: 100, min: 1}`. Now enforced in `cors_policy.go`'s
   `PutCorsPolicy` (`len(rules) > maxCorsPolicyRules` -> the new
   `ErrTooManyCorsRules`, also `ValidationException`-mapped). Tested in
   `handler_cors_policy_test.go`'s existing `TestHandler_PutCorsPolicy_Validation`
   table (new cases: `too_many_rules_101`, `exactly_100_rules_allowed`, via
   the new `makeCorsRules(n)` helper in that file).
3. **Container lifecycle instantaneity** -- re-examined against the rest of
   the codebase rather than re-asserting "never causes a hang, so left
   as-is." A `grep` for state-progression patterns
   (`time.AfterFunc|go func()` and `reconcil(e/ing)`) turns up genuine
   async-lifecycle simulation in `services/redshift` (`clusterActivationDelay`
   + `clusterTransitions` + a lazy-advance-on-read reconciler,
   `reconciler.go`) and `services/efs` (`fsActivationDelay` + a
   self-terminating per-create goroutine, `file_systems.go`). Critically,
   BOTH of those precedents default their delay knob to **zero** (fully
   synchronous, matching mediastore's old behavior) and only make the
   transient state observable when a caller explicitly opts in
   (`redshift.SetClusterActivationDelay`, `efs`'s equivalent) -- this is
   confirmed by reading `services/redshift/store.go:190-193` and
   `services/efs/file_systems.go:150-151`, both of which gate the initial
   `CREATING` status assignment behind `if b.*ActivationDelay > 0`. That is
   direct evidence "instantaneous by default" is this repo's deliberate house
   convention for lightweight, fast-provisioning resources -- not an
   oversight -- so the honest fix was to implement the SAME real,
   non-stub mechanism (not merely re-document the excuse): mediastore now has
   `InMemoryBackend.SetActivationDelay(d time.Duration)` (a real exported
   method, not a test seam living in export_test.go per the no-export_test.go
   rule) and `containerTransitions` (out-of-band scheduled transitions,
   modeled directly on `services/redshift`'s `clusterTransition`/
   `scheduleClusterTransitionLocked`/`advanceClusterStates`). With a positive
   delay configured, `CreateContainer` returns `Status: "CREATING"` and
   `DeleteContainer` sets `Status: "DELETING"` (container stays queryable via
   `DescribeContainer` until the delay elapses), both genuinely observable by
   a polling `DescribeContainer`/`ListContainers` caller -- unlike
   `services/redshift`, no periodic background-reconciler goroutine was
   added; transitions are advanced purely lazily (`advanceContainerStates`,
   called at the top of `CreateContainer`/`DeleteContainer`/
   `DescribeContainer`/`ListContainers`), which is sufficient because every
   realistic caller (including AWS SDK waiters) polls a Describe/List
   endpoint in a loop, and it keeps this service's "no goroutines" leak
   invariant intact. Default behavior (`activationDelay == 0`) is completely
   unchanged, so every pre-existing test continues to pass unmodified.
   `containerTransitions` is intentionally not persisted across Snapshot/
   Restore (see the comment in `persistence.go`'s `Restore`), matching
   `services/redshift`'s same choice for `clusterTransitions`. Tested in
   `containers_test.go`'s new
   `TestInMemoryBackend_ContainerActivationDelay` table (`zero_delay_is_synchronous`,
   `positive_delay_is_observable`), using a `waitForContainerStatus` poll
   helper modeled on `services/redshift/reconciler_test.go`'s `waitFor`.

Self-gates re-run after these changes: `go build ./services/mediastore/...`,
`go vet ./services/mediastore/...`, `go test -race -count=1
./services/mediastore/...`, `gofmt -l services/mediastore/` all clean; see
the top-level parity-3 phase-2 session receipt for verbatim output.

## mediastore (this session, 2026-08-20)

Wrapper-key / nested-shape wire-parity sweep, all 21 ops, against the
**pinned** `aws-sdk-go-v2/service/mediastore@v1.32.4` (go.mod), read fresh
from `$(go env GOMODCACHE)/.../mediastore@v1.32.4/{api_op_*.go,types/types.go,
types/enums.go,types/errors.go,deserializers.go}` -- not trusted from the
prior audit's v1.29.23-era conclusions. The SDK's own CHANGELOG.md for
v1.29.24..v1.32.4 lists only dependency/infra bumps (smithy-go updates,
clock-skew option, snapshot tests) for this service, no operation or shape
changes, so no drift existed between audits -- confirmed by direct reading of
the pinned version regardless.

Protocol: confirmed JSON-RPC 1.1 (`awsAwsjson11_*` serializer/deserializer
prefix in `serializers.go`/`deserializers.go`), single `POST /` with
`X-Amz-Target: MediaStore_20170901.<Op>` (`serializers.go:59` for
CreateContainer, same pattern every op). The restjson `cnhp` dead-code trap
(`deserializeOpDocument<Op>Output` never called, live path decodes flat) does
**not** apply to JSON-RPC services -- confirmed the opposite is true here:
every op's `awsAwsjson11_deserializeOpDocument<Op>Output` **is** the live
per-op deserialize path (called directly from each op's `HandleDeserialize`),
and it decodes the body as a real wrapper-key map (e.g. `CreateContainerOutput`
switches on top-level key `"Container"` -> `deserializeDocumentContainer`,
`GetMetricPolicyOutput` switches on `"MetricPolicy"` ->
`deserializeDocumentMetricPolicy`), not flat. Verified directly in
`deserializers.go` for `CreateContainerOutput` (line ~3524),
`DescribeContainerOutput` (~3715), `ListContainersOutput` (~3903),
`GetMetricPolicyOutput` (~3867), `GetCorsPolicyOutput` (~3791),
`GetLifecyclePolicyOutput` (~3827), `GetContainerPolicyOutput` (~3751),
`ListTagsForResourceOutput` (~3948).

**Full-field-list diff, every op, optional members included** (gopherstack
`models.go`/`containers.go`/`cors_policy.go`/`lifecycle_policy.go`/
`metric_policy.go`/`tags.go`/`handler.go` vs SDK `api_op_<Op>.go` Input/Output
structs): all 21 ops match exactly, member-for-member, including optional
fields (`Tags` on `CreateContainerInput`, `NextToken`/`MaxResults` on
`ListContainersInput`, `AllowedMethods`/`ExposeHeaders`/`MaxAgeSeconds` on
`CorsRule`, `MetricPolicyRules` on `MetricPolicy`). No fabricated members, no
missing members. `DescribeContainerInput.ContainerName` is genuinely NOT
`// This member is required` in the SDK (confirmed by reading the struct
directly, not a sibling) -- gopherstack's server-side non-empty check on it is
a defensible superset guard, not a shape mismatch (same conclusion as the
prior audit, re-verified independently).

**Enum check, both directions**:
- `ContainerStatus` (`ACTIVE`/`CREATING`/`DELETING`, `types/enums.go`): all
  three values represented by gopherstack's `containerStatusActive`/
  `containerStatusCreating`/`containerStatusDeleting` constants
  (`store.go:17,21,25`), byte-for-byte. Every constant gopherstack emits is a
  real SDK value; every SDK value is representable.
- `ContainerLevelMetrics` (`ENABLED`/`DISABLED`): `metric_policy.go`'s
  `PutMetricPolicy` checks `policy.ContainerLevelMetrics != "ENABLED" &&
  != "DISABLED"` -- both directions covered, no third value invented.
- `MethodName` (`PUT`/`GET`/`DELETE`/`HEAD`, used in `CorsRule.AllowedMethods`):
  gopherstack models `AllowedMethods []string` and does not validate rule
  values against this enum server-side (accepts any string). This is a
  **Layer 3 completeness gap** (missing validation), not a wire-shape bug --
  disclosed, not fixed, consistent with the brief's Layer 3 being out of scope
  for this hunt. No fabricated `MethodName` constant is emitted anywhere in
  gopherstack's own code, so the "SDK value not representable" direction is
  clean; only the "server accepts a value the enum doesn't have" direction is
  an open (pre-existing, unchanged) gap.

**`Container` item shape across CreateContainer / DescribeContainer /
ListContainers**: identical. All three ops share `models.go`'s
`containerObject` (`ARN`, `AccessLoggingEnabled`, `CreationTime`, `Endpoint`,
`Name`, `Status` -- exactly the 6 fields of SDK `types.Container`) via the
single `toContainerObject` conversion function (`handler.go`). CreateContainer/
DescribeContainer nest it under `"Container"`; ListContainers nests the slice
under `"Containers"` plus optional `"NextToken"` -- matches
`deserializeOpDocumentListContainersOutput`'s two-key switch exactly.

**`MaxAgeSeconds` / lifecycle-policy-as-string**: `CorsRule.MaxAgeSeconds` is
Go `int` marshaled as a JSON number (`models.go`), matching the SDK's
`json.Number`-decoded `int32` (`deserializeDocumentCorsRule`,
`MaxAgeSeconds` case) -- same wire type, no string/number mismatch.
`LifecyclePolicy`/`getLifecyclePolicyResponse.LifecyclePolicy` and
`PutLifecyclePolicyInput.LifecyclePolicy` are both plain JSON strings in
gopherstack (`lifecycle_policy.go`, `models.go`), matching
`GetLifecyclePolicyOutput.LifecyclePolicy *string` / `PutLifecyclePolicyInput.
LifecyclePolicy *string` in the SDK -- not a structure, confirmed correct.

**Error mapping**: gopherstack's `writeBackendError` (`handler.go`) maps
`ContainerNotFoundException` (404), `PolicyNotFoundException` (404, shared by
container/lifecycle/metric policy not-found), `CorsPolicyNotFoundException`
(404, its own type, not conflated with `PolicyNotFoundException`),
`ContainerInUseException` (409), `ValidationException` (400), default
`InternalFailure` (500). Cross-checked against every op's own
`awsAwsjson11_deserializeOpError<Op>` switch in `deserializers.go`: each op's
declared typed-exception set (e.g. `GetMetricPolicy` recognizes
`ContainerInUseException`/`ContainerNotFoundException`/`InternalServerError`/
`PolicyNotFoundException`; `ListContainers` recognizes only
`InternalServerError`) is a *client-decode* allowlist, not a server
restriction -- any op can still emit a generic `smithy.GenericAPIError` for a
code outside its switch, so gopherstack's app-wide (not per-op) error mapping
is not a shape bug. `LimitExceededException` is declared by the real SDK only
for `CreateContainer` (per-account container-count quota) and is not modeled
by gopherstack (no container-count cap enforced) -- a genuine, pre-existing,
disclosed-not-fixed gap (quota/limit simulation, not a wire-shape defect).
`InternalServerError` (fault, 500) is intentionally never deterministically
triggered by any backend path, matching the fact that it models a genuine
service-fault condition, not a client-reachable one.

**Families**: Container, ContainerPolicy, CorsPolicy, LifecyclePolicy,
MetricPolicy, Tags -- all CLEAN, zero bugs found this pass.

**Existing tests**: read `containers_test.go`, `cors_policy_test.go`(n/a --
folded into `handler_cors_policy_test.go`), `handler_containers_test.go`,
`handler_cors_policy_test.go`, `handler_lifecycle_policy_test.go`,
`handler_metric_policy_test.go`, `handler_policies_test.go`,
`handler_tags_test.go`, `handler_sdk_route_table_test.go`,
`sdk_completeness_test.go` -- no test asserts a wrong key, nesting, JSON type,
or enum value; nothing corrected.

**Hand-revert proof skipped for wire shape**: no shape bug was found to prove
via revert. Instead, the wrapper-key claims above were proven by direct,
independent reading of `deserializers.go`'s per-op switch statements (pasted
inline above), which is the authoritative live-path check the brief's `cnhp`
section calls for.

**Provenance**: `last_audit_commit: 7e4e35369` -> `git show -s --format=%ad
7e4e35369` = `Fri Jul 24 09:07:46 2026 -0500`; `last_audit_date: 2026-07-24`.
Same day, no gap -- clean, not a stale stamp. Refreshed this pass to
`last_audit_commit: 67b92e0b9` (repo HEAD at session start) /
`last_audit_date: 2026-08-20`.

**Gates** (verbatim outcomes): `go build ./services/mediastore/...` clean;
`go vet ./services/mediastore/...` clean; `go fix -diff
./services/mediastore/...` empty; `gofmt -l services/mediastore/` empty;
`go test -race ./services/mediastore/...` `ok` (1.1s); `golangci-lint run
./services/mediastore/...` `0 issues.`; line-length-120 sweep (`awk 'length >
120'` over `services/mediastore/*.go`) empty; banned-nolint grep
(cyclop/gocyclo/gocognit/funlen) empty. `git status --short` shows no changes
under `services/mediastore/` from this session (only this `PARITY.md` edit);
unrelated dirty files exist under `services/iotdataplane/` and `.claude/` from
other concurrent work, not touched here.

**Zero bugs found this session.** Every brief hint (Container/CorsRule/
MetricPolicy field lists, `MaxAgeSeconds` int32, lifecycle policy as string,
enum sets) matched the pinned v1.32.4 SDK exactly -- nothing in the brief
disagreed with the pinned SDK.

## Audit 2026-09-07 (gopherstack-apg3 -- DeleteContainer empty-container precondition)

Title-only issue, no description. Verdict: **STRUCTURAL GAP, not fixed** --
see the `gaps` entry above for the full citation trail. Short version: real
AWS's DeleteContainer doc prose requires an empty container, but neither the
Go SDK's nor botocore's modeled error set for the op names a distinct
"container not empty" exception to enforce it against, and this repo's
object data plane (`services/mediastoredata`) is not merely isolated from
`services/mediastore` the way appconfig/appconfigdata are (separate
`Provider.Init`, no cross-service handle in `pkgs/service.AppContext`) --
its `InMemoryBackend.states` is keyed only by region
(`services/mediastoredata/store.go:38-42`), with **no container dimension in
the data model at all**. Fixing this for real needs two things, neither of
which this pass built: (1) a cross-service handle from mediastore to
mediastoredata's backend (or a shared store), and (2) adding a container key
to mediastoredata's object table, which today only exists per-region.
Building either is new cross-service architecture, correctly out of scope
for a parity fix.

Also checked the scope note's three adjacent questions: DeleteContainer's
error type for a missing container is correct
(`ContainerNotFoundException`, already verified and pinned by prior audits);
container-name format validation runs only on `CreateContainer`
(`validateContainerName`, `containers.go:19-26`), not on `DeleteContainer` or
any other container-name-consuming op -- this is a pre-existing, uniform
choice across the whole file, not something introduced or specific to
`DeleteContainer`, and the real SDK's client-side validator
(`validators.go:33-51`) only checks required-ness, not the `ContainerName`
shape's `pattern`/`max` traits, so there's no clear SDK-side evidence this
actually diverges from real AWS; left unverified rather than "fixed" on a
guess. DeleteContainer has no `Status` precondition: deleting a `CREATING`
container is intentional, tested behavior (supersedes the pending
`CREATING`->`ACTIVE` transition, per `containers.go:125-129`'s comment and
`TestInMemoryBackend_ContainerActivationDelay`); a second `DeleteContainer`
call while a container is already `DELETING` is untested and its real-AWS
behavior could not be established from the SDK/botocore model available
here -- flagged as an open question, not asserted as a bug.

No code changed this session (no fixable defect within reach). Gates run
against unmodified code for the record: `GOTOOLCHAIN=go1.27.0 golangci-lint
run ./services/mediastore/... ./services/mediastoredata/...` -> `0 issues.`;
`GOTOOLCHAIN=go1.27.0 go test -race ./services/mediastore/...
./services/mediastoredata/...` -> both `ok`. Confidence: high on the
structural-isolation finding (multiple independent code-level confirmations:
provider wiring, `AppContext` shape, and mediastoredata's region-only
storage keying); lower on the two flagged-but-unverified adjacent items
(name-format validation on non-Create ops, double-delete-while-DELETING
semantics), which are genuinely unconfirmed against real AWS rather than
dismissed.
