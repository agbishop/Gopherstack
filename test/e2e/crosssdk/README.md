# Azure cross-SDK smoke test

Proves gopherstack's four Azure services work against real, unmodified Node.js and Python Azure SDKs — not just the Go SDK exercised by `test/integration/azure*_test.go` / `cosmosdb_test.go`. This is [AZURE.md](../../../AZURE.md) section 7's M4 deliverable.

Driven by [`../azure_crosssdk_test.go`](../azure_crosssdk_test.go) (`//go:build e2e`), which starts a gopherstack subprocess with the four Azure listeners on ephemeral ports, then runs both scripts below against it.

## Running by hand

Start gopherstack with the four Azure ports (any free ports; the defaults below match Azurite's/the real Cosmos DB Emulator's own conventions):

```bash
go run . --azure-blob-port 10000 --azure-queue-port 10001 --azure-table-port 10002 --cosmosdb-port 8081
```

### Node.js

```bash
cd test/e2e/crosssdk
npm ci
AZURE_BLOB_ENDPOINT=http://localhost:10000 \
AZURE_QUEUE_ENDPOINT=http://localhost:10001 \
AZURE_TABLE_ENDPOINT=http://localhost:10002 \
COSMOSDB_ENDPOINT=http://localhost:8081 \
node azure_smoke.mjs
```

### Python

```bash
cd test/e2e/crosssdk
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
AZURE_BLOB_ENDPOINT=http://localhost:10000 \
AZURE_QUEUE_ENDPOINT=http://localhost:10001 \
AZURE_TABLE_ENDPOINT=http://localhost:10002 \
COSMOSDB_ENDPOINT=http://localhost:8081 \
python3 azure_smoke.py
```

Both scripts print one `OK` line per service and exit 0 on success, or print `FAILED: ...` and exit non-zero on any failure.

## Why no credentials are needed

Auth verification is off by default for all four gopherstack Azure services (see [`docs/services/`](../../../docs/services/)), so both scripts connect without signing requests beyond what each SDK's client constructor requires structurally (a `StorageSharedKeyCredential`/`AzureNamedKeyCredential` for Blob/Queue/Table, a key credential for Cosmos) — gopherstack accepts them without verifying the signature.

## Go driver

`../azure_crosssdk_test.go`'s `TestE2E_AzureCrossSDK` skips cleanly (not a failure) when `node`, `python3`, or either script's SDK dependencies are missing, so `go test -tags=e2e ./test/e2e/...` still passes on a bare dev machine.
