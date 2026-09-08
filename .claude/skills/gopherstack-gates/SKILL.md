---
name: gopherstack-gates
description: Pick the narrowest correct build/test/lint command for a gopherstack change and decode golangci-lint/CI failures without trial and error. Use whenever about to run tests or lint, before pushing, when a linter fires (fieldalignment, cyclop, gocognit, exhaustive, mnd, gochecknoglobals, depguard, forbidigo, sloglint, nolintlint), when writing a test that waits on async state, or when integration tests fail mysteriously (missing build-linux). Also use to predict what CI will do before opening a PR.
---

# gopherstack gates

## What changed → narrowest command

| change | command |
|---|---|
| One service's Go code | `go test -race ./services/<svc>/...` |
| One service's lint | `golangci-lint run ./services/<svc>/...` |
| Wire-shape confidence | `go test -race -run TestSDKCompleteness ./services/<svc>/...` plus that service's roundtrip tests |
| Shared type in `pkgs/` changed | add `go build ./...` to the above — catches breaks in every consumer |
| Anything touching persistence/snapshot | run that service's `persistence_test.go` explicitly |
| Full gate before push | `make lint` → `make test` → `make build-linux` → `make integration-test`, in that order |

## Makefile targets

| target | runs | prereq |
|---|---|---|
| `build` | `go build` with UI embed | `ui-build` |
| `build-linux` | `CGO_ENABLED=0 GOOS=linux go build -o bin/gopherstack` | none |
| `lint` | `golangci-lint run --timeout 20m ./...` + `go vet -vettool=$(go tool -n mulint-vet) ./...` + `go tool govulncheck ./...` | `install-deps ui-lint ui-fmt ui-check` |
| `lint-fix` | `fieldalignment -fix ./...` then `golangci-lint run --fix ./...` | `install-deps ui-lint-fix ui-fmt-fix` |
| `test` | `go tool gotestsum --format pkgname -- -race -shuffle on -short ./...` | none |
| `integration-test` | `gotestsum ... -race -shuffle on -timeout 10m ./test/integration/...` | **`build-linux`** — the Docker image copies `bin/gopherstack`, so a stale/missing binary makes integration tests fail against old code with no obvious error |
| `terraform-test` | `gotestsum -v -race -parallel 8 -timeout 45m ./test/terraform/...` | `install-tofu` |
| `e2e-test` | `gotestsum ... -tags=e2e ./test/e2e/...` | `ui-build` |
| `total-coverage` | unit+integration+terraform+e2e merged into coverage.out/.html | `build-linux` |
| `docs` | `go run ./cmd/gendocs` — regenerates per-service README.md, root README table, `.badges/*.svg` | none |
| `all` | `make lint-fix && make total-coverage` | — |

If you edit code and then run `make integration-test` without `make build-linux`
first, you are testing the OLD binary. Always rebuild first, or just run
`make integration-test` (it depends on `build-linux` already) rather than
invoking gotestsum directly.

## golangci-lint gotchas that actually bite

`.golangci.yml` is a large (~65 linter) config. These are the ones that
routinely surprise agents:

| gotcha | detail |
|---|---|
| `fieldalignment` | force-enabled via `settings.govet.enable: [fieldalignment]` on top of `enable-all: true`. Bites on every new struct. Fix: `fieldalignment -fix ./...` (also runs as part of `make lint-fix`), don't hand-reorder fields |
| golines, not `lll` | max line length is **120**, enforced by the `golines` formatter, not a linter check. Run `gofmt`/the formatter rather than manually wrapping |
| `exhaustive` | fires on both `switch` AND `map` literals over enum-like types. A `default:` case satisfies it — add one instead of enumerating every case if the switch isn't meant to be exhaustive |
| `mnd` (magic numbers) | bare numeric literals need a named const, even small ones in non-test files |
| `gochecknoglobals` | package-level `var` needs `//nolint:gochecknoglobals // <reason>` — see account's `operationNames` map for the accepted pattern |
| `depguard` | bans `github.com/golang/protobuf`, `satori/go.uuid`, `gofrs/uuid` (pre-v5), `math/rand` in non-test files, `log` in non-main files |
| `forbidigo` | bans `fmt.Print*` and ad-hoc `slog.Default()`/`slog.New()` outside `pkgs/logger` — route all logging through `pkgs/logger` |
| `sloglint` | `no-global: all`, `context: scope` — no global loggers, pass context-scoped ones |
| `nolintlint` | any `//nolint:` needs an explanation AND names a specific linter (`require-explanation: true`, `require-specific: true`). Only `funlen`/`gocognit`/`golines` may skip the explanation (`allow-no-explanation`) |
| `_test.go` files | broadly waive ireturn/bodyclose/cyclop/dupl/errcheck/funlen/gocognit/goconst/gosec/noctx/wrapcheck — don't fight these in tests |

**Disabled — don't waste time appeasing them**: `nlreturn` ("too strict") and
`lll` (superseded by golines). If a linter output mentions either, something
is misconfigured in your local run, not a real gate.

## Banned-nolint rule

`//nolint:cyclop`, `//nolint:gocyclo`, `//nolint:gocognit`, `//nolint:funlen`
are banned by project convention (not CI-enforced — a human/agent convention).
Current count is 0; keep it that way. Verify:

```bash
grep -rn "nolint:cyclop\|nolint:gocyclo\|nolint:gocognit\|nolint:funlen" --include="*.go" .
```

When one of these linters fires, decompose — never re-suppress. Patterns
used in this repo:

- flat routing switch too complex → `map[routeKey]string` built via `sync.OnceValue`
- long validate-then-build function → extract `validateXInput`/`resolveXLocked` helpers
- big test function → `t.Helper()` sub-helpers

## synctest / require.Eventually rule

`time.Sleep` **inside** a `synctest.Test(t, func(t *testing.T){...})` bubble
is fine — the bubble has a fake clock that advances instantly. The banned
pattern is an *unbubbled* `time.Sleep` used to await async state outside a
bubble. Real Docker/loopback I/O can't run in a bubble (not durably
blocking), so those use `require.Eventually(t, condFn, timeout, tick, msg)`
instead.

```go
synctest.Test(t, func(t *testing.T) {
    b := NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")
    created, _ := b.CreateCluster(...)
    time.Sleep(clusterTransitionDelay + time.Millisecond) // fake clock inside bubble
    // assert ACTIVE
})
```

(services/eks/async_lifecycle_test.go:35-58)

`mulint` (declared as a go.mod `tool`, invoked separately by `make lint` via
`go vet -vettool=...`, NOT a golangci-lint plugin) catches mutex misuse —
recursive locks, copied locks — relevant given the coarse-lock convention in
`pkgs-catalog.md`.

## What CI runs (predict failures before opening a PR)

`.github/workflows/ci.yml`: `ui-lint` → `lint` → `modernize` (`go fix -diff
./...`) → `govulncheck` → `codeql` → `unit-tests` (4-way sharded:
`-race -shuffle on -short -timeout 5m`, split by `awk "NR % 4 == chunk"` over
`go list ./...`) → `build` (static linux binary, tags `netgo osusergo
static_build`) → `integration-tests` (needs `build`; downloads the built
artifact, never builds inline, 4-way sharded by discovered `^func Test`
names, `-timeout 10m`) → `terraform-tests`.

Branch protection on `main`: 12 required contexts (lint, unit, e2e, ui-lint,
ui-test, integration, terraform, modernize, govulncheck, build, CodeFactor,
CodeQL), `enforce_admins=true`, no bypass. If your change doesn't pass
locally with `make lint && make test && make build-linux &&
make integration-test`, it will not pass CI either.
