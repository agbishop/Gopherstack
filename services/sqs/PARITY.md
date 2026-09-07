---
service: sqs
sdk_module: aws-sdk-go-v2/service/sqs@v1.46.4
last_audit_commit: f51bf624e
last_audit_date: 2026-08-10
overall: A
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateQueue: {wire: ok, errors: ok, state: ok, persist: ok, note: "idempotent-on-existing-config check via isConfigurableQueueAttribute; RedrivePolicy applied inline so denyAll/byQueue RedriveAllowPolicy is now enforced at create time too (see families.dlq_redrive). Fixed this pass: AWS's QueueDeletedRecently (\"you must wait 60 seconds after deleting a queue before creating another with the same name\") was not enforced at all — a real aws-sdk-go-v2/service/sqs/types.QueueDeletedRecently error type exists but had zero code path producing it. checkQueueDeletedRecently now enforces the 60s cooldown, keyed like the queue table by (region, name); the cooldown map is persisted (backendSnapshot.RecentlyDeleted) and janitor-pruned (see leaks). Also fixed this pass: validateQueueAttributes accepted ANY attribute key — an unrecognized/misspelled name (e.g. a typo'd \"VisibilityTimeOut\") was silently stored and echoed back instead of rejected with InvalidAttributeName, the exact kind of over-permissiveness parity-principles.md warns about. Now checked against isConfigurableQueueAttribute's 15-name allowlist (AWS's 21 settable QueueAttributeName enum values minus the 6 read-only/computed ones), shared by CreateQueue and SetQueueAttributes"}
  DeleteQueue: {wire: ok, errors: ok, state: ok, persist: ok, note: "closes q.notify (wakes long-pollers with QueueDoesNotExist), closes tag store, cancels involved move tasks. Fixed this pass: now records the deletion timestamp into b.recentlyDeleted for CreateQueue's new QueueDeletedRecently cooldown check"}
  ListQueues: {wire: ok, errors: ok, state: ok, persist: n/a, note: "region-scoped, prefix filter, pkgs/page pagination"}
  GetQueueUrl: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetQueueAttributes: {wire: ok, errors: ok, state: ok, persist: n/a, note: "dynamic attrs (ApproximateNumberOfMessages/NotVisible/Delayed) computed live from q.delayedCount + slice lens under q.mu"}
  SetQueueAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "FifoQueue immutable; full range validation (VisibilityTimeout/DelaySeconds/MessageRetentionPeriod/MaximumMessageSize/KmsDataKeyReusePeriodSeconds/FIFO enum attrs/RedriveAllowPolicy shape); RedriveAllowPolicy enforcement added this pass. Fixed this pass (gopherstack-gcpg): unrecognized attribute names now rejected (see CreateQueue note, same allowlist). Also fixed: AWS's \"FifoThroughputLimit=perMessageGroupId is allowed only when DeduplicationScope=messageGroup\" pairing rule (aws-sdk-go-v2/service/sqs@v1.46.4 api_op_SetQueueAttributes.go:179-180) was enforced only when BOTH attributes were present in the same call — a queue with DeduplicationScope=queue set by an earlier call could later have FifoThroughputLimit flipped to perMessageGroupId by a call that didn't mention DeduplicationScope at all, silently producing the AWS-invalid combination. fifoThroughputPairingValid now checks the effective (existing-queue-state merged with the incoming call) pair at both CreateQueue and SetQueueAttributes"}
  SendMessage: {wire: ok, errors: fixed, state: ok, persist: ok, note: "fixed this pass: ErrInvalidDelaySeconds (returned when DelaySeconds is outside [0,900]) was declared in errors.go but absent from every errorDetails/invalidParameterValueMessage lookup table, so it fell through to the default com.amazonaws.sqs#InternalError/500 branch instead of the AWS-accurate 400 InvalidParameterValue — the exact missing-errCodeLookup-entry bug class called out in parity-principles.md item 2. Also: MD5OfBody + MD5OfMessageAttributes + MD5OfMessageSystemAttributes all correct AWS byte-packing algorithm; FIFO validated (MessageGroupId required, no per-message DelaySeconds, dedup ID required unless content-based); delay/queue-default resolution correct"}
  ReceiveMessage: {wire: fixed, errors: fixed, state: fixed, persist: n/a, note: "fixed this pass: (1) VisibilityTimeout range was validated only in the JSON handler, not centrally — Query protocol silently accepted out-of-range values; (2) re-queued (visibility-expired or explicitly zeroed) FIFO messages were appended to the tail of the pending list instead of reinserted by SequenceNumber, letting a newer same-group message jump ahead — see families.fifo_ordering; (3) field-diffed the MessageSystemAttributeName enum against aws-sdk-go-v2/service/sqs/types.enums.go (10 values: All/SenderId/SentTimestamp/ApproximateReceiveCount/ApproximateFirstReceiveTimestamp/SequenceNumber/MessageDeduplicationId/MessageGroupId/AWSTraceHeader/DeadLetterQueueSourceArn) and found DeadLetterQueueSourceArn was entirely unimplemented — a message auto-redriven to a DLQ never carried the source queue's ARN. tryRouteToDLQ now sets it; flows through filterSystemAttrs generically (both JSON and Query protocol) and round-trips through Snapshot/Restore since it lives in the generic Message.Attributes map"}
  DeleteMessage: {wire: ok, errors: ok, state: ok, persist: ok, note: "O(1) via inFlightByHandle (#56)"}
  ChangeMessageVisibility: {wire: ok, errors: ok, state: fixed, persist: ok, note: "0-timeout re-queue had the same FIFO tail-append ordering bug as ReceiveMessage's janitor sweep; both now go through the shared requeueMessage helper"}
  SendMessageBatch: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "fixed this pass: entries were routed straight to sendMessageLocked, which — unlike the top-level SendMessage entry point — never checked for an empty MessageBody, never range-checked per-entry DelaySeconds ([0,900]), and never called validateMessageAttributes (reserved AWS./Amazon. name prefixes, DataType shape, >10 attributes). A batch entry with any of these was silently accepted as a normal message instead of surfaced as a per-entry BatchResultErrorEntry — a disguised stub on the validation path even though the surrounding op is fully real. All three checks now also run inside sendMessageLocked so both the single-message and batch paths share one validation path. Otherwise: per-entry BatchResultErrorEntry, 10-entry cap, BatchRequestTooLong on combined-size overflow, order preserved"}
  DeleteMessageBatch: {wire: ok, errors: ok, state: ok, persist: ok, note: "batch-level QueueDoesNotExist, per-entry delegates to DeleteMessage"}
  ChangeMessageVisibilityBatch: {wire: ok, errors: ok, state: ok, persist: ok}
  PurgeQueue: {wire: ok, errors: ok, state: ok, persist: ok, note: "60s cooldown enforced (PurgeQueueInProgress); FIFO dedup state reset on purge"}
  TagQueue/UntagQueue/ListQueueTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "pkgs/tags-backed. VERIFIED CLEAN (wrapper-key sweep, 2026-08-29): checked for the stepfunctions-class bug (a Tags field typed as a Go map when the SDK sends an array, or vice versa). sqs@v1.46.4 TagQueueInput.Tags is genuinely map[string]string (a JSON object on the wire, unlike RDS/SNS/CloudWatch's array-of-{Key,Value}) and UntagQueueInput.TagKeys is []string — handler_tags.go's jsonTagQueueReq/jsonUntagQueueReq (JSON-RPC, the pinned SDK's only real wire path; X-Amz-Target: AmazonSQS.*) already decode exactly these shapes via pkgs/tags.Tags' map-backed (Un)MarshalJSON. Confirmed via TestTagQueueFamily_SDKRoundTrip (tag_queue_sdk_test.go) driving the real SDK client."}
  ListDeadLetterSourceQueues: {wire: ok, errors: ok, state: ok, persist: n/a}
  AddPermission/RemovePermission: {wire: ok, errors: ok, state: ok, persist: ok, note: "rebuilds an IAM policy doc into Attributes[Policy], deterministic (sorted labels)"}
  StartMessageMoveTask: {wire: ok, errors: ok, state: ok, persist: partial, note: "RUNNING tasks are correctly NOT persisted (goroutine can't resume); default-destination lookup via RedrivePolicy scan; rate-limited via ticker; TOCTOU-safe under b.mu"}
  CancelMessageMoveTask: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMessageMoveTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "TaskHandle only populated for RUNNING (AWS behavior); MaxResults default 1 / cap 10; newest-first"}
families:
  message_attribute_md5: {status: ok, note: "computeMD5OfMessageAttributes matches the AWS wire algorithm exactly: sorted names, 4-byte-BE-length-prefixed name/dataType/value, 1-byte transport type (1=String|Number, 2=Binary); subset-MD5 on filtered receive re-hashes only when the returned set is a strict subset, reuses the send-time digest otherwise"}
  fifo_dedup: {status: ok, note: "explicit MessageDeduplicationId vs ContentBasedDeduplication (SHA-256 of body, NOT MD5) correctly mutually validated; 5-minute window; DeduplicationScope=queue|messageGroup key scoping; bounded map (100k) with oldest-expiry eviction + janitor sweep"}
  fifo_ordering: {status: ok, note: "fixed this pass (see ReceiveMessage/ChangeMessageVisibility above): requeueMessage now reinserts by SequenceNumber (fixed-width zero-padded decimal, so lexicographic sort == numeric sort) instead of appending to the tail, preserving strict per-MessageGroupId ordering across visibility resets. Confirmed correct behavior (not a bug): only one message per group may be in-flight at a time — this matches real AWS FIFO semantics, not an over-restriction"}
  fifo_throughput_limit: {status: partial, note: "perMessageGroupId (300 TPS/group) enforced via checkFIFOPerGroupRateLimit; perQueue (the AWS default, ~3000/300 TPS) has no limiter at all — see gaps. Re-evaluated this pass (gopherstack-gcpg, the qgh other half): confirmed no real per-operation-type budget matrix is derivable from the pinned SDK/botocore (neither documents the actual numbers, only prose describing the 300 TPS/3000 batched figures elsewhere); implementing a single-counter approximation would be exactly the invented-formula mistake this campaign has already reverted once, so deliberately left unimplemented. Fixed instead: the FifoThroughputLimit=perMessageGroupId / DeduplicationScope=messageGroup pairing rule is now enforced against effective (not just same-call) state — see ops.SetQueueAttributes"}
  visibility_timeout_and_inflight: {status: ok, note: "12h (43200s) max validated on both ChangeMessageVisibility and now ReceiveMessage (backend-level, was JSON-only before this pass); in-flight caps 120k standard / 20k FIFO -> OverLimit; sweepInFlight/pickVisibleMessages single-pass janitor+receive-path cleanup"}
  dlq_redrive: {status: fixed, note: "fixed this pass: RedriveAllowPolicy (allowAll/denyAll/byQueue+sourceQueueArns) was shape-validated by validateRedriveAllowPolicy but never enforced — any source queue could point RedrivePolicy at any DLQ regardless of the DLQ's declared permission. checkRedriveAllowPolicy now enforces it in applyRedrivePolicy (shared by CreateQueue/SetQueueAttributes/Restore). maxReceiveCount routing (tryRouteToDLQ), DLQ must be same region + same FIFO-ness, StartMessageMoveTask default-destination-by-RedrivePolicy all verified correct"}
  delay_queues: {status: ok, note: "queue-level DelaySeconds + per-message DelaySeconds (message wins), FIFO rejects per-message delay, delayedCount maintained incrementally for O(1) GetQueueAttributes"}
  long_polling: {status: ok, note: "broadcast notify-channel-close-and-replace pattern wakes all waiters on send or 0-timeout visibility reset; 1s recheck interval catches janitor-driven requeues without a new SendMessage"}
  receive_request_attempt_id: {status: ok, note: "5-minute exactly-once-retry cache keyed by ReceiveRequestAttemptId, pruned alongside dedup IDs"}
  error_codes: {status: fixed, note: "fixed this pass: the previous audit note here claimed Query protocol \"correctly uses legacy codes ... AWS.SimpleQueueService.PurgeQueueInProgress, etc.\", but independent field-diff against the actual code showed queryErrorDetails only special-cased ErrQueueNotFound and fell through to the shared JSON errorDetails table for every other error — so the Query/XML <Code> element for PurgeQueueInProgress, EmptyBatchRequest, TooManyEntriesInBatchRequest, BatchEntryIdsNotDistinct, and BatchRequestTooLong literally contained the JSON protocol's \"com.amazonaws.sqs#\"-namespaced Smithy shape ID, which is never valid in an XML Query-protocol response (that prefix has no meaning outside the JSON __type field). The correct legacy codes were already sitting unused as those five sentinels' own .Error() text in errors.go (encoded there in an earlier pass, apparently for exactly this purpose, but never wired up). legacyQueryErrorOverride now uses them. QueueDeletedRecently (new this pass) got the same treatment from the start. Query protocol correctly uses legacy codes for these 6 plus NonExistentQueue's pre-existing override vs JSON protocol's com.amazonaws.sqs# namespaced codes for everything else"}
  persistence: {status: fixed, note: "fixed this pass: (1) hasActivity (janitor skip-idle-queue flag) was never restored, so a restored non-FIFO queue with pending messages was silently invisible to the background janitor until an unrelated SendMessage touched it again; (2) fifoSeqCounter was not persisted, so SequenceNumber could regress/duplicate for a FIFO queue that already had messages sent before a snapshot/restore; (3) lastPurgedAt (PurgeQueue 60s cooldown) was not persisted, resetting the cooldown on every restart. This pass added: the new QueueDeletedRecently cooldown map (b.recentlyDeleted) is persisted as a new top-level backendSnapshot.RecentlyDeleted field (region/name -> unix-milli, no version bump needed since it's an additive omitempty field), following the same rationale as lastPurgedAt — otherwise a restore immediately followed by CreateQueue would silently drop the 60s wait-before-recreate rule for a queue deleted just before the snapshot"}
gaps:
  - "FifoThroughputLimit=perQueue (AWS default) is not rate-limited at all; only perMessageGroupId is (checkFIFOPerGroupRateLimit in fifo.go). Deliberately left unfixed this pass after evaluating it: the existing perMessageGroupId limiter's sliding-1s-window pattern generalizes trivially to a queue-wide version, but AWS's real perQueue limit is actually a matrix of separate per-operation-type budgets (SendMessage/SendMessageBatch/ReceiveMessage/DeleteMessage each have their own 300 msg/s unbatched or 3000 msg/s batched budget), not one shared counter — a naive single-counter port would be a gopherstack-invented approximation of AWS's real behavior, not a faithful emulation of it. It also defaults to ON for every FIFO queue (FifoThroughputLimit's AWS default is perQueue), so enabling it without the correct per-operation granularity risks spuriously throttling any existing or future test that sends bursts of FIFO messages in a tight loop (real wall-clock time.Now() in this backend means hundreds of sends in a Go test loop can execute in under 1 second). Confirmed via grep this pass that no current test exercises >300 FIFO sends/second, so the gap is not silently masked by working-by-accident test coverage — it is a genuine, correctly-labeled gap. (bd: gopherstack-qgh)"
  - "KMS SSE (SqsManagedSseEnabled/KmsMasterKeyId/KmsDataKeyReusePeriodSeconds) are accepted, range/shape-validated, and round-trip through GetQueueAttributes, but no actual encryption is modeled (expected for this class of emulator; would require cross-service KMS integration — out of scope for services/sqs/)."
  - "SqsManagedSseEnabled/KmsMasterKeyId mutual exclusion is documented (\"Only one server-side encryption option is supported per queue (for example, SSE-KMS or SSE-SQS)\", aws-sdk-go-v2/service/sqs@v1.46.4 api_op_SetQueueAttributes.go:139-141) but NOT enforced: a queue can have SqsManagedSseEnabled=true (the CreateQueue default, buildDefaultAttributes) and a non-empty KmsMasterKeyId simultaneously, and both are stored/echoed. Evaluated this pass (gopherstack-gcpg) and deliberately left unenforced: unlike the FifoThroughputLimit/DeduplicationScope pairing (which has an explicit \"allowed only when\" sentence), this line is advisory/descriptive and neither the SDK doc comments nor the AWS console-configuration guide (checked via web fetch) specify the API-level enforcement mechanics — reject vs. auto-clear vs. last-key-wins. Also: the existing test suite (TestSSE_KmsMasterKeyId_Configurable, TestSSE_KMS_SetViaSetQueueAttributes, TestKMSAttrsConfigurable, TestSSE_Idempotency_SameKMSKey) already exercises KmsMasterKeyId being set against the default SqsManagedSseEnabled=true and expects success, so a guessed enforcement rule risks the same invented-behavior mistake flagged for the throughput limiter. Real encryption is out of scope regardless (see gap above); if this gets revisited, resolve the reject-vs-auto-clear question against a live AWS account or an authoritative source first."
  - "2026-08-14 (gopherstack-3tpf): independently re-confirmed via a mechanical struct-field diff (cmd/structfielddiff), not by re-reading this file's prior claims -- all 23 ops, every Input/Output/nested struct (BatchResultErrorEntry, Message/MessageAttributeValue, ListMessageMoveTasksResultEntry, etc.) diffed field-by-field against aws-sdk-go-v2/service/sqs@v1.46.4. Zero new gaps: every real field this SDK declares has a matching gopherstack field. Confirmed MessageAttributeValue.BinaryListValues/StringListValues are correctly absent (SDK doc comment: 'Not implemented. Reserved for future use.', types/types.go:226,232 -- known noise, not a gap). No REST/header-bound members exist for this service (JSON + Query protocols only, no header bindings in serializers.go). No code changes made to this service this pass."
deferred:
  - "This pass (gopherstack-gcpg): re-verified sdk_module (aws-sdk-go-v2/service/sqs@v1.46.4, matches go.mod) and audited FifoThroughputLimit=perQueue rate limiting + KMS SSE modeling per the issue. Confirmed via live-code inspection (not PARITY prose) that both areas already had partial implementations: checkFIFOPerGroupRateLimit (fifo.go) for perMessageGroupId, and KMS/SqsManagedSseEnabled accept+range-validate+round-trip (queue_attributes.go/queues.go). Did NOT implement perQueue rate limiting — no real per-operation-type budget matrix could be established from the pinned SDK/botocore, and the issue explicitly warns against inventing one. Did NOT implement KMS encryption or the SqsManagedSseEnabled/KmsMasterKeyId mutual-exclusion rejection — see the two new gaps entries for the reasoning on each. Fixed two real, previously-untested gaps found by sweeping the wider service per the bug-class list in the issue: (1) CreateQueue/SetQueueAttributes accepted ANY attribute key, not just AWS's 21-value QueueAttributeName enum (\"more permissive than AWS\" class); (2) FifoThroughputLimit=perMessageGroupId's DeduplicationScope=messageGroup pairing requirement was only checked within a single SetQueueAttributes call, not against the queue's effective/persisted state (a state-validated-against-the-wrong-scope class, adjacent to the state-mutated-before-validation pattern called out in the issue). Both had zero prior test coverage; both are now covered by table-driven tests in queue_attributes_test.go."
  - "SDK-driven integration tests (test/integration/*_parity_test.go) were not run this pass — per parity-principles.md, unit tests are not full parity proof. Recommend a follow-up integration-suite pass."
  - "This pass: aws-sdk-go-v2/service/sqs bumped 1.44.2 -> 1.45.0 between audits (dependency-upgrade commit, not an sqs-specific change); diffed the two module versions and confirmed no operation/shape changes (only CHANGELOG/generated.json/go_module_metadata.go and new auto-generated serde_snapshot fixtures differ), so no new API surface to audit. Also reviewed the backend.go/persistence.go migration of b.queues/b.moveTasks from bare maps to the new pkgs/store.Table[V] generic collection (shared pkg, out of scope to edit here): locking discipline is preserved (Table performs no internal locking by design; every call site still holds b.mu exactly as it did with the bare map), Snapshot/Restore round-trip through a throwaway DTO registry rather than the live table (correctly excludes non-serialisable fields and RUNNING move tasks), and DLQ pointer re-wiring after Restore iterates the now-populated table correctly. No bugs found in the refactor itself."
  - "This pass: re-verified sdk_module is still current (aws-sdk-go-v2/service/sqs@v1.45.0 at the time -- gopherstack-u8my's later pin-correction pass found go.mod had since moved to v1.46.4 and corrected sdk_module above; diffed v1.45.0 vs v1.46.4: types/{types,enums,errors}.go, serializers.go, deserializers.go, validators.go byte-identical, op count unchanged, so every claim below still holds) and independently re-diffed the full operation set (22/22 match exactly, no invented ops), the QueueAttributeName enum (all 21 non-'All' values present in models.go's attr* constants), and the MessageSystemAttributeName enum (10 values; found and fixed the DeadLetterQueueSourceArn gap — see ops.ReceiveMessage). Fixed 4 real gaps this pass: (1) gopherstack-qgh's SNS/DLQ region-unawareness (sns_delivery.go now recovers the queue's region from the subscription/redrive-policy ARN via parseQueueARNOrURL instead of always falling back to the backend default region); (2) QueueDeletedRecently was entirely unimplemented (see ops.CreateQueue); (3) DeadLetterQueueSourceArn was entirely unimplemented (see ops.ReceiveMessage); (4) the Query/XML protocol error_codes bug (see families.error_codes) where 5 pre-existing sentinel errors' already-correct legacy .Error() text was never wired into queryErrorDetails, leaking the JSON protocol's com.amazonaws.sqs# namespace into XML responses. gopherstack-qgh's remaining item (FifoThroughputLimit=perQueue) was evaluated and deliberately left open — see gaps for the specific reasoning (AWS's real per-operation-type budget matrix vs a single naive counter)."
leaks: {status: clean, note: "fixed this pass: restoreQueueFromSnapshot now seeds hasActivity so the background janitor doesn't silently ignore restored queues forever (previously an unbounded-lifetime leak of retention-expired/DLQ-eligible messages on any queue restored with pending state and no subsequent SendMessage). Also this pass: the new b.recentlyDeleted cooldown map (see ops.CreateQueue) is bounded — pruneRecentlyDeleted sweeps entries past the 60s window every janitor tick (extracted from pruneState to stay under gocognit), and checkQueueDeletedRecently also lazily self-prunes the entry it just found expired, so a backend that creates/deletes many differently-named queues over a long run does not accumulate stale map entries; covered by TestQueueDeletedRecently's RunJanitorOnceForTest assertion. Verified clean (pre-existing, unchanged): janitor ticker + StartMessageMoveTask goroutines are ctx-scoped and cancelled on Close/DeleteQueue/queue-involved-in-task; dedup maps are bounded (100k) with eviction; receiveAttempts/fifoSendTimes pruned each janitor tick; long-poll goroutines exit via the recheck-interval timer or notify-channel close, no goroutine leak on DeleteQueue mid-poll (input queue lookup re-checked each loop iteration)."}
---

## Notes

### Protocol
SQS implements **both** protocols side by side, dispatched in `handler.go`'s
`Handler()`/`RouteMatcher()`:
- **Query (XML) protocol**: `Content-Type: application/x-www-form-urlencoded` POST with an
  `Action=` form field (query.go). Legacy AWS.SimpleQueueService.* error codes.
- **JSON protocol** (`application/x-amz-json-1.1`, `X-Amz-Target: AmazonSQS.<Action>`):
  handler.go. `com.amazonaws.sqs#`-namespaced error codes (JSON `__type` field).

**Trap fixed this pass**: every Query-protocol response handler built its XML via
`marshalXML`, which prepended `xml.Header`, and then handed the bytes to echo's
`c.XMLBlob` — which **also** writes `xml.Header` before the blob. Every single
Query-protocol response (success AND error paths) therefore had **two** `<?xml ...?>`
prologs, which is not well-formed XML (a second XML declaration is a reserved/invalid
processing instruction anywhere but byte offset 0). Go's `encoding/xml` silently
tolerates it (which is why `xml.Unmarshal`-based tests never caught it), but a strict
XML parser in a non-Go AWS SDK could reject the response outright. Fixed by removing
the manual prepend from `marshalXML`, `writeQueryError`, and `buildQueryError` — the
declaration is written exactly once, by `c.XMLBlob`. See
`TestQueryProtocol_SingleXMLDeclaration`.

**Second trap fixed this pass, same family**: `queryErrorDetails` only special-cased
`ErrQueueNotFound` with a legacy `AWS.SimpleQueueService.NonExistentQueue` code and fell
through to the shared JSON-API `errorDetails` table for every other error — so the
Query/XML `<Code>` element for e.g. `PurgeQueueInProgress` literally read
`com.amazonaws.sqs#PurgeQueueInProgress`, a JSON-only Smithy shape ID with no meaning in
XML. The correct legacy codes were already encoded as the affected sentinels' own
`.Error()` text in errors.go (apparently for exactly this purpose, in an earlier pass) but
were never read by `queryErrorDetails`. `legacyQueryErrorOverride` now uses them for
`ErrInvalidBatchEntry`, `ErrTooManyEntriesInBatch`, `ErrBatchEntryIDsNotDistinct`,
`ErrPurgeQueueInProgress`, `ErrBatchRequestTooLong`, and the new `ErrQueueDeletedRecently`.
See `TestQueryErrorResponse_LegacyCodes`.

### MD5 algorithms (both SQS-specific, do not use general-purpose hashing intuition)
- `MD5OfBody` / `MD5OfMessageAttributes` / `MD5OfMessageSystemAttributes`: plain MD5 of
  the body, and MD5 of the AWS wire-packed attribute encoding (sorted attr names, each
  encoded as 4-byte-BE-length name + 4-byte-BE-length dataType + 1-byte transport type
  (1=String/Number, 2=Binary) + 4-byte-BE-length value). This is a documented AWS
  wire-integrity checksum, not a security hash — do not "fix" it to SHA-256.
- FIFO **ContentBasedDeduplication** uses **SHA-256** of the message body, NOT MD5 —
  a different algorithm for a different purpose (dedup identity vs wire checksum). Easy
  to conflate; `computeSHA256` is deliberately a separate function from
  `computeBodyChecksumMD5`.
- `MD5OfMessageAttributes` on `ReceiveMessage` must be recomputed over exactly the
  *returned* attribute subset when the caller's `MessageAttributeNames` filters out some
  attributes — reusing the send-time digest for a filtered receive would fail SDK-side
  checksum verification. `computeMD5OfSubset` handles this using pre-encoded
  per-attribute byte caches (`msg.encodedAttrs`) so repeated filtered receives don't
  re-sort/re-encode every attribute each time.

### FIFO ordering / visibility timers (the class of bug this pass focused on)
- Only **one message per MessageGroupId may be in-flight at a time** in this backend —
  confirmed this is *correct*, not an over-restriction: real AWS FIFO queues block
  further delivery from a group until the earlier message is deleted or its visibility
  expires, to guarantee a single consumer sees strict order. Do not "fix" this into
  allowing N-in-flight-per-group.
- Sends are **never** blocked by an in-flight predecessor in the same group — only
  *receives* are. This means a newer message can be sitting in `q.messages` behind an
  older, currently-in-flight message from the same group. When that older message is
  returned to the visible pool (via `ChangeMessageVisibility(0)` or automatic
  visibility-timeout expiry in the janitor's `sweepInFlight`), it **must** be reinserted
  ahead of the newer one, not appended to the tail — `requeueMessage` does a
  `sort.Search`-based insert by `SequenceNumber` (a fixed-width zero-padded decimal
  string, so lexicographic compare == numeric compare) to restore this invariant.
  `q.messages` is otherwise naturally kept in ascending SequenceNumber order because
  `SendMessage` only ever appends and `pickVisibleMessages` compacts in place without
  reordering.
- `ReceiveMessageInput.VisibilityTimeout` is a plain `int`, not `*int` — AWS's own
  `aws-sdk-go-v2` model uses `*int32` specifically to distinguish "caller didn't specify
  a value" from "caller explicitly wants 0". This codebase uses a sentinel instead
  (`NoVisibilityTimeout = -1`, exported this pass — it was `noVisibilitySet`,
  unexported, forcing external test code to hardcode the literal `-1`). **Any direct
  Go-level caller of `InMemoryBackend.ReceiveMessage` (tests, or a future cross-service
  integration) that leaves `VisibilityTimeout` unset gets an explicit 0-second
  visibility timeout, NOT the queue's configured default** — the Go zero value
  collides with a legitimate explicit value. The JSON handler (`*int`, nil-checked) and
  Query-protocol parser (empty-string-checked) both correctly translate "field absent"
  to `NoVisibilityTimeout` before calling the backend; anything bypassing those two
  front ends must do the same explicitly. This is documented on the field's/sentinel's
  doc comment in `types.go` but is still a live footgun for future Go-level callers —
  worth reconsidering a `*int` refactor in a future pass if a real cross-service caller
  ever needs this API directly (none exist today; verified via repo-wide grep).

### Locking
`InMemoryBackend` uses one coarse `lockmetrics.RWMutex` (`b.mu`) guarding the top-level
`queues`/`moveTasks` maps, PLUS a per-`Queue` `sync.Mutex` (`q.mu`) guarding that queue's
own message/dedup/in-flight state (introduced in an earlier pass, tagged `#55` in
comments, to avoid one hot queue blocking all others). The established lock order is
always `b.mu` (RLock to resolve/look up the queue) **then** `q.mu` — never the reverse;
several functions (e.g. `GetQueueAttributes`, `Snapshot`) legitimately nest `q.mu.Lock()`
inside an `b.mu.RLock()` critical section. Do not add a call path that acquires `q.mu`
first and `b.mu` afterward.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher's legacy Query-protocol branch swallowed a
body-read failure as a 404**: same survey/rationale as gopherstack-3a8t (see autoscaling's
entry). `RouteMatcher`'s form-urlencoded (Query-protocol) branch now falls back to
`service.MatchesUserAgentMarker(r.Header, "api/sqs")` (verified against the pinned
`sqs@v1.46.4/api_client.go:640` `AddSDKAgentKeyValue` call) only on the `ReadBody` failure
branch. No `ParseForm` migration needed: `handleQueryProtocol` (`query.go`) already used
`httputils.ReadBody`+`url.ParseQuery` exclusively and already wrote a typed `InternalError`
on a read failure -- only `RouteMatcher` had the bug.

**The pinned aws-sdk-go-v2 sqs client (v1.46.4) sends JSON-RPC** (`X-Amz-Target:
AmazonSQS.*`), which `RouteMatcher`'s first branch matches on the header alone, without
ever reading the body -- so a real client can't drive the Query-protocol branch this fix
targets. That branch still matters for the AWS CLI, boto3, or other non-JSON SQS callers,
so `TestRouteMatcher_OversizedQueryProtocolBodyRoutesInsteadOf404` sends a raw
form-urlencoded POST with the real `api/sqs` marker rather than going through the SDK
client, still routed through `service.NewRegistry`/`service.NewServiceRouter`. Separately,
`TestHandler_OversizedBodySurfacesInternalFailure` drives the real (JSON-RPC) SDK client
and confirms `pkgs/service.HandleTarget`'s pre-existing `ReadBody`-failure handling
(typed `InternalFailure`) was already correct and is unaffected by this change.

`TestHandler_NormalSizedBodyStillRoutes` is the regression guard. Gates: `go build`,
`go vet`, `gofmt -l` (clean), `go test -race ./services/sqs/...` (pass; fixed a goroutine
leak in the new test file by adding the package's established
`t.Cleanup(backend.Close)` for the janitor goroutine), `golangci-lint run
./services/sqs/...` (0 issues).

**Per-item-failure sweep (this pass):** checked `ChangeMessageVisibilityBatch`,
`DeleteMessageBatch`, and `SendMessageBatch` -- the three ops whose SDK output models
a per-item `Failed`/`Successful` pair (`types.BatchResultErrorEntry` alongside each
op's own `*BatchResultEntry` type). All three correctly populate `Failed` per-entry
(`message_visibility.go`'s `ChangeMessageVisibilityBatch` for invalid/not-inflight
receipt handles, `messages.go`'s `processSendMessageBatchEntries` for per-entry send
failures, `messages.go`'s `DeleteMessageBatch` for per-entry delete failures) while
still processing every other entry in the batch. No bugs found in this class; this
sweep targets a different response field than the earlier error-code-selection pass
noted above.

**`cmd/errcodeaudit` no-near-miss sweep (gopherstack-r3pr, this pass):** 2 findings,
both confirmed false positives, both on the JSON-RPC path (the pinned SDK's real
protocol; query.go's Query/XML path is unreachable by a real client and was checked
for relevance -- neither sentinel is referenced there). `ErrQueueAlreadyExists`
("QueueAlreadyExists", errors.go:12) is matched only by `errors.Is` identity in
`handler.go`'s central `errorDetails`/`sqsCoreErrorDetails` mapper, which emits the
correct wire type `com.amazonaws.sqs#QueueNameExists` -- confirmed against
`CreateQueue`'s own `deserializeOpError` (`case strings.EqualFold("QueueNameExists",
errorCode)`), and `QueueNameExists.ErrorCode()` returns `"QueueNameExists"`. The
sentinel's own literal never reaches the wire. `ErrMessageTooLarge` ("MessageTooLarge",
errors.go:20) is the same mapper shape for `SendMessage` -- mapped to
`com.amazonaws.sqs#InvalidMessageContents`, confirmed against `SendMessage`'s own
`deserializeOpError` and `InvalidMessageContents.ErrorCode() == "InvalidMessageContents"`.
Its raw sentinel text ("MessageTooLarge") does surface unmapped in
`processSendMessageBatchEntries`'s `BatchResultErrorEntry.Code` field
(`Code: err.Error()`, messages.go:641) for the `SendMessageBatch` per-entry-failure
case -- but that field lives inside a 200-response `Failed` array, not a wire error
envelope, so it has no `errors.As` ground truth (the free-form-ErrorCode-on-a-success-
response false-positive class); recorded here, not changed.

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `QueueUrl`/`QueueURL`
acronym casing gives it 1 op/handler pair needing the ambiguous fold, a
genuine collision between an exported backend method and the real
unexported handler: `GetQueueUrl`.

Verified directly: ran the unpatched tool from `ef0eef041~1` five times and
diffed against the fixed tool at HEAD. `cmd/reqfieldscan` was byte-identical
across all 5 runs and HEAD -- zero damage. `cmd/reqfielddiff` was not: old
runs found 1 or 2 findings vs 1 at HEAD; `GetQueueUrl.QueueName` flickered,
present only in some old (misresolved) runs, never at HEAD. Read the source
(handler_queues.go:137-154): `jsonGetQueueURLReq` declares `QueueName`
(`json.Unmarshal`'d) and forwards it into `GetQueueURLInput`. Confirmed
genuine -- not a bug.

Verdict: zero real bugs, safe direction only.

## cmd/errtargetaudit class A finding on ChangeMessageVisibilityBatch (2026-09-07, gopherstack-opzq)

`cmd/errtargetaudit -dir sqs` reported one class A finding: `ChangeMessageVisibilityBatch`
emits `MessageNotInflight` (`message_visibility.go:411`, via the `changeVisibility`
constructor classifier), a code `deserializeOpErrorChangeMessageVisibilityBatch` does not
declare. Compared the two deserializers directly:
`deserializeOpErrorChangeMessageVisibilityBatch` declares `UnknownError`,
`BatchEntryIdsNotDistinct`, `EmptyBatchRequest`, `InvalidAddress`, `InvalidBatchEntryId`,
`InvalidSecurity`, `QueueDoesNotExist`, `RequestThrottled`, `TooManyEntriesInBatchRequest`,
`UnsupportedOperation`; `deserializeOpErrorChangeMessageVisibility` (singular) declares
`UnknownError`, `InvalidAddress`, `InvalidSecurity`, `MessageNotInflight`,
`QueueDoesNotExist`, `ReceiptHandleIsInvalid`, `RequestThrottled`, `UnsupportedOperation`.
`MessageNotInflight` is real, but declared only on the singular op's top-level exception
list, never the batch op's.

**False positive.** `ChangeMessageVisibilityBatchOutput` (`api_op_ChangeMessageVisibilityBatch.go`)
is `{ Failed []types.BatchResultErrorEntry; Successful []types.ChangeMessageVisibilityBatchResultEntry }`.
`types.BatchResultErrorEntry` is `{ Code *string; Id *string; SenderFault bool; Message *string }`
(`types/types.go:11-32`) -- a per-entry business-data field inside a 200-response body, not a
top-level wire error. The SDK's own deserializer confirms this: `awsAwsjson10_deserializeDocumentBatchResultErrorEntry`
(`deserializers.go:4062+`) reads `Code` as a bare JSON string field with no `errors.As`/exception-type
resolution -- it is never checked against the operation's declared top-level exception list at
all, on either side of the wire. This repo already emits at exactly that layer:
`message_visibility.go:411-419`'s `ChangeMessageVisibilityBatch` captures `changeVisibility`'s
error into `out.Failed = append(..., BatchErrorEntry{Code: "MessageNotInflight", ...})` and
continues the loop -- it does not `return nil, err`. Contrast the singular op
(`message_visibility.go:334`, `return ErrMessageNotInflight`), which *does* propagate to the
top level, where `handler.go:492-493`'s `sqsCoreErrorDetails` maps it to the real namespaced
wire code `com.amazonaws.sqs#MessageNotInflight` in the `__type` field -- correctly declared by
the singular op's own deserializer. The two ops emit `MessageNotInflight` at two different wire
layers on purpose; the tool's constructor-classifier walk doesn't distinguish a per-entry
`Failed`-slice append from a top-level `return ..., err` and flagged both as "this operation's
top-level error code." No code change made.

Added `TestHandlerActions_ChangeMessageVisibilityBatch/partial_failure`
(`handler_message_visibility_test.go`) driving the JSON handler directly: a batch of two
good entries plus one entry with an invalid receipt handle returns HTTP 200 with
`Failed: [{Id: "bad", Code: "MessageNotInflight", SenderFault: true}]` (no
`com.amazonaws.sqs#` prefix, matching the SDK's plain-string per-entry field) and
`Successful: [{Id: "good-1"}, {Id: "good-2"}]`, then re-receives both messages to confirm
their visibility actually changed. Neutered to confirm the test isn't hollow: reverting the
literal to `"WrongCode"` compiles and fails the test on the `Code` assertion; reverting the
per-entry `Failed` append to an early `return nil, err` (simulating a wholesale-fail bug)
compiles and fails the test on the HTTP-200 assertion. Both reverted back to the original
before running the gates below.

Gates: `go test -race -count=1 ./services/sqs/...` (pass), `golangci-lint run
./services/sqs/...` (0 issues).

Verdict: tool false positive, current behaviour correct -- no fix needed.
