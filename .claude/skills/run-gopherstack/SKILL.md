---
name: run-gopherstack
description: Build, run, and drive the Gopherstack AWS-emulator server locally — start the server, call it with the AWS CLI or raw HTTP, screenshot the dashboard, and run unit/integration tests. Use when asked to run, start, launch, smoke-test, screenshot, or manually verify Gopherstack or one of its 161 services.
---

# Run Gopherstack

Gopherstack is a single Go binary that emulates ~161 AWS services over one HTTP
port (default `8000`), plus a SvelteKit dashboard SPA embedded in the binary at
`/dashboard/`. It is driven programmatically by
`.claude/skills/run-gopherstack/driver.sh` — a wrapper around the server process,
the AWS CLI, raw `curl`, and headless Chrome.

All paths below are relative to the repo root. Run every command from there.

## Prerequisites

Already present in this container; no `apt-get` was needed:

- Go 1.26.5 (`go version`)
- `aws` CLI (v1 / botocore 1.43), `curl`, `jq`
- `google-chrome` (dashboard screenshots)
- Docker daemon (integration tests only — `docker info` must succeed)
- Node 24 + npm — **only** needed if you change `ui/`

## Build

```bash
go build -o bin/gopherstack .
```

~16 s warm. That is enough: the dashboard SPA is committed under
`dashboard/static/spa` and `//go:embed`-ed by `dashboard/ui.go`, so you do
**not** need npm to get a working dashboard. `make build` re-runs `npm --prefix
ui ci` + a Vite build first — use it only when you edited `ui/`.

For integration tests, build the static Linux binary the test container copies:

```bash
make build-linux
```

## Run (agent path)

```bash
.claude/skills/run-gopherstack/driver.sh up          # start on :8123, block until healthy
.claude/skills/run-gopherstack/driver.sh up 8126 --demo   # any serve flags pass through
.claude/skills/run-gopherstack/driver.sh health     # GET /_gopherstack/health
.claude/skills/run-gopherstack/driver.sh logs 40
.claude/skills/run-gopherstack/driver.sh down
```

`up` prints e.g. `up on http://localhost:8123 (161 services)`. State (pid, port,
log, data dir) lives in `$TMPDIR/gopherstack-driver`, so `aws`/`api`/`shot` in
later shells find the running server.

### Call it with the AWS CLI

```bash
.claude/skills/run-gopherstack/driver.sh aws sts get-caller-identity
.claude/skills/run-gopherstack/driver.sh aws s3 mb s3://my-bucket
.claude/skills/run-gopherstack/driver.sh aws dynamodb list-tables
.claude/skills/run-gopherstack/driver.sh aws sqs create-queue --queue-name q
```

The wrapper injects `--endpoint-url` and dummy creds. Verified output:
`sts get-caller-identity` → `{"Account": "000000000000", ...}`.

### Raw HTTP (wire-shape work)

`api` prints response headers plus body — this is the tool for checking XML
roots, list wrappers, and error codes against the real SDK.

```bash
.claude/skills/run-gopherstack/driver.sh api GET /dashboard/api/system/health
.claude/skills/run-gopherstack/driver.sh api POST / \
  'Action=ListQueues&Version=2012-11-05' 'application/x-www-form-urlencoded'
```

The second returns real query-protocol XML:
`<ListQueuesResponse xmlns="http://queue.amazonaws.com/doc/2012-11-05/">…`.

### Screenshot the dashboard

```bash
.claude/skills/run-gopherstack/driver.sh shot out.png
```

Headless Chrome with an 8 s virtual-time budget (the SPA needs it — see
Gotchas). Verified: renders the full console — sidebar of services, "161
Services" tile, live event stream.

### One-shot smoke

```bash
.claude/skills/run-gopherstack/driver.sh smoke
```

Builds and starts if needed, checks health / dashboard bundle / STS / S3
mb+cp+ls / DynamoDB create-table / SQS create-queue / Lambda list-functions,
then stops the server **only if it started it**. Verified all 9 checks `ok`.

## Run (human path)

```bash
./bin/gopherstack serve --port 8000
```

Logs the endpoints and blocks; Ctrl-C to stop. `./bin/gopherstack --help` shows
only two commands: `serve` and `health`. `bin/gopherstack health --port 8123`
prints `ok` and exits 0 — that is the Docker healthcheck.

## Test

```bash
go test -short ./services/<svc>/...              # one service, ~3 s
make test                                        # all unit tests (gotestsum, -race -shuffle)
make build-linux && go test -count=1 -run 'TestIntegration_ACM_' ./test/integration/
make integration-test                            # all integration tests
```

Integration tests spin up a Docker container from `Dockerfile.test`, which
copies `bin/gopherstack` — **stale or non-Linux binary = you test old code.**
Always `make build-linux` first. Verified: `TestIntegration_ACM_*` passes in 14 s.

## Gotchas

- **`GET /health` is not the health check.** It hits the S3 path-style bucket
  router and returns `<Code>NoSuchBucket</Code>`. The real endpoint is
  `/_gopherstack/health` → `{"status":"ok","services":161}`.
- **`AWS_ENDPOINT_URL` alone is not reliable** with the aws-cli v1 here. With
  only the env var set, `aws sqs create-queue` failed with
  `An error occurred (InvalidClientTokenId)` — it went to real AWS, and nothing
  appeared in the server log — while `s3`/`dynamodb`/`sts` worked. Always pass
  `--endpoint-url` explicitly. The driver's `aws` subcommand already does.
- **Fixed side ports are global, not per-instance.** A second server on another
  HTTP port still logs
  `listen tcp :1883: bind: address already in use` (IoT MQTT broker) and
  `listen tcp :8111: bind: address already in use` (DAX data plane). HTTP still
  works, so this is survivable — but don't chase those errors when you meant to
  run two instances.
- **`/dashboard` 301-redirects to `/dashboard/`.** Use `curl -L`.
- **The dashboard HTML is an empty SvelteKit shell** — no visible text, no
  service names. Never grep the HTML for UI content; assert on
  `/dashboard/_app/` or take a screenshot instead.
- **Successful requests are not logged.** Only warnings/errors reach the log, so
  an empty log does not mean nothing was served — and a request that never
  arrived (see the `AWS_ENDPOINT_URL` gotcha) looks identical.
- `--demo` loads sample state (`demo-bucket`, DynamoDB table `Movies`) — handy
  when you need the dashboard to show something.
- Data dir defaults to `~/.gopherstack/data`; the driver overrides
  `GOPHERSTACK_DATA_DIR` to its own state dir so runs don't pollute `$HOME`.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `NoSuchBucket` XML from your health probe | You hit `/health`. Use `/_gopherstack/health`. |
| `InvalidClientTokenId` from an `aws` call | Missing `--endpoint-url`; the call went to real AWS. |
| `driver.sh up` prints `FAILED to come up` + log tail | Port in use, or the build is broken — read the tail, then `driver.sh build`. |
| Integration test asserts against code you just changed and fails | You skipped `make build-linux`; the container has the old binary. |
| Screenshot is blank/white | Chrome exited before hydration — raise `--virtual-time-budget` in `driver.sh`. |
| `driver.sh aws` hits the wrong server | Stale `$TMPDIR/gopherstack-driver/port`. Run `driver.sh down`, then `up <port>`. |
