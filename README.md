<p align="center">
  <img src="assets/logo.png" width="640" alt="Gopherstack">
</p>

<p align="center">
  <strong>150+ AWS services. One Go binary. Milliseconds, not minutes.</strong>
</p>

<p align="center">
  <a href="https://github.com/blackbirdworks/gopherstack/actions/workflows/ci.yml"><img src="https://github.com/blackbirdworks/gopherstack/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/blackbirdworks/gopherstack/actions/workflows/release.yml"><img src="https://github.com/blackbirdworks/gopherstack/actions/workflows/release.yml/badge.svg" alt="Release"></a>
  <a href="https://codecov.io/gh/BlackbirdWorks/gopherstack"><img src="https://codecov.io/gh/BlackbirdWorks/gopherstack/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="https://www.codefactor.io/repository/github/blackbirdworks/gopherstack"><img src="https://www.codefactor.io/repository/github/blackbirdworks/gopherstack/badge" alt="CodeFactor"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/BlackbirdWorks/gopherstack"><img src="https://api.scorecard.dev/projects/github.com/BlackbirdWorks/gopherstack/badge" alt="OpenSSF Scorecard"></a>
  <a href="https://pkg.go.dev/github.com/blackbirdworks/gopherstack"><img src="https://pkg.go.dev/badge/github.com/blackbirdworks/gopherstack.svg" alt="Go Reference"></a>
</p>

<p align="center">
  <a href="#services"><img src=".badges/operations.svg" alt="PARITY entries"></a>
  <a href="#services"><img src=".badges/services.svg" alt="AWS services"></a>
  <a href="#services"><img src=".badges/parity.svg" alt="parity"></a>
  <a href="go.mod"><img src=".badges/go.svg" alt="go"></a>
  <a href="LICENSE"><img src=".badges/license.svg" alt="license"></a>
</p>

Gopherstack is a lightweight, in-memory AWS cloud stack you can run locally. It emulates
**150+ AWS service APIs** in a single Go binary — no AWS account, no credentials, no network
calls — so you can develop and test against realistic AWS behaviour in milliseconds.

Point any AWS SDK, the AWS CLI, Terraform, or the CDK at `http://localhost:8000` and it just
works. Beyond simple CRUD mocks, Gopherstack implements real cross-service integration:
**Event Source Mappings**, **EventBridge Scheduler**, **EventBridge Pipes**, **DynamoDB
Streams → Lambda**, **SNS → SQS fan-out**, and container-backed **Lambda** execution.

> [!TIP]
> Gopherstack is significantly faster and lighter than LocalStack, making it ideal for unit
> and integration tests where speed is critical.

> [!IMPORTANT]
> **This project is vibe coded.** 🚀 It's built for speed, performance, and developer experience.

---

## Quick Start

Pull and run the image:

```bash
docker run -p 8000:8000 ghcr.io/blackbirdworks/gopherstack:latest
```

Open the built-in web dashboard:

```
http://localhost:8000/dashboard
```

Then point any AWS tooling at the endpoint — no credentials required:

```bash
aws dynamodb list-tables --endpoint-url http://localhost:8000
aws s3 mb s3://my-bucket --endpoint-url http://localhost:8000
```

## Installation

### Docker

```bash
docker pull ghcr.io/blackbirdworks/gopherstack:latest
docker run -p 8000:8000 ghcr.io/blackbirdworks/gopherstack:latest
```

The API and the dashboard are both served on port `8000`.

### Docker Compose

```yaml
services:
  gopherstack:
    image: ghcr.io/blackbirdworks/gopherstack:latest
    ports:
      - "8000:8000"
    environment:
      - LOG_LEVEL=info
```

```bash
docker compose up -d
```

For Lambda support the container needs access to a container runtime — see
[docs/docker.md](docs/docker.md).

### Testcontainers (Go)

Gopherstack ships a reusable [Testcontainers for Go](https://golang.testcontainers.org/)
module so you can spin up the whole stack from any Go test suite:

```bash
go get github.com/blackbirdworks/gopherstack/modules/gopherstack
```

```go
import (
    "context"
    "testing"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/testcontainers/testcontainers-go"

    gopherstack "github.com/blackbirdworks/gopherstack/modules/gopherstack"
)

func TestMyService(t *testing.T) {
    ctx := context.Background()

    container, err := gopherstack.Run(ctx, gopherstack.DefaultImage)
    if err != nil {
        t.Fatal(err)
    }
    defer testcontainers.TerminateContainer(container)

    endpoint, err := container.BaseURL(ctx)
    if err != nil {
        t.Fatal(err)
    }

    cfg, _ := config.LoadDefaultConfig(ctx,
        config.WithRegion("us-east-1"),
        config.WithCredentialsProvider(
            credentials.NewStaticCredentialsProvider("test", "test", ""),
        ),
    )

    ddb := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
        o.BaseEndpoint = aws.String(endpoint)
    })

    // … use ddb in your tests
}
```

Pass environment variables with `gopherstack.WithEnv`:

```go
container, err := gopherstack.Run(ctx, gopherstack.DefaultImage,
    gopherstack.WithEnv(map[string]string{
        "LOG_LEVEL": "debug",
        "DEMO":      "true",
    }),
)
```

### As a Go library

Use the in-memory backends directly, without any HTTP server:

```go
import "github.com/blackbirdworks/gopherstack/services/dynamodb"

db := dynamodb.NewInMemoryDB()
// Use db for your application logic…
```

### From source

```bash
git clone https://github.com/blackbirdworks/gopherstack.git
cd gopherstack
go run .            # serves on :8000
```

## Features

### Web Dashboard

A built-in UI at `http://localhost:8000/dashboard` for inspecting and managing local state:

- **DynamoDB** — list tables, view details (keys, indexes, item count), run Query and Scan,
  create tables
- **S3** — list buckets, browse files with folder support, upload/download, manage
  versioning, view object metadata

### Configuration

Every setting has a CLI flag and (usually) an equivalent env var; a flag always wins. When
`--persist` is enabled, values loaded from the persisted config file sit between the two:
precedence is **defaults < persisted config < env vars / CLI flags**.

#### Server

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--port` | `PORT` | `8000` | HTTP server port. |
| `--region` | `REGION` | `us-east-1` | Mock AWS region (also honors `AWS_REGION` / `AWS_DEFAULT_REGION`, see below). |
| `--log-level` | `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |
| `--data-dir` | `GOPHERSTACK_DATA_DIR` | *(empty)* | Directory for persistence data files. Empty resolves to `~/.gopherstack/data`, or `/data` inside a container. |
| `--account-id` | `ACCOUNT_ID` | `000000000000` | Mock AWS account ID used in ARNs. |
| `--persist` | `PERSIST` | `false` | Enable snapshot-based persistence across restarts. |
| `--demo` | `DEMO` | `false` | Load demo data on startup. |
| `--enforce-iam` | `GOPHERSTACK_ENFORCE_IAM` | `false` | Evaluate every request against attached IAM policies. |
| `--tls` | `TLS` | `false` | Serve over HTTPS (self-signed certificate unless `--tls-cert`/`--tls-key` are set). |
| `--tls-cert` | `TLS_CERT` | *(empty)* | Path to a TLS certificate (PEM). Requires `--tls-key`. |
| `--tls-key` | `TLS_KEY` | *(empty)* | Path to a TLS private key (PEM). |
| `--validate-sigv4` | `VALIDATE_SIGV4` | `false` | Cryptographically validate AWS SigV4 request signatures (opt-in). |
| `--sigv4-secret` | `SIGV4_SECRET` | `test` | Secret access key SigV4 validation signs against (only used with `--validate-sigv4`). |
| `--dns-addr` | `DNS_ADDR` | *(empty)* | Address for the embedded DNS server (e.g. `:10053`). Empty disables it. |
| `--dns-resolve-ip` | `DNS_RESOLVE_IP` | `127.0.0.1` | IP address synthetic hostnames resolve to. |
| `--port-range-start` | `PORT_RANGE_START` | `10000` | Start of the port range used for allocated resource endpoints. |
| `--port-range-end` | `PORT_RANGE_END` | `10100` | End (exclusive) of that port range. |
| `--init-script` | `INIT_SCRIPTS` | *(none)* | Shell script(s) to run on startup (repeatable flag / comma-separated env var). |
| `--init-timeout` | `INIT_TIMEOUT` | `30s` | Per-script timeout for init hooks. |
| `--s3-bucket` | `S3_BUCKETS` | *(none)* | S3 bucket(s) to create on startup (repeatable flag / comma-separated env var). |
| `--janitor-timeout` | `JANITOR_TIMEOUT` | `30s` | Per-task timeout for the global janitor loop (TTL sweeps, cleaners). `0` disables it. |
| `--latency-ms` | `LATENCY_MS` | `0` | Inject random latency `[0,N)` ms per request. `0` disables it. |
| `--auto-purge-ttl` | `AUTO_PURGE_TTL` | *(disabled)* | If set, automatically reset all services on a timer (e.g. `10m`). |
| *(none)* | `GOPHERSTACK_PPROF_ADDR` | *(empty)* | Opt-in pprof debug address (e.g. `localhost:6060`) for local profiling / PGO capture. Off unless set; never enable in shared environments. |

#### AWS credentials & region overrides

Gopherstack's own AWS SDK clients read a few standard AWS environment variables, so tooling
that already exports them (e.g. `awslocal`) works without remapping. These are not flags.

| Env var | Default | Description |
|---------|---------|-------------|
| `AWS_DEFAULT_REGION` | *(unset)* | Highest-precedence region override — wins over `AWS_REGION` and `--region`/`REGION`. |
| `AWS_REGION` | *(unset)* | Region override — wins over `--region`/`REGION` but loses to `AWS_DEFAULT_REGION`. |
| `AWS_ACCESS_KEY_ID` | `dummy` | Used for Gopherstack's own internal AWS SDK clients. Incoming request credentials are never validated. |
| `AWS_SECRET_ACCESS_KEY` | `dummy` | Paired with `AWS_ACCESS_KEY_ID` for Gopherstack's internal SDK clients. Incoming request credentials are never validated. |

#### Per-service engine & provider selection

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--elasticache-engine` | `ELASTICACHE_ENGINE` | `embedded` | ElastiCache engine mode: `embedded` (miniredis), `stub`, or `docker`. |
| `--opensearch-engine` | `OPENSEARCH_ENGINE` | `stub` | OpenSearch engine mode: `stub` (API-only) or `docker`. |
| `--elasticsearch-engine` | `ELASTICSEARCH_ENGINE` | `stub` | Elasticsearch engine mode: `stub` (API-only) or `docker`. |
| `--ec2-provider` | `EC2_PROVIDER` | `inmemory` | EC2 compute provider: `inmemory` (stub) or `docker` (launches real containers as instances). |
| `--ec2-docker-image` | `EC2_DOCKER_IMAGE` | `amazonlinux:2` | Docker image used by the EC2 docker provider. |
| `--ec2-docker-network` | `EC2_DOCKER_NETWORK` | *(empty)* | Docker network EC2-docker containers attach to. Empty uses the daemon default bridge. |
| `--ec2-docker-ssh-host-ip` | `EC2_DOCKER_SSH_HOST_IP` | `127.0.0.1` | Host IP that mapped EC2-docker SSH ports bind to. |
| `--ec2-docker-ssh-port-min` | `EC2_DOCKER_SSH_PORT_MIN` | `0` | Lower bound of the host port range for EC2-docker SSH mapping (`0` = let Docker pick). |
| `--ec2-docker-ssh-port-max` | `EC2_DOCKER_SSH_PORT_MAX` | `0` | Upper bound of that host port range. |

#### Lambda & containers

The core Lambda/container-runtime flags — `LAMBDA_DOCKER_HOST`, `LAMBDA_POOL_SIZE`,
`LAMBDA_IDLE_TIMEOUT`, `CONTAINER_RUNTIME` — are documented in [Lambda
configuration](#lambda-configuration) below. A few more exist:

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--lambda-max-runtimes` | `LAMBDA_MAX_RUNTIMES` | `50` | Maximum number of simultaneous per-function Lambda runtimes. |
| `--lambda-keep-containers` | `LAMBDA_KEEP_CONTAINERS` | `false` | If true, keep Lambda containers alive for debugging. |
| *(none)* | `CONTAINER_HOST` | *(empty)* | Generic container endpoint override (e.g. a Podman socket URL). Takes precedence over runtime auto-detection. |
| *(none)* | `GOPHERSTACK_ECS_RUNTIME` | *(unset = no-op)* | Set to `docker` to run ECS tasks as real containers. Unset or any other value is a no-op runner. |
| *(none)* | `GOPHERSTACK_ENABLE_LOCAL_REGISTRY` | `false` (set to `1` to enable) | Run a real embedded Docker Registry v2 backend for ECR instead of the in-memory stub. |

#### Per-service tuning (janitor intervals & TTLs)

Most services expose a background "janitor" tick interval and TTLs for evicting stale data.
These have CLI flags too (run `gopherstack serve --help` for exact names) but are most
commonly set via env var:

| Env var | Default | Description |
|---|---|---|
| `ATHENA_JANITOR_INTERVAL` | `1m` | Athena janitor tick interval. |
| `ATHENA_EXECUTION_TTL` | `24h` | TTL for completed Athena query executions. |
| `BACKUP_JANITOR_INTERVAL` | `1m` | Backup janitor tick interval. |
| `BACKUP_JOB_TTL` | `24h` | TTL for completed Backup jobs. |
| `BATCH_JANITOR_INTERVAL` | `1m` | Batch janitor tick interval. |
| `BATCH_INACTIVE_JOB_DEF_TTL` | `24h` | TTL for inactive Batch job definitions. |
| `BATCH_COMPLETED_JOB_TTL` | `24h` | TTL for completed or failed Batch jobs. |
| `CLOUDWATCHLOGS_JANITOR_INTERVAL` | `1m` | CloudWatch Logs janitor tick interval. |
| `CLOUDWATCHLOGS_MAX_RETENTION_DAYS` | `14` | Global max log retention for groups without an explicit policy. |
| `CODEBUILD_JANITOR_INTERVAL` | `1m` | CodeBuild janitor tick interval. |
| `CODEBUILD_BUILD_TTL` | `24h` | TTL for completed CodeBuild builds. |
| `DYNAMODB_REGION` | `us-east-1` | Default region for DynamoDB (independent of `--region`). |
| `DYNAMODB_JANITOR_INTERVAL` | `500ms` | DynamoDB janitor tick interval. |
| `DYNAMODB_TTL_SWEEP_BATCH_SIZE` | `1000` | Max items checked per TTL sweep lock acquisition. |
| `DYNAMODB_CREATE_DELAY` | `0s` | Simulated CREATING→ACTIVE delay. `0` disables the lifecycle transition. |
| `DYNAMODB_ENFORCE_THROUGHPUT` | `false` | Enforce provisioned RCU/WCU limits (token bucket per table). |
| `EC2_JANITOR_INTERVAL` | `1m` | EC2 janitor tick interval. |
| `EC2_TERMINATED_TTL` | `1h` | TTL for terminated EC2 instances. |
| `EC2_CANCELLED_SPOT_TTL` | `6h` | TTL for cancelled EC2 spot requests. |
| `EMR_JANITOR_INTERVAL` | `1m` | EMR janitor tick interval. |
| `EMR_TERMINATED_TTL` | `1h` | TTL for terminated EMR clusters. |
| `FIS_JANITOR_INTERVAL` | `1m` | FIS janitor tick interval. |
| `FIS_EXPERIMENT_TTL` | `24h` | TTL for completed FIS experiments. |
| `KINESIS_JANITOR_INTERVAL` | `1m` | Kinesis janitor tick interval. |
| `KMS_JANITOR_INTERVAL` | `1m` | KMS janitor tick interval. |
| `REDSHIFTDATA_JANITOR_INTERVAL` | `1m` | Redshift Data janitor tick interval. |
| `REDSHIFTDATA_STATEMENT_TTL` | `24h` | TTL for completed Redshift Data statements. |
| `S3_REGION` | `us-east-1` | Default region for S3 (independent of `--region`). |
| `S3_JANITOR_INTERVAL` | `500ms` | S3 janitor tick interval. |
| `S3_COMPRESSION_MIN_BYTES` | `1024` | Minimum object size for gzip compression. `0` compresses everything. |
| `SES_JANITOR_INTERVAL` | `1m` | SES janitor tick interval. |
| `SES_EMAIL_TTL` | `24h` | TTL for stored sent emails. |
| `SFN_EXECUTION_RETENTION` | `24h` | How long Step Functions execution history is retained. |
| `SFN_JANITOR_INTERVAL` | `1m` | Step Functions janitor tick interval. |
| `SFN_TASK_TOKEN_TTL` | `1h` | Max lifetime of an unreceived Step Functions task token. |
| `SSM_JANITOR_INTERVAL` | `30s` | SSM janitor tick interval. |
| `SSM_COMMAND_TTL` | `1h` | TTL for SendCommand results (matches AWS's 1h default). |
| `STS_JANITOR_INTERVAL` | `30s` | STS janitor tick interval. |
| `XRAY_JANITOR_INTERVAL` | `1m` | X-Ray janitor tick interval. |
| `XRAY_TRACE_TTL` | `30m` | TTL for stored X-Ray traces. |

### DynamoDB

- **In-memory storage** — blazing fast tables and items
- **Secondary indexes** — full Global (GSI) and Local (LSI) secondary index support
- **Rich querying** — sort-key conditions, pagination (`Limit`, `ExclusiveStartKey`), ordering
- **Efficient scanning** — filtering and projection with DynamoDB expressions
- **Expression support** — expression attribute values and names
- **Streams** — change capture that can drive Lambda via Event Source Mappings

### S3

- **Bucket management** — full lifecycle for versioned and unversioned buckets
- **Object operations** — Get, Put, Head, List, multipart uploads
- **Versioning & tagging** — first-class object versioning and metadata tagging
- **Data integrity** — automatic checksums (CRC32, CRC32C, SHA1, SHA256)
- **Compression** — integrated gzip compression for efficient memory usage

### Lambda (Zip and Image packaging)

Gopherstack runs real Lambda functions in containers, supporting both **Zip**
(`PackageType: Zip`) and **container image** (`PackageType: Image`) packaging.

- **Zip functions** — the archive is extracted and run on the matching AWS runtime base
  image, so standard managed runtimes work unmodified: `python3.9`–`python3.13`,
  `nodejs18.x`/`20.x`/`22.x`, `java11`/`17`/`21`, `dotnet8`/`dotnet9`, `ruby3.2`/`3.3`, and
  `provided.al2`/`provided.al2023`
- **Image functions** — provide an `ImageUri` (an AWS base image or your own)
- **Lambda Runtime API** — full implementation, so standard AWS base images work as-is
- **Invocation modes** — `RequestResponse` (sync) and `Event` (async / fire-and-forget)
- **Warm container pool** — configurable per-function pool to cut cold starts
- **Reserved concurrency** — enforced for both sync and async calls
- **Async realism** — AWS-realistic retry semantics and dead-letter queues (via SNS/SQS)
- **Environment variables** — passed straight through to the container

> [!IMPORTANT]
> Both packaging modes require a running Docker (or Podman) daemon to execute invocations.
> S3-based code delivery and direct Go binary execution on the host are not supported.
> **All other Gopherstack services work without Docker.**

```bash
# Create an image-based Lambda function
aws lambda create-function \
    --endpoint-url http://localhost:8000 \
    --function-name my-function \
    --package-type Image \
    --code ImageUri=public.ecr.aws/lambda/python:3.12 \
    --role arn:aws:iam::000000000000:role/my-role

# Invoke synchronously
aws lambda invoke \
    --endpoint-url http://localhost:8000 \
    --function-name my-function \
    --payload '{"key":"value"}' \
    response.json

# List functions
aws lambda list-functions --endpoint-url http://localhost:8000
```

#### Lambda configuration

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--lambda-docker-host` | `LAMBDA_DOCKER_HOST` | `172.17.0.1` | Host/IP that Lambda containers use to reach Gopherstack's Runtime API |
| `--lambda-pool-size` | `LAMBDA_POOL_SIZE` | `3` | Maximum warm containers per function |
| `--lambda-idle-timeout` | `LAMBDA_IDLE_TIMEOUT` | `10m` | Idle container lifetime before reaping |
| `--lambda-container-runtime` | `CONTAINER_RUNTIME` | `docker` | Container runtime: `docker`, `podman`, or `auto` |

#### Using Podman

[Podman](https://podman.io/) works as a drop-in replacement for Docker via its
Docker-compatible API socket.

```bash
# Enable the Podman socket for your user (rootless, Linux)
systemctl --user enable --now podman.socket

# Point Gopherstack at Podman
export CONTAINER_RUNTIME=podman

# Optional: override the socket path
export CONTAINER_HOST=unix://${XDG_RUNTIME_DIR}/podman/podman.sock
```

**Rootless networking note:** in rootless Podman the Docker bridge (`172.17.0.1`) is not
available — use the host's routable IP or `host.containers.internal`:

```bash
export LAMBDA_DOCKER_HOST=host.containers.internal
```

Set `CONTAINER_RUNTIME=auto` to probe Docker first, then Podman, and use whichever socket is
reachable.

### Event Source Mappings (ESM)

Automatically trigger Lambda functions from **DynamoDB Streams** — Gopherstack manages the
polling and invocation for you once an ESM exists. Respects batch size, starting position
(`TRIM_HORIZON`, `LATEST`), and enable/disable state.

### EventBridge Scheduler & Pipes

- **Scheduler** — recurring or one-time scheduled tasks that trigger AWS targets (Lambda,
  SQS, SNS, and more)
- **Pipes** — point-to-point integrations from sources (SQS) to targets (Lambda, Step
  Functions) with optional filtering and enrichment

### Performance & Scalability

- **O(1) ARN indexing** — tagging and resource lookup use a centralized ARN index, so
  operations stay fast with thousands of resources
- **Memory optimization** — struct field alignment and efficient data structures keep the
  footprint small
- **Profile-guided optimization** — the binary ships with a PGO profile captured from real
  workloads (see [PGO](#profile-guided-optimization-pgo))

## Examples

Complete, runnable examples live in [`examples/`](examples/) — each ships a
`docker-compose.yml` and a `demo.sh` so you can run it end to end.

| Example | What it demonstrates | Requires |
|---------|----------------------|----------|
| [`apigw-websocket-chat`](examples/apigw-websocket-chat) | Real-time chat over API Gateway WebSockets with a Node.js Lambda backend | OpenTofu, Node.js (`wscat`) |
| [`cognito-api-auth`](examples/cognito-api-auth) | Securing API Gateway with a Cognito User Pool authorizer and JWTs | OpenTofu, Bash |
| [`ddb-lambda-chain`](examples/ddb-lambda-chain) | 3-table event pipeline where DynamoDB Streams trigger a Go Lambda | Go, Bash |
| [`ec2-docker`](examples/ec2-docker) | Provisioning EC2 instances and running SSH commands against them | Bash (`ssh`, `dig`) |
| [`elasticache-valkey`](examples/elasticache-valkey) | ElastiCache Valkey cluster reachable via embedded DNS | Bash (`valkey-cli`/`redis-cli`) |
| [`eventbridge-sqs`](examples/eventbridge-sqs) | Routing custom EventBridge events to an SQS queue | OpenTofu |
| [`kinesis-lambda-aggregator`](examples/kinesis-lambda-aggregator) | Kinesis stream invoking a Node.js Lambda that aggregates into DynamoDB | OpenTofu, Bash |
| [`s3-access-logs`](examples/s3-access-logs) | Configuring S3 server access logging | OpenTofu |
| [`s3-lambda-processor`](examples/s3-lambda-processor) | S3 uploads triggering a Go Lambda to process the object | OpenTofu, Go, Bash |
| [`sns-sqs-fanout`](examples/sns-sqs-fanout) | Pub/sub fan-out from SNS to multiple SQS queues | OpenTofu |
| [`stepfunctions-order-workflow`](examples/stepfunctions-order-workflow) | Order-processing state machine orchestrated by Step Functions | OpenTofu, Bash |

Running one:

```bash
cd examples/kinesis-lambda-aggregator
docker compose up -d
./demo.sh
```

See [`examples/README.md`](examples/README.md) for prerequisites and teardown.

<!-- BEGIN GENERATED SERVICES -->
## Services

Every service links to its own page with a coverage breakdown — audited operations, known gaps, deferred items and resource-leak status — generated from that service's `PARITY.md` audit.

**Parity** is the overall grade recorded by that service's most recent audit. **PARITY Entries** is the number of `ops:` entries audited in that service's PARITY.md -- a hand-grouped audit unit, not a raw operation count (one entry can name more than one real operation); a dash means the service is tracked by feature family instead. Run `make docs` to refresh this table.

### Compute

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [App Runner](services/apprunner/README.md) | A | 37 | 2 gaps |
| [Auto Scaling](services/autoscaling/README.md) | A | 66 | 2 gaps |
| [Batch](services/batch/README.md) | A | 45 | 7 gaps |
| [EC2](services/ec2/README.md) | A | — | 21 families; 2 gaps; 1 structural gap; 8 deferred |
| [Elastic Beanstalk](services/elasticbeanstalk/README.md) | A | 47 | 11 gaps; 3 deferred |
| [Lambda](services/lambda/README.md) | A | — | 9 families |

### Containers

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [ECR](services/ecr/README.md) | A | 58 | 3 gaps; 2 deferred |
| [ECS](services/ecs/README.md) | A | 65 | 7 gaps; 3 deferred |
| [EKS](services/eks/README.md) | A | 65 | 7 gaps; 1 deferred |

### Storage

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [Backup](services/backup/README.md) | A | 58 | clean |
| [Data Lifecycle Manager](services/dlm/README.md) | A | 8 | clean |
| [EFS](services/efs/README.md) | A | 31 | 2 gaps; 2 deferred |
| [FSx](services/fsx/README.md) | A | — | 13 families; 10 gaps |
| [S3](services/s3/README.md) | A | 20 | 10 gaps |
| [S3 Control](services/s3control/README.md) | A | 43 | 7 gaps; 3 deferred |
| [S3 Glacier](services/glacier/README.md) | A | 33 | 2 gaps |
| [S3 Tables](services/s3tables/README.md) | A | 49 | 1 gap |

### Database

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [DAX](services/dax/README.md) | A | 21 | 1 deferred |
| [DocumentDB](services/docdb/README.md) | A | 55 | 8 gaps; 1 deferred |
| [DynamoDB](services/dynamodb/README.md) | A | — | 12 families; 6 gaps; 2 deferred |
| [DynamoDB Streams](services/dynamodbstreams/README.md) | A | 4 | clean |
| [ElastiCache](services/elasticache/README.md) | A | 75 | 1 gap; 2 deferred |
| [MemoryDB](services/memorydb/README.md) | A | 45 | 7 gaps; 3 deferred |
| [Neptune](services/neptune/README.md) | A | — | 13 families; 5 gaps; 2 deferred |
| [QLDB](services/qldb/README.md) | Removed | — | removed service |
| [QLDB Session](services/qldbsession/README.md) | Removed | — | removed service |
| [RDS](services/rds/README.md) | A | 52 | 4 gaps |
| [RDS Data](services/rdsdata/README.md) | A | 6 | 3 gaps |
| [Redshift](services/redshift/README.md) | A | 9 | clean |
| [Redshift Data](services/redshiftdata/README.md) | A | 12 | 8 gaps; 1 deferred |
| [Timestream Query](services/timestreamquery/README.md) | A | 12 | 5 gaps; 1 deferred |
| [Timestream Write](services/timestreamwrite/README.md) | A | 19 | 5 gaps |

### Networking & Content Delivery

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [API Gateway](services/apigateway/README.md) | A | 122 | 10 gaps; 2 deferred |
| [API Gateway Management API](services/apigatewaymanagementapi/README.md) | A | 3 | 1 gap; 2 deferred |
| [API Gateway v2](services/apigatewayv2/README.md) | A | 77 | 2 gaps; 4 deferred |
| [App Mesh](services/appmesh/README.md) | A | 38 | 2 gaps |
| [Cloud Map](services/servicediscovery/README.md) | A | 30 | 4 gaps; 1 deferred |
| [CloudFront](services/cloudfront/README.md) | A | 60 | 4 deferred |
| [CloudWatch Network Monitor](services/networkmonitor/README.md) | A | 12 | 1 deferred |
| [ELB (Classic)](services/elb/README.md) | A | 29 | 2 gaps; 1 deferred |
| [ELBv2](services/elbv2/README.md) | A | 51 | 3 gaps; 6 deferred |
| [Route 53](services/route53/README.md) | A | 67 | 3 deferred |
| [Route 53 Resolver](services/route53resolver/README.md) | A | 72 | 6 gaps; 1 deferred |
| [VPC Lattice](services/vpclattice/README.md) | A | 73 | 4 gaps |

### Messaging & Integration

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [Amazon MQ](services/mq/README.md) | A | 25 | 3 gaps; 1 deferred |
| [AppSync](services/appsync/README.md) | A | 74 | 4 gaps; 2 deferred |
| [EventBridge](services/eventbridge/README.md) | A | 62 | 1 gap; 2 deferred |
| [EventBridge Pipes](services/pipes/README.md) | A | 10 | 1 gap |
| [EventBridge Scheduler](services/scheduler/README.md) | A | 12 | 1 gap |
| [Pinpoint](services/pinpoint/README.md) | A | 48 | 3 deferred |
| [SES](services/ses/README.md) | A | 71 | 6 gaps; 1 deferred |
| [SES v2](services/sesv2/README.md) | A | 112 | clean |
| [SNS](services/sns/README.md) | A | 34 | 2 gaps; 2 deferred |
| [SQS](services/sqs/README.md) | A | 20 | 4 gaps; 4 deferred |
| [SWF](services/swf/README.md) | A | 39 | 12 gaps; 1 deferred |
| [Step Functions](services/stepfunctions/README.md) | A | 37 | 7 gaps |
| [WorkMail](services/workmail/README.md) | A | 92 | 3 gaps |

### Analytics

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [Athena](services/athena/README.md) | A | 25 | 3 gaps; 1 deferred |
| [Clean Rooms](services/cleanrooms/README.md) | A | — | 17 families; 7 gaps; 2 deferred |
| [EMR](services/emr/README.md) | A | 65 | 1 gap; 6 structural gaps |
| [EMR Serverless](services/emrserverless/README.md) | A | 22 | 1 gap |
| [Elasticsearch](services/elasticsearch/README.md) | A | 51 | 5 gaps |
| [Glue](services/glue/README.md) | A | 59 | 18 gaps; 6 deferred |
| [Glue DataBrew](services/databrew/README.md) | A | 44 | 6 gaps |
| [Kinesis](services/kinesis/README.md) | A | 39 | 12 gaps; 1 deferred |
| [Kinesis Analytics](services/kinesisanalytics/README.md) | A | 20 | 2 gaps |
| [Kinesis Analytics v2](services/kinesisanalyticsv2/README.md) | A | 33 | 6 gaps; 1 deferred |
| [Kinesis Data Firehose](services/firehose/README.md) | A | 12 | 4 gaps; 6 deferred |
| [Lake Formation](services/lakeformation/README.md) | A | 61 | 8 gaps |
| [Managed Streaming for Kafka](services/kafka/README.md) | A | 64 | 3 gaps |
| [Managed Workflows for Apache Airflow](services/mwaa/README.md) | A | 12 | 3 gaps; 1 deferred |
| [OpenSearch](services/opensearch/README.md) | A | 14 | 1 deferred |
| [QuickSight](services/quicksight/README.md) | A | 74 | 1 gap |

### Security

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [ACM](services/acm/README.md) | A | 38 | 6 gaps; 3 deferred |
| [ACM PCA](services/acmpca/README.md) | A | 23 | 9 gaps |
| [Detective](services/detective/README.md) | A | 29 | 3 gaps; 2 deferred |
| [GuardDuty](services/guardduty/README.md) | A | 66 | 5 gaps; 1 structural gap; 5 deferred |
| [Inspector](services/inspector2/README.md) | A | 13 | 8 gaps; 1 deferred |
| [KMS](services/kms/README.md) | A | 54 | 5 gaps; 2 deferred |
| [Macie](services/macie2/README.md) | A | 81 | clean |
| [Secrets Manager](services/secretsmanager/README.md) | A | 24 | 8 gaps; 2 deferred |
| [Security Hub](services/securityhub/README.md) | A | 116 | 5 gaps |
| [Shield](services/shield/README.md) | A | 36 | 2 gaps; 3 deferred |
| [Verified Permissions](services/verifiedpermissions/README.md) | A | 34 | 5 gaps |
| [WAF](services/waf/README.md) | A | 4 | 1 gap; 2 structural gaps |
| [WAFv2](services/wafv2/README.md) | A | 59 | 3 gaps; 1 structural gap |

### Identity & Access

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [Cognito Identity](services/cognitoidentity/README.md) | A | 23 | 2 gaps; 4 deferred |
| [Cognito Identity Provider](services/cognitoidp/README.md) | A | 67 | 5 gaps; 6 deferred |
| [Directory Service](services/directoryservice/README.md) | A | 80 | 8 gaps; 2 deferred |
| [IAM](services/iam/README.md) | A | 33 | clean |
| [IAM Access Analyzer](services/accessanalyzer/README.md) | A | 39 | 5 gaps; 1 deferred |
| [IAM Identity Center (SSO)](services/ssoadmin/README.md) | A | 56 | 4 gaps |
| [IAM Roles Anywhere](services/rolesanywhere/README.md) | A | 30 | 4 gaps |
| [Identity Store](services/identitystore/README.md) | A | 19 | 2 gaps; 1 deferred |
| [STS](services/sts/README.md) | A | 11 | 4 gaps; 1 deferred |

### Management & Governance

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [Account](services/account/README.md) | A | 16 | 5 gaps; 1 deferred |
| [AppConfig](services/appconfig/README.md) | A | 56 | 7 gaps; 1 deferred |
| [AppConfig Data](services/appconfigdata/README.md) | A | 2 | 2 gaps |
| [Application Auto Scaling](services/applicationautoscaling/README.md) | A | 14 | 4 gaps; 2 deferred |
| [Cloud Control API](services/cloudcontrol/README.md) | A | 8 | 3 gaps |
| [CloudFormation](services/cloudformation/README.md) | A | 73 | 5 gaps |
| [CloudTrail](services/cloudtrail/README.md) | A | 60 | 11 gaps |
| [CloudWatch](services/cloudwatch/README.md) | A | 50 | 5 deferred |
| [CloudWatch Logs](services/cloudwatchlogs/README.md) | A | 84 | 30 gaps; 3 deferred |
| [Config](services/awsconfig/README.md) | A | 102 | 5 gaps; 1 deferred |
| [Cost Explorer](services/ce/README.md) | A | 37 | 3 gaps; 2 deferred |
| [Fault Injection Simulator](services/fis/README.md) | A | 26 | 2 gaps; 1 deferred |
| [OpsWorks](services/opsworks/README.md) | B | 32 | 5 gaps; 1 deferred |
| [Organizations](services/organizations/README.md) | A | 63 | 7 gaps |
| [Resource Access Manager](services/ram/README.md) | A | 36 | 3 deferred |
| [Resource Groups](services/resourcegroups/README.md) | A | 23 | 3 gaps |
| [Resource Groups Tagging API](services/resourcegroupstaggingapi/README.md) | A | 9 | 9 gaps; 2 deferred |
| [Systems Manager](services/ssm/README.md) | A | 105 | 29 gaps |

### Developer Tools

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [Amplify](services/amplify/README.md) | A | 37 | 4 gaps |
| [CodeArtifact](services/codeartifact/README.md) | A | 48 | 8 gaps; 3 deferred |
| [CodeBuild](services/codebuild/README.md) | A | 59 | 1 gap; 1 deferred |
| [CodeCommit](services/codecommit/README.md) | A | 79 | 5 gaps |
| [CodeConnections](services/codeconnections/README.md) | A | 27 | clean |
| [CodeDeploy](services/codedeploy/README.md) | A | 47 | 4 gaps; 2 deferred |
| [CodePipeline](services/codepipeline/README.md) | A | 20 | 9 gaps; 3 deferred |
| [CodeStar Connections](services/codestarconnections/README.md) | A | 27 | 1 gap; 2 structural gaps |
| [Serverless Application Repository](services/serverlessrepo/README.md) | A | 14 | clean |
| [X-Ray](services/xray/README.md) | A | 38 | 9 gaps; 1 deferred |

### Machine Learning

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [Bedrock](services/bedrock/README.md) | A | 80 | 14 gaps |
| [Bedrock Agent](services/bedrockagent/README.md) | A | 77 | 5 gaps; 2 deferred |
| [Bedrock Runtime](services/bedrockruntime/README.md) | A | 11 | 7 gaps |
| [Comprehend](services/comprehend/README.md) | A | 28 | 3 gaps; 1 deferred |
| [Forecast](services/forecast/README.md) | A | 21 | 3 gaps |
| [Personalize](services/personalize/README.md) | A | 73 | clean |
| [Polly](services/polly/README.md) | A | 10 | clean |
| [Rekognition](services/rekognition/README.md) | A | 50 | 1 gap; 4 deferred |
| [SageMaker](services/sagemaker/README.md) | A | 69 | 22 gaps; 5 deferred |
| [SageMaker Runtime](services/sagemakerruntime/README.md) | A | 3 | 1 gap |
| [Textract](services/textract/README.md) | A | 25 | 2 gaps; 1 structural gap; 1 deferred |
| [Transcribe](services/transcribe/README.md) | A | 43 | 2 gaps |
| [Translate](services/translate/README.md) | A | 19 | 7 gaps |

### Media

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [MediaConvert](services/mediaconvert/README.md) | A | 34 | 7 gaps; 1 deferred |
| [MediaLive](services/medialive/README.md) | A | — | 26 families; 5 gaps |
| [MediaPackage](services/mediapackage/README.md) | A | 19 | 1 deferred |
| [MediaStore](services/mediastore/README.md) | A | 21 | clean |
| [MediaStore Data](services/mediastoredata/README.md) | A | 5 | 4 gaps; 1 deferred |
| [MediaTailor](services/mediatailor/README.md) | A | 48 | 2 gaps |

### IoT

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [IoT Analytics](services/iotanalytics/README.md) | A | 34 | 3 gaps |
| [IoT Core](services/iot/README.md) | A | 86 | clean |
| [IoT Data Plane](services/iotdataplane/README.md) | A | 11 | 5 gaps; 1 deferred |
| [IoT Wireless](services/iotwireless/README.md) | A | 15 | 1 gap |

### Migration & Transfer

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [DataSync](services/datasync/README.md) | A | 53 | 4 gaps; 1 deferred |
| [Database Migration Service](services/dms/README.md) | A | 97 | clean |
| [Transfer Family](services/transfer/README.md) | A | — | 20 families |

### Azure

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [Azure Blob Storage](services/azureblob/README.md) | C | 8 | 8 gaps; 2 deferred |
| [Azure Cosmos DB](services/cosmosdb/README.md) | C | 15 | 9 gaps; 5 deferred |
| [Azure Queue Storage](services/azurequeue/README.md) | C | 9 | 7 gaps; 1 deferred |
| [Azure Service Bus](services/azureservicebus/README.md) | C | 11 | 8 gaps; 1 deferred |
| [Azure Table Storage](services/azuretable/README.md) | C | 10 | 6 gaps; 3 deferred |

### Other

| Service | Parity | PARITY Entries | Notes |
|---|---|---|---|
| [AppStream 2.0](services/appstream/README.md) | A | 44 | clean |
| [Cloudfrontkeyvaluestore](services/cloudfrontkeyvaluestore/README.md) | B | 6 | 3 gaps; 1 structural gap |
| [Directconnect](services/directconnect/README.md) | A | 64 | 3 gaps; 8 structural gaps; 1 deferred |
| [Grafana](services/grafana/README.md) | A | 25 | 2 gaps; 1 structural gap |
| [HealthOmics](services/omics/README.md) | A | — | 25 families; 3 gaps; 1 deferred |
| [Lightsail](services/lightsail/README.md) | A | — | 28 families; 11 gaps; 2 deferred |
| [Managed Blockchain](services/managedblockchain/README.md) | A | 27 | 3 gaps |
| [Mgn](services/mgn/README.md) | A | 95 | 1 gap; 5 structural gaps; 1 deferred |
| [Networkmanager](services/networkmanager/README.md) | A | 95 | 5 gaps; 2 structural gaps |
| [Outposts](services/outposts/README.md) | A | 43 | 3 gaps; 6 structural gaps |
| [Resiliencehub](services/resiliencehub/README.md) | A | 63 | 1 gap; 7 structural gaps |
| [Support](services/support/README.md) | A | 16 | 1 deferred |
| [WorkSpaces](services/workspaces/README.md) | A | 34 | 3 gaps |
<!-- END GENERATED SERVICES -->

## Using Gopherstack

### AWS CLI

Everything works with the standard AWS CLI — just pass `--endpoint-url`:

```bash
# DynamoDB
aws dynamodb create-table \
    --endpoint-url http://localhost:8000 \
    --table-name Users \
    --attribute-definitions AttributeName=ID,AttributeType=S \
    --key-schema AttributeName=ID,KeyType=HASH \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5

aws dynamodb list-tables --endpoint-url http://localhost:8000

# S3
aws s3 mb s3://my-bucket --endpoint-url http://localhost:8000
aws s3 cp myfile.txt s3://my-bucket/ --endpoint-url http://localhost:8000
aws s3 ls s3://my-bucket/ --endpoint-url http://localhost:8000
```

Alternatively set `AWS_ENDPOINT_URL=http://localhost:8000` once and drop the flag.

### Terraform / OpenTofu

Point the AWS provider at Gopherstack by overriding the service endpoints. No real
credentials are needed — any non-empty string works.

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    dynamodb       = "http://localhost:8000"
    s3             = "http://localhost:8000"
    sqs            = "http://localhost:8000"
    sns            = "http://localhost:8000"
    ssm            = "http://localhost:8000"
    kms            = "http://localhost:8000"
    secretsmanager = "http://localhost:8000"
    iam            = "http://localhost:8000"
    sts            = "http://localhost:8000"
    lambda         = "http://localhost:8000"
    cloudformation = "http://localhost:8000"
    cloudwatch     = "http://localhost:8000"
    cloudwatchlogs = "http://localhost:8000"
    stepfunctions  = "http://localhost:8000"
    eventbridge    = "http://localhost:8000"
    apigateway     = "http://localhost:8000"
  }
}
```

Terraform uses path-style S3 URLs, so set `use_path_style = true` on the S3
provider/resource when creating `aws_s3_bucket` resources.

```bash
docker compose up -d   # or: go run .
terraform init
terraform apply
```

### AWS CDK

CDK synthesises CloudFormation locally and deploys it via the AWS SDK, so pointing the SDK at
Gopherstack is enough:

```bash
export AWS_ENDPOINT_URL=http://localhost:8000
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1
export CDK_DEFAULT_ACCOUNT=000000000000
export CDK_DEFAULT_REGION=us-east-1

docker compose up -d   # start Gopherstack
cdk bootstrap          # creates the CDKToolkit stack via CloudFormation
cdk deploy
```

`AWS_ENDPOINT_URL` is picked up automatically by the AWS SDK v2 used by the CDK CLI.

## Documentation

- [Quickstart](docs/quickstart.md)
- [Running with Docker](docs/docker.md)
- [Migrating from LocalStack](docs/migration.md)
- [Architecture](docs/architecture/README.md)
- [Examples](examples/README.md)
- [Chaos Testing](docs/chaos.md)

## Development

**Prerequisites:** Go 1.26+, Docker or Podman (only for Lambda invocations), AWS CLI
(optional).

```bash
make all             # lint + all tests with coverage
make test            # unit tests (short mode)
make total-coverage  # unit + integration + E2E with combined coverage
make lint            # linters
make docs            # regenerate the service docs from each PARITY.md
make pgo             # regenerate the PGO profile
```

## Versioning

Releases are tagged **`v1.<major>.<minor>`** — a major bump moves the middle number, a minor
bump moves the last one. Releases are cut from the **Release** workflow, which only offers
those two increments.

The leading `1` is deliberate and stays put. The Go module path is
`github.com/blackbirdworks/gopherstack` with no `/vN` suffix, and Go requires any module at
major version 2 or higher to carry that suffix in its path. A tag like `v18.0.0` is therefore
invisible to the Go module proxy and pkg.go.dev — staying on the `v1.x.y` line keeps
`go get` working without rewriting every import across the tree on each release.

`v1.0.x` is reserved and must never be reused. An earlier release line published
`v1.0.0`–`v1.0.18`, and `proxy.golang.org` caches module versions permanently while
`sum.golang.org` records their checksums — re-tagging one of those numbers would serve the
old cached code and break checksum verification for anyone fetching it. Releases therefore
start at `v1.1.0`, which also sorts above the highest cached version. The release workflow
enforces both rules and refuses to create a tag that violates them.

## Profile-Guided Optimization (PGO)

The repo root ships `default.pgo`, a CPU profile that Go's
[Profile-Guided Optimization](https://go.dev/doc/pgo) automatically consumes from the main
package directory on every `go build` — no extra flags. It's captured from a representative
workload (heavy DynamoDB GSI/LSI and S3 traffic, plus broad multi-service coverage) so the
compiler can optimize the real hot paths.

Regenerate it with:

```bash
make pgo
```

This runs `scripts/pgo.sh`, which builds the server and the `cmd/pgoload` load generator, runs
the server with pprof enabled, captures CPU profiles while driving load against it, and writes
the merged result to `default.pgo`. It validates the profile with `go tool pprof` and
`go build -pgo=auto` before finishing. Knobs (capture/load duration, concurrency, …) are
environment variables — see the header of `scripts/pgo.sh`.

**Per-PR:** if your change measurably shifts the server's hot paths, run `make pgo` and commit
the updated `default.pgo` alongside it.

**CI:** a weekly workflow (`.github/workflows/pgo.yml`) also runs `make pgo` and opens a pull
request with a refreshed profile.

## License

Gopherstack is released under the [MIT License](LICENSE).
