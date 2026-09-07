---
service: azureservicebus
sdk_module: azure-sdk-for-go/sdk/messaging/azservicebus@v1.10.0   # is not a go.mod dependency of
                                                                    # this repo -- it is an AMQP-only
                                                                    # client with no HTTP/REST
                                                                    # transport, so it cannot be
                                                                    # pointed at this REST emulator
                                                                    # and nothing imports it (see
                                                                    # families.sdk_compat below).
                                                                    # Version pin is for reference
                                                                    # only; audited by reading the
                                                                    # module source directly.
last_audit_commit: a5f048bc5
last_audit_date: 2026-09-07
overall: B
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateQueue: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /<queue>. Entity-kind (queue vs topic) resolution order: ?type=topic/?type=queue query param (either value wins outright), then a real Atom+XML parse of the request body (yielding LockDuration/MaxDeliveryCount/DefaultMessageTimeToLive too), then a case-insensitive substring sniff as a last resort, then default-to-queue -- see families.entity_kind_detection. Per-entity LockDuration/MaxDeliveryCount/DefaultMessageTimeToLive are now stored and honored (see families.lock_duration_and_long_poll); an absent/unparseable property falls back to the package-level default. LockDuration above MaxLockDuration (5 minutes) is rejected 400 Bad Request rather than silently accepted -- see the Notes section."}
  DeleteQueue: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<queue>. Also matches a topic name (DeleteEntity tries queue then topic) -- see DeleteTopic."}
  GetQueue: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<queue>. Returns an Atom+XML entry with a QueueDescription (LockDuration/MaxDeliveryCount/DefaultMessageTimeToLive). 404 if the name exists as neither a queue nor a topic (getEntity tries queue then topic, matching deleteEntity's order)."}
  ListQueues: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /$Resources/Queues, real Service Bus's management-API listing endpoint. Returns an Atom+XML feed of entries, sorted by name. No pagination -- returns every queue in one page, matching services/azurequeue's/azuretable's List fidelity bar (see PARITY.md's Get/List fidelity note)."}
  CreateTopic: {wire: ok, errors: ok, state: ok, persist: ok, note: "Same PUT /<name> endpoint as CreateQueue; disambiguated by the same resolution order, see CreateQueue's note. Only DefaultMessageTimeToLive is meaningful for a topic (applied as the per-message TTL cap at Send time, since a topic never itself holds messages)."}
  DeleteTopic: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<topic>, deletes all subscriptions and their messages too."}
  GetTopic: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<topic>. Returns an Atom+XML entry with a TopicDescription (DefaultMessageTimeToLive)."}
  ListTopics: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /$Resources/Topics. Same shape/fidelity as ListQueues."}
  CreateSubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /<topic>/subscriptions/<name>. Request body may carry a SqlFilter rule (RuleDescription); it is read (via the same Atom+XML parse used for entity-kind detection, to also extract LockDuration/MaxDeliveryCount) and the filter itself discarded -- every subscription still behaves as Service Bus's default TrueFilter (match-all). See families.filter_evaluation. LockDuration above MaxLockDuration (5 minutes) is rejected 400 Bad Request, same as CreateQueue -- see the Notes section."}
  DeleteSubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<topic>/subscriptions/<name>."}
  GetSubscription: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<topic>/subscriptions/<name>. Returns an Atom+XML entry with a SubscriptionDescription (LockDuration/MaxDeliveryCount)."}
  ListSubscriptions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<topic>/subscriptions (list) -- fell out naturally from the path parser alongside $Resources, so it's included even though it wasn't explicitly required. Same feed shape/fidelity as ListQueues/ListTopics, scoped to one topic."}
  SendMessage: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /<queue-or-topic>/messages. Message body is opaque bytes; BrokerProperties request header (JSON) supplies MessageId/Label/CorrelationId/ReplyTo/SessionId/TimeToLive (seconds). Sending to a topic fans the message out to every current subscription as an independent copy (services/sns's topic/subscription bookkeeping used as the structural reference). Sending directly to a subscription path is rejected 405, matching real Service Bus (subscriptions are receive-only). An explicit TimeToLive is now capped at the target entity's configured DefaultMessageTimeToLive, matching real Service Bus; an absent one uses that TTL outright."}
  PeekLockMessage: {wire: ok, errors: ok, state: ok, persist: n/a, note: "POST /<queue-or-topic-subscription>[/$DeadLetterQueue]/messages/head. Destructive read: locks the oldest available message for the entity's own configured LockDuration (falling back to DefaultLockDuration, 60s, if unconfigured -- see families.lock_duration_and_long_poll) and returns it with a BrokerProperties response header (LockToken/LockedUntilUtc/SequenceNumber/DeliveryCount) plus a Location header giving the complete/abandon URI. The real API's long-poll \"?timeout=<seconds>\" query parameter is now implemented (clamped to MaxPeekLockWaitTimeout, 30s) -- see families.lock_duration_and_long_poll."}
  CompleteMessage: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<queue-or-topic-subscription>[/$DeadLetterQueue]/messages/<id>/<locktoken>. A stale/unknown/expired lock returns 410 Gone (MessageLockLost), matching real Service Bus's MessageLockLostException mapping."}
  AbandonMessage: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT on the same path as CompleteMessage. Releases the lock immediately (no visibility delay, unlike Azure Queue's UpdateMessage) so the message becomes available for redelivery right away, waking any long-polling PeekLock waiter; if DeliveryCount has reached the entity's configured MaxDeliveryCount (falling back to 10 if unconfigured) the message is moved to the dead-letter sub-queue instead of being released."}
  DeadLetter: {wire: partial, errors: ok, state: ok, persist: ok, note: "Reached via the $DeadLetterQueue path segment on PeekLock/Complete/Abandon, and automatically by the Janitor (lock-expiry with DeliveryCount exhausted, or TTL expiry -- see families.dead_lettering). Still no explicit \"dead-letter this message now via BrokerProperties on the Abandon PUT\" client operation: research into real Service Bus's REST wire contract (the current learn.microsoft.com/rest/api/servicebus/unlock-message reference page, which documents that PUT explicitly as taking no request body and no BrokerProperties at all) did not turn up authoritative confirmation that this shape exists on the REST surface gopherstack emulates -- see the DeadLetter research note below and families.dead_lettering. Implementing nothing beyond what already existed, rather than inventing an unconfirmed endpoint."}
families:
  sdk_compat: {status: gap, note: "azure-sdk-for-go/sdk/messaging/azservicebus is an AMQP 1.0 client (built on github.com/Azure/go-amqp) with no HTTP/REST transport option -- it cannot be pointed at this (or any) REST-only Service Bus emulator. This was flagged as a blocker per AZURE.md's M5 testing-strategy note; test/integration/azureservicebus_test.go therefore exercises the REST surface directly via net/http instead of azservicebus, with a comment explaining why. AMQP 1.0 support is out of scope for this MVP per AZURE.md section 9's M5 rationale (sessions and full AMQP compatibility are both explicitly deferred there)."}
  entity_kind_detection: {status: ok, note: "Real Service Bus's management REST API disambiguates queue vs. topic creation by the root XML element name in an Atom+XML entry body (<QueueDescription> vs <TopicDescription>). handler.go's resolveEntityKind now does a real encoding/xml parse (atom.go's parseAtomEntityBody) of that shape, namespace-tolerant (see the Notes section below), before falling back to the previous case-insensitive substring sniff (kept only for the REST integration test's deliberately simplified bodies) and finally defaulting to a queue. The ?type= query-param escape hatch recognizes both \"topic\" and \"queue\" explicitly (a code-review pass caught that only \"topic\" was wired up, contradicting the doc comment). Malformed XML never fails the create -- it simply falls through the same resolution chain."}
  filter_evaluation: {status: deferred, note: "Subscription SQL-filter rules (SqlFilter/CorrelationFilter) are accepted structurally in CreateSubscription's request body but never parsed or evaluated -- every subscription receives every message sent to its topic, as if it had the default TrueFilter rule. Real filter evaluation (a SQL-subset parser/evaluator, following services/azuretable's $filter or services/cosmosdb's SQL-subset precedent) is deferred, per AZURE.md section 9's M5 scope note."}
  sessions_and_amqp: {status: deferred, note: "Sessions (ordered/exclusive per-SessionId message groups) and the full AMQP 1.0 protocol are explicitly out of scope for this MVP per AZURE.md section 9's M5 rationale. SessionId is accepted and stored/round-tripped on messages (BrokerProperties.SessionId) but has no session-lock/FIFO/exclusivity semantics."}
  lock_duration_and_long_poll: {status: ok, note: "LockDuration/MaxDeliveryCount/DefaultMessageTimeToLive are now per-entity properties (EntityConfig, stored on storedQueue/storedTopic/storedSubscription and round-tripped through backendSnapshot), parsed from the Atom+XML create-request body as ISO 8601 duration strings (LockDuration/DefaultMessageTimeToLive) or a plain integer (MaxDeliveryCount) via a small hand-written parser (iso8601.go's parseISO8601Duration) -- no new module dependency. An absent/unparseable property falls back to the existing package-level default. LockDuration is additionally validated at CreateQueue/CreateSubscription time against MaxLockDuration (5 minutes, real Service Bus's documented maximum) and rejected with 400 Bad Request if it exceeds that -- a fidelity improvement over the initial pass. PeekLock's long-poll \"?timeout=<seconds>\" query parameter is now implemented: gopherstack waits up to that many seconds (clamped to MaxPeekLockWaitTimeout, 30s) for a message to arrive, via a broadcast notify channel woken by Send, by Abandon on EITHER of its two outcomes (released back to availability, or moved to dead-letter), and by the Janitor's sweep on any of its three outcomes (lock release, lock-expiry dead-letter, TTL-expiry dead-letter) -- so a long-poll against $DeadLetterQueue wakes immediately when the Janitor or an Abandon dead-letters a message, not just on the live-list path. Mirrors services/sqs's ReceiveMessage/pollReceive/receiveOnce shape but additionally honoring the request's context for prompt cancellation on client disconnect (an improvement over the SQS precedent), and correctly sizes its first recheck-timer tick to min(timeout, 1s) so sub-second timeouts aren't stretched to a full second. See the Notes section for the 30s cap's rationale (corrected from an earlier draft) and the ISO 8601 parser's overflow clamp."}
  dead_lettering: {status: ok, note: "A background Janitor (janitor.go, 10s default interval) proactively releases expired locks (dead-lettering on the entity's configured MaxDeliveryCount exhaustion) and moves TTL-expired live messages straight to dead-letter, mirroring services/azurequeue's Janitor shape but adapted to Service Bus's richer lock/delivery-count/dead-letter model instead of a flat TTL sweep. A message that expires while already sitting in the dead-letter sub-queue is dropped outright (nowhere further to move it). See the DeadLetter op note above for the still-unimplemented explicit client-initiated dead-letter operation."}
  auth: {status: partial, note: "Authorization: SharedAccessSignature sr=<resource>&sig=<hmac>&se=<expiry>&skn=<keyname> is always structurally parsed (sas.go's ParseSASAuthorization extracts key name, resource scope, and expiry). Cryptographic HMAC-SHA256 verification (VerifySAS) exists and is unit-tested but is opt-in via Handler.WithSASValidation / --azure-servicebus-validate-sas (mirroring services/s3's WithPresignValidation and Blob/Queue/Table's WithSharedKeyValidation) -- off by default, an absent/malformed/wrong-signature header is accepted."}
  routing_isolation: {status: ok, note: "Runs on its own dedicated *http.Server, bound synchronously in StartWorker to a fixed port (default 10003 via --azure-servicebus-port/AZURE_SERVICEBUS_PORT; no fallback pool -- fails fast if unavailable, mirroring services/azureblob/azurequeue/azuretable), never registered into the shared AWS single-port Router. cli.go's reserveFixedServicePorts additionally reserves this port in the shared PortAlloc pool at startup."}
  observability: {status: ok, note: "StartWorker wraps its Echo handler with telemetry.WrapEchoHandler so ExtractOperation/ExtractResource feed Prometheus metrics, and derives its listener logger via logger.WithWorker(ctx, \"azureservicebus\", \"listener\")."}
gaps:
  - "azservicebus (the only azure-sdk-for-go Service Bus client) is AMQP-only and cannot be pointed at this REST emulator -- see families.sdk_compat. This is a genuine SDK-compatibility limitation, not an MVP scope cut; a real AMQP 1.0 listener would be required to support it, which AZURE.md section 9's M5 rationale explicitly defers."
  - "No SQL-filter rule evaluation for subscriptions -- every subscription is effectively TrueFilter (match-all). See families.filter_evaluation."
  - "No sessions (ordered/exclusive per-SessionId delivery) -- SessionId round-trips but has no locking/FIFO semantics. See families.sessions_and_amqp."
  - "No explicit client-initiated DeadLetter operation. Research into real Service Bus's REST wire contract did not turn up authoritative confirmation of the BrokerProperties-DeadLetterReason-on-the-Abandon-PUT shape (the current official REST API reference for the Unlock Message operation, which shares that same PUT .../messages/<id>/<token> URI, documents it as taking no request body at all) -- rather than inventing an unconfirmed endpoint, this gap is left as-is (still only reached via the $DeadLetterQueue path or the Janitor's automatic sweep) pending a human decision on how to proceed. See the DeadLetter research note in the Notes section below."
  All gaps above are intentional MVP scope per AZURE.md section 9's M5 entry (or, for DeadLetter, a deliberate pause pending human input), not oversights.
deferred:
  - "Initial implementation pass (2026-09-06): seeded this service from scratch per AZURE.md M5 (see AZURE.md section 9). Structurally mirrors services/azurequeue's pop-receipt/visibility-timeout pattern for message locks and services/sns's topic/subscription fan-out bookkeeping; no prior audit history to reconcile."
  - "Follow-up pass (2026-09-07): closed five bounded parity gaps -- per-entity LockDuration/MaxDeliveryCount/DefaultMessageTimeToLive, PeekLock long-poll, full Atom+XML entity-kind parsing, and Get/List operations. Left the explicit client-initiated DeadLetter gap as-is after research found the wire shape unconfirmed (see gaps above); SQL-filter evaluation, sessions, and the AMQP/REST SDK-compatibility gap remain deferred per AZURE.md section 9."
leaks: {status: clean, note: "The dedicated *http.Server started by StartWorker is stopped by Shutdown via srv.Shutdown(ctx) (falling back to srv.Close() on a graceful-shutdown error, both logged), mirroring services/azurequeue. The Janitor's single background goroutine (started from StartWorker) runs on a worker.Group ticker that stops when its context is cancelled -- verified by leak_test.go starting and immediately tearing one down without leaking."}
---

## Notes

### Why Service Bus gets its own port, and why 10003 specifically
Azure Service Bus has no Azurite equivalent to mirror a port number from, so
`10003` is simply gopherstack's own next-available slot after Azure Table's
`10002`, following the same fixed-port, synchronous-bind, fail-fast pattern
as `services/azureblob`/`azurequeue`/`azuretable` (see AZURE.md section 4 and
section 9's M5 entry). `cli.go`'s `reserveFixedServicePorts` reserves it in
the shared `PortAlloc` pool at startup so an unrelated `Acquire` caller never
collides with it.

### Why the integration test doesn't use azure-sdk-for-go's azservicebus
`azure-sdk-for-go/sdk/messaging/azservicebus` is built entirely on top of
`github.com/Azure/go-amqp` -- there is no HTTP/REST transport path in that
client at all, only AMQP 1.0 over TLS (or WebSockets carrying AMQP frames).
Since gopherstack's Service Bus emulation deliberately targets the Brokered
Messaging REST API (see AZURE.md section 9's M5 rationale: implementing a
binary AMQP 1.0 stack would be a materially larger and different-shaped
effort than every other Azure service in this repo), there is no SDK client
that can be pointed at it unmodified. `test/integration/azureservicebus_test.go`
therefore talks to the REST surface directly via `net/http`, with a comment
at the top of the file explaining this exact limitation. This is flagged as
the AMQP/REST compatibility gap this milestone's brief asked to escalate
rather than silently work around.

### Queue/topic entity-kind detection
Real Service Bus's management REST API is Atom+XML and disambiguates a `PUT
/<name>` queue-vs-topic create by the request body's root element name
(`<QueueDescription>` vs `<TopicDescription>`). `createEntity`/
`resolveEntityKind` (`handler.go`) now does a real `encoding/xml` parse of
that shape (`atom.go`'s `parseAtomEntityBody`), in this resolution order:
(1) an explicit `?type=topic` query param always wins; (2) a successful
Atom+XML parse determines the kind and also yields the create body's
`LockDuration`/`MaxDeliveryCount`/`DefaultMessageTimeToLive` properties (see
the per-entity configuration note below); (3) the previous case-insensitive
substring sniff for `"topicdescription"`, kept as a last resort for the REST
integration test's deliberately simplified hand-built bodies (there's no SDK
that can generate a proper Atom+XML body against this REST-only emulator --
see `families.sdk_compat`); (4) default to a queue. Malformed XML never
fails the create -- `xml.Unmarshal` returning an error simply falls through
to (3)/(4), matching this repo's permissive-by-default philosophy. The same
parser serves `createSubscription` for a `SubscriptionDescription`'s
`LockDuration`/`MaxDeliveryCount`; any nested `<RuleDescription>`/`<SqlFilter>`
is structurally ignored by `encoding/xml` (no matching struct field), which
is exactly the existing "accept and discard" filter behavior.

**Namespace tolerance.** Real Service Bus's `QueueDescription`/
`TopicDescription`/`SubscriptionDescription` elements are seen in the wild
under two different namespace URIs --
`http://schemas.microsoft.com/netservices/2010/10/servicebus/connect` (newer)
and `http://schemas.microsoft.com/servicebus/2010/10/` (older). Rather than
picking one and rejecting the other, every parsing struct field tag in
`atom.go` omits the namespace prefix entirely (e.g. `` `xml:"QueueDescription"` ``
rather than a namespace-qualified tag); Go's `encoding/xml` matches such a
tag purely on local element name regardless of the element's actual
namespace, which is exactly the tolerance wanted here. `atom.go`'s own
*response*-side serialization (Get/List) has to commit to emitting one
namespace, and uses the newer one (`sbDescriptionNS`).

### Per-entity LockDuration/MaxDeliveryCount/DefaultMessageTimeToLive
`EntityConfig` (`interfaces.go`) now holds these three properties per queue/
topic/subscription (`storedQueue`/`storedTopic`/`storedSubscription`'s
`Config` field, round-tripped through `backendSnapshot` automatically since
it's a plain exported struct field), parsed out of the Atom+XML create-request
body (see above). A zero-valued field (absent from the body, or present but
unparseable) falls back to the existing package-level default
(`DefaultLockDuration`/`MaxDeliveryCount`/`DefaultMessageTTL`) via
`EntityConfig`'s unexported `lockDuration()`/`maxDeliveryCount()`/
`defaultMessageTTL()` accessor methods -- callers never see the zero value
directly. `PeekLock` uses the entity's `LockDuration` whenever the caller
passes a non-positive `lockDuration` (the backend resolves it, not the
handler -- see `StorageBackend.PeekLock`'s doc comment for why this shape was
chosen over threading it through the handler); `Abandon` and the Janitor's
lock-expiry sweep both use the entity's `MaxDeliveryCount` for the
dead-letter-on-exhaustion decision. `Send` uses the target entity's
`DefaultMessageTimeToLive` when the caller supplies no per-message
`TimeToLive`, and **caps** an explicit per-message `TimeToLive` at that same
value if it's larger -- matching real Service Bus's documented behavior that
a message's effective TTL is `min(requested TTL, entity's
DefaultMessageTimeToLive)`.

**The ISO 8601 duration parser and its overflow clamp.** `LockDuration` and
`DefaultMessageTimeToLive` are ISO 8601 duration strings on the wire (e.g.
`PT1M`, `PT30S`, `P14D`); `MaxDeliveryCount` is a plain integer and needs no
such parsing. Rather than add a new module dependency, `iso8601.go` implements
a small, purpose-built `parseISO8601Duration`/`formatISO8601Duration` pair
supporting exactly the `PnDTnHnMnS`/`PTnHnMnS` subset (with optional
fractional seconds) that Service Bus itself ever emits or expects for these
two properties -- weeks, months, and years are deliberately unsupported.
Real Service Bus uses `P10675199DT2H48M5.4775807S` as its documented
"infinite"/max-value sentinel for these fields (it's .NET's
`TimeSpan.MaxValue` spelled out in ISO 8601); parsed literally, that value
would overflow `time.Duration`'s int64-nanosecond range. `parseISO8601Duration`
detects this (the accumulated nanosecond total reaching or exceeding
`math.MaxInt64`) and clamps to the maximum representable `time.Duration`
(~292 years) rather than erroring -- a deliberate, documented approximation
rather than silent data loss, and unit-tested (`iso8601_internal_test.go`)
both for that exact sentinel string and for values beyond it. The boundary
comparison is deliberately `>=`, not `>`: `float64(math.MaxInt64)` rounds up
to exactly 2^63 (one more than the actual maximum, 2^63-1, which has no exact
float64 representation), so a `nanos == 2^63` input would fail a plain `>`
check and fall through to converting an out-of-range float64 to `int64` --
implementation-defined behavior that yields a negative duration on common
platforms. A code-review pass caught this off-by-one before it shipped;
`iso8601_internal_test.go` has a case for exactly the boundary value.

### LockDuration's 5-minute maximum
Real Service Bus caps a queue's or subscription's `LockDuration` at 5 minutes
(its documented default is 1 minute); a create request specifying more is
rejected. `handler.go`'s `validateEntityConfig` (called from `createEntity`
for a queue and unconditionally from `createSubscription`) now enforces the
same bound via the new `MaxLockDuration` constant, returning 400 Bad Request
-- matching real Service Bus's own validation instead of silently persisting
an unenforceable value. This is deliberately narrow: only `LockDuration`'s
upper bound and `MaxDeliveryCount`'s non-negativity are checked; no other
property is bounded, and nothing is clamped (out-of-range values are
rejected outright, not silently corrected).

### PeekLock long-poll ("?timeout=") and its 30-second cap
Real Service Bus clients pass `?timeout=<seconds>` on `POST .../messages/head`
to wait up to that long for a message before giving up, rather than getting
an immediate 204. This is now implemented: `messageQueue` (`store.go`) gained
a broadcast generation `notify chan struct{}`, closed and replaced under the
backend's write lock (`broadcastLocked`) by anything that makes a new message
newly available on *either* its live list or its dead-letter sub-queue --
`Send`, `Abandon` (both its release-back-to-availability outcome and its
dead-letter-on-exhaustion outcome), and the Janitor's sweep (both its
lock-release path and its lock-expiry/TTL-expiry dead-letter paths) all call
it. One channel is shared by both lists, so a dead-letter transition also
wakes a live-list waiter and vice versa; the wrong-list waiter simply
re-checks, finds nothing, and goes back to sleep -- a harmless spurious
wakeup, not a bug, and not worth a second channel. `PeekLockWait`
(`message_ops.go`) mirrors `services/sqs`'s
`ReceiveMessage`/`pollReceive`/`receiveOnce` long-poll shape closely: an
initial immediate attempt (`peekLockOnce`, which -- like SQS's `receiveOnce`
-- captures the entity's current `notify` channel under the *same* lock as
the failed read, closing the lost-wakeup race window), then a wait loop
selecting on that channel, a recheck-timer backstop (sized to
`min(timeout, 1s)` so a sub-second timeout isn't stretched out to a full
second), and the overall deadline (re-checked before every subsequent
`peekLockOnce` attempt, so a notification racing the deadline can't
resurrect an already-expired wait into a false success), with the same
careful timer stop-and-drain discipline SQS uses before each `Reset`.

**Why 30 seconds.** `MaxPeekLockWaitTimeout` is a deliberate cap on
server-side resource use per in-flight long-poll request (one goroutine and
one held connection for up to this long), chosen to match real Service Bus's
own documented 30-second long-poll/operation-timeout value rather than SQS's
unrelated 20-second SQS-specific maximum. This is **not** related to
`azureServiceBusReadTimeout` (60s): that setting is `http.Server.ReadTimeout`,
which only bounds how long reading the incoming request (headers/body) may
take -- it does not bound handler execution time, and this server sets
neither a `WriteTimeout` nor any handler-level deadline. A long-poll
handler blocking for the full 30s cap is not at risk of being torn down by
any `http.Server` timeout; the wait is bounded purely by the cap itself and
by request-context cancellation on client disconnect. (An earlier version of
this note incorrectly attributed the 30s choice to avoiding a race with
`ReadTimeout` -- that reasoning was wrong and has been corrected here.) A
`?timeout=` above the cap is clamped down to it (not rejected), matching
this repo's permissive-by-default stance elsewhere.

**A deliberate improvement over the SQS precedent: context-awareness.**
SQS's `pollReceive`/`receiveOnce` take no `context.Context` at all --the wait
loop only ever exits on deadline or message arrival. `PeekLockWait` instead
takes a `ctx context.Context` (the handler passes `c.Request().Context()`)
and selects on `ctx.Done()` too, so a client that disconnects mid-long-poll
releases the waiting goroutine immediately rather than holding it (and the
underlying connection's resources) until the full timeout elapses.

### Explicit client-initiated DeadLetter: research finding
The brief asked to verify, before implementing, whether real Service Bus's
Brokered Messaging REST API expresses an explicit "dead-letter this message
now" client operation as a `PUT` on the same lock URI Abandon uses
(`.../messages/<id>/<token>`), carrying `DeadLetterReason`/
`DeadLetterErrorDescription` in a `BrokerProperties` *request* header. That
research did not confirm this shape:

- The current official REST API reference page for this exact endpoint,
  [Unlock Message](https://learn.microsoft.com/en-us/rest/api/servicebus/unlock-message)
  (`PUT .../messages/{messageId}/{lockToken}`), documents its Request Body as
  **"None"** and lists no `BrokerProperties` (or any other) request header
  beyond `Authorization`. It does not mention dead-lettering as a possible
  outcome of this call at all.
- No separate "Deadletter Message" (or similarly named) REST operation page
  exists in Microsoft's current REST API reference for Service Bus; searching
  both the docs site and its source repository turned up nothing.
- The `DeadLetterReason`/`DeadLetterErrorDescription` *properties themselves*
  are real and well-documented -- they appear on a dead-lettered message once
  it's in the DLQ (e.g. `ServiceBusReceivedMessage.DeadLetterReason` in the
  .NET SDK) -- but every source found describes them as populated by the
  *SDK's* `DeadLetterMessageAsync`/`DeadLetter()` call, which is itself
  layered on AMQP 1.0's message-disposition mechanism (a `<modified>` or
  `<rejected>` outcome with annotations), not as a REST wire-level request
  header on a documented HTTP endpoint.

Given the brief's explicit instruction not to invent an unconfirmed
endpoint, **nothing was implemented for this gap.** It remains exactly as
before: dead-lettering is only reachable via the `$DeadLetterQueue` path
segment on PeekLock/Complete/Abandon, or automatically via the Janitor's
lock-expiry/TTL sweep. This is flagged back to a human to weigh in on: either
(a) accept that this really is an AMQP-only capability with no REST
equivalent (consistent with `families.sdk_compat`'s existing AMQP/REST
distinction), or (b) provide a primary source confirming the REST wire shape
so it can be implemented with confidence.

### Get/List operations and their fidelity bar
`GET /<queue-or-topic>`, `GET /<topic>/subscriptions/<name>`,
`GET /$Resources/Queues`, `GET /$Resources/Topics`, and (falling out
naturally from the same path-parsing work, so included even though only
"nice to have") `GET /<topic>/subscriptions` are all new. `$Resources/*` is
matched and routed *before* any generic `/<entity>` parsing in
`parseRequestPath` -- a queue or topic literally named `$Resources` is not a
real-world concern, but the precedence is explicit and commented rather than
accidental. Response `Content-Type` matches real Service Bus:
`application/atom+xml;type=entry;charset=utf-8` for a single Get,
`application/atom+xml;type=feed;charset=utf-8` for a List.

**Fidelity bar.** `services/azurequeue`'s `ListQueues` and
`services/azuretable`'s `ListTables`/`GetEntity`/`QueryEntities` are all
un-paginated, single-page, name-sorted listings with no metadata beyond a
few flat fields (see their own PARITY.md notes) -- there is no Atom+XML
envelope in either sibling, since Azure Storage's (Blob/Queue/Table) REST
surface isn't Atom+XML to begin with. Service Bus's *real* management API
genuinely is Atom+XML, unlike those siblings, so this MVP does implement a
real (if minimal) `<entry>`/`<feed>` envelope with a `<content
type="application/xml">` holding a `QueueDescription`/`TopicDescription`/
`SubscriptionDescription` -- matching Service Bus's actual wire shape more
closely than the Storage siblings' pattern would allow. Within that
envelope, though, the same "flat, un-paginated, sorted-by-name, only the
properties this MVP already models" simplification the siblings use is kept:
no `updated`/`id`/`author` Atom boilerplate beyond `<title>`, no
continuation tokens, and no properties beyond
`LockDuration`/`MaxDeliveryCount`/`DefaultMessageTimeToLive`. Duration
fields are serialized back to ISO 8601 via `formatISO8601Duration`
(`iso8601.go`), unit-tested for round-trip fidelity against
`parseISO8601Duration`.

### Auth
`Authorization: SharedAccessSignature sr=...&sig=...&se=...&skn=...` is
always structurally parsed (`sas.go`'s `ParseSASAuthorization`) for its key
name and resource scope, but a missing, malformed, or (when validation is
off) incorrect signature is still accepted -- matching this repo's
permissive-by-default philosophy for every other Azure service.
Cryptographic verification (`VerifySAS`) is opt-in via
`Handler.WithSASValidation` / `--azure-servicebus-validate-sas`, mirroring
`services/s3`'s `WithPresignValidation` and Blob/Queue/Table's
`WithSharedKeyValidation`. `DefaultNamespace`/`DefaultKeyName`/`DefaultKeyValue`
ship a fixed dev identity (analogous to Azurite's `devstoreaccount1`) so a
`Endpoint=sb://sbemulatorns.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=...`-shaped
connection string has a stable, documented default to point at.

### Message locks reuse Azure Queue's pop-receipt pattern, adapted
A message's `LockToken` plays the same role as Azure Queue's `PopReceipt`:
issued fresh on every successful `PeekLock`, required to match on
`Complete`/`Abandon`, and rejected with a specific error
(`ErrLockTokenMismatch`/`ErrMessageNotLocked` here, `ErrPopReceiptMismatch`
there) otherwise. The key difference is what happens on release: Azure
Queue's `UpdateMessage` re-hides a message behind a new visibility timeout,
while Service Bus's `Abandon` makes the message immediately available again
(no re-hide delay) -- and additionally checks `DeliveryCount` against
`MaxDeliveryCount` to decide whether to dead-letter it instead of releasing
it, which Azure Queue has no equivalent of.

## More

- [Full parity audit](PARITY.md)
- [All services](../../README.md#services)
