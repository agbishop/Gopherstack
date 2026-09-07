# AzureQueue — Azure Queue Storage

Gopherstack provides an in-memory Azure Queue Storage implementation (Azurite-compatible wire protocol) with full message lifecycle support: put/get/peek/update/delete, visibility timeouts, and TTL expiry.

## Supported Operations

| Operation | Description |
|-----------|-------------|
| `ListQueues` | List all queues in the account (single page, no pagination) |
| `CreateQueue` | Create a new queue |
| `DeleteQueue` | Delete a queue |
| `PutMessage` | Enqueue a message (supports `visibilitytimeout`/`messagettl`) |
| `GetMessages` | Dequeue up to 32 visible messages, hiding them until their visibility timeout elapses |
| `PeekMessages` | Read messages without changing visibility or `PopReceipt` |
| `DeleteMessage` | Delete a message by id + `popreceipt` |
| `UpdateMessage` | Update a message's visibility timeout and/or body |
| `ClearMessages` | Remove all messages from a queue |

## Configuration

Azure Queue runs on its own dedicated, synchronously-bound TCP port, separate from Azure Blob/Table, to avoid colliding with their identical `/<account>/<resource>` path shape.

| Setting | Default | Flag | Env var |
|---|---|---|---|
| Port | `10001` (Azurite's own Queue port) | `--azure-queue-port` | `AZURE_QUEUE_PORT` |

Startup fails fast if the port is unavailable — there is no fallback pool.

## Authentication

Same well-known Azurite development account as Azure Blob:

```text
Account name: devstoreaccount1
Account key:  Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==
```

The `Authorization` header is parsed (SharedKey/SharedKeyLite) but **signature verification is not enforced** — any (or no) credentials are accepted. There is no `--azure-queue-validate-auth` flag.

## Go SDK Example

```go
import "github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"

cred, _ := azqueue.NewSharedKeyCredential("devstoreaccount1",
    "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==")

svc, _ := azqueue.NewServiceClientWithSharedKeyCredential(
    "http://localhost:10001/devstoreaccount1", cred, nil)

ctx := context.Background()
queue := svc.NewQueueClient("my-queue")
_, _ = queue.Create(ctx, nil)
_, _ = queue.EnqueueMessage(ctx, "hello world", nil)
resp, _ := queue.DequeueMessages(ctx, nil)
```

## curl Examples

```bash
# Create a queue
curl -X PUT "http://localhost:10001/devstoreaccount1/my-queue"

# Put a message
curl -X POST "http://localhost:10001/devstoreaccount1/my-queue/messages" \
    --data '<QueueMessage><MessageText>hello world</MessageText></QueueMessage>'

# Get messages
curl "http://localhost:10001/devstoreaccount1/my-queue/messages?numofmessages=1"

# Delete a message (id and popreceipt from the Get response)
curl -X DELETE "http://localhost:10001/devstoreaccount1/my-queue/messages/<id>?popreceipt=<popreceipt>"
```

## Known Limitations

- No queue metadata (`x-ms-meta-*`) support — `CreateQueue` on a pre-existing queue is always idempotent (204); `QueueAlreadyExists` (409) is unreachable.
- `messagettl=-1` ("never expire") is modeled as a 100-year TTL, not true infinite retention.
- `ListQueues` returns every result in one page — no prefix/marker/maxresults pagination.
- No Set/Get Queue Metadata, no queue ACL / SAS-scoped access policies.
- No poison-message / dead-letter handling — `DequeueCount` is tracked but nothing acts on it.
- Visibility-timeout expiry is checked lazily at read time (Get/Peek), not proactively swept (functionally correct, but differs from TTL expiry, which a background janitor does sweep).
- Auth verification is not enforced (see Authentication above).

## More

- [Service README](../../services/azurequeue/README.md)
- [Full parity audit](../../services/azurequeue/PARITY.md)
- [AZURE.md](../../AZURE.md) — overall Azure support plan and milestones
- [All services](../../README.md#services)
