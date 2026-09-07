# Gopherstack Examples

This directory contains complete, runnable examples demonstrating how to use Gopherstack to build and test serverless architectures locally. These examples cover a variety of AWS services interacting with each other, such as API Gateway, Lambda, DynamoDB, S3, Step Functions, and more.

## Prerequisites

To run these examples locally, you need:
1. **Gopherstack** running on `http://localhost:8000` (e.g. `LAMBDA_DOCKER_HOST=host.docker.internal go run .` from the repo root).
2. **OpenTofu** (or Terraform) for the Infrastructure-as-Code examples.
3. **AWS CLI** configured to hit the local endpoint (the scripts typically handle this via `--endpoint-url http://localhost:8000`).
4. **Docker** (if running the Docker Compose examples).
5. **Node.js** or **Go** to build the Lambda functions, depending on the example.

## Available Examples

| Example | Purpose | Tools Required |
|---------|---------|----------------|
| **`apigw-websocket-chat`** | Real-time chat app using API Gateway WebSockets and Node.js Lambda backend. | OpenTofu, Node.js (`wscat`) |
| **`azure-services`** | Exercises Azure Blob/Queue/Table Storage and Azure Cosmos DB side-by-side in one gopherstack instance. | Bash (`curl`, `grep`, `head`, `sed`) |
| **`cognito-api-auth`** | Secures API Gateway endpoints using Cognito User Pool Authorizer and JWTs. | OpenTofu, Bash |
| **`ddb-lambda-chain`** | A 3-table event stream pipeline where DynamoDB Streams trigger a Go Lambda function. | Go, Bash |
| **`ec2-docker`** | Demonstrates provisioning EC2 instances and running SSH commands on them locally. | Bash (`ssh`, `dig`) |
| **`elasticache-valkey`** | Demonstrates provisioning an ElastiCache Valkey cluster and connecting via embedded DNS. | Bash (`valkey-cli` or `redis-cli`) |
| **`eventbridge-sqs`** | Routes custom events from Amazon EventBridge to an SQS queue. | OpenTofu |
| **`kinesis-lambda-aggregator`** | Kinesis stream invokes a Node.js Lambda to aggregate events into DynamoDB. | OpenTofu, Bash |
| **`s3-access-logs`** | Demonstrates configuring S3 server access logging. | OpenTofu |
| **`s3-lambda-processor`** | S3 object uploads trigger a Go Lambda function to process the object. | OpenTofu, Go, Bash |
| **`sns-sqs-fanout`** | A pub/sub fanout architecture distributing SNS messages to multiple SQS queues. | OpenTofu |
| **`stepfunctions-order-workflow`** | Serverless orchestration using Step Functions to manage an order processing state machine. | OpenTofu, Bash |

## Running an Example

All examples include a `docker-compose.yml` file to quickly spin up Gopherstack and any necessary network dependencies.

To run an example, first start Gopherstack in the background:
```bash
cd examples/kinesis-lambda-aggregator
docker-compose up -d
```

Then, run the demo script on your host machine to execute the example:
```bash
./demo.sh
```

When you are finished, you can easily tear down the environment:
```bash
docker-compose down -v
```
