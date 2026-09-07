#!/usr/bin/env node
// Cross-SDK smoke test (Node.js side): proves @azure/storage-blob,
// @azure/storage-queue, @azure/data-tables, and @azure/cosmos -- unmodified,
// real Azure SDKs, not hand-rolled HTTP -- work against a live gopherstack
// instance. Driven by test/e2e/azure_crosssdk_test.go, which starts
// gopherstack on ephemeral ports and passes them in via env vars.
//
// Exits 0 on success, non-zero with a clear message on any failure.

import { randomUUID } from "node:crypto";
import { BlobServiceClient, StorageSharedKeyCredential as BlobSharedKeyCredential } from "@azure/storage-blob";
import { QueueServiceClient, StorageSharedKeyCredential as QueueSharedKeyCredential } from "@azure/storage-queue";
import { TableServiceClient, TableClient, AzureNamedKeyCredential } from "@azure/data-tables";
import { CosmosClient } from "@azure/cosmos";

const ACCOUNT_NAME = "devstoreaccount1";
const ACCOUNT_KEY =
  "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==";
const COSMOS_MASTER_KEY =
  "C2y6yDjf5/R+ob0N8A7Cgv30VRDJIWEHLM+4QDU5DE2nQ9nDuVTqobD4b8mGGyPMbIZnqyMsEcaGQy67XIw/Jw==";

// The Go driver runs this script and azure_smoke.py in parallel against the
// SAME gopherstack instance (test/e2e/azure_crosssdk_test.go), so every
// resource name below must be collision-proof against the Python side, not
// just unique within one run -- a shared "node-"/"py"-less prefix or
// millisecond-only suffix would let same-millisecond starts collide on
// create. RUN_ID mixes a language tag with a random UUID. Azure Table names
// must be alphanumeric only (no hyphens), so TABLE_RUN_ID strips them.
const RUN_ID = `node-${randomUUID()}`;
const TABLE_RUN_ID = `node${randomUUID().replace(/-/g, "")}`;

function requireEnv(name) {
  const v = process.env[name];
  if (!v) {
    throw new Error(`missing required env var ${name}`);
  }
  return v;
}

async function smokeBlob(endpoint) {
  const cred = new BlobSharedKeyCredential(ACCOUNT_NAME, ACCOUNT_KEY);
  const client = new BlobServiceClient(`${endpoint}/${ACCOUNT_NAME}`, cred);

  const containerName = `${RUN_ID}-container`;
  const container = client.getContainerClient(containerName);
  await container.create();

  const blob = container.getBlockBlobClient("hello.txt");
  const content = "hello from node";
  await blob.upload(content, content.length);

  const download = await blob.downloadToBuffer();
  if (download.toString() !== content) {
    throw new Error(`blob round-trip mismatch: got ${download.toString()}`);
  }

  await blob.delete();
  await container.delete();
  console.log("[node] Azure Blob Storage: OK");
}

async function smokeQueue(endpoint) {
  const cred = new QueueSharedKeyCredential(ACCOUNT_NAME, ACCOUNT_KEY);
  const client = new QueueServiceClient(`${endpoint}/${ACCOUNT_NAME}`, cred);

  const queueName = `${RUN_ID}-queue`;
  const queue = client.getQueueClient(queueName);
  await queue.create();

  await queue.sendMessage("hello from node");
  const received = await queue.receiveMessages({ numberOfMessages: 1 });
  if (received.receivedMessageItems.length !== 1) {
    throw new Error("expected exactly one message from receiveMessages");
  }
  const msg = received.receivedMessageItems[0];
  if (msg.messageText !== "hello from node") {
    throw new Error(`queue message mismatch: got ${msg.messageText}`);
  }

  await queue.deleteMessage(msg.messageId, msg.popReceipt);
  await queue.delete();
  console.log("[node] Azure Queue Storage: OK");
}

async function smokeTable(endpoint) {
  const cred = new AzureNamedKeyCredential(ACCOUNT_NAME, ACCOUNT_KEY);
  // gopherstack serves plain HTTP (no TLS); the SDK otherwise refuses to
  // send SharedKey credentials over an insecure connection.
  const clientOptions = { allowInsecureConnection: true };
  const serviceClient = new TableServiceClient(`${endpoint}/${ACCOUNT_NAME}`, cred, clientOptions);

  const tableName = `${TABLE_RUN_ID}table`;
  await serviceClient.createTable(tableName);

  const tableClient = new TableClient(`${endpoint}/${ACCOUNT_NAME}`, tableName, cred, clientOptions);
  await tableClient.createEntity({
    partitionKey: "p1",
    rowKey: "r1",
    name: "example",
  });

  const entity = await tableClient.getEntity("p1", "r1");
  if (entity.name !== "example") {
    throw new Error(`table entity mismatch: got ${entity.name}`);
  }

  await tableClient.deleteEntity("p1", "r1");
  await serviceClient.deleteTable(tableName);
  console.log("[node] Azure Table Storage: OK");
}

async function smokeCosmos(endpoint) {
  const client = new CosmosClient({ endpoint, key: COSMOS_MASTER_KEY });

  const dbId = `${RUN_ID}-db`;
  const { database } = await client.databases.create({ id: dbId });
  const { container } = await database.containers.create({
    id: `${RUN_ID}-coll`,
    partitionKey: { paths: ["/pk"] },
  });

  const { resource: created } = await container.items.create({
    id: "doc1",
    pk: "partition1",
    name: "example",
  });
  if (created.name !== "example") {
    throw new Error(`cosmos document mismatch: got ${created.name}`);
  }

  const { resources } = await container.items
    .query({
      query: "SELECT * FROM c WHERE c.id = @id",
      parameters: [{ name: "@id", value: "doc1" }],
    })
    .fetchAll();
  if (resources.length !== 1) {
    throw new Error(`cosmos query expected 1 result, got ${resources.length}`);
  }

  await database.delete();
  console.log("[node] Azure Cosmos DB: OK");
}

async function main() {
  const blobEndpoint = requireEnv("AZURE_BLOB_ENDPOINT");
  const queueEndpoint = requireEnv("AZURE_QUEUE_ENDPOINT");
  const tableEndpoint = requireEnv("AZURE_TABLE_ENDPOINT");
  const cosmosEndpoint = requireEnv("COSMOSDB_ENDPOINT");

  await smokeBlob(blobEndpoint);
  await smokeQueue(queueEndpoint);
  await smokeTable(tableEndpoint);
  await smokeCosmos(cosmosEndpoint);

  console.log("[node] all four Azure SDKs round-tripped successfully");
}

main().catch((err) => {
  console.error(`[node] FAILED: ${err.stack || err}`);
  process.exit(1);
});
