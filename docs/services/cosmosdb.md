# CosmosDB — Azure Cosmos DB (Core/SQL API)

Gopherstack provides an in-memory Azure Cosmos DB Core/SQL API implementation with databases, containers, documents, and a hand-written Cosmos SQL query subset (real lexer/parser/AST/executor with three-valued-logic `WHERE` evaluation).

## Supported Operations

| Operation | Description |
|-----------|-------------|
| `GetDatabaseAccount` | Database-account root resource (required by every real Cosmos SDK before any data-plane call) |
| `CreateDatabase` | Create a new database |
| `GetDatabase` | Get a database by id |
| `ListDatabases` | List all databases (single page, no pagination) |
| `DeleteDatabase` | Delete a database (cascades to its containers/documents) |
| `CreateContainer` | Create a container (exactly one partition-key path required) |
| `GetContainer` | Get a container by id |
| `ListContainers` | List containers in a database (single page, no pagination) |
| `DeleteContainer` | Delete a container (cascades to its documents) |
| `CreateDocument` | Create a document (also serves as Upsert with `x-ms-documentdb-is-upsert: true`) |
| `QueryDocuments` | Run a Cosmos SQL query (cross-partition scan) |
| `ListDocuments` | Read-feed list of documents (single page, no pagination) |
| `GetDocument` | Get a document (requires `x-ms-documentdb-partitionkey`) |
| `ReplaceDocument` | Full-body replace (upsert if no `If-Match`; `412` on `If-Match` mismatch) |
| `DeleteDocument` | Delete a document (requires `x-ms-documentdb-partitionkey`) |

## Configuration

Cosmos DB runs on its own dedicated, synchronously-bound TCP port, mirroring the real Cosmos DB Local Emulator's own published default rather than Azurite's port convention (Cosmos is a separate tool, not part of the Azurite family).

| Setting | Default | Flag | Env var |
|---|---|---|---|
| Port | `8081` (real Cosmos DB Local Emulator's default) | `--cosmosdb-port` | `COSMOSDB_PORT` |
| Master key | real emulator's well-known fixed key (below) | `--cosmosdb-master-key` | `COSMOSDB_MASTER_KEY` |
| Validate auth | `false` | `--cosmosdb-validate-auth` | `COSMOSDB_VALIDATE_AUTH` |

Startup fails fast if the port is unavailable — there is no fallback pool. **TLS is not implemented** — clients must connect over plain HTTP (`http://localhost:8081`), or explicitly disable SSL verification if their SDK defaults to HTTPS.

## Authentication

Gopherstack accepts the real Cosmos DB Local Emulator's published well-known master key by default:

```text
C2y6yDjf5/R+ob0N8A7Cgv30VRDJIWEHLM+4QDU5DE2nQ9nDuVTqobD4b8mGGyPMbIZnqyMsEcaGQy67XIw/Jw==
```

Unlike Blob/Queue/Table (which have no enforcement flag at all), Cosmos DB has an **opt-in** `--cosmosdb-validate-auth` flag. With it off (the default), any Authorization header is accepted. With it on, a *present* header must carry a correctly signed master-key signature (`401 Unauthorized` otherwise) — but an *absent* header is still accepted either way, so anonymous local-dev workflows keep working.

## Go SDK Example

```go
import "github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"

cred, _ := azcosmos.NewKeyCredential(
    "C2y6yDjf5/R+ob0N8A7Cgv30VRDJIWEHLM+4QDU5DE2nQ9nDuVTqobD4b8mGGyPMbIZnqyMsEcaGQy67XIw/Jw==")

client, _ := azcosmos.NewClientWithKey("http://localhost:8081", cred, &azcosmos.ClientOptions{
    EnableContentResponseOnWrite: true,
})

ctx := context.Background()
_, _ = client.CreateDatabase(ctx, azcosmos.DatabaseProperties{ID: "my-db"}, nil)
db, _ := client.NewDatabase("my-db")
_, _ = db.CreateContainer(ctx, azcosmos.ContainerProperties{
    ID: "my-container",
    PartitionKeyDefinition: azcosmos.PartitionKeyDefinition{Paths: []string{"/pk"}},
}, nil)
```

## curl Examples

```bash
# Database account root (required before any data-plane call)
curl "http://localhost:8081/"

# Create a database
curl -X POST "http://localhost:8081/dbs" \
    -H "Content-Type: application/json" \
    --data '{"id":"my-db"}'

# Create a container
curl -X POST "http://localhost:8081/dbs/my-db/colls" \
    -H "Content-Type: application/json" \
    --data '{"id":"my-container","partitionKey":{"paths":["/pk"],"kind":"Hash"}}'

# Create a document
curl -X POST "http://localhost:8081/dbs/my-db/colls/my-container/docs" \
    -H "Content-Type: application/json" \
    --data '{"id":"doc1","pk":"partition1","name":"example"}'
```

## Known Limitations

- No RU (request unit) accounting — `x-ms-request-charge` is a static `"1"` on every response.
- No continuation-token pagination on List/Query operations — all return every result in one page.
- Hierarchical (multi-path) partition keys are not supported — `CreateContainer` requires exactly one `partitionKey.paths` entry.
- SQL query subset does not support subqueries, `JOIN`, aggregate functions (`COUNT`/`SUM`/`AVG`/`MIN`/`MAX`), built-in scalar/array/object functions, `GROUP BY`, or array/object literals in projections/`WHERE`.
- Query execution is always cross-partition (no partition-key-scoped optimization).
- TLS is not implemented — plain HTTP only.
- No container-level `DefaultTimeToLive` (TTL) enforcement — documents never expire.
- Database/container ETags are static (derived from their RID, never versioned).
- Auth verification is off by default; `--cosmosdb-validate-auth` opts in, but anonymous (no-header) requests are always accepted regardless.

## More

- [Service README](../../services/cosmosdb/README.md)
- [Full parity audit](../../services/cosmosdb/PARITY.md)
- [AZURE.md](../../AZURE.md) — overall Azure support plan and milestones
- [All services](../../README.md#services)
