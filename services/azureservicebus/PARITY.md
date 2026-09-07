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
last_audit_commit: 8e0c328ac
last_audit_date: 2026-09-06
overall: C
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateQueue: {wire: partial, errors: ok, state: ok, persist: ok, note: "PUT /<queue>. Entity-kind (queue vs topic) is sniffed from the request body (case-insensitive substring match on \"TopicDescription\") rather than a full Atom+XML parse, or the query param ?type=topic as an MVP escape hatch -- see families.entity_kind_detection. No queue metadata (LockDuration, MaxDeliveryCount, etc.) is stored; all entities use the fixed package-level defaults (DefaultLockDuration, MaxDeliveryCount)."}
  DeleteQueue: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<queue>. Also matches a topic name (DeleteEntity tries queue then topic) -- see DeleteTopic."}
  CreateTopic: {wire: partial, errors: ok, state: ok, persist: ok, note: "Same PUT /<name> endpoint as CreateQueue; disambiguated by body sniff/query param, see CreateQueue's note."}
  DeleteTopic: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<topic>, deletes all subscriptions and their messages too."}
  CreateSubscription: {wire: partial, errors: ok, state: ok, persist: ok, note: "PUT /<topic>/subscriptions/<name>. Request body may carry a SqlFilter rule (RuleDescription); it is read and discarded -- every subscription behaves as Service Bus's default TrueFilter (match-all). See families.filter_evaluation."}
  DeleteSubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<topic>/subscriptions/<name>."}
  SendMessage: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /<queue-or-topic>/messages. Message body is opaque bytes; BrokerProperties request header (JSON) supplies MessageId/Label/CorrelationId/ReplyTo/SessionId/TimeToLive (seconds). Sending to a topic fans the message out to every current subscription as an independent copy (services/sns's topic/subscription bookkeeping used as the structural reference). Sending directly to a subscription path is rejected 405, matching real Service Bus (subscriptions are receive-only)."}
  PeekLockMessage: {wire: partial, errors: ok, state: ok, persist: n/a, note: "POST /<queue-or-topic-subscription>[/$DeadLetterQueue]/messages/head. Destructive read: locks the oldest available message for DefaultLockDuration (60s, fixed -- real Service Bus's LockDuration is a per-entity property this MVP doesn't model) and returns it with a BrokerProperties response header (LockToken/LockedUntilUtc/SequenceNumber/DeliveryCount) plus a Location header giving the complete/abandon URI. No support for the real API's long-poll \"timeout\" query parameter -- returns 204 immediately if no message is available rather than waiting. See families.lock_duration_and_long_poll."}
  CompleteMessage: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<queue-or-topic-subscription>[/$DeadLetterQueue]/messages/<id>/<locktoken>. A stale/unknown/expired lock returns 410 Gone (MessageLockLost), matching real Service Bus's MessageLockLostException mapping."}
  AbandonMessage: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT on the same path as CompleteMessage. Releases the lock immediately (no visibility delay, unlike Azure Queue's UpdateMessage) so the message becomes available for redelivery right away; if DeliveryCount has reached MaxDeliveryCount (10, fixed) the message is moved to the dead-letter sub-queue instead of being released."}
  DeadLetter: {wire: partial, errors: ok, state: ok, persist: ok, note: "Reached via the $DeadLetterQueue path segment on PeekLock/Complete/Abandon, and automatically by the Janitor (lock-expiry with DeliveryCount exhausted, or TTL expiry -- see families.dead_lettering). There is no explicit \"dead-letter this message now\" client operation (real Service Bus's DeadLetter() SDK call, which the AMQP-only azservicebus can't be exercised through here anyway -- see families.sdk_compat)."}
families:
  sdk_compat: {status: gap, note: "azure-sdk-for-go/sdk/messaging/azservicebus is an AMQP 1.0 client (built on github.com/Azure/go-amqp) with no HTTP/REST transport option -- it cannot be pointed at this (or any) REST-only Service Bus emulator. This was flagged as a blocker per AZURE.md's M5 testing-strategy note; test/integration/azureservicebus_test.go therefore exercises the REST surface directly via net/http instead of azservicebus, with a comment explaining why. AMQP 1.0 support is out of scope for this MVP per AZURE.md section 9's M5 rationale (sessions and full AMQP compatibility are both explicitly deferred there)."}
  entity_kind_detection: {status: partial, note: "Real Service Bus's management REST API disambiguates queue vs. topic creation by the root XML element name in an Atom+XML entry body (<QueueDescription> vs <TopicDescription>). This MVP does a case-insensitive substring sniff of the raw body for \"topicdescription\" (or an explicit ?type=topic query param for callers building requests by hand, e.g. the REST integration test) instead of a full Atom+XML parser -- sufficient for the MVP's HTTP surface but not a faithful implementation of the real content-negotiation contract."}
  filter_evaluation: {status: deferred, note: "Subscription SQL-filter rules (SqlFilter/CorrelationFilter) are accepted structurally in CreateSubscription's request body but never parsed or evaluated -- every subscription receives every message sent to its topic, as if it had the default TrueFilter rule. Real filter evaluation (a SQL-subset parser/evaluator, following services/azuretable's $filter or services/cosmosdb's SQL-subset precedent) is deferred, per AZURE.md section 9's M5 scope note."}
  sessions_and_amqp: {status: deferred, note: "Sessions (ordered/exclusive per-SessionId message groups) and the full AMQP 1.0 protocol are explicitly out of scope for this MVP per AZURE.md section 9's M5 rationale. SessionId is accepted and stored/round-tripped on messages (BrokerProperties.SessionId) but has no session-lock/FIFO/exclusivity semantics."}
  lock_duration_and_long_poll: {status: gap, note: "LockDuration is a fixed package-level constant (DefaultLockDuration, 60s) rather than a per-queue/per-subscription configurable property (real Service Bus's CreateQueue/CreateSubscription accept a LockDuration element). PeekLock's long-poll \"timeout\" query parameter (real clients use it to wait for a message rather than immediately getting 204) is not implemented -- PeekLock always returns immediately."}
  dead_lettering: {status: ok, note: "A background Janitor (janitor.go, 10s default interval) proactively releases expired locks (dead-lettering on MaxDeliveryCount exhaustion) and moves TTL-expired live messages straight to dead-letter, mirroring services/azurequeue's Janitor shape but adapted to Service Bus's richer lock/delivery-count/dead-letter model instead of a flat TTL sweep. A message that expires while already sitting in the dead-letter sub-queue is dropped outright (nowhere further to move it)."}
  auth: {status: partial, note: "Authorization: SharedAccessSignature sr=<resource>&sig=<hmac>&se=<expiry>&skn=<keyname> is always structurally parsed (sas.go's ParseSASAuthorization extracts key name, resource scope, and expiry). Cryptographic HMAC-SHA256 verification (VerifySAS) exists and is unit-tested but is opt-in via Handler.WithSASValidation / --azure-servicebus-validate-sas (mirroring services/s3's WithPresignValidation and Blob/Queue/Table's WithSharedKeyValidation) -- off by default, an absent/malformed/wrong-signature header is accepted."}
  routing_isolation: {status: ok, note: "Runs on its own dedicated *http.Server, bound synchronously in StartWorker to a fixed port (default 10003 via --azure-servicebus-port/AZURE_SERVICEBUS_PORT; no fallback pool -- fails fast if unavailable, mirroring services/azureblob/azurequeue/azuretable), never registered into the shared AWS single-port Router. cli.go's reserveFixedServicePorts additionally reserves this port in the shared PortAlloc pool at startup."}
  observability: {status: ok, note: "StartWorker wraps its Echo handler with telemetry.WrapEchoHandler so ExtractOperation/ExtractResource feed Prometheus metrics, and derives its listener logger via logger.WithWorker(ctx, \"azureservicebus\", \"listener\")."}
gaps:
  - "azservicebus (the only azure-sdk-for-go Service Bus client) is AMQP-only and cannot be pointed at this REST emulator -- see families.sdk_compat. This is a genuine SDK-compatibility limitation, not an MVP scope cut; a real AMQP 1.0 listener would be required to support it, which AZURE.md section 9's M5 rationale explicitly defers."
  - "No SQL-filter rule evaluation for subscriptions -- every subscription is effectively TrueFilter (match-all). See families.filter_evaluation."
  - "No sessions (ordered/exclusive per-SessionId delivery) -- SessionId round-trips but has no locking/FIFO semantics. See families.sessions_and_amqp."
  - "No per-entity LockDuration/MaxDeliveryCount/DefaultMessageTimeToLive configuration -- every queue/topic/subscription uses the same fixed package-level defaults."
  - "PeekLock's long-poll \"timeout\" query parameter is not implemented; the call always returns immediately (204 if nothing is available)."
  - "No explicit client-initiated DeadLetter operation -- only reached via the $DeadLetterQueue path or the Janitor's automatic sweep."
  - "Entity-kind (queue vs topic) detection on PUT /<name> is a body-content sniff, not a full Atom+XML parse -- see families.entity_kind_detection."
  - "No queue/topic/subscription metadata is stored or returned on any Get/List-style call, because no Get/List operations are implemented at all in this MVP (only Create/Delete) -- AZURE.md's M5 wire-protocol scope did not include entity enumeration."
  All gaps above are intentional MVP scope per AZURE.md section 9's M5 entry, not oversights.
deferred:
  - "Initial implementation pass (2026-09-06): seeded this service from scratch per AZURE.md M5 (see AZURE.md section 9). Structurally mirrors services/azurequeue's pop-receipt/visibility-timeout pattern for message locks and services/sns's topic/subscription fan-out bookkeeping; no prior audit history to reconcile."
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
(`<QueueDescription>` vs `<TopicDescription>`). This MVP does not implement a
full Atom+XML parser; `createEntity` (`handler.go`) instead does a
case-insensitive substring sniff of the raw body for `"topicdescription"`,
plus an `?type=topic` query-parameter escape hatch for callers building
requests by hand (as the REST integration test does, since there's no SDK to
generate a proper Atom+XML body). A body containing neither creates a queue
by default.

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
