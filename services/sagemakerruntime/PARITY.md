---
service: sagemakerruntime
sdk_module: aws-sdk-go-v2/service/sagemakerruntime@v1.43.4
last_audit_commit: 73dccf417
last_audit_date: 2026-09-04
overall: A            # fixed the one open gap: InvokeEndpointAsync now rejects Body/InputLocation both-or-neither with ValidationError, matching the real API's "provide exactly one of them" constraint (unenforceable client-side, so it must be a server check)
ops:
  InvokeEndpoint: {wire: ok, errors: ok, state: ok, persist: n/a, note: "sync op; EndpointName is now validated against the wired services/sagemaker endpoint registry (existence + InService); NewSessionId's Expires= attribute now matches the SDK's RFC-3339 wire format; ClosedSessionId is now emitted when an expired session is touched. body is an opaque mock, other headers round-trip correctly"}
  InvokeEndpointAsync: {wire: ok, errors: ok, state: ok, persist: ok, note: "returns InferenceId (JSON body)/OutputLocation/FailureLocation headers correctly; EndpointName now validated like the other two ops; Body and InputLocation are now enforced as mutually exclusive/exactly-one-of (handler.go's handleInvokeEndpointAsync), matching api_op_InvokeEndpointAsync.go's doc comment on InvokeEndpointAsyncInput.Body -- previously neither, or both, was silently accepted"}
  InvokeEndpointWithResponseStream: {wire: ok, errors: ok, state: ok, persist: n/a, note: "event-stream framing (prelude/header/payload/CRC) verified against smithy-go wire format; EndpointName now validated; SessionId only touches, never creates (per SDK doc: sessions can't be created via this op), and InvokeEndpointWithResponseStreamOutput has no ClosedSessionId member so expiry-driven closure is a side effect only here, never surfaced on this response"}
families:
  sessions: {status: ok, note: "NEW_SESSION creation, FIFO eviction (maxSessions=1000), TouchSession on subsequent calls, ExpiresAt now enforced (session past its ExpiresAt is evicted and reported via ClosedSessionId on InvokeEndpoint; see SessionTouchOutcome) -- all covered."}
  invocation_history: {status: ok, note: "bounded FIFO (maxInvocationHistory=1000), persisted."}
  endpoint_validation: {status: ok, note: "EndpointLookup (endpoint_lookup.go) is a minimal interface satisfied directly by *sagemaker.InMemoryBackend's exported DescribeEndpoint method; wired at Provider.Init via wireEndpointLookup (provider.go), following the services/cloudwatchlogs/provider.go s3HandlerProvider precedent -- no change to services/sagemaker was needed, since DescribeEndpoint was already an exported, lock-safe read accessor. Unknown EndpointName and known-but-not-InService both surface real AWS's 'Endpoint <name> of account <account> not found.' ValidationError message (confirmed against real-world AWS error reports: an endpoint still Creating is reported as not-found from InvokeEndpoint's perspective too, since the runtime routing table only serves InService endpoints). When no lookup is wired (bare NewInMemoryBackend, e.g. every pre-existing test in this package), validation is a no-op, preserving standalone behaviour."}
gaps: []
deferred: []
leaks: {status: clean, note: "sessions/asyncInvocations/invocations are all FIFO-capped (maxSessions/maxAsyncInvocations/maxInvocationHistory=1000); no goroutines, no janitor (Shutdown is a documented no-op). New endpointLookup field is a plain interface reference (no goroutine, no owned resource); SetEndpointLookup/validateEndpoint both take/release the backend's own lock before calling out to the (separately-locked) sagemaker backend, so no lock is held across the cross-service call and no lock-ordering cycle is introduced."}
---

## Notes

Protocol: restjson1. Three ops, disambiguated purely by path suffix (no
X-Amz-Target header): `/endpoints/{EndpointName}/invocations`,
`/endpoints/{EndpointName}/async-invocations`,
`/endpoints/{EndpointName}/invocations-response-stream`. EndpointName is a
path segment (`extractEndpointName` cuts on the first `/` after the
`/endpoints/` prefix), never a query param or header.

**Bugs found and fixed this pass (2026-07-24; see git diff for exact lines):**

1. **EndpointName existence/InService was never validated.** Real AWS returns
   `ValidationError` ("Endpoint <name> of account <account> not found.") for
   both an unknown EndpointName and one that has not yet reached InService
   (confirmed against real-world AWS error reports: an endpoint still
   `Creating` is reported the same way, since the runtime's routing table
   only serves InService endpoints). The previous audit correctly identified
   this as a genuine gap but deferred it as requiring cross-service backend
   wiring "out of scope for a same-service-only pass." This pass wired it:
   `endpoint_lookup.go` defines a minimal `EndpointLookup` interface
   (`DescribeEndpoint(ctx, name) (*sagemaker.Endpoint, error)`), already
   satisfied by `*sagemaker.InMemoryBackend` with zero changes to
   `services/sagemaker` (its `DescribeEndpoint` was already an exported,
   lock-safe read accessor). `provider.go`'s `wireEndpointLookup` connects
   the two at `Provider.Init` time via a private `sagemakerHandlerProvider`
   interface type-asserted against `ctx.Config`, following the exact
   precedent of `services/cloudwatchlogs/provider.go`'s
   `s3HandlerProvider`/`wireExportSink` (NOT the much larger
   `services/cloudformation`-style `BackendsProvider`, which would have been
   overkill for a single dependency). When unwired (every pre-existing test
   in this package constructs a bare `NewInMemoryBackend`), validation is a
   no-op, preserving prior behaviour for standalone use.

2. **`NewSessionId`'s `Expires=` attribute used the wrong wire format.**
   `handler.go` formatted it as an RFC 1123 HTTP-date
   (`"Mon, 02 Jan 2006 15:04:05 GMT"`), but the SDK model's
   `NewSessionResponseHeader` shape declares the pattern
   `^[a-zA-Z0-9](-*[a-zA-Z0-9])*;\sExpires=[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`
   (confirmed against botocore's `sagemaker-runtime` `service-2.json`) --
   i.e. an RFC 3339 timestamp with no fractional seconds. A client validating
   or parsing the header against that pattern would have rejected
   gopherstack's previous output. Fixed to
   `session.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z")`; see
   `TestNewSessionHeaderFormat_MatchesSDKPattern`.

3. **`ClosedSessionId` was never emitted; session `ExpiresAt` was tracked but
   never enforced.** `TouchSession` now returns a `SessionTouchOutcome`: if
   the touched session has passed its `ExpiresAt`, it is evicted and
   `ClosedSessionID` is set, which `InvokeEndpoint` surfaces as the
   `X-Amzn-SageMaker-Closed-Session-Id` response header (matching
   `InvokeEndpointOutput.ClosedSessionId`). `InvokeEndpointWithResponseStream`
   calls `TouchSession` too but discards the outcome, since
   `InvokeEndpointWithResponseStreamOutput` has no `ClosedSessionId` member
   in the SDK model -- only `InvokeEndpointOutput` does. A test-only
   `ExpireSessionForTest` helper (`export_test.go`, following
   `services/sagemaker/export_test.go`'s precedent) forces a session's
   `ExpiresAt` into the past deterministically, since AWS provides no real
   API to trigger session closure (it's entirely model/container-driven).

**Traps for the next auditor (looks-wrong-but-correct):**

- `handleInvokeEndpointAsync` accepts a client-supplied
  `X-Amzn-Sagemaker-Outputlocation` *request* header and uses it verbatim. This
  header does **not** exist in the real `InvokeEndpointAsyncInput` wire shape
  (checked `serializers.go`'s `awsRestjson1_serializeOpHttpBindingsInvokeEndpointAsyncInput`:
  the only headers bound are Accept/Content-Type/CustomAttributes/Filename/
  InferenceId/InputLocation/InvocationTimeoutSeconds/RequestTTLSeconds/
  S3OutputPathExtension). It's a harmless gopherstack-only testing backdoor: real
  SDK callers never send it, so they always take the "generate a fake S3 path"
  branch. Not a parity bug, just don't mistake it for a real SDK field.
- `X-Amzn-Sagemaker-*` header constants use lowercase "maker" in the Go source
  (e.g. `headerCustomAttributes = "X-Amzn-Sagemaker-Custom-Attributes"`) instead
  of the docs' `X-Amzn-SageMaker-*` casing. This is **not** a bug: Go's
  `net/http` canonicalizes every header name via
  `textproto.CanonicalMIMEHeaderKey` on both `Set` and `Get`, which title-cases
  each hyphen-separated segment (`SageMaker` -> `Sagemaker`) regardless of the
  literal casing used in source -- so the wire bytes and the client's `Header.
  Get` lookups are unaffected either way. Confirmed by cross-referencing
  `deserializers.go`'s literal `response.Header.Values("X-Amzn-SageMaker-...")`
  calls, which resolve to the same canonical key. This also holds for
  botocore's `service-2.json`, which even declares `InvokeEndpointOutput.
  InvokedProductionVariant`'s `locationName` as `"x-Amzn-Invoked-Production-
  Variant"` (lowercase leading `x`) -- same reasoning, same non-issue.
- `InvokeEndpointWithResponseStream`'s `SessionId` header is only ever passed to
  `TouchSession`, never `StartSession` -- this is correct: the SDK doc for
  `InvokeEndpointWithResponseStreamInput.SessionId` explicitly states "You can't
  create a stateful session by using the InvokeEndpointWithResponseStream action."
- `pathToOperation`'s default `"Unknown"` branch (unmatched path suffix under
  `/endpoints/`) returns a non-SDK `"UnknownOperationException"` __type. Left
  as-is: unreachable by any real SDK call (the SDK only ever generates the three
  known suffixes), so it's defensive-only code, not a wire-parity concern.
- `TargetModel`/`TargetContainerHostname`/`EnableExplanations`/
  `InferenceComponentName` request headers (confirmed against
  `service-2.json`: `X-Amzn-SageMaker-Target-Model`,
  `X-Amzn-SageMaker-Target-Container-Hostname`,
  `X-Amzn-SageMaker-Enable-Explanations`,
  `X-Amzn-SageMaker-Inference-Component`) are intentionally accepted but not
  consumed by the handler: none of them appear in any operation's *output*
  shape, so there is nothing wire-visible to get wrong by ignoring them --
  they only affect real model-container routing/behaviour, which this
  emulator does not simulate (deterministic synthetic responses are the
  documented parity bar for this service).
- `InvokeEndpointWithResponseStreamInput.ContentType` is bound to the plain
  `Content-Type` header (same as the sync op), while its sibling `Accept` is
  bound to `X-Amzn-SageMaker-Accept` -- an intentional asymmetry in the real
  SDK model, not a copy-paste error to "fix": the *output* side is
  asymmetric too (`InvokeEndpointWithResponseStreamOutput.ContentType` binds
  to `X-Amzn-SageMaker-Content-Type`, unlike the sync op's plain
  `Content-Type` output). Confirmed against `service-2.json`; `handler.go`'s
  `headerAsyncAccept`/`headerStreamContentType` constants already matched
  this correctly before this pass.
- `ModelError`/`ModelNotReadyException`/`ServiceUnavailable`/
  `InternalDependencyException`/`ModelStreamError`/`InternalStreamFailure`
  error paths have no request-side trigger (there is no real model to
  fail). `InternalStreamFailure`/`ModelStreamError` additionally have no
  `httpStatusCode` in `service-2.json`'s `error` trait at all (unlike the
  other six, which map to 530/500/424/429/503/400 respectively) --
  confirmed they are delivered as in-band event-stream exception events, not
  top-level HTTP error responses, which is why they're absent from the
  status-code table. Chaos-injection (ChaosServiceName/ChaosOperations/
  ChaosRegions) is wired at the framework level (pkgs/chaos) and can inject
  arbitrary faults independent of handler code, so this is not a gap in the
  handler itself -- just no organic way to hit these paths without chaos
  rules configured.

**Verified correct without changes:** `InvokeEndpoint` request/response header
binding (Accept/ContentType negotiation, CustomAttributes echo,
X-Amzn-Invoked-Production-Variant default `"AllTraffic"` / target-variant
override), the event-stream binary frame format for
`InvokeEndpointWithResponseStream` (prelude length/headers-length/CRC32,
`:message-type`/`:event-type`/`:content-type` headers, payload-only
`PayloadPart.Bytes`), `InvokeEndpointAsyncOutput`'s wire shape
(`InferenceId` is a plain JSON body field with no `location` binding in the
SDK model -- unlike `OutputLocation`/`FailureLocation`, which are
header-bound -- and gopherstack's JSON-body-plus-headers response shape
matches exactly), FIFO eviction bounds for sessions/async-invocations/
invocation-history, and `Handler.Snapshot`/`Restore` delegation to the backend
(all exercised by `persistence_test.go`).

## 2026-08-20 wrapper-key/nested-shape sweep

Re-derived every wire shape from scratch against the pinned
`aws-sdk-go-v2/service/sagemakerruntime@v1.43.4` source
(`serializers.go`/`deserializers.go`/`api_op_*.go`/`types/errors.go`),
independent of this file's prior claims. **Zero bugs found.** This service
is structurally almost immune to the wrapper-key/nested-shape bug class the
rest of this campaign hunts: all three ops are pure HTTP header/URI binding
plus an opaque `[]byte` payload -- `InvokeEndpointInput`/`Output`,
`InvokeEndpointAsyncInput`/`Output`, and
`InvokeEndpointWithResponseStreamInput`/`Output` have no nested structs, no
lists, no maps, and (`ModelError`'s three extra scalar fields aside) no
non-flat error shapes to mis-nest a key under.

Verified line-for-line against the SDK source, all matching gopherstack's
existing implementation exactly (see file:line citations already in this
document from the 2026-07-24 pass, re-confirmed this pass):

- Every request header binding, all three ops
  (`serializers.go:82-149,215-273,343-402`), including the
  `InvokeEndpointWithResponseStream`/`InvokeEndpoint` asymmetries
  (`ContentType` on plain `Content-Type` vs `X-Amzn-Sagemaker-Accept` for
  `Accept`; `TargetModel`/`EnableExplanations` present on `InvokeEndpoint`
  only, absent from the stream op's input).
- Every response header binding, all three ops
  (`deserializers.go:141-166,317-333,483-497`).
- Each op's own error switch is a **distinct set**, not shared across ops:
  `InvokeEndpoint` = InternalDependencyException/InternalFailure/
  ModelError/ModelNotReadyException/ServiceUnavailable/ValidationError
  (`deserializers.go:113-131`); `InvokeEndpointAsync` = InternalFailure/
  ServiceUnavailable/ValidationError only, no ModelError
  (`deserializers.go:296-306`); `InvokeEndpointWithResponseStream` =
  InternalFailure/InternalStreamFailure/ModelError/ModelStreamError/
  ServiceUnavailable/ValidationError, no InternalDependencyException, no
  ModelNotReadyException (`deserializers.go:451-471`). gopherstack never
  hand-codes these switches (mock-only synchronous success path), so there
  is nothing to mismatch, but the sets themselves are worth recording so a
  future auditor doesn't assume they're identical.
- `ModelError`'s extra members (`OriginalStatusCode *int32`,
  `OriginalMessage *string`, `LogStreamArn *string`) and `ModelStreamError`'s
  (`ErrorCode_ *string`) confirmed against `types/errors.go` -- unreachable
  organically (no real model container to fail), same as the 2026-07-24
  finding.
- `X-Amzn-ErrorType` is never set by `handler.go`'s `errorResponse` path
  (checked: only a JSON body with `__type`/`message` is written). Verified
  this is **not** a bug here (unlike the mediastoredata precedent this
  campaign flagged): the SDK's `awsRestjson1_deserializeOpErrorInvoke*`
  functions call `restjson.GetErrorInfo`, which falls back to the body's
  `__type` field whenever the header is absent
  (`aws-sdk-go-v2@v1.43.4/aws/protocol/restjson/decoder_util.go:15-46`), and
  `errorResponse`'s `{"__type": code, "message": msg}` shape is exactly what
  that fallback expects -- confirmed a client still gets the correct typed
  exception (e.g. `types.ValidationError`) without the header.
- `InvokeEndpointAsyncInput`'s `Body []byte` member (the mutual-exclusivity
  gap flagged 2026-08-11) re-confirmed present and unenforced; not fixed
  this pass either -- it's an unenforced-validation gap, not one of the five
  wrapper-key/nesting bug shapes this sweep targets, and was already
  correctly disclosed.

**New this pass:** added `TestSDKEventStreamFraming_RealReader`
(`invoke_endpoint_stream_test.go`) -- drives
`InvokeEndpointWithResponseStream` through the real SDK client and reads the
response with the SDK's own `GetStream().Events()` reader (not hand-parsed
bytes), asserting every event decodes to
`*types.ResponseStreamMemberPayloadPart` (never `UnknownUnionMember`) with
the expected `Bytes` content. The pre-existing `TestSDKResponseBindings`
only asserted `GetStream().Close()` didn't error, which doesn't prove frame
correctness (a truncated/malformed frame can still close cleanly without
being read). Proved this test is not vacuous by hand-corrupting
`eventStreamHeaderValueTypeString` from `7` to `3` (via the scratchpad
`cp`-based hand-revert method): the new test fails with "unexpected EOF"
while the pre-existing stream tests still pass, confirming only the new
test actually exercises frame-level correctness.

**Provenance:** the prior stamp (`last_audit_commit: 95ab0584`,
`last_audit_date: 2026-07-24`) checked out: `git show -s --format=%ad
95ab0584` -> `Mon Jul 13 10:54:25 2026 -0500`, an 11-day gap before the
claimed audit date, which per this campaign's provenance heuristic
("sha predates date" is the tell) is suspicious -- though note that commit
is unrelated to this service (a `dlm` fix), consistent with this campaign's
observation that commit-doesn't-touch-the-directory is not itself
disqualifying. Separately, the 2026-08-11 SDK pin-correction pass
(`d39bf33e4`, bumped `v1.39.3` -> `v1.43.4` and added the `Body`-field gap
entry) touched this file's content but did **not** advance
`last_audit_commit`/`last_audit_date` -- the stamp had gone stale relative
to real work already done on the file. Both are refreshed by this pass to
current HEAD (`914e8b59`, 2026-08-20).

Gates: `go build`, `go vet`, `go fix -diff` (empty), `gofmt -l` (empty),
`go test -race` (all green, including the new round-trip test),
`golangci-lint run` (0 issues). `git status --short` outside this service
directory: clean.

## 2026-09-04 diff-scoped re-audit and gap fix

Diffed `914e8b59..HEAD` for this directory rather than re-deriving from
scratch: only test-only churn (an unrelated `services/sagemaker`
`CreateEndpointFSM` signature update in `endpoint_validation_test.go`, the
repo-wide mechanical `iam_enforcement_test.go` IAM-enforcement harness, and a
regenerated `README.md`), plus a `PARITY.md` stamp bump that lands the
2026-08-20 wrapper-key-sweep content -- no production wire-shape code
changed since the last real audit. Confirmed `914e8b59` itself is not an
ancestor of this branch's HEAD (squash-merge history rewrite, per this
repo's own documented `Closes`-trailer-loss behaviour) but its tree content
matches what this file already describes, so nothing was missed by treating
it as the baseline.

**Fixed the one disclosed-but-unfixed gap:** `InvokeEndpointAsyncInput.Body`'s
doc comment (`api_op_InvokeEndpointAsync.go`) states "Body and InputLocation
are mutually exclusive. Provide exactly one of them," and
`validateOpInvokeEndpointAsyncInput` (`validators.go:84-97`) confirms this is
**not** a client-side check -- only `EndpointName` is required there -- so a
real client can construct and send a request with neither or both, meaning
this must be a server-side validation in real AWS. `handleInvokeEndpointAsync`
(`handler.go`) previously read the raw HTTP body unconditionally and never
read the `X-Amzn-Sagemaker-Inputlocation` header at all (confirmed via
`serializers.go`'s `awsRestjson1_serializeOpHttpBindingsInvokeEndpointAsyncInput`,
which binds `InputLocation` to that exact header) -- so a request with
neither field, or with both, was silently accepted with 202 Accepted. Fixed:
`handleInvokeEndpointAsync` now compares `len(body) > 0` against the new
`X-Amzn-Sagemaker-Inputlocation` header's presence and returns `ValidationError`
(400) when they're equal (both true or both false). Added
`headerInputLocation` header constant. Regression test
`TestAsyncInvocation_BodyInputLocationMutualExclusion`
(`invoke_endpoint_async_test.go`) covers all four combinations; proved
non-vacuous by neutering the guard (`if (len(body) > 0) == hasInputLocation`
-> `if false && ...`), confirming exactly the two reject cases fail (202
instead of 400) while the two accept cases still pass, then restoring from a
backup copy. Updated `TestHandler_EndpointValidation`'s async-invocations
subtests and `TestAsyncInvocationInferenceIDPreserved` (both previously sent
neither field, relying on the bug) to send a body instead, since their intent
is EndpointName/InferenceId behaviour, not this constraint.

**Five dimensions:**
1. AWS behavior compliance: fixed the Body/InputLocation gap above; everything
   else re-confirmed unchanged against the pinned SDK (no other wire-shape
   code changed since 914e8b59).
2. LocalStack parity: NOT CHECKED this pass (no LocalStack instance
   available in this environment; prior passes found no LocalStack-specific
   divergence to compare against either).
3. Cross-service integration: re-confirmed clean -- `validateEndpoint`
   correctly rejects an EndpointName the wired `services/sagemaker` registry
   doesn't know about, or hasn't reached `InService`
   (`TestHandler_EndpointValidation`, `TestProvider_WiresSageMakerEndpointLookup`).
   The new IAM-enforcement test (`iam_enforcement_test.go`, added by an
   unrelated repo-wide campaign since the last audit) was read in full and
   correctly maps `POST /endpoints/{name}/invocations` -> denied without
   `sagemaker-runtime:InvokeEndpoint` -- not specific to this service, so not
   re-derived from the SDK independently.
4. Performance: NOT CHECKED beyond what the 2026-08-20 pass already recorded
   (no hot-path code changed this pass); no new `sort.Slice` or O(n)
   under-lock scans introduced or found (`grep -n 'sort\.\(Slice\|SliceStable\)'`
   across the package: zero hits).
5. Resource leaks: re-confirmed clean -- `grep -n 'go func'` across the
   package: zero hits (no goroutines in this service at all); FIFO caps on
   sessions/asyncInvocations/invocations unchanged.

Baseline (before this pass's fix): `golangci-lint run` 0 issues,
`go test -race -count=1` all green -- i.e. the Body/InputLocation gap was a
genuine silent-accept bug, not something already caught by an existing test.
Gates after the fix: `go build`, `go vet`, `gofmt -l` (empty),
`go test -race -count=1` (all green, including the new regression test),
`golangci-lint run` (0 issues, after `--fix` reordered the new test table's
struct fields for `fieldalignment`).
