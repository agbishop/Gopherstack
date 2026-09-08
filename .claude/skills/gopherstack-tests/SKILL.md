---
name: gopherstack-tests
description: Write or convert gopherstack tests using this repo's real conventions — table-driven with t.Parallel() in both the outer func and each subtest, short lowercase subtest names (not long sentences), require/assert split, and require.Eventually instead of unbubbled sleeps. Use whenever writing any _test.go in this repo, adding a test for a new operation, converting a test to table-driven, writing a test that waits on async/backend state, writing integration tests under test/integration/, or when asked "add tests for X".
---

# gopherstack tests

## Canonical template

Real example, trimmed from `services/grafana/workspaces_test.go:73-116`:

```go
func TestCreateWorkspace_Validation(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)

	tests := []struct {
		mutate func(*grafanasdk.CreateWorkspaceInput)
		name   string
	}{
		{
			name: "invalid accountAccessType",
			mutate: func(in *grafanasdk.CreateWorkspaceInput) {
				in.AccountAccessType = "BOGUS"
			},
		},
		{
			name: "ORGANIZATION without organizational units",
			mutate: func(in *grafanasdk.CreateWorkspaceInput) {
				in.AccountAccessType = types.AccountAccessTypeOrganization
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			in := minimalCreateWorkspaceInput()
			tc.mutate(in)

			_, err := client.CreateWorkspace(t.Context(), in)
			require.Error(t, err)

			var ve *types.ValidationException
			require.ErrorAs(t, err, &ve, "expected a real ValidationException from the SDK deserializer")
		})
	}
}
```

Shape: anonymous `[]struct{...}` (not a named type, not `map[string]struct`), loop var `tc` or `tt`, `t.Run(tc.name, ...)`, `t.Parallel()` in the outer func **and** inside each subtest closure.

## Naming: do / don't

The subtest `name:` field is a short lowercase phrase, not a sentence and not the test's own restated purpose.

| do | don't |
|---|---|
| `"not found"` | `"should return an error when the workspace does not exist"` |
| `"invalid accountAccessType"` | `"TestCreateWorkspace_should_fail_with_invalid_accountAccessType"` |
| `"empty target"` | `"the target list is empty and should be rejected"` |
| `"already_exists"` (also seen, `pkgs/store` style) | a name that repeats the enclosing func's name |

Data: across ~32k table-case names in `services/`, 73% are a single word, 92% are three words or fewer. Longer ones exist mostly as `"<Operation> <condition>"` (e.g. `"DeleteTask unknown ARN returns 404"`) when the case needs to name which SDK op it targets — still one clause, no filler.

## Top-level func naming

Real convention (measured, not assumed) is `Test<Subject>_<Scenario>`, one underscore joining a CamelCase subject and a CamelCase-or-lowercase scenario:

- `TestCreateWorkspace_Validation`, `TestDescribeWorkspace_NotFound`, `TestUpdateWorkspace_NetworkAccessRemoveAndSetConflict` — subject is the SDK operation.
- `TestInMemoryBackend_SnapshotRestore`, `TestHandler_ExtractOperation` — subject is the type/method for backend-scaffolding tests (these repeat near-identically across dozens of services; match the sibling files in your service, don't invent a new shape).

Across ~29.9k top-level `Test` funcs repo-wide: 52% have exactly one underscore (this `Subject_Scenario` form), 20% have two, 20% have none (short, single-purpose test, no scenario suffix needed). Don't stack three-plus scenario clauses into one func name — split into more table cases instead.

## Parallelism

`t.Parallel()` is convention here, not lint-enforced: `paralleltest` is explicitly disabled in `.golangci.yml:152` ("too many false positives"). The repo calls it anyway — 3,588 of 4,231 test files (85%) do, in both the outer func and subtests, per the template above. Skip it only when you have a real reason (shared mutable fixture, ordering requirement) and say why in a `//nolint:paralleltest` next to `t.Run`, the same way `test/integration/grafana_test.go:171` etc. do it for sequential subtests sharing one workspace.

Go 1.22+ removed the per-iteration loop variable footgun. Don't add `tt := tt` / `tc := tc` before `t.Run`. One straggler (`services/appsync/data_sources_config_test.go:385`) still has it — that's a pre-existing miss, not something to copy.

## Assertions: require vs assert

Both are used heavily (`require` ~76k call sites, `assert` ~62k). The real split:

- `require` for anything that must hold for the rest of the test to make sense — a setup call's error, a nil check before dereferencing (`require.NoError(t, err)` then `require.NotNil(t, out.Workspace)`).
- `assert` for independent field checks after the precondition already passed — see `test/integration/grafana_test.go`, where every `t.Run` does one `require.NoError` up front and then a run of `assert.Equal`/`assert.Contains` for unrelated fields, so one failing assertion doesn't hide the rest.

In pure unit tests (non-integration), `require` alone for the whole body is also common and fine when there's nothing to gain from continuing past a failure.

## Async and timing

- `time.Sleep` **inside** `synctest.Test(t, func(t *testing.T) {...})` is fine — fake clock, resolves instantly, no wall-clock cost. See `services/eks/async_lifecycle_test.go:35-58`: the whole backend, create call, and sleep-then-assert all happen inside one `synctest.Test` bubble. 21 files use `synctest` today.
- An **unbubbled** sleep used to wait for async backend state is banned — real Docker/loopback I/O can't run inside a synctest bubble and isn't durably blocking, so a sleep just makes the test flaky under load. Use `require.Eventually(t, condFn, timeout, tick, msg)` instead. Commit `3b90d4523` converted ~45 of these; `services/grafana/workspaces_test.go:23-41`'s `waitForWorkspaceActive` is the canonical shape — poll, return the last good result once the condition holds.
- Prefer `synctest` when the whole test is self-contained (fake backend, no real network); prefer `require.Eventually` when the test drives a real `httptest.Server`/SDK client or Docker container, which is most of the round-trip and all of the `test/integration/` suite.

## Test package: in-package vs external

- Family unit tests (`workspaces_test.go`, `tags_test.go`, `persistence_test.go`, ...) are external — `package grafana_test` — driven through the real SDK client over `newRoundTripClient`, proving wire compatibility, not just calling Go methods directly.
- `sdk_completeness_test.go` is also external (`package grafana_test`).
- In-package (`package grafana`, or elsewhere `package eks`) is reserved for white-box tests that need an unexported type or field — `.golangci.yml` has explicit per-file `testpackage` exemptions for exactly this (`elasticache/isolation_test.go`, `route53/routing_test.go`, `autoscaling/scheduled_action_cron_test.go`, `pkgs/service/registry_test.go`). Default to external; only go in-package when you're touching something unexported and say why.

## Fixtures and helpers

- Build a fresh backend + handler + real SDK client per test with a small helper: `newTestHandlerAndClient(t)` in `services/grafana/sdk_roundtrip_helper_test.go:58` wraps `grafana.NewInMemoryBackend(t.Context(), acctID, region)` and `t.Cleanup(backend.Close)`, then stands up an `httptest.Server` over the real `pkgs/service` router (`newRoundTripClient`, same file, line 31) — this is what proves the route/serializer, not just the Go method.
- Every helper that takes `*testing.T` calls `t.Helper()` first.
- Use `t.Cleanup`, not `defer`, for teardown that must run even if the test fails partway (server close, backend close) — except inside a `t.Cleanup` callback itself, `t.Context()` is already cancelled (Go 1.24+), so cleanups needing a live context build their own (`grafanaCleanupCtx()` in `test/integration/grafana_test.go:107`).
- Goroutine-leak checking (`pkgs/testleak.VerifyTestMain`) is opt-in per package via a one-line `TestMain` in a dedicated `leak_main_test.go` (12 packages do this: eks, s3, lambda, dynamodb, sqs, ...). Add one when a service spawns its own background goroutines (async timers, workers) worth guarding.

## Integration tests (`test/integration/`)

External `package integration_test`, real Docker-built binary, real `aws-sdk-go-v2` clients pointed at the container's endpoint.

- **Build the binary first**: `make build-linux` before `go test ./test/integration/...` — the Docker image copies `bin/gopherstack`; skipping this fails in confusing ways.
- `dumpContainerLogsOnFailure(t)` (`test/integration/main_test.go:1395`) at the top of a test dumps container logs if it fails — call it.
- `startChaosContainer(t)` (`test/integration/chaos_test.go:47`) gives an isolated container for tests that need to control fault injection or avoid shared-container state races; `postChaosRules(t, ep, rules)` (`chaos_test.go:103`) posts/clears fault rules.
- Async state: `require.Eventually` against `Describe*`, exactly like unit tests — see any `awaitStatus` helper in `test/integration/grafana_test.go`.
- Errors: assert the real smithy error code via a small `awsErrorCode(err)` helper (`errors.As(err, &apiErr smithy.APIError)` then `apiErr.ErrorCode()`), not string-matching the message.
- Sequential subtests sharing one resource (e.g. one workspace across `ListWorkspaces`/`UpdateWorkspace`/`Tags`/...) carry `//nolint:paralleltest // sequential by design` — that's the accepted escape hatch, not a sign to force parallelism where state is shared.

## The two SDK test files every service needs

- `sdk_completeness_test.go` (158 of 161 services): one test, `sdkcheck.CheckCompleteness(t, &<svc>sdk.Client{}, h.GetSupportedOperations(), []string{})`. Fails when the upstream SDK adds an operation the handler doesn't route. The fix is to implement the operation — never widen that final `[]string{}` notImplemented list to silence it.
- `sdk_roundtrip_helper_test.go` (currently on 7 services, more expected as services adopt it): `newRoundTripClient`/`newTestHandlerAndClient` stand up an `httptest.Server` on the real `pkgs/service` router and drive it with the real AWS SDK client. This is what actually proves wire compatibility (URL encoding, timestamp formats, error shapes) — calling `h.Handler()(c)` directly in a unit test bypasses the router and proves less.

## Hard constraints

- No `//nolint:cyclop|gocyclo|gocognit|funlen` — currently 0 in the repo. If a test trips a complexity linter, split with a `t.Helper()` sub-helper. In practice this rarely fires: `.golangci.yml:530-542` already waives `cyclop`, `dupl`, `errcheck`, `funlen`, `gocognit`, `goconst`, `gosec`, `noctx`, `wrapcheck` (and `bodyclose`, `ireturn`) for every `_test.go`.
- No new `export_test.go`. 128 already exist (pre-existing debt, not a license to add more) — prefer unexported test-file helpers/constants, or exercise state through the real exported API/SDK client the way the grafana/networkmanager tests above do.
- File names describe contents, never sequence tags (no `handler_test2.go`).
- Comments: default to none. Only for a non-obvious why, a landmine, or a cited external fact — see `services/grafana/tags_test.go:11-18`'s comment on the ARN-with-slash test for the bar to clear.
- Fastest check: `go test -race ./services/<svc>/...`. Full unit gate: `make test` (`gotestsum -- -race -shuffle on -short ./...`) — `-shuffle on` means tests must not depend on run order.
