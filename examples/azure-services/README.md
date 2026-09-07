# azure-services

Demonstrates gopherstack's Azure support — Azure Blob Storage, Azure Queue Storage, Azure Table Storage, and Azure Cosmos DB (Core/SQL API) — all four running side-by-side in one gopherstack instance, each on its own dedicated port.

## What this demonstrates

- Azure Blob Storage (port `10000`): create a container, upload a blob, list/download it, then clean up.
- Azure Queue Storage (port `10001`): create a queue, enqueue a message, dequeue it, delete it, then clean up.
- Azure Table Storage (port `10002`): create a table, insert an entity, read it back, then clean up.
- Azure Cosmos DB (port `8081`): the mandatory database-account root call every real Cosmos SDK makes first, then create a database/container/document, run a parameterized SQL query, then clean up.

All four requests use plain `curl` with no credentials — auth verification is off by default for every Azure service in gopherstack (see [`docs/services/`](../../docs/services/) for each service's auth stance), so no SharedKey signing or Cosmos master-key HMAC is required to talk to them locally.

## Prerequisites

- **Docker** (for `docker-compose up`), or a local Go toolchain to run gopherstack directly from the repo root.
- **curl**, plus the POSIX text utilities `grep`, `head`, and `sed` (used to pull the pop receipt out of the Queue response). No Azure SDKs or `az` CLI needed.

## Running

```bash
cd examples/azure-services
docker-compose up -d
./demo.sh
```

Or, without Docker:

```bash
go run ../.. --azure-blob-port 10000 --azure-queue-port 10001 --azure-table-port 10002 --cosmosdb-port 8081 &
./demo.sh
```

## Expected output

The script echoes each step and its response, ending with:

```
=== All four Azure services exercised successfully ===
```

Any non-2xx response from a step causes the script to exit non-zero with a `FAILED: <step> returned HTTP <status>` message.

## Teardown

```bash
docker-compose down -v
```

## More

- [docs/services/azureblob.md](../../docs/services/azureblob.md)
- [docs/services/azurequeue.md](../../docs/services/azurequeue.md)
- [docs/services/azuretable.md](../../docs/services/azuretable.md)
- [docs/services/cosmosdb.md](../../docs/services/cosmosdb.md)
- [AZURE.md](../../AZURE.md) — overall Azure support plan and milestones
