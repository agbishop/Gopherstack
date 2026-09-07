# AzureTable — Azure Table Storage

Gopherstack provides an in-memory Azure Table Storage implementation (Azurite-compatible wire protocol, REST+JSON/OData) with a full `$filter` grammar (real lexer/parser/evaluator) and EDM type fidelity.

## Supported Operations

| Operation | Description |
|-----------|-------------|
| `CreateTable` | Create a new table |
| `DeleteTable` | Delete a table |
| `ListTables` | List all tables (single page, no pagination) |
| `InsertEntity` | Insert a new entity |
| `GetEntity` | Get an entity by `PartitionKey`/`RowKey` (supports `$select`) |
| `QueryEntities` | Query entities with `$filter`/`$top`/`$select` |
| `ReplaceEntity` | Full-body replace (or upsert when no `If-Match` header) |
| `MergeEntity` | Partial-property merge (via `PATCH` or the `X-Http-Method: MERGE` tunneling header) |
| `DeleteEntity` | Delete an entity (`If-Match` is mandatory) |
| `Batch` | Returns a clean `501 NotImplemented` — see Known Limitations |

## Configuration

Azure Table runs on its own dedicated, synchronously-bound TCP port, separate from Azure Blob/Queue, to avoid colliding with their identical `/<account>/<resource>` path shape.

| Setting | Default | Flag | Env var |
|---|---|---|---|
| Port | `10002` (Azurite's own Table port) | `--azure-table-port` | `AZURE_TABLE_PORT` |

Startup fails fast if the port is unavailable — there is no fallback pool.

## Authentication

Same well-known Azurite development account as Azure Blob/Queue:

```text
Account name: devstoreaccount1
Account key:  Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==
```

The `Authorization` header is parsed (SharedKey/SharedKeyLite) but **signature verification is not enforced** — any (or no) credentials are accepted. There is no `--azure-table-validate-auth` flag.

## Go SDK Example

```go
import "github.com/Azure/azure-sdk-for-go/sdk/data/aztables"

cred, _ := aztables.NewSharedKeyCredential("devstoreaccount1",
    "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==")

svc, _ := aztables.NewServiceClientWithSharedKey(
    "http://localhost:10002/devstoreaccount1", cred, nil)

ctx := context.Background()
table := svc.NewClient("my-table")
_, _ = table.CreateTable(ctx, nil)

entity := aztables.EDMEntity{
    Entity: aztables.Entity{PartitionKey: "partition1", RowKey: "row1"},
    Properties: map[string]any{"Name": "example"},
}
body, _ := json.Marshal(entity)
_, _ = table.AddEntity(ctx, body, nil)
```

## curl Examples

```bash
# Create a table
curl -X POST "http://localhost:10002/devstoreaccount1/Tables" \
    -H "Content-Type: application/json" \
    --data '{"TableName":"my-table"}'

# Insert an entity
curl -X POST "http://localhost:10002/devstoreaccount1/my-table" \
    -H "Content-Type: application/json" \
    --data '{"PartitionKey":"p1","RowKey":"r1","Name":"example"}'

# Query entities with $filter
curl "http://localhost:10002/devstoreaccount1/my-table()?\$filter=PartitionKey%20eq%20%27p1%27"
```

## Known Limitations

- `Batch` (`POST /<account>/$batch`, multipart/mixed changesets) is not implemented — returns a clean `501 NotImplemented`.
- No continuation-token pagination on `ListTables`/`QueryEntities` — both return every result in one page.
- `$select` is honored for custom properties, but `PartitionKey`/`RowKey`/`Timestamp` are always returned regardless of the `$select` list.
- A whole-number `Edm.Double` value (e.g. `4.0`) round-trips as `Edm.Int32` when written without an explicit `@odata.type` annotation — an inherent ambiguity the real `aztables` client has too.
- No SAS / Set-Get Table ACL support.
- No TTL/expiry concept for entities (Table Storage has none), so there is no background janitor.
- Auth verification is not enforced (see Authentication above).

## More

- [Service README](../../services/azuretable/README.md)
- [Full parity audit](../../services/azuretable/PARITY.md)
- [AZURE.md](../../AZURE.md) — overall Azure support plan and milestones
- [All services](../../README.md#services)
