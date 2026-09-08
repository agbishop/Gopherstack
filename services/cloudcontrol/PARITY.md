---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: cloudcontrol
sdk_module: aws-sdk-go-v2/service/cloudcontrol@v1.32.4
last_audit_commit: 569c029d
last_audit_date: 2026-08-20
overall: A            # wrapper-key/nested-shape sweep (2026-08-20): zero bugs found, real-SDK round-trip test added
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "DesiredState now enforced as required (was silently accepted empty, matching CreateResourceInput.DesiredState 'This member is required'); ProgressEvent.ResourceModel populated; AlreadyExistsException/InvalidRequestException HTTP 400"}
  GetResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "ResourceNotFoundException HTTP 400"}
  UpdateResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "PatchDocument now enforced as required (was silently no-op'd by applyPatch on an empty/missing patch, matching UpdateResourceInput.PatchDocument 'This member is required'); ClientToken idempotency added (real UpdateResourceInput.ClientToken field was previously dropped entirely -- accepted on the wire but never passed to the backend); ProgressEvent.ResourceModel reflects post-patch Properties; applyPatch now resolves each Path as a real RFC 6901 JSON Pointer (nested objects + array elements/indices), fixing a bug where a multi-segment Path (e.g. /Tags/0/Value) was treated as a literal top-level map key instead of navigating -- see Notes below (2026-09-04 pass); all six RFC 6902 op types now implemented -- move/copy/test were previously accepted on the wire and silently skipped, now applied with real cross-path/value semantics and a failed test/move/copy aborts the WHOLE patch (InvalidRequestException), matching RFC 6902's atomic-patch contract -- see Notes below (2026-09-06 pass, gopherstack-j6lv)"}
  DeleteResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "ClientToken idempotency added (real DeleteResourceInput.ClientToken field was previously dropped entirely)"}
  ListResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "pagination via pkgs/page; InvalidRequestException on malformed TypeName; now returns defensive copies (see leaks note) instead of live backend pointers; ResourceModel (real 'resource model to use to select the resources to return' field) is now applied as a real filter -- see gopherstack-c9yf fix below"}
  GetResourceRequestStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "unknown token -> RequestTokenNotFoundException (the only error this op declares); output now includes HooksProgressEvent (real field on GetResourceRequestStatusOutput, always empty/omitted -- this backend has no Hooks concept). ERROR-CODE FIX (2026-09-07, gopherstack-v5eb): empty RequestToken previously returned InvalidRequestException, not declared by this op (confirmed: deserializeOpErrorGetResourceRequestStatus's only named case is RequestTokenNotFoundException) -- see Notes below."}
  CancelResourceRequest: {wire: ok, errors: ok, state: ok, persist: ok, note: "unknown token -> RequestTokenNotFoundException; non-IN_PROGRESS status -> ConcurrentModificationException/HTTP 500 -- confirmed against live API reference. ERROR-CODE FIX (2026-09-07, gopherstack-v5eb): empty RequestToken previously returned InvalidRequestException, not declared by this op either (only RequestTokenNotFoundException/ConcurrentModificationException) -- see Notes below."}
  ListResourceRequests: {wire: ok, errors: ok, state: ok, persist: ok, note: "INVENTED-FIELD FIX: ResourceRequestStatusFilter.TypeName was NOT a real field (confirmed against both aws-sdk-go-v2/service/cloudcontrol/types and botocore's service-2.json -- the real filter shape has exactly Operations + OperationStatuses, no TypeName) and was silently narrowing results below what real AWS would return for the same filter body; deleted the field and the filtering logic that used it. ERROR-CODE FIX (2026-09-07, gopherstack-v5eb): the prior 'enum validation confirmed correct' framing here was wrong -- an unrecognized Operations/OperationStatuses value returned InvalidRequestException, but this op declares ZERO errors in the real model (confirmed: botocore's service-2.json has an empty errors list for ListResourceRequests, unique among this service's 8 ops). Fixed to never error; an unrecognized value now simply matches no tracked request -- see Notes below."}
families:
  progress_event_lifecycle: {status: ok, note: "every mutating op completes synchronously to a terminal SUCCESS (or CANCEL_COMPLETE) in the same call -- no PENDING/IN_PROGRESS hang risk since GetResourceRequestStatus/ListResourceRequests read the same requests table that was just written"}
  persistence: {status: ok, note: "Handler/InMemoryBackend both implement Snapshot/Restore (persistence.go), versioned, wired via store.Registry (store_setup.go); confirmed round-trips resources+requests+clientTokens in persistence_test.go. cloudcontrolSnapshotVersion bumped 1->2 this pass: ClientTokens' value type changed from a bare requestToken string to clientTokenEntry{RequestToken,Fingerprint} to support ClientTokenConflictException detection (see client_token_idempotency below) -- a real shape change, so old snapshots are discarded cleanly rather than risking a partial/wrong decode."}
  client_token_idempotency: {status: ok, note: "CreateResource/UpdateResource/DeleteResource all implement ClientToken idempotency (return the cached ProgressEvent on token replay instead of re-processing), matching the real SDK: all three *Input shapes declare a ClientToken member. NEW this pass (gopherstack-c9yf): ClientTokenConflictException is now detected for real -- each cached entry also stores a deterministic fingerprint of the original request (op+TypeName+Identifier+DesiredState/PatchDocument); replaying a token with the SAME fingerprint still idempotently returns the cached event, but replaying it with a DIFFERENT fingerprint (a genuinely different request reusing someone else's token) now returns ClientTokenConflictException/HTTP 400. This is a real, deterministic check -- no fabrication, no fault injection needed. clientTokens' persisted value type changed from a bare string to {requestToken, fingerprint} (backendSnapshot.ClientTokens), so cloudcontrolSnapshotVersion bumped 1->2 (old snapshots discarded cleanly per the existing version-mismatch protocol, not partially decoded)."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "cloudcontrol keeps its own generic resource store; it does NOT delegate to the real per-service backend (e.g. AWS::S3::Bucket via CreateResource does not create a row visible to services/s3's ListBuckets, and vice versa). This is explicitly allowed by the task brief (either design is parity-correct) but is a real cross-service gap for any test that mixes CloudControl and native-service calls against the same logical resource. No bd issue filed yet -- flagging for triage."
  - "TypeNotFoundException (extension not registered in the CFN registry) is unreachable: this backend has no type registry, so any well-formed TypeName (ns::svc::type) is implicitly accepted. GENUINELY IMPOSSIBLE without fabrication (re-triaged gopherstack-c9yf, not fixed): real CloudFormation/CloudControl's registry spans thousands of AWS-published + arbitrarily many privately-registered third-party extension types, and whether a given TypeName is 'registered' is fundamentally an account-specific, mutable fact (types get (de)activated per account/region via RegisterType/DeactivateType, which cloudcontrol's own SDK surface doesn't even expose -- that's CloudFormation's API). Any registry gopherstack could build here would be one of: (a) an arbitrary hardcoded allowlist of 'known' AWS types, which would be incomplete by construction and would make ListResources/CreateResource start REJECTING valid TypeNames this emulator previously accepted -- a regression, not a fix, and itself a fabricated 'known types' dataset; or (b) accept-everything, which is exactly today's (correct, honest) behavior. There is no third option that adds real signal without inventing data. Not fixed."
chaos_coverage:           # errors reachable via pkgs/chaos fault injection rather than backend logic — verified, not a gap
  - "The remaining 12 documented-but-unreachable exceptions from gopherstack-c9yf (ThrottlingException, ServiceLimitExceededException, HandlerFailureException, NotStabilizedException, NotUpdatableException, ResourceConflictException, PrivateTypeException, GeneralServiceException, NetworkFailureException, InvalidCredentialsException, HandlerInternalFailureException, ConcurrentOperationException) are ALREADY COVERED by pkgs/chaos, not a gap needing backend code. Verified concretely: Handler implements service.ChaosProvider (ChaosServiceName()==\"cloudcontrol\", ChaosOperations()==GetSupportedOperations(), ChaosRegions()), so it is enumerated by GET /_gopherstack/chaos/targets. The chaos middleware (pkgs/chaos/middleware.go) runs as global Echo middleware registered via registry.Use(chaos.Middleware(...)) (cli.go:5754) OUTSIDE/BEFORE any service's own routing, and extracts service+operation from the same SigV4 Authorization header + X-Amz-Target header this service's own RouteMatcher/ExtractOperation already rely on (cloudcontrol is awsjson1.0 with X-Amz-Target: CloudApiService.<Op>, so extractOperationFromRequest's X-Amz-Target-after-the-dot parsing resolves the exact operation name, e.g. \"CreateResource\") -- so a fault rule {service: \"cloudcontrol\", operation: \"CreateResource\", error: {code: \"ThrottlingException\", statusCode: 400}} deterministically short-circuits that op with an arbitrary injected Code+StatusCode (FaultError carries both, pkgs/chaos/fault_response.go) before this handler ever runs. Synthesizing these from backend state instead (e.g. fabricating a request-rate counter under a single coarse lock with no real concurrency contention) would be exactly the kind of invented signal this project's honesty rules forbid; fault injection is the correct, non-fabricated mechanism for exceptions AWS only returns under real infrastructure conditions this emulator doesn't have."
deferred: []              # consciously not audited this pass (scope) — next pass targets; none this pass
leaks: {status: clean, note: "no goroutines/timers/janitors; InMemoryBackend is pure lockmetrics.RWMutex + store.Table state, no background work. FIXED this pass: ListResources/ListAllResources previously returned *Resource pointers live inside the backend's store.Table -- a caller mutating one directly corrupted backend state without the lock, a real (if previously unexploited -- no current caller retains/mutates the result) mutation-safety hole. Now copies on the way out, matching every other accessor. Locked in by TestBackend_ListAllResources_ReturnsCopiesNotLiveState / TestBackend_ListResources_ReturnsCopiesNotLiveState."}
---

## Notes

**Fixed this pass (2026-09-07, bd issue gopherstack-v5eb)**:

`cmd/errtargetaudit` flagged 3 class A findings (8/8 ops resolved, 8/8 emissions
found, no coverage warning) -- all three were `domain=Handler`, all top-level
exceptions (none embedded in a `ProgressEvent.ErrorCode`/`FailureReason`, so the
sqs-batch-style false-positive class does not apply here), and all three were
confirmed real, not tool artifacts:

- `GetResourceRequestStatus` (`handler.go:409`, since renumbered) and
  `CancelResourceRequest` (`handler.go:435`) both rejected an empty/missing
  `RequestToken` with `InvalidRequestException` via the same copy-pasted
  `ErrValidation` guard used by every other required-field check in this file.
  Verified against the pinned SDK
  (`aws-sdk-go-v2/service/cloudcontrol@v1.32.4/deserializers.go`,
  `awk "/deserializeOpError<Op>\(/,/^}/" | grep -oE '"[A-Za-z0-9]+"'`):
  `GetResourceRequestStatus` declares only `UnknownError, RequestTokenNotFoundException`;
  `CancelResourceRequest` declares only
  `UnknownError, ConcurrentModificationException, RequestTokenNotFoundException`.
  Neither declares `InvalidRequestException`. `RequestToken` is `required` on both
  input shapes (`GetResourceRequestStatusInput`/`CancelResourceRequestInput`,
  botocore's `service-2.json`), so a conformant SDK client can never even send an
  empty value -- the Go SDK's own `validators.go` rejects it client-side
  (`smithy.InvalidParamsError`, never touches the wire) -- but this backend is
  also driven directly over the wire (tests, raw HTTP, `errtargetaudit` itself),
  so the server-side guard was still reachable and still wrong. Fixed by deleting
  both guards outright: an empty `RequestToken` never matches a tracked request,
  so it now falls through naturally to the same `RequestTokenNotFoundException`
  path an unrecognized token already takes -- no new code, no special-casing.
- `ListResourceRequests`' `validateFilter` (`resource_requests.go:138`, since
  deleted) rejected an unrecognized `Operations`/`OperationStatuses` value with
  `InvalidRequestException`. Verified this op declares **zero** errors in the
  real model: `deserializeOpErrorListResourceRequests` has no named-error `case`
  at all (falls straight to the generic/`UnknownError` default), confirmed
  independently against botocore's `service-2.json`
  (`operations.ListResourceRequests.errors == []`) -- the only one of this
  service's 8 ops with an empty declared-error list. The prior PARITY.md note
  claiming "enum validation confirmed correct" for this filter was itself wrong;
  validating and rejecting was the bug, not a feature. Fixed by deleting
  `validateFilter` (and the now-unused `validOperations`/`validOperationStatuses`
  lookup sets) entirely: `eventMatchesFilter`'s existing `slices.Contains` checks
  already fail an unrecognized value closed on their own -- no real
  `ProgressEvent.Operation`/`OperationStatus` will ever equal e.g. `"BOGUS"`, so
  the op now returns 200 with that criterion matching nothing, never a 400.
- None of the three findings interact with this service's synchronous-completion
  design (`families.progress_event_lifecycle` above): all three are input-shape
  validation on the request itself, not a provisioning outcome, so there is no
  FAILED-state/ProgressEvent angle here -- confirmed by reading each call site,
  not assumed.
- Root cause: a single copy-pasted "required field -> `ErrValidation`" pattern
  (correct for `CreateResource`/`UpdateResource`/`DeleteResource`/`GetResource`/
  `ListResources`, which all genuinely declare `InvalidRequestException`) was
  applied uniformly to `GetResourceRequestStatus`/`CancelResourceRequest`/
  `ListResourceRequests` without checking each op's own declared set -- not a
  global sentinel-map issue (gopherstack-hdvu): each op still has its own
  `errors.go` sentinel and its own `handleError` case, just misapplied per call
  site, exactly the "fix per call site" shape that issue already prescribes.
  No class-8 (consumed downstream) or class-9 (helper-sentinel-survives-fix)
  shape present: all three sentinels are returned directly to `handleError` with
  nothing intervening.
- Pre-existing tests corrected (2, both previously pinned the wrong code with no
  note): `TestHandler_ListResourceRequests_EnumValidation` asserted 400 for an
  unrecognized enum value in the filter (now asserts 200 with zero matching
  summaries); the table-driven `TestHandler_GetResourceRequestStatus`/
  `TestHandler_CancelResourceRequest` "missing RequestToken returns 400" cases
  were left as-is (400 is still correct, they never asserted the specific wire
  code) but are now supplemented by dedicated tests below that do.
- Regression tests added: `TestHandler_GetResourceRequestStatus_EmptyRequestToken_IsRequestTokenNotFound`,
  `TestHandler_CancelResourceRequest_EmptyRequestToken_IsRequestTokenNotFound` (each
  asserts `RequestTokenNotFoundException` and explicitly asserts NOT
  `InvalidRequestException`); `TestHandler_ListResourceRequests_EnumValidation`
  rewritten to assert 200 + empty-match for an unrecognized `Operations`/
  `OperationStatuses` value, and 200 + a real match for valid values.
- Per-line neuter results (each reverted immediately after): reinstating the old
  `InvalidRequestException` guard in `handleGetResourceRequestStatus` failed
  `TestHandler_GetResourceRequestStatus_EmptyRequestToken_IsRequestTokenNotFound`
  at its `RequestTokenNotFoundException`/`InvalidRequestException` assertions;
  same guard in `handleCancelResourceRequest` failed the equivalent Cancel test
  the same way; reinstating an `Operations`-only enum check in
  `ListResourceRequests` failed
  `TestHandler_ListResourceRequests_EnumValidation/unrecognized_operation_value_matches_nothing,_no_error`
  at its `200` HTTP-status assertion. All three neuters compiled; all three
  failed the test named for them, nothing else.
- Re-ran `cmd/errtargetaudit` after the fix: cloudcontrol no longer appears in
  the report at all (the tool omits a service entirely once it has zero class A
  findings and no coverage warning). Confirmed directly with
  `go run ./cmd/errtargetaudit -dir cloudcontrol -json <path>`:
  `opsGroundTruth: 8, opsResolved: 8, opsWithEmission: 7` (down from 8; expected --
  `ListResourceRequests` now has no error path left to emit from at all, which is
  correct given it declares none), `0 class A findings, 0 coverage warnings`.

**Fixed this pass (2026-09-06, bd issue gopherstack-j6lv)**:

- `applyPatch`'s `move`, `copy` and `test` RFC 6902 op types are now implemented
  (previously accepted on the wire and silently skipped, per the 2026-09-04 note
  below). All three resolve their pointers via the same RFC 6901 walk `add`/
  `replace`/`remove` already use (`splitPointer`/a new read-only `resolvePointer`).
  - `move` (RFC 6902 4.4: "remove the value at a specified location and add it to
    the target location... functionally identical to a 'remove' operation... followed
    immediately by an 'add' operation") is implemented as exactly that sequence, so
    within-array moves reindex correctly (RFC 6902 4.4's own example). Per the RFC,
    "a location cannot be moved into one of its children" -- `from` being a proper
    prefix of `path` is rejected (`isProperPrefix`).
  - `copy` (RFC 6902 4.5: "the value at a specified location... copied to the target
    location") adds a **deep** copy of the source value (`deepCopyJSON`) so `path`
    and `from` never alias the same backing `map[string]any`/`[]any` -- a mutation
    reaching one through a later op in the SAME patch document must not corrupt the
    other. Regression: `TestBackend_UpdateResource_CopyOp_DeepCopyIndependence`, which
    copies a two-level-deep source (`{"Source":{"Inner":{"A":1}}}`) and mutates
    `/Source/Inner/A` in the SAME patch document as the `copy` -- both properties
    (depth >= 2, single call) are load-bearing: `Properties` is a JSON string, so a
    SEPARATE `UpdateResource` call re-`Unmarshal`s it and destroys any aliasing before
    a later call could observe it, and a one-level-deep source can't distinguish a
    real deep copy from an outer-map-only shallow copy (a copied scalar value is a Go
    value type regardless of how it got there). An earlier version of this test made
    both mistakes (two separate `UpdateResource` calls, one-level-deep source) and
    could not actually detect a missing/shallow `deepCopyJSON` -- confirmed by
    reverting `deepCopyJSON(value)` to `value` at its call site and separately
    shallowing `deepCopyJSON`'s map branch, one at a time: the old test passed either
    way. The rewritten test fails under both neuters.
  - `test` (RFC 6902 4.6: "test that a value at the target location is equal to a
    specified value") compares by JSON structural equality, not Go `==`/
    `reflect.DeepEqual` semantics assumed blindly: `json.Marshal`-then-compare on the
    decoded `any` values (`jsonValuesEqual`), the same convention `matchesResourceModel`
    already uses in this file. This is correct because `encoding/json` decodes every
    JSON number to `float64` regardless of `1` vs `1.0` source spelling (so both marshal
    back identically) and `json.Marshal` on a Go map always emits keys in sorted order
    (so member order in the original document never affects the comparison), while
    array element order is preserved and does affect it -- exactly RFC 6902 4.6's rule.
  - **Failed-test error**: `UpdateResource` declares no `TestOperationFailedException`
    or equivalent -- the SDK's declared error set for this op (`aws-sdk-go-v2/service/
    cloudcontrol@v1.32.4`, `awk "/deserializeOpErrorUpdateResource\(/,/^}/" deserializers.go`)
    is `UnknownError, AlreadyExistsException, ClientTokenConflictException,
    ConcurrentOperationException, GeneralServiceException, HandlerFailureException,
    HandlerInternalFailureException, InvalidCredentialsException,
    InvalidRequestException, NetworkFailureException, NotStabilizedException,
    NotUpdatableException, PrivateTypeException, ResourceConflictException,
    ResourceNotFoundException, ServiceInternalErrorException,
    ServiceLimitExceededException, ThrottlingException, TypeNotFoundException,
    UnsupportedActionException`. A failed `test` (or an unresolvable `move`/`copy`
    `from`/`path`, or a rejected `move` prefix) now maps to **`InvalidRequestException`**
    (gopherstack's existing `ErrValidation`), whose own SDK doc comment reads "The
    resource handler has returned that invalid input from the user has generated a
    generic exception" (`types/errors.go`) -- this is CloudControl's only generic
    client-input-validation error (confirmed no `ValidationException` exists in this
    service's model, per the 2026-07-24 note below) and is already this exact file's
    established mapping for every other malformed-request condition (bad `TypeName`,
    missing `PatchDocument`, etc. -- `ErrValidation` throughout `resources.go`/
    `handler.go`). No candidate in the declared set is a closer semantic fit: the
    `*Fault`-suffixed handler errors (`HandlerFailureException`,
    `NotStabilizedException`, `NotUpdatableException`, etc.) all describe a downstream
    resource-handler-reported failure during actual provisioning, not a client-supplied
    patch that fails to apply before any handler would even run. **Confidence: high**
    for consistency with this codebase's own established error taxonomy; **moderate**
    for exact real-AWS behavior, since a live account probe of "what does Cloud Control
    actually return for a failed JSON Patch test op" isn't available in this
    environment -- there is no dedicated exception for this condition in the SDK's
    declared set either way, so `InvalidRequestException` is the least-fabricated,
    best-evidenced choice available.
  - **Atomicity**: per RFC 6902 3 ("Operations are applied sequentially in the order
    they appear... If a normative requirement is violated... the entire patch
    document... SHALL NOT be applied"), a failure at any op discards the WHOLE patch.
    `applyPatch`'s existing structure already supported this without further changes:
    it unmarshals `Properties` into a fresh local `doc` map, mutates only that local
    value across the whole loop, and `UpdateResource` assigns the result to
    `r.Properties` in a single statement only after `applyPatch` returns successfully
    -- so an error return (which always carries back the original, pre-loop `document`
    string, never the partially-mutated one) can never leak a half-applied patch into
    backend state. Regression:
    `TestBackend_UpdateResource_TestOp_Fails_AbortsWholePatch` (an earlier `replace` in
    the same patch that already mutated the local `doc` is discarded when a later `test`
    in the same patch fails).
  - Regression tests: `TestBackend_UpdateResource_MoveOp` (3 subtests: top-level field,
    into nested object, array element shifts index),
    `TestBackend_UpdateResource_MoveOp_FromIsPrefixOfPath_Rejected`,
    `TestBackend_UpdateResource_CopyOp` (2 subtests: top-level field, nested object),
    `TestBackend_UpdateResource_CopyOp_DeepCopyIndependence`,
    `TestBackend_UpdateResource_TestOp_Passes` (2 subtests: matching scalar, integer
    matches float value), `TestBackend_UpdateResource_TestOp_Fails` (2 subtests: value
    mismatch, path missing), `TestBackend_UpdateResource_TestOp_Fails_AbortsWholePatch`.
  - **Known limitation, not fixed**: a root (`""`/`"/"`) `path` is silently skipped for
    every op (pre-existing behavior inherited unchanged from `add`/`replace`/`remove`
    -- this simplified engine has no well-defined "replace/remove the whole document"
    container to mutate in place), and a root `from` on `move`/`copy` is rejected as
    `InvalidRequestException` rather than supported (real RFC 6901 allows the empty
    string as a valid whole-document pointer; this backend does not implement moving/
    copying the entire document as a unit). Neither case is exercised by any real
    resource-property patch this emulator has seen, and no test in this pass depends on
    it, but it is a real, intentional gap worth flagging for the next auditor.

**Fixed this pass (2026-09-04)**:

- `UpdateResource`'s `applyPatch` treated `PatchDocument`'s per-op `Path` as a literal
  top-level map key (`strings.TrimPrefix(op.Path, "/")` used directly as `doc[field]`)
  instead of resolving it as an RFC 6901 JSON Pointer. The real
  `UpdateResourceInput.PatchDocument` is "a JSON document listing the patch operations
  that ... adheres to the RFC 6902 ... standard" (`api_op_UpdateResource.go`), and RFC
  6902 paths are routinely multi-segment for real resource shapes (nested objects like
  Lambda's `Environment.Variables`, array elements like a `Tags[i].Value`). A patch
  `{"op":"replace","path":"/Tags/0/Value","value":"c"}` against
  `{"Tags":[{"Key":"a","Value":"b"}]}` silently corrupted the document into
  `{"Tags":[{"Key":"a","Value":"b"}],"Tags/0/Value":"c"}` -- the real field was left
  unchanged and a bogus literal-slash-named top-level key was added instead. Fixed by
  giving `applyPatch` a real pointer walk (`splitPointer`/`applyPointerOp`/
  `applyArrayPointerOp`) that decodes RFC 6901 escaping (`~1`->`/`, `~0`->`~`) and
  navigates nested `map[string]any`/`[]any` structures, including the `-` end-of-array
  token and index-shifting array insert/remove. `move`/`copy`/`test` remain
  unimplemented (accepted on the wire, silently skipped) since they need cross-path/
  value semantics this best-effort engine doesn't attempt -- not a new gap, matches the
  prior "simplified" scope, just now documented explicitly rather than silently
  degrading `add`/`replace`/`remove` themselves. Regression test:
  `TestBackend_UpdateResource_NestedPatchPaths` (7 subtests: nested-object
  replace/add/remove, array-index replace/remove, array insert via `-` and via index).

**Wrapper-key/nested-shape sweep (2026-08-20)**: all 8 ops (CreateResource,
DeleteResource, GetResource, GetResourceRequestStatus, ListResourceRequests,
ListResources, UpdateResource, CancelResourceRequest) re-verified field-by-field
against `aws-sdk-go-v2/service/cloudcontrol@v1.32.4`'s live
`deserializers.go` (`awsAwsjson10_deserializeOpDocument<Op>Output`, confirmed
by grep-count==2 -- defined AND called -- and by reading the function bodies,
not just the names). **Zero bugs found.** Confirmed correct, with citations:

- Protocol is `awsjson1.0` (JSON-RPC-style, whole body is the shape, no
  restjson body-flattening trap applies here -- `deserializers.go:84` prefix
  `awsAwsjson10_`).
- All three list/get wrapper keys distinct and correct: `GetResourceOutput`
  is `{ResourceDescription, TypeName}` (`deserializers.go:3257`),
  `ListResourcesOutput` is `{NextToken, ResourceDescriptions, TypeName}`
  (`deserializers.go:3388`), `ListResourceRequestsOutput` is `{NextToken,
  ResourceRequestStatusSummaries}` (`deserializers.go:3343`).
- All five `ProgressEvent`-wrapping ops (`Create`/`Delete`/`UpdateResource`,
  `CancelResourceRequest`, and the `ProgressEvent` half of
  `GetResourceRequestStatus`) each independently confirmed to wrap the
  single key `ProgressEvent` (`deserializers.go:3149,3185,3221,3302,3442`);
  `GetResourceRequestStatusOutput` additionally carries `HooksProgressEvent`
  (`deserializers.go:3302`), a flat JSON array of `HookProgressEvent`
  (`deserializers.go` `awsAwsjson10_deserializeDocumentHooksProgressEvent`),
  each of whose 8 members (`FailureMode`, `HookEventTime`, `HookStatus`,
  `HookStatusMessage`, `HookTypeArn`, `HookTypeName`, `HookTypeVersionId`,
  `InvocationPoint`) gopherstack's `hookProgressEvent` (handler.go) models
  exactly, field-for-field.
- `ResourceDescription.Properties` (`types/types.go`, confirmed a `*string`)
  and `ProgressEvent.ResourceModel` / `UpdateResourceInput.PatchDocument` /
  `CreateResourceInput.DesiredState` are all JSON **strings** on the wire,
  never decoded objects -- matches gopherstack throughout
  (`resourceDescription.Properties string`, `ProgressEvent.ResourceModel
  string` in models.go/handler.go).
- `ProgressEvent`'s full 12-member field list
  (`ErrorCode, EventTime, HooksRequestToken, Identifier, Operation,
  OperationStatus, RequestToken, ResourceModel, RetryAfter, StatusMessage,
  TypeName` plus the implicit `noSmithyDocumentSerde`) verified against
  `types/types.go` and its deserializer (`deserializers.go`
  `awsAwsjson10_deserializeDocumentProgressEvent`) -- all 11 real data
  members present on gopherstack's `ProgressEvent` (models.go), correct
  types (`EventTime`/`RetryAfter` epoch-seconds JSON numbers via
  `unixEpochTime`, everything else a string).
- Enums checked both directions. `HandlerErrorCode` (16 values, `types/
  enums.go`) -- all 16 representable as plain strings on gopherstack's
  `ProgressEvent.ErrorCode string` field (never populated today since no op
  leaves a request FAILED, but the field itself can carry any of them).
  `Operation` (`CREATE`/`DELETE`/`UPDATE`) and `OperationStatus` (`PENDING`/
  `IN_PROGRESS`/`SUCCESS`/`FAILED`/`CANCEL_IN_PROGRESS`/`CANCEL_COMPLETE`)
  both closed 3- and 6-value Smithy enums, matching
  `validOperations`/`validOperationStatuses` (resource_requests.go) exactly
  in both directions. **At this pinned SDK version, `HookStatus`,
  `HookInvocationPoint`, and `HookFailureMode` are NOT distinct Go enum
  types** -- `types.HookProgressEvent`'s `FailureMode`, `HookStatus`, and
  `InvocationPoint` fields are all plain `*string` (`types/types.go`); the
  task brief's framing of these as enums to check "both directions" does not
  match the pinned SDK, so there is no enum type to validate values against
  on the Go side. Noted as a brief/SDK mismatch, not a bug.
- Round-trip proof: added `wire_sdk_roundtrip_test.go`
  (`TestCloudControl_SDKRoundTrip`,
  `TestCloudControl_CancelResourceRequest_SDKRoundTrip`), which drive all 8
  ops through the real `aws-sdk-go-v2/service/cloudcontrol` client against
  this package's `Handler` via `pkgs/service`'s router (same pattern as
  `services/dax/wire_sdk_roundtrip_test.go`) -- both pass, proving every
  wrapper key and JSON-string field against the SDK's own generated
  deserializer, not gopherstack's own struct tags agreeing with themselves.
- Existing `handler_sdk_route_table_test.go` (`X-Amz-Target` routing) and all
  other existing tests read and re-verified; none assert a wrong
  key/nesting/type/value. Nothing corrected.

**Provenance note**: `last_audit_commit` had been stuck at `0689b86e`
(2026-07-13) across three successive audit passes that each *did* update
`last_audit_date` (2026-07-13 -> 2026-07-24 -> 2026-07-26) but never
advanced the commit pointer to the commit that actually landed that pass's
work -- a genuine internal self-contradiction (the file's own "re-audit
protocol" comment says to diff from `last_audit_commit`, which would have
replayed all three later passes' own changes as unreviewed drift). This
matches the flagged pattern (gap of days-to-weeks, cited sha predating the
claimed date) and is one of the 53 manifests a prior survey flagged for
exactly this. The underlying audit *content* was accurate on inspection
(confirmed field-by-field above) -- only the provenance stamp was stale.
Both `last_audit_commit` and `last_audit_date` refreshed to this pass's HEAD
(`569c029d`, 2026-08-20) above.

Protocol: awsjson1.0 (`application/x-amz-json-1.0`, `X-Amz-Target: CloudApiService.<Op>`).
Confirmed against the real SDK client package (target prefix, content-type, error envelope
`{"__type": "...", "message": "..."}`).

**Every op completes synchronously to a terminal status** (SUCCESS on Create/Update/Delete,
CANCEL_COMPLETE on Cancel). This is a deliberate, parity-acceptable design choice per the task
brief ("EITHER can be parity-correct") -- there is no PENDING/IN_PROGRESS hang risk because
GetResourceRequestStatus and ListResourceRequests read the same `requests` store.Table that
CreateResource/UpdateResource/DeleteResource just wrote to in the same call. The only way to
observe an IN_PROGRESS event is the test-only `AddProgressEvent` helper, used to exercise the
CancelResourceRequest "only PENDING/IN_PROGRESS is cancellable" rule.

**Fixed this pass (2026-07-26, bd issue gopherstack-c9yf)**:

- `ListResources`' `ResourceModel` field ("The resource model to use to select the resources
  to return", confirmed real on `ListResourcesInput` in `aws-sdk-go-v2/service/cloudcontrol/
  types/types.go` and its serializer, `serializers.go:702-704` -- a plain JSON string field,
  same convention as `DesiredState`/`PatchDocument`) is now decoded and applied as a real
  filter: `InMemoryBackend.ListResources` takes a `resourceModel` JSON-object string and
  `matchesResourceModel` requires every key/value pair in it to match the corresponding key in
  the resource's current `Properties`. An unparseable `ResourceModel` matches nothing (fails
  closed) rather than erroring or being silently ignored. Previously this field was not even
  present on the local decode struct, so it degraded to accepted-and-ignored -- a real gap now
  closed with a generic, non-fabricated match (no per-type schema knowledge needed: the filter
  works identically for any resource type since it only compares against propertydata this
  backend already stores).
- `ClientTokenConflictException` is now detected for real (previously deferred as "would
  require persisting and diffing the full original request, out of scope" -- re-triaged this
  pass and judged not out of scope, since the fingerprint needed is trivial: the op name plus
  the exact fields already being passed to Create/Update/DeleteResource). Each `ClientToken`'s
  cache entry (`clientTokenEntry`) now stores a deterministic fingerprint of the original
  request (`op + TypeName + Identifier + DesiredState/PatchDocument`) alongside the cached
  `RequestToken`. A replay with a matching fingerprint still idempotently returns the cached
  `ProgressEvent` (unchanged behavior); a replay with a *different* fingerprint (same token,
  genuinely different request) now returns `ClientTokenConflictException`/HTTP 400 (confirmed
  `ErrorFault() == smithy.FaultClient` on the real SDK's
  `types.ClientTokenConflictException`, so 400 not 500, matching every other client fault in
  this handler). `clientTokens`' persisted value type changed from a bare `string` to
  `clientTokenEntry{RequestToken, Fingerprint}`, so `cloudcontrolSnapshotVersion` bumped 1->2;
  an old (v1) snapshot is discarded cleanly by the existing version-mismatch path in
  `Restore`, exactly like the v0 (no-persistence-at-all) case before it -- never partially
  decoded as the new shape.
- `TypeNotFoundException` was re-triaged and confirmed GENUINELY IMPOSSIBLE without
  fabrication, not merely deferred -- see the `gaps` entry above for the full reasoning
  (no third design option between "arbitrary incomplete allowlist that regresses previously-
  accepted TypeNames" and "accept everything", which is already the correct, honest behavior).
- The remaining 12 previously-listed "unreachable without chaos" exceptions
  (`ThrottlingException`, `ServiceLimitExceededException`, `HandlerFailureException`,
  `NotStabilizedException`, `NotUpdatableException`, `ResourceConflictException`,
  `PrivateTypeException`, `GeneralServiceException`, `NetworkFailureException`,
  `InvalidCredentialsException`, `HandlerInternalFailureException`,
  `ConcurrentOperationException`) were reclassified from "deferred" to "ALREADY COVERED by
  chaos fault injection" -- see `chaos_coverage` above for the concrete verification (Handler
  implements `service.ChaosProvider`, chaos middleware runs globally ahead of this handler and
  resolves the exact operation via the same `X-Amz-Target` header this service's own routing
  uses, `FaultError` carries an arbitrary injected `Code`+`StatusCode`). These are correctly
  NOT implemented as backend logic: synthesizing them from this emulator's single coarse lock
  with no real concurrency/quota/IAM/network state would itself be fabrication.

**Fixed this pass (2026-07-24)**:

- `ProgressEvent` gained `ErrorCode`, `HooksRequestToken`, `RetryAfter` (all real fields on
  `types.ProgressEvent`, confirmed against `aws-sdk-go-v2/service/cloudcontrol/types/types.go`
  and its deserializer -- `RetryAfter` is epoch-seconds like `EventTime`, not an ISO8601
  string). All three are always empty/omitted today since this backend never leaves a request
  non-terminal or FAILED -- modeled for wire-shape parity, not gold-plated behavior.
- `GetResourceRequestStatusOutput` gained `HooksProgressEvent` (real field on the real output
  shape); always empty/omitted since this backend has no Hooks concept.
- **Deleted an invented field**: `ResourceRequestStatusFilter.TypeName`. Confirmed absent from
  both the aws-sdk-go-v2 types package and botocore's `service-2.json` -- the real filter has
  only `Operations` and `OperationStatuses`. The prior implementation filtered
  `ListResourceRequests` results by it, a genuine wire-shape bug (narrower results than real AWS
  would ever return for an equivalent filter body). `TestHandler_ListResourceRequests_TypeNameFilter`
  (which asserted the old, wrong filtering behavior) was rewritten as
  `TestHandler_ListResourceRequests_TypeNameFilterIsIgnored` to assert the correct (no-op)
  behavior.
- `CreateResource` now rejects a missing/empty `DesiredState` with `InvalidRequestException` --
  `DesiredState` is "This member is required" on the real `CreateResourceInput`; previously
  silently accepted and created a resource with empty `Properties`.
- `UpdateResource` now rejects a missing/empty `PatchDocument` with `InvalidRequestException` --
  `PatchDocument` is "This member is required" on the real `UpdateResourceInput`; previously
  silently accepted and `applyPatch` no-op'd on the unparseable empty string instead of the
  request being rejected.
- `UpdateResource`/`DeleteResource` now accept and honor `ClientToken` for idempotency (real
  `UpdateResourceInput.ClientToken` / `DeleteResourceInput.ClientToken` were previously entirely
  absent from gopherstack's input structs -- accepted-and-dropped on the wire, not merely
  unused). Shared `cachedEventForToken`/`rememberClientToken` helpers factor the now-3x logic
  that used to live only in `CreateResource`.
- `ListResources`/`ListAllResources` now return defensive copies of `*Resource` instead of the
  live pointers held inside the backend's `store.Table` (`pkgs/store.Table.Range`/`All` perform
  no copying themselves -- that is documented as the owning backend's responsibility). Every
  other accessor (`GetResource`, the `ProgressEvent` returned by Create/Update/DeleteResource)
  already copied; this closes the one remaining hole where a caller could mutate backend state
  without holding the lock, bypassing `UpdateResource`'s own patch semantics entirely.

**Error-code bugs fixed in a prior pass, still correct, don't re-flag**:

- `ValidationException` does not exist anywhere in CloudControl's error model (verified: absent
  from botocore's `service-2.json` shapes). Every operation instead declares
  `InvalidRequestException` ("invalid input from the user has generated a generic exception")
  as the generic input-validation error.
- `GetResourceRequestStatus` declares **only** `RequestTokenNotFoundException` as an error --
  an unrecognized RequestToken must not surface as `ResourceNotFoundException`.
  `CancelResourceRequest` declares the same plus `ConcurrentModificationException`.
- `CancelResourceRequest` on a non-PENDING/non-IN_PROGRESS request returns
  `ConcurrentModificationException` (HTTP 500), not a client validation error.
- HTTP status codes: per the live API reference, virtually every CloudControl client-fault
  exception (`ResourceNotFoundException`, `AlreadyExistsException`, `InvalidRequestException`,
  `RequestTokenNotFoundException`, etc.) is **HTTP 400**. Only a handful of server-fault
  exceptions (`ConcurrentModificationException`, `ServiceInternalErrorException`, etc.) are
  HTTP 500.

**This pass's field-diff method**: `aws-sdk-go-v2/service/cloudcontrol@v1.29.15` was already
present in the module cache (`go env GOMODCACHE`), so every `types.*` struct, every
`api_op_*.go` Input/Output struct, and the `awsAwsjson10_deserializeDocumentProgressEvent`
generated deserializer were read directly -- not inferred from documentation prose. The
`ResourceRequestStatusFilter.TypeName` deletion was additionally cross-checked against
botocore's `service-2.json` (gunzipped from the installed `botocore` package) to rule out a
version-lag false positive in the Go SDK snapshot; both sources agree the field does not exist.

**Traps for the next auditor** (already correct, don't re-flag):

- `Properties`/`DesiredState`/`PatchDocument`/`ResourceModel` are all JSON **strings**, never
  decoded objects, on the wire -- confirmed correct throughout.
- `EventTime` and `RetryAfter` are epoch-seconds as a JSON **number** via the local
  `unixEpochTime` wrapper, not a timestamp string -- correct, and covered by
  `TestHandler_EventTimeIsUnixNumber`. `RetryAfter` is `*unixEpochTime` (nullable) since real
  AWS only ever sets it on a non-terminal PENDING/IN_PROGRESS status, which this backend never
  produces -- always nil/omitted today, that is correct, not a bug.
- `identifierKeys` in resources.go is a best-effort heuristic (no CFN schema registry backs this
  emulator), documented inline with the specific resource types each key maps to. This is an
  intentional simplification, not a bug: real CloudControl derives the identifier from the
  resource type's schema-declared `primaryIdentifier`, which isn't tracked here.
- The backend is a single self-contained generic store, not a fan-out to per-service backends
  (see gaps above) -- this was independently verified against the task brief's explicit
  either-is-acceptable framing, not overlooked.
- `RoleArn`/`TypeVersionId` are real input members on every mutating op's `*Input` shape but are
  not modeled on gopherstack's decode structs. This is NOT a wire bug: `encoding/json` silently
  ignores unrecognized object members on decode, so a real client sending these fields degrades
  gracefully (accepted-and-ignored) rather than erroring. Not worth the struct-field noise across
  every op given neither field changes response shape or observable behavior in this emulator
  (no IAM role assumption, no private-type-version registry).
