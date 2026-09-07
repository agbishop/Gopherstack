---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: bedrockruntime
sdk_module: aws-sdk-go-v2/service/bedrockruntime@v1.57.1   # unchanged this pass; re-verified against go.mod pin
last_audit_commit: 861de3270 # set on commit -- 2026-09-04 parity sweep, see below
last_audit_date: 2026-09-04
overall: A            # 2026-09-04: fixed two real bugs. (1) ListAsyncInvokes silently ignored
                      # submitTimeAfter/submitTimeBefore/sortOrder (ListAsyncInvokesInput fields,
                      # httpQuery-bound in serializers.go's
                      # awsRestjson1_serializeOpHttpBindingsListAsyncInvokesInput) -- every request matched
                      # every invocation and always returned ascending order regardless of what the caller
                      # asked for. Now parsed (parseListAsyncInvokesFilter, handler_async_invoke.go) and
                      # applied (matchesAsyncInvokeFilter/descending sort, async_invoke.go). Proven via
                      # TestHandler_ListAsyncInvokes_SubmitTimeFilters and
                      # TestHandler_ListAsyncInvokes_SortOrder (hand-neutered/confirmed-failing/restored).
                      # (2) Handler.StartWorker hardcoded a 1-hour janitor interval while
                      # defaultAsyncInvokeCompletionDelay (janitor.go) is 5s -- in a real running server (not
                      # AdvanceAsyncInvokesForTest-driven tests), an InProgress invocation could sit
                      # unadvanced for up to an hour instead of ~5s, unlike sibling services/bedrock's
                      # janitor, which matches its interval to its own completion delay. Fixed:
                      # StartWorker now passes defaultAsyncInvokeCompletionDelay as the interval. Proven via
                      # TestStartWorker_AdvancesAsyncInvoke_NearCompletionDelay (synctest, hand-neutered to
                      # time.Hour/confirmed-failing/restored). Grade held at A.
                      # 2026-08-20: fixed ApplyGuardrailOutput's top-level Action -- gopherstack sent the fabricated
                      # enum value "BLOCKED" where the real types.GuardrailAction enum only has "NONE"/
                      # "GUARDRAIL_INTERVENED" (types/enums.go:161-166 in the pinned SDK); see ApplyGuardrail op note.
                      # Also added two real-SDK-client round-trip tests (wire_sdk_roundtrip_test.go) proving the
                      # event-stream chunk-payload and ConverseStream union-discriminator fixes from the 2026-08-07
                      # pass still hold against the actual aws-sdk-go-v2 client/eventstream reader, not hand-parsed
                      # bytes. Grade held at A (one real bug found and fixed, rest of the surface re-verified clean).
                      # 2026-08-07 (gopherstack-ayfw): fixed ChaosServiceName ("bedrockruntime" -> "bedrock", matching
                      # real SigV4 signing name) -- see chaos-fault-injection family below. Closes the gap the
                      # 2026-07-25 pass below left open; grade held at A.
                      # 2026-07-25: genuine fixes found this pass
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  InvokeModel: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed this pass: guardrailIdentifier-without-guardrailVersion now ValidationException (matches documented InvokeModelInput precondition); PerformanceConfigLatency request header now echoed onto the response header (was silently dropped, always empty to real SDK callers)"}
  InvokeModelWithResponseStream: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed this pass (CRITICAL): the 'chunk' event payload was the raw mock-response JSON; the real client (types.PayloadPart via awsRestjson1_deserializeDocumentPayloadPart) requires the payload to be a JSON document {\"bytes\":\"<base64>\"} -- previously every real SDK client's InvokeModelWithResponseStream call against gopherstack decoded to an EMPTY body. Also added: X-Amzn-Bedrock-Content-Type response header (was never set --  bound to a *different* header than plain InvokeModel's Content-Type), same guardrail-header and PerformanceConfigLatency fixes as InvokeModel, chunk event :content-type fixed from the wrong 'application/octet-stream' to 'application/json'"}
  InvokeModelWithBidirectionalStream: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed this pass: same chunk-payload {\"bytes\":<base64>} wrapping bug as InvokeModelWithResponseStream (types.BidirectionalOutputPayloadPart has the identical Bytes-wrapped shape). No guardrail/PerformanceConfigLatency headers exist on this op's real Input struct (verified against api_op_InvokeModelWithBidirectionalStream.go: ModelId is its only member), so those fixes do not apply here"}
  Converse: {wire: ok, errors: ok, state: ok, persist: n/a, note: "field-diffed against ConverseInput/ConverseOutput -- messages/system/inferenceConfig/toolConfig/guardrailConfig accepted (not fabricated-away), output.message/stopReason/usage{input,output,totalTokens}/metrics{latencyMs} all match required members"}
  ConverseStream: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed this pass: contentBlockStart event no longer sends a fabricated 'start':{'text':''} field -- types.ContentBlockStart's union has only image/toolResult/toolUse variants (verified against deserializeDocumentContentBlockStart in the real SDK), no 'text' member exists, so a plain-text content block must omit 'start' entirely rather than emit a non-existent union tag. Event names (messageStart/contentBlockStart/contentBlockDelta/contentBlockStop/messageStop/metadata) and their field shapes (contentBlockIndex/delta.text/stopReason/usage/metrics.latencyMs) verified against awsRestjson1_deserializeEventStreamConverseStreamOutput -- all correct, unchanged"}
  CountTokens: {wire: ok, errors: ok, state: ok, persist: n/a, note: "unchanged this pass -- previously fixed request body wire shape ({input:{invokeModel:{body}}} / {input:{converse:{...}}}) re-verified still correct"}
  ApplyGuardrail: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed 2026-08-07 pass: assessments was ALWAYS an empty array, including when action=BLOCKED -- a disguised no-op (PARITY.md previously and incorrectly claimed 'assessments... reflect the real input content', which was false: only outputs did). Now a BLOCKED action reports a types.GuardrailWordPolicyAssessment-shaped wordPolicy.customWords entry naming the matched keyword, matching the real GuardrailAssessment union's required member shapes. FIXED 2026-08-20 (real bug, this pass): the top-level response field 'action' was the literal string \"BLOCKED\", which is not a member of ApplyGuardrailOutput.Action's real type, types.GuardrailAction (verified: aws-sdk-go-v2/service/bedrockruntime@v1.57.1/types/enums.go:161-166 -- exactly two values, GuardrailActionNone=\"NONE\" and GuardrailActionGuardrailIntervened=\"GUARDRAIL_INTERVENED\"). \"BLOCKED\" is a real value but belongs to a DIFFERENT enum at a DIFFERENT nesting level -- types.GuardrailWordPolicyAction (enums.go:835-840), used correctly by assessments[].wordPolicy.customWords[].action. The same literal constant was being reused for both the outer op-level decision and the inner per-word-policy-hit decision; Go does not reject an out-of-enum string at JSON-decode time (GuardrailAction is a plain string type), so this produced no client error -- any real caller branching on resp.Action (e.g. `if action == types.GuardrailActionGuardrailIntervened`) would silently treat every BLOCKED call as unblocked. Fixed via a new topLevelGuardrailAction() mapping function (handler_guardrails.go) so the outer 'action' key and the inner wordPolicy action stay independently correct. Proven both ways: TestApplyGuardrail_ActionEnum_SDKRoundTrip (wire_sdk_roundtrip_test.go) asserts out.Action == types.GuardrailActionGuardrailIntervened via the real SDK client; hand-revert (cp method) reproduced the exact real-world symptom -- types.GuardrailAction(\"BLOCKED\"), an unmatched enum value the client received with no error. Pre-existing unit tests in handler_guardrails_test.go asserted the wrong wire value (\"BLOCKED\" at the top level) and were corrected to \"GUARDRAIL_INTERVENED\" alongside the fix; the nested wordPolicy.customWords[].action assertion (still \"BLOCKED\") was left unchanged since it was already correct."}
  StartAsyncInvoke: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04: AsyncInvokeStatus's Failed value (types/enums.go) is declared and read (buildAsyncInvokeResponse's isTerminal/failureMessage handling, models.go) but no code path in this service ever sets it -- the janitor only ever advances InProgress -> Completed. Flagged, not fixed: a deterministic trigger would have to be invented (no real StartAsyncInvokeInput field models a designed-to-fail request), and this service's own gaps list already rejects fabricating mock behavior beyond what's directly evidenced by the SDK docs. See gaps."}
  GetAsyncInvoke: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-21 (gopherstack-y1zn): buildAsyncInvokeResponse emitted a \"tags\" key whenever Tags was non-empty; neither GetAsyncInvokeOutput nor AsyncInvokeSummary (bedrockruntime@v1.57.1 types/api_op_GetAsyncInvoke.go) has a Tags member, and this service has no TagResource/ListTagsForResource op at all -- real AWS gives no way to read an async invoke's tags back, ever. Removed. Proven via raw-body assertion (TestGetAsyncInvoke_NoInventedTagsKey_RealClient), hand-reverted/confirmed-failing/restored/md5sum-verified."}
  ListAsyncInvokes: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-21 (gopherstack-y1zn): shares buildAsyncInvokeResponse with GetAsyncInvoke; same tags-key fix applies to each summary entry. FIXED 2026-09-04 (real bug, this pass): submitTimeAfter/submitTimeBefore/sortOrder (ListAsyncInvokesInput fields, httpQuery-bound -- serializers.go's awsRestjson1_serializeOpHttpBindingsListAsyncInvokesInput) were never read by handleListAsyncInvokes -- every request silently matched every invocation regardless of the time-range filters, and results always came back ascending regardless of sortOrder. sortBy is still not dispatched, deliberately: SortAsyncInvocationBy (types/enums.go) has exactly one value, SubmissionTime, which is already the only order ListAsyncInvokes ever sorts by. Fixed via parseListAsyncInvokesFilter/matchesAsyncInvokeFilter (handler_async_invoke.go/async_invoke.go) plus a descending sort branch. Proven via TestHandler_ListAsyncInvokes_SubmitTimeFilters and TestHandler_ListAsyncInvokes_SortOrder (hand-neutered to `false && ...`/confirmed-failing/restored)."}
  InvokeGuardrailChecks: {wire: partial, errors: ok, state: partial, persist: n/a, note: "NEW this pass (POST /guardrail-checks/invoke, confirmed from serializers.go's awsRestjson1_serializeOpInvokeGuardrailChecks path literal; field-diffed contentFilter/promptAttack/sensitiveInformation config+result shapes against types.go/deserializers.go). Does NOT reference a stored 'bedrock'-service guardrail resource -- Checks is supplied inline in the request, so there is no guardrailIdentifier and nothing in the separate bedrock control-plane backend to consult (confirmed unreachable/inapplicable here, not silently ignored). contentFilter/promptAttack: no real ML classifier exists, so each requested group is present (matches whether the caller asked for it) but its 'results' array is honestly EMPTY rather than carrying a fabricated severityScore per category -- this is a genuine wire-completeness gap (real AWS always returns one entry per requested category) traded deliberately against the alternative of inventing classifier output; wire: partial reflects this. sensitiveInformation: genuinely evaluated via literal, deterministic pattern/format detectors (see handler_guardrail_checks.go's piiDetectors) for EMAIL/PHONE/IP_ADDRESS/URL/AWS_ACCESS_KEY/MAC_ADDRESS/US_SOCIAL_SECURITY_NUMBER/CREDIT_DEBIT_CARD_NUMBER (the last Luhn-checksum validated); every other GuardrailChecksSensitiveInformationEntityType value requires free-text NER or a jurisdiction-specific checksum not implemented, and is honestly never matched -- not fabricated. confidenceScore is always exactly 1.0 for a detected entity (a deterministic pattern hit, not an invented probability). usage.*.textUnits is always 0, matching the same not-metered precedent ApplyGuardrail already established for its usage block."}
families:
  model-path-routing: {status: ok, note: "unchanged this pass; extractModelID/ExtractResource ARN-embedded-slash fix from the prior audit re-verified still correct"}
  guardrail-path-routing: {status: ok, note: "unchanged this pass"}
  async-invoke: {status: ok, note: "2026-09-04: fixed two real bugs, see StartAsyncInvoke/ListAsyncInvokes op notes -- (1) ListAsyncInvokes' submitTimeAfter/submitTimeBefore/sortOrder filters were parsed nowhere, (2) StartWorker's janitor ran on a 1-hour tick while defaultAsyncInvokeCompletionDelay is 5s, so a real running server (not test-helper-driven) left InProgress invocations unadvanced for up to an hour. StartAsyncInvoke/GetAsyncInvoke wire shapes, idempotency, and persistence re-verified against StartAsyncInvokeInput/GetAsyncInvokeOutput/AsyncInvokeSummary -- all required members present, no fabricated fields"}
  error-codes: {status: ok, note: "unchanged this pass; resolveErrorType/fallback confirmed to map to the REAL modeled 'InternalServerException' (not a fabricated 'InternalFailure') at all 9 handler.go/handler_*.go call sites -- re-verified, not a regression"}
  event-stream-chunk-payload: {status: ok, note: "NEW this pass (see InvokeModelWithResponseStream/InvokeModelWithBidirectionalStream op notes): the 'chunk' event's payload must be the smithy PayloadPart/BidirectionalOutputPayloadPart document shape {\"bytes\":\"<base64>\"}, not the raw response JSON. This was the highest-impact bug found this audit -- it broke response-body delivery for every real aws-sdk-go-v2 client streaming call against gopherstack, silently (no error, just an empty Body on the client side)."}
  chaos-fault-injection: {status: ok, note: "2026-08-07 (gopherstack-ayfw): ChaosServiceName was \"bedrockruntime\", but real Bedrock Runtime signs every request with SigV4 service name \"bedrock\" (verified: aws-sdk-go-v2/service/bedrockruntime@v1.57.1 auth.go's serviceAuthOptions, unconditional for every operation) -- the same signing name the sibling services/bedrock control-plane handler already declares. pkgs/chaos's Middleware extracts the fault-matching service string straight from the real Authorization header's SigV4 credential scope, so the old value could never match real client traffic; a fault rule created from the chaos dashboard's own GET /targets discovery (which surfaced \"bedrockruntime\") would silently never fire. Fixed to \"bedrock\" -- getTargets already merges entries sharing one signing name across handlers (its own doc comment cites S3/S3 Control as precedent), so this needed no pkgs/chaos change. New chaos_test.go proves both the fix (a \"bedrock\"-targeted rule now intercepts a real InvokeModel call before the handler runs) and the regression it fixes (a \"bedrockruntime\"-targeted rule does not). This resolves the bd issue's premise -- once the service name matches, the existing generic mechanism already supports injecting ModelError/ModelNotReady/Throttling/ServiceUnavailable (or any other) error code/status for InvokeModel/Converse/any op; see gaps for the one remaining, out-of-scope refinement (ModelErrorException's extra OriginalStatusCode/ResourceName members)."}
gaps:
  - "The generic pkgs/chaos FaultError shape ({code, statusCode} -> a plain {__type, message} JSON body) can inject any error code/status for InvokeModel/Converse/etc (verified: real ModelErrorException/ModelNotReadyException/ThrottlingException/ServiceUnavailableException are all restjson1 GetErrorInfo-resolvable from a body __type field, no X-Amzn-ErrorType header required), but cannot reproduce ModelErrorException's two extra members (OriginalStatusCode, ResourceName) since chaos.FaultError has no per-service extension point for them. Buildable (add optional extra-fields support to chaos.FaultError) but out of this pass's scope: it is shared pkgs/chaos infrastructure, not bedrockruntime-local, and touching it has blast radius across all 137 chaos-registered services. (bd: gopherstack-ayfw)"
  - "CountTokens' invokeModel-body token estimate uses raw decoded-byte length as a chars proxy (cannot know the tokenizer for arbitrary model-specific InvokeModel body formats); acceptable per parity rules (deterministic mock), documented as an approximation in code comments"
  - "Converse's guardrailConfig body field (GuardrailIdentifier/GuardrailVersion) is accepted opaquely (json.RawMessage, unparsed) but not validated for the identifier-requires-version precondition that InvokeModel's equivalent HEADER fields now enforce -- both fields are optional/unrequired on types.GuardrailConfiguration (no smithy 'required' trait, verified), so the real SDK client does not enforce this combination client-side either; low-value/out-of-budget this pass since Converse's mock inference doesn't depend on guardrail semantics to produce a valid response"
  - "StartAsyncInvoke does not validate the real, client-side-required 'modelInput' body member is present -- deliberately not added: the real aws-sdk-go-v2 client enforces this required struct field before ever constructing the HTTP request (addOpStartAsyncInvokeValidationMiddleware), so no real SDK-driven caller can produce a request that omits it; adding server-side validation for it would only add risk (touches ~8 existing test bodies) for a scenario no real client can trigger"
  - "InvokeGuardrailChecks' contentFilter (VIOLENCE/HATE/SEXUAL/MISCONDUCT/INSULTS) and promptAttack (JAILBREAK/PROMPT_INJECTION/PROMPT_LEAKAGE) checks always return an empty results list for a requested group instead of one severityScore entry per requested category: gopherstack has no real ML content/prompt-injection classifier, and a per-category score would be pure fabrication. Documented, not hidden -- see the op note above."
  - "InvokeGuardrailChecks' sensitiveInformation check only genuinely detects EMAIL/PHONE/IP_ADDRESS/URL/AWS_ACCESS_KEY/MAC_ADDRESS/US_SOCIAL_SECURITY_NUMBER/CREDIT_DEBIT_CARD_NUMBER (literal, deterministic formats). Every other GuardrailChecksSensitiveInformationEntityType (NAME, ADDRESS, AGE, PASSWORD, DRIVER_ID, LICENSE_PLATE, AWS_SECRET_KEY, and the various bank/tax/passport/health-ID entity types) requires free-text NER or a jurisdiction-specific checksum this backend does not implement, so those types are honestly never matched rather than fabricated."
  - "DISCLOSED, NOT FIXED (2026-09-04): AsyncInvokeStatusFailed (models.go) and AsyncInvoke.FailureMessage are declared and consumed in the response builder (buildAsyncInvokeResponse's isTerminal/failureMessage branches, handler_async_invoke.go) but no code path in this service ever produces them -- the janitor (janitor.go's advanceAsyncInvokes) only ever transitions InProgress -> Completed, never -> Failed, and AdvanceAsyncInvokesForTest mirrors that. Real AWS clearly models this transition: GetAsyncInvokeOutput.FailureMessage's doc comment is 'An error message' (api_op_GetAsyncInvoke.go) and AsyncInvokeStatus.Values() (types/enums.go) lists exactly {InProgress, Completed, Failed}, so a real async invocation genuinely can end up Failed. NOT changed this pass: making it reachable would require inventing a deterministic mock trigger (e.g. a magic modelId/s3Uri marker, mirroring services/bedrock/agents.go's missing-FoundationModel-triggers-FAILED precedent or this file's own guardrail-keyword convention) -- there is no SDK-documented condition gopherstack can honestly key off of, since StartAsyncInvoke's only content field (modelInput) is deliberately unparsed (see the modelInput gap above). Flagging for the next auditor: AsyncInvokeStatusFailed is dead code today, not merely rare."
  - "DISCLOSED, NOT FIXED (2026-08-20): GetAsyncInvoke's not-found path (handler_async_invoke.go's handleGetAsyncInvoke -> handleError) returns wire code 'ResourceNotFoundException' (HTTP 404) for an unknown invocationArn. Verified against the pinned SDK: awsRestjson1_deserializeOpErrorGetAsyncInvoke's (deserializers.go:796-859) declared error set is exactly {AccessDeniedException, InternalServerException, ThrottlingException, ValidationException} -- no ResourceNotFoundException case, unlike 8 of this service's other 11 ops (ApplyGuardrail, Converse, ConverseStream, CountTokens, InvokeModel, InvokeModelWithBidirectionalStream, InvokeModelWithResponseStream, StartAsyncInvoke all declare it; ListAsyncInvokes also lacks it). A real aws-sdk-go-v2 client hitting this exact response therefore cannot produce a typed *types.ResourceNotFoundException via errors.As -- it falls through to the generic default case (smithy.GenericAPIError, which still carries the correct Code/Message strings, so plain ErrorCode()-string matching still works; only the typed-exception idiom breaks). NOT changed this pass: it is genuinely unclear whether this reflects real AWS's documented behavior (GetAsyncInvoke's smithy model may simply omit a not-found error AWS's live API does throw, an SDK-codegen/model gap outside gopherstack's control) or whether real AWS truly never signals not-found this way for this specific operation (in which case ValidationException, the only remotely-fitting code left in the declared set, would be the correct replacement). Existing tests (TestHandler_GetAsyncInvoke's '404 for unknown ARN' case, TestAsyncInvoke_GetNotFound) assume the current 404/ResourceNotFoundException shape and were left as-is. Flagging with exact file:line citations for the next auditor rather than guessing at a behavioral change with no way to confirm it against live AWS."
deferred: []
leaks: {status: clean, note: "2026-09-04: re-verified; janitor (RunJanitor/StartWorker/Shutdown) uses context-bounded worker.Group with proper cancel+done-channel wiring, no goroutine leaks found. Model-invoke and stream handlers (InvokeModel/InvokeModelWithResponseStream/InvokeModelWithBidirectionalStream/ConverseStream) write synchronously to the response and spawn no per-request goroutines, so there is nothing there to leak on client disconnect. Fixed this pass: StartWorker's janitor interval (see async-invoke family) -- not a leak, but the same worker-lifecycle surface. No new goroutines/locks introduced."}
---

## Notes

- **2026-08-20 wrapper-key/nested-shape sweep**: added `wire_sdk_roundtrip_test.go`, driving
  InvokeModelWithResponseStream, ConverseStream, and ApplyGuardrail through the real
  `aws-sdk-go-v2/service/bedrockruntime` client (`newTestBedrockRuntimeSDKClient`, same
  `pkgs/service` router pattern as `services/dax/wire_sdk_roundtrip_test.go`) instead of hand-parsing
  wire bytes or calling `h.Handler()(c)` directly. This is what actually caught the
  ApplyGuardrail `Action` enum bug (unit tests asserted gopherstack's own -- wrong -- output, so
  they couldn't catch a value the real client would reject as unrecognized). Also re-proved, via
  the real SDK's own `GetStream()`/`Events()` eventstream reader (not `eventstream_test.go`'s
  hand-rolled frame parser), that the 2026-08-07 pass's event-stream-chunk-payload and
  ConverseStream union-discriminator fixes still hold: `TestInvokeModelWithResponseStream_SDKRoundTrip`
  asserts `types.PayloadPart.Bytes` decodes non-nil; `TestConverseStream_SDKRoundTrip` asserts no
  event ever decodes to `types.UnknownUnionMember`, including a dedicated check that
  `ContentBlockStartEvent.Start` is nil (not an `UnknownUnionMember` for a fabricated "text" tag).
  All three new tests were proven to actually catch their target bug via hand-revert (`cp` to
  scratchpad, mutate, confirm red, restore, confirm green) before being trusted.

- **ContentBlock union (Converse/ConverseStream) coverage this pass**: gopherstack's mock only ever
  *emits* the `text` variant of `types.ContentBlock`/`types.ContentBlockDelta`
  (`ContentBlockMemberText`/`ContentBlockDeltaMemberText` -- confirmed via the SDK round-trip test's
  type assertion), which is a real, correctly-discriminated union member -- not a wrong or invented
  tag. On *ingest*, gopherstack's `converseContent{Text string}` struct silently ignores any other
  member key (image/document/video/toolUse/toolResult/guardContent/cachePoint/reasoningContent/
  citationsContent) via plain `encoding/json` unknown-field tolerance -- this is the same
  "mock inference doesn't interpret block types it can't act on" limitation already documented for
  Converse's opaque `toolConfig`/`guardrailConfig` handling, not a new gap. **Not reached this
  pass**: `SystemContentBlock`, `ToolConfiguration`/`Tool`/`ToolSpec`/`ToolInputSchema`,
  `GuardrailConverseContentBlock` unions -- all three are accepted as unparsed `json.RawMessage`
  and never reinterpreted by gopherstack's mock, so there is no discriminator logic in this codebase
  to verify for them (out of scope as "nothing to check", not skipped).

- **Enums checked both directions this pass**: `ConversationRole` (user/assistant/system -- gopherstack
  only emits `assistant`, matches `ConversationRoleAssistant`), `StopReason` (gopherstack only emits
  `end_turn`, matches `StopReasonEndTurn` exactly, verified against enums.go:976), `AsyncInvokeStatus`
  (all 3 real values -- InProgress/Completed/Failed -- match exactly, both directions), `GuardrailAction`
  (**bug found and fixed** -- see ApplyGuardrail op note), `GuardrailWordPolicyAction` (BLOCKED/NONE,
  correctly used for the nested wordPolicy field). Not independently re-verified this pass (unchanged
  surface, not exercised by any wire-shape change): `Trace`, `GuardrailContentFilterType`,
  `DocumentFormat`, `ImageFormat`, `VideoFormat`, `ToolResultStatus`, `PerformanceConfigLatency`,
  `CachePointType` -- none of these are read back out of a hardcoded gopherstack constant (grepped,
  zero hits outside enums.go itself), so there is no wrong-value risk for them to find; they are
  either opaquely passed through (headers) or unused by the current mock surface.

- **Protocol**: restjson1. Request/response bodies for InvokeModel/InvokeModelWithResponseStream/
  InvokeModelWithBidirectionalStream are raw `application/json` blob passthrough (the Body field is
  opaque bytes whose schema is model-specific); Converse/ConverseStream/CountTokens/ApplyGuardrail/
  async-invoke ops are structured JSON.

- **modelId / guardrailIdentifier can be ARNs containing an embedded `/`.** Real examples:
  inference-profile ARN `arn:aws:bedrock:<region>:<acct>:inference-profile/us.anthropic.claude-3-sonnet-...`,
  custom-model ARN `arn:aws:bedrock:<region>:<acct>:custom-model/<base-model-id>/<unique-id>`. Both are
  non-greedy `{modelId}`/`{guardrailIdentifier}` smithy URI labels (verified against
  aws-sdk-go-v2/service/bedrockruntime@v1.50.1 serializers.go), so the SDK percent-encodes the embedded
  '/' as `%2F` on the wire; net/http's URL parsing then decodes it back to a literal '/' in
  `r.URL.Path` server-side. **Bug fixed this pass**: `extractModelID`/`ExtractResource` used to cut the
  modelId at the FIRST '/' after "/model/", silently truncating ARN modelIds and losing the trailing
  model-family marker (e.g. "claude") that `mockInvokeModelResponse` keys off of -- this caused ARN-style
  invocations to silently fall back to the WRONG response envelope (legacy/default format instead of the
  correct Claude 3 Messages API format, etc). Fix: bound the modelId segment using the *known literal
  suffix* for the already-resolved operation (`/invoke`, `/converse`, ...) via `modelPathSuffixForOp`,
  trimming from the tail instead of cutting at the first '/'. `extractGuardrailIDAndVersion` was already
  correct -- it delimits on the literal substring `/version/`, not the first '/', so ARN
  guardrailIdentifiers were never actually affected there (verified, not a bug).
  **Trap for the next auditor**: don't "fix" the guardrail path parser by analogy -- it doesn't need it.

- **CountTokens wire shape**: the request body is `{"input": {"invokeModel": {"body": "<base64
  blob>"}}}` or `{"input": {"converse": {"messages": [...], "system": [...], ...}}}` -- a discriminated
  union under "input", NOT top-level `prompt`/`messages`/`system` fields (verified against
  awsRestjson1_serializeDocumentCountTokensInput / serializeDocumentInvokeModelTokensRequest /
  serializeDocumentConverseTokensRequest in the real SDK's serializers.go). The `body` member is a
  smithy blob -- base64-encoded JSON text on the wire; Go's `encoding/json` transparently
  base64-decodes into a `[]byte` struct field, so `Body []byte \`json:"body"\`` round-trips it directly.
  Fixed this pass: the handler previously looked for fields that never exist on this operation's real
  request, silently falling back to counting the raw envelope's byte length (JSON structural overhead)
  instead of the actual model input -- always producing an inflated, content-independent token
  estimate. Now parses the real union and measures the decoded invokeModel body / converse message text.

- **Error codes**: this service's real exception codes (verified against
  aws-sdk-go-v2/service/bedrockruntime@v1.50.1 types/errors.go and deserializers.go's
  errorCode switch) are AccessDeniedException, ConflictException, InternalServerException,
  ModelErrorException, ModelNotReadyException, ModelStreamErrorException, ModelTimeoutException,
  ResourceNotFoundException, ServiceQuotaExceededException, ServiceUnavailableException,
  ThrottlingException, ValidationException. Fixed this pass: internal-error responses used the
  Query/EC2-protocol-style code "InternalFailure" (a generic gopherstack pattern copied from other
  services), which does not match any case in the SDK's restjson1 error-code switch -- the aws-sdk-go-v2
  client would fall through to a generic/untyped smithy API error instead of a typed
  `*types.InternalServerException`, breaking client code that does typed exception matching (a common
  Bedrock retry-logic pattern). All 9 call sites now use "InternalServerException".
  "UnknownOperationException" for unmatched routes is left as-is: it's an internal gopherstack-wide
  sentinel for unrouted paths (same convention as ecs/ecr/glue/cloudwatchlogs/redshift/resourcegroups),
  not a real AWS wire code, and out of scope to change service-by-service.

- **Timestamps**: StartAsyncInvoke/GetAsyncInvoke/ListAsyncInvokes use `time.RFC3339` string
  formatting for submitTime/lastModifiedTime/endTime, which matches the smithy `date-time` timestamp
  format the real client parses via `smithytime.ParseDateTime` (verified in deserializers.go) --
  correct, NOT an epoch-seconds numeric field like some other services. Don't "fix" this to
  `awstime.Epoch` by reflex; it would break the wire shape here.

- **Unit tests are external black-box** (`package bedrockruntime_test`), calling `h.Handler()(c)` via
  Echo directly. This exercises the actual dispatch/extraction code path (including the modelId-ARN fix)
  but bypasses `RouteMatcher()` itself for the invoke dispatch tests; `RouteMatcher()` is separately
  covered by `TestHandler_RouteMatcher`. The matcher's prefix check (`HasPrefix(path, "/model/")`) is
  unaffected by embedded slashes in modelId, so no matcher-specific regression test was needed for the
  ARN fix -- the bug lived entirely in modelId *extraction* downstream of routing, which the existing
  `doRequest`-driven tests do exercise for real (`TestParity_InvokeModel_ARNModelIDWithEmbeddedSlash`
  asserts the correct Claude 3 Messages-API envelope is chosen, not just that extraction returns the
  right string).

- **ApplyGuardrail** mock is deterministic: BLOCKED for content containing "blocked"/"harmful"/
  "toxic"/"unsafe" (case-insensitive substring), NONE otherwise; usage counters are always 0 (all
  required int32 fields in the real GuardrailUsage struct, present with zero values -- acceptable mock).
  **Corrected this pass**: the previous version of this note claimed "assessments/outputs do reflect
  the real input content" -- that was only ever true of `outputs`. `assessments` was unconditionally
  `[]` regardless of action, including BLOCKED, which IS a disguised no-op (the one thing a caller most
  wants explained -- *why* was I blocked -- was always empty). Fixed: BLOCKED now returns one
  `types.GuardrailAssessment`-shaped entry with a `wordPolicy.customWords` array containing the matched
  keyword, `action: "BLOCKED"`, `detected: true` (see `types.GuardrailWordPolicyAssessment`/
  `types.GuardrailCustomWord`'s required members in the real SDK's types.go). NONE still returns `[]`,
  which is correct (no policy violation to report) -- verified this is not a second disguised no-op by
  re-reading `buildGuardrailAssessments`'s NONE branch.

- **Converse/ConverseStream** mock reads the actual request (messages/system) to estimate input
  tokens and does NOT ignore them; the completion text itself is a fixed deterministic string, which is
  the explicitly-acceptable "mock inference" behavior per parity rules (no real LLM backing this).
  **Fixed this pass**: ConverseStream's `contentBlockStart` event sent a fabricated
  `"start":{"text":""}` field. The real `types.ContentBlockStart` union (verified against
  `awsRestjson1_deserializeDocumentContentBlockStart` in deserializers.go) has exactly three variants --
  image, toolResult, toolUse -- and no "text" variant, because plain-text content blocks carry no
  meaningful start-of-block payload in the real API. gopherstack's mock only ever emits plain-text
  content, so `contentBlockStart` now omits `start` entirely (real clients tolerate an unset/nil
  `ContentBlockStartEvent.Start`; sending an unrecognized union tag instead produces a `types.UnknownUnionMember`
  substitution client-side, which is the wrong outcome for a field the mock never needed to send).

- **InvokeModelWithResponseStream / InvokeModelWithBidirectionalStream "chunk" event payload** (CRITICAL
  bug fixed this pass): the event message's payload bytes were the raw mock model-response JSON
  (`{"completion":"...", ...}`) written directly. The real aws-sdk-go-v2 client deserializes a "chunk"
  event's payload as `types.PayloadPart` / `types.BidirectionalOutputPayloadPart`
  (`awsRestjson1_deserializeDocumentPayloadPart` in deserializers.go), which looks for exactly one JSON
  key, `"bytes"`, holding the base64-encoded actual response bytes -- any other shape leaves
  `PayloadPart.Bytes` **nil**. This means every real SDK client streaming call
  (`InvokeModelWithResponseStream`/`InvokeModelWithBidirectionalStream`) against gopherstack silently
  received an EMPTY response body -- no error, just nothing, because the raw JSON's top-level keys
  ("completion", "id", "role", ...) never matched the "bytes" key the deserializer looks for. Fixed via
  `modelResponsePayloadPart()`, which now wraps the mock response as
  `{"bytes":"<base64 of the response JSON>"}` before framing it into the chunk event. Also fixed in the
  same pass: the chunk event's `:content-type` message header was the wrong `"application/octet-stream"`
  (now `"application/json"`, matching a structured-document payload), and
  `InvokeModelWithResponseStream`'s HTTP-level `X-Amzn-Bedrock-Content-Type` response header (bound to a
  *different* wire location than plain InvokeModel's `Content-Type` -- verified against
  `awsRestjson1_deserializeOpHttpBindingsInvokeModelWithResponseStreamOutput`) was never set at all.

- **InvokeModel / InvokeModelWithResponseStream guardrail headers**: `GuardrailIdentifier`
  (`X-Amzn-Bedrock-Guardrailidentifier`) and `GuardrailVersion` (`X-Amzn-Bedrock-Guardrailversion`) are
  documented as jointly required in `InvokeModelInput.GuardrailIdentifier`'s doc comment ("An error will
  be thrown ... You provide a guardrail identifier, but guardrailVersion isn't specified"). Fixed this
  pass: gopherstack previously accepted any combination silently. `validateGuardrailHeaders` now returns
  `ValidationException` when an identifier is set without a version, or when a guardrail identifier is
  combined with a non-`application/json` content type (also documented). `InvokeModelWithBidirectionalStream`
  does NOT get this check -- its real `Input` struct has only `ModelId` (verified against
  `api_op_InvokeModelWithBidirectionalStream.go`), no guardrail headers exist on that operation.

- **InvokeGuardrailChecks routing**: `POST /guardrail-checks/invoke` is a fixed, standalone endpoint --
  it is NOT under `guardrailPathPrefix` (`/guardrail/`) despite the name similarity to ApplyGuardrail's
  `/guardrail/{id}/version/{ver}/apply`. `"/guardrail-checks/invoke"` does not share that prefix (the
  character after `/guardrail` is `-`, not `/`), so it is matched/dispatched as its own case in
  `RouteMatcher`/`Handler()`/`asyncOrGuardrailOperation`, not folded into `handleGuardrailPath`. It also
  takes its check configuration inline in the request body rather than a path-embedded
  guardrailIdentifier/guardrailVersion -- there is no guardrail resource lookup at all for this
  operation, in gopherstack or in real AWS.

- **PerformanceConfigLatency echo**: `InvokeModel`/`InvokeModelWithResponseStream`'s
  `X-Amzn-Bedrock-Performanceconfig-Latency` request header now echoes onto the response (real output
  struct's `PerformanceConfigLatency` member, read back from the same header name -- verified against
  `awsRestjson1_deserializeOpHttpBindingsInvokeModelOutput`). Previously always empty to callers who set
  it. gopherstack has no real latency-optimized inference tier, so it reflects the caller's request value
  instead of fabricating one; omitted entirely (not defaulted to "standard") when the caller didn't send
  it, to avoid inventing a value with no backing semantics.

## 2026-08-21: gopherstack-r80d batch 21 -- required-output-member cut, 0 bugs

Second-largest remaining `gopherstack-r80d` candidate after sagemaker (20
required output fields, 11 ops, 8 with >=1, per `cmd/requiredoutputfields`).
Instrument cross-checked three ways (character-level brace matcher,
`go/parser` AST walk, raw `grep -c`) across `types/types.go` + every
`api_op_*.go` file -- all three agree at 161 total required fields / 90
structs. Most of that 90-struct surface is the `Converse`/`ConverseStream`
polymorphic `ContentBlock`/`*EventAttributes` union family (`ImageBlock`,
`DocumentBlock`, `VideoBlock`, `AudioBlock`, `ToolUseBlock`,
`ToolResultBlock`, `SearchResultBlock`, the `GuardrailChecks*`/`Guardrail*`
assessment leaf types, and every `ConverseStream` event-detail type) --
`handler_converse.go`'s `buildConverseResponse`/`handleConverseStream` only
ever construct a single plain-text content block (`ContentBlockMemberText`),
never any of the other union variants, a documented, disclosed scope limit
(not a dropped-required-field bug, since those variants are never
constructed at all).

Read all 8 ops with required output fields end to end against their
handlers (`handler_converse.go`, `handler_guardrails.go`,
`handler_guardrail_checks.go`, `handler_invoke.go`,
`handler_async_invoke.go`), verified against the real
`awsRestjson1_deserializeOpDocument<Op>Output`/
`awsRestjson1_deserializeDocument<Type>` functions directly (not just Go
field names) for `Converse`'s `output`/`message`/`content` union chain and
`GuardrailChecks*`'s result/usage wrapper keys. Every required member is
present on every real-client-reachable path: `ConverseOutput.Metrics/
Output/StopReason/Usage`, `ApplyGuardrailOutput.Action/Assessments/Outputs/
Usage` (including the nested `GuardrailUsage`'s 6 required unit-count
fields, `GuardrailWordPolicyAssessment.CustomWords/ManagedWordLists`, and
`GuardrailCustomWord.Action/Match`), `GetAsyncInvokeOutput`/
`AsyncInvokeSummary`'s `InvocationArn/ModelArn/OutputDataConfig/SubmitTime`
(`Status` is genuinely NOT required on `AsyncInvokeSummary`, confirmed by
reading the struct directly -- only on `GetAsyncInvokeOutput`),
`InvokeGuardrailChecksOutput.Results/Usage` and each check family's own
required `Results`/`TextUnits` sub-fields (contentFilter/promptAttack
results lists are honestly empty per the pre-existing ML-classifier gap,
already disclosed above -- not a new finding), `InvokeModelOutput.Body/
ContentType`, `InvokeModelWithResponseStreamOutput.ContentType`,
`CountTokensOutput.InputTokens`, `StartAsyncInvokeOutput.InvocationArn`. No
code changes; see `services/_REQUIRED_OUTPUT_CANDIDATES.md`'s
settled-services table.

## 2026-08-23: ListAsyncInvokes pagination bug

`handleListAsyncInvokes` ignored `maxResults`/`nextToken` (both real,
httpQuery-bound `ListAsyncInvokesInput` members —
`awsRestjson1_serializeOpHttpBindingsListAsyncInvokesInput`,
serializers.go:1115-1146) and always returned every invocation in one
unbounded page with no `nextToken`, discovered while auditing the pagination
bug class found in medialive (every List handler ignoring its real
request's pagination member). Unlike medialive's bug this did not loop
forever (no artificial default cap was applied to be re-exceeded — the
handler just returned everything), but it did mean a client requesting
`maxResults` never got a bounded page. Fixed: reads `maxResults` (default
10, matching the real API's documented default) and `nextToken`
(`InvocationArn` of the last item on the previous page), returns `nextToken`
in the response when truncated. Proven with
`TestListAsyncInvokes_SDKRoundTrip_Pagination`
(`handler_list_async_invokes_pagination_test.go`), which drives the real SDK
client across two 10-item pages of 25 seeded invocations and asserts the
pages are disjoint; fails against the unfixed handler (`should have 10
item(s), but has 25`), hand-reverted and confirmed.

## 2026-09-06: AsyncInvokeStatus Failed confirmed unreachable by construction (gopherstack-0c1r)

`buildAsyncInvokeResponse` (`handler_async_invoke.go`) has terminal-state
handling for `AsyncInvokeStatusFailed`/`FailureMessage`, and
`AsyncInvokeStatus.Values()` (bedrockruntime@v1.57.1 types/enums.go) lists
`InProgress`, `Completed`, `Failed`, but nothing in this backend ever sets
`Failed`. Re-audited per gopherstack-0c1r rather than re-asserting the prior
pass's conclusion; found no reason to overturn it. Checked, in order:

1. **The `services/bedrock/agents.go` missing-FoundationModel precedent.**
   `advanceAgentStatus` fails an agent only when `ag.FoundationModel == ""`.
   That is legitimate because `CreateAgentInput.FoundationModel` is
   genuinely optional in the SDK (no "This member is required" doc comment,
   `api_op_CreateAgent.go:119`) — an agent can really be created without
   one, and `PrepareAgent` legitimately has nothing to compile. The
   analogous field in `bedrockruntime`, `StartAsyncInvokeInput.ModelId`, is
   the opposite: marked required (`api_op_StartAsyncInvoke.go:44-47`,
   "This member is required"), and this backend already rejects
   `modelID == ""` synchronously (`async_invoke.go` `StartAsyncInvoke`,
   `ErrValidation`). The precedent's condition ("required field left empty,
   caught at a later async step") has no equivalent here — the empty case
   is already caught synchronously, before any invocation exists to later
   fail.

2. **`StartAsyncInvokeInput`'s full field list**
   (`api_op_StartAsyncInvoke.go:42-65`): `ModelId *string` (required),
   `ModelInput document.Interface` (required, opaque smithy document —
   deliberately unparsed, documented in `handler_async_invoke.go`'s
   `startAsyncInvokeInput` comment), `OutputDataConfig
   types.AsyncInvokeOutputDataConfig` (required), `ClientRequestToken
   *string`, `Tags []types.Tag`. `OutputDataConfig` resolves to
   `AsyncInvokeS3OutputDataConfig{S3Uri *string (required), BucketOwner
   *string, KmsKeyId *string}` (types/types.go:64-78) — an S3 URI string,
   `BucketOwner`, and a KMS key ID, none of which this backend can verify
   are real/reachable (no S3/KMS bucket-existence or key-existence
   cross-check exists anywhere in this backend, and inventing one would be
   the same kind of fabrication the issue warns against). No doc comment on
   any of these three fields describes bucket/key validation as an async
   failure mode; `S3Uri`'s only documented constraint is the `s3://` prefix,
   already enforced synchronously in `StartAsyncInvoke`
   (`strings.HasPrefix` check). `ModelInput` is confirmed the only
   content field with no wire-decodable schema — nothing changed here.

3. **`GetAsyncInvokeOutput.FailureMessage`'s doc comment**
   (`api_op_GetAsyncInvoke.go:69-70`): `"An error message."` — no
   elaboration on what conditions produce it. No evidence here either way.

4. **Error extraction, both ops** (raw, `[A-Za-z0-9]+`, from
   `bedrockruntime@v1.57.1/deserializers.go`):

   `deserializeOpErrorStartAsyncInvoke`: `UnknownError`,
   `AccessDeniedException`, `ConflictException`, `InternalServerException`,
   `ResourceNotFoundException`, `ServiceQuotaExceededException`,
   `ServiceUnavailableException`, `ThrottlingException`,
   `ValidationException`.

   `deserializeOpErrorGetAsyncInvoke`: `UnknownError`,
   `AccessDeniedException`, `InternalServerException`,
   `ThrottlingException`, `ValidationException`.

   This is the decisive evidence. `StartAsyncInvoke` declares
   `ResourceNotFoundException` in its own error set — the one error code
   that would plausibly fire for "ModelId names a model that doesn't
   exist." Its presence there means AWS rejects an unknown `ModelId`
   *synchronously*, as a direct `StartAsyncInvoke` error, before an
   `AsyncInvoke` resource is ever created — not as a `Failed` status
   discovered later via `GetAsyncInvoke`. `GetAsyncInvoke`'s own error set
   has no `ResourceNotFoundException`-shaped signal about invocation
   content either (its `ResourceNotFoundException`-free list only covers
   "the ARN itself doesn't exist", already handled by this backend's
   existing `ErrNotFound` path). So the one candidate this repo's own
   precedent pattern would suggest (`ModelId` naming an unknown model, by
   analogy to bedrock's missing-`FoundationModel` check) is contradicted by
   the SDK's own error taxonomy: it is modeled as a synchronous rejection,
   not an async terminal state. Using it to drive `Failed` would be wrong,
   not just unproven.

**Verdict: confirmed, not overturned.** No SDK-evidenced precondition
exists that `StartAsyncInvoke`/`GetAsyncInvoke` validates or requires whose
violation AWS surfaces as an async `Failed` rather than a synchronous
error. `AsyncInvokeStatusFailed` and the `FailureMessage`-population branch
in `buildAsyncInvokeResponse` remain unreachable by construction, in the
same class as gopherstack-h3th's five unreachable-by-construction status
constants and gopherstack-glw7's unrepresentable clause. No code changed.

What would change this answer: a documented AWS failure mode for async
invocations (a user-guide page, a re-read of a newer SDK's doc comments, or
an observed real API failure report) that ties a specific `StartAsyncInvoke`
input condition to a later `Failed` status rather than a synchronous error
— e.g. if AWS ever added a `ModelNotReadyException`-style deferred-failure
code to `GetAsyncInvoke`'s error set, or documented that S3 write failures
at completion time populate `FailureMessage`. Absent that, fabricating a
trigger (time-based, random, or a test-only backdoor) is explicitly out of
scope per gopherstack-0c1r and would be worse than leaving the constant
unreachable.

