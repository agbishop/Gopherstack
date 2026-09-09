# AzureServiceBus — Azure Service Bus

Gopherstack provides an in-memory Azure Service Bus implementation of the **Brokered Messaging REST API**: queue and topic/subscription CRUD, and the full send/peek-lock/complete/abandon/dead-letter message lifecycle. AMQP 1.0, sessions, and subscription SQL-filter evaluation are out of scope for this MVP — see Known Limitations below.

## Supported Operations

| Operation | Description |
|-----------|-------------|
| `CreateQueue` / `DeleteQueue` | `PUT`/`DELETE /<queue>` |
| `CreateTopic` / `DeleteTopic` | `PUT`/`DELETE /<topic>` (entity kind sniffed from the request body or `?type=topic`) |
| `CreateSubscription` / `DeleteSubscription` | `PUT`/`DELETE /<topic>/subscriptions/<name>` (filter rules accepted but not evaluated) |
| `SendMessage` | `POST /<queue-or-topic>/messages` — metadata via the `BrokerProperties` header |
| `PeekLockMessage` | `POST /<queue-or-topic-subscription>[/$DeadLetterQueue]/messages/head` — destructive read with a lock timeout |
| `CompleteMessage` | `DELETE /.../messages/<id>/<locktoken>` |
| `AbandonMessage` | `PUT /.../messages/<id>/<locktoken>` — releases the lock, or dead-letters on delivery-count exhaustion |

## Configuration

Azure Service Bus runs on its own dedicated, synchronously-bound TCP port.

| Setting | Default | Flag | Env var |
|---|---|---|---|
| Port | `10003` | `--azure-servicebus-port` | `AZURE_SERVICEBUS_PORT` |
| SAS validation | off | `--azure-servicebus-validate-sas` | `AZURE_SERVICEBUS_VALIDATE_SAS` |

Startup fails fast if the port is unavailable — there is no fallback pool.

## Authentication

A fixed default namespace and root shared-access key, analogous to Azurite's `devstoreaccount1`:

```text
Namespace: sbemulatorns
Key name:  RootManageSharedAccessKey
Key value: 2R1W2VORtFi9HrmRQ1Gxp7xbySq7W0FAs2BvTZDdXeo=
```

The `Authorization: SharedAccessSignature sr=...&sig=...&se=...&skn=...` header is always structurally parsed for its key name and resource scope. Cryptographic HMAC-SHA256 verification is opt-in via `--azure-servicebus-validate-sas`; off by default, any (or no) credentials are accepted.

## curl Examples

```bash
# Create a queue
curl -X PUT "http://localhost:10003/my-queue"

# Send a message
curl -X POST "http://localhost:10003/my-queue/messages" --data "hello world"

# Peek-lock a message (destructive read with a lock timeout)
curl -i -X POST "http://localhost:10003/my-queue/messages/head"
# -> BrokerProperties header carries MessageId/LockToken; Location header
#    gives the complete/abandon URI

# Complete (permanently remove) using the Location header from above
curl -X DELETE "http://localhost:10003/my-queue/messages/<id>/<locktoken>"

# Abandon (release the lock for redelivery) instead
curl -X PUT "http://localhost:10003/my-queue/messages/<id>/<locktoken>"

# Topic + subscription fan-out
curl -X PUT "http://localhost:10003/my-topic?type=topic"
curl -X PUT "http://localhost:10003/my-topic/subscriptions/my-sub"
curl -X POST "http://localhost:10003/my-topic/messages" --data "fan-out message"
curl -X POST "http://localhost:10003/my-topic/subscriptions/my-sub/messages/head"

# Dead-letter sub-queue
curl -X POST "http://localhost:10003/my-queue/\$DeadLetterQueue/messages/head"
```

## SDK Compatibility Note

`azure-sdk-for-go/sdk/messaging/azservicebus` is an AMQP 1.0 client (built on `github.com/Azure/go-amqp`) with no HTTP/REST transport option, so it cannot be pointed at this REST emulator. Integration tests exercise the REST surface directly via `net/http` instead — see `services/azureservicebus/PARITY.md`'s `sdk_compat` entry.

## Known Limitations

- No SQL-filter rule evaluation for subscriptions — every subscription behaves as `TrueFilter` (match-all).
- No sessions — `SessionId` round-trips on messages but has no locking/FIFO semantics.
- No per-entity `LockDuration`/`MaxDeliveryCount`/`DefaultMessageTimeToLive` configuration — fixed package-level defaults (60s lock, 10 max deliveries, 14-day TTL) apply to every entity.
- `PeekLock`'s long-poll `timeout` query parameter is not implemented — it always returns immediately (204 if nothing is available).
- No explicit client-initiated `DeadLetter` operation — reached only via the `$DeadLetterQueue` path or the janitor's automatic sweep.
- Entity-kind (queue vs. topic) detection on `PUT /<name>` is a body-content sniff, not a full Atom+XML parse.
- No Get/List operations for queues, topics, or subscriptions — only Create/Delete.
- No AMQP 1.0 support (see SDK Compatibility Note above).

## More

- [Service README](../../services/azureservicebus/README.md)
- [Full parity audit](../../services/azureservicebus/PARITY.md)
- [AZURE.md](../../AZURE.md) — overall Azure support plan and milestones
- [All services](../../README.md#services)
