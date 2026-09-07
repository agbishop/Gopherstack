#!/usr/bin/env python3
"""Cross-SDK smoke test (Python side).

Proves azure-storage-blob, azure-storage-queue, azure-data-tables, and
azure-cosmos -- unmodified, real Azure SDKs, not hand-rolled HTTP -- work
against a live gopherstack instance. Driven by
test/e2e/azure_crosssdk_test.go, which starts gopherstack on ephemeral ports
and passes them in via environment variables.

Exits 0 on success, non-zero with a clear message on any failure.
"""

import os
import sys
import uuid

from azure.core.credentials import AzureNamedKeyCredential
from azure.cosmos import CosmosClient
from azure.data.tables import TableServiceClient
from azure.storage.blob import BlobServiceClient
from azure.storage.queue import QueueServiceClient

ACCOUNT_NAME = "devstoreaccount1"
ACCOUNT_KEY = (
    "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="
)
COSMOS_MASTER_KEY = (
    "C2y6yDjf5/R+ob0N8A7Cgv30VRDJIWEHLM+4QDU5DE2nQ9nDuVTqobD4b8mGGyPMbIZnqyMsEcaGQy67XIw/Jw=="
)

# The Go driver runs this script and azure_smoke.mjs in parallel against the
# SAME gopherstack instance (test/e2e/azure_crosssdk_test.go), so every
# resource name below must be collision-proof against the Node side, not
# just unique within one run -- a shared prefix or millisecond-only suffix
# would let same-millisecond starts collide on create. RUN_ID mixes a
# language tag with a random UUID; TABLE_RUN_ID strips hyphens since Azure
# Table names must be alphanumeric only.
RUN_ID = f"py-{uuid.uuid4()}"
TABLE_RUN_ID = f"py{uuid.uuid4().hex}"


def require_env(name: str) -> str:
    value = os.environ.get(name)
    if not value:
        raise RuntimeError(f"missing required env var {name}")
    return value


def smoke_blob(endpoint: str) -> None:
    client = BlobServiceClient(
        account_url=f"{endpoint}/{ACCOUNT_NAME}",
        credential={"account_name": ACCOUNT_NAME, "account_key": ACCOUNT_KEY},
    )

    container_name = f"{RUN_ID}-container"
    container = client.create_container(container_name)

    content = b"hello from python"
    blob = container.get_blob_client("hello.txt")
    blob.upload_blob(content)

    downloaded = blob.download_blob().readall()
    if downloaded != content:
        raise RuntimeError(f"blob round-trip mismatch: got {downloaded!r}")

    blob.delete_blob()
    container.delete_container()
    print("[python] Azure Blob Storage: OK")


def smoke_queue(endpoint: str) -> None:
    client = QueueServiceClient(
        account_url=f"{endpoint}/{ACCOUNT_NAME}",
        credential={"account_name": ACCOUNT_NAME, "account_key": ACCOUNT_KEY},
    )

    queue_name = f"{RUN_ID}-queue"
    queue = client.create_queue(queue_name)

    queue.send_message("hello from python")
    messages = list(queue.receive_messages(max_messages=1))
    if len(messages) != 1:
        raise RuntimeError("expected exactly one message from receive_messages")

    msg = messages[0]
    if msg.content != "hello from python":
        raise RuntimeError(f"queue message mismatch: got {msg.content!r}")

    queue.delete_message(msg)
    queue.delete_queue()
    print("[python] Azure Queue Storage: OK")


def smoke_table(endpoint: str) -> None:
    credential = AzureNamedKeyCredential(ACCOUNT_NAME, ACCOUNT_KEY)
    service_client = TableServiceClient(
        endpoint=f"{endpoint}/{ACCOUNT_NAME}", credential=credential
    )

    table_name = f"{TABLE_RUN_ID}table"
    table_client = service_client.create_table(table_name)

    table_client.create_entity(
        {"PartitionKey": "p1", "RowKey": "r1", "Name": "example"}
    )

    entity = table_client.get_entity("p1", "r1")
    if entity["Name"] != "example":
        raise RuntimeError(f"table entity mismatch: got {entity['Name']!r}")

    table_client.delete_entity("p1", "r1")
    service_client.delete_table(table_name)
    print("[python] Azure Table Storage: OK")


def smoke_cosmos(endpoint: str) -> None:
    client = CosmosClient(endpoint, credential=COSMOS_MASTER_KEY)

    db_id = f"{RUN_ID}-db"
    database = client.create_database(db_id)
    container = database.create_container(
        id=f"{RUN_ID}-coll", partition_key={"paths": ["/pk"], "kind": "Hash"}
    )

    created = container.create_item(
        {"id": "doc1", "pk": "partition1", "name": "example"}
    )
    if created["name"] != "example":
        raise RuntimeError(f"cosmos document mismatch: got {created['name']!r}")

    results = list(
        container.query_items(
            query="SELECT * FROM c WHERE c.id = @id",
            parameters=[{"name": "@id", "value": "doc1"}],
            enable_cross_partition_query=True,
        )
    )
    if len(results) != 1:
        raise RuntimeError(f"cosmos query expected 1 result, got {len(results)}")

    client.delete_database(db_id)
    print("[python] Azure Cosmos DB: OK")


def main() -> int:
    try:
        blob_endpoint = require_env("AZURE_BLOB_ENDPOINT")
        queue_endpoint = require_env("AZURE_QUEUE_ENDPOINT")
        table_endpoint = require_env("AZURE_TABLE_ENDPOINT")
        cosmos_endpoint = require_env("COSMOSDB_ENDPOINT")

        smoke_blob(blob_endpoint)
        smoke_queue(queue_endpoint)
        smoke_table(table_endpoint)
        smoke_cosmos(cosmos_endpoint)

        print("[python] all four Azure SDKs round-tripped successfully")
        return 0
    except Exception as exc:  # noqa: BLE001 - surface any failure clearly, then exit non-zero
        print(f"[python] FAILED: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
