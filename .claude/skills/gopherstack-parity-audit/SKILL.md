---
name: gopherstack-parity-audit
description: Audit a gopherstack service for AWS parity and write or refresh its PARITY.md honestly — grading it A/B, distinguishing real gaps from structural (unfixable) ones, and hunting stubs without false-positiving on legitimate void-result ops. Trigger when asked to "audit parity", "grade this service", "update PARITY.md", "find stubs", "check for stub operations", or before claiming a service is done/complete/A-grade.
---

# gopherstack-parity-audit

## The 5 parity principles

1. **Never ship stub methods.** No-op handlers, `&StubOutput{}`, fabricated
   IDs are all disqualifying. Every routed op must mutate/read real state,
   return AWS-accurate shapes + error codes, persist when persistence is on.
   If something genuinely can't be implemented, say so in `structural_gaps:`
   — never a half-working stub.
2. **Verify wire shapes against the real SDK, not the handler's own output.**
   Real bug classes found this way: wrong XML list wrapper (`<member>` vs
   `<Item>`); wrong root elements; missing response-root nesting when Output
   has no httpPayload member; ISO8601 strings where the JSON protocol wants
   epoch-seconds numbers; a hardcoded `xml:"StubResponse"` XMLName silently
   overriding every runtime root; stub registrations overwriting real
   handlers by registration order; missing errCodeLookup entries → 500
   InternalFailure. Use `gopherstack-sdk-shape` to do the verification —
   don't restate its recipes here.
3. **Unit tests are not parity proof.** SDK-driven integration tests have
   caught 8 client-breaking wire bugs that green unit tests missed. Run
   `make build-linux` before `go test ./test/integration/...` — integration
   tests exercise the real Docker-built binary, not a Go-level shortcut.
4. **Grep-based stub hunting has false positives in both directions.** An op
   that returns an empty envelope *after* real backend logic ran is correct
   (void-result ops — read the backend method before flagging it). Conversely
   a "real-looking" op can be a disguised stub, e.g. filtering a map that is
   never actually populated. Read the backend method, not just the handler.
5. **De-stub hygiene, once a stub is found and fixed:** remove the stub
   registration, delete the orphaned stub handler func entirely, drop it from
   any bare-call stub test manifests, and run golangci-lint before calling it
   done.

## The EC2 stub mechanism (read before touching services/ec2)

There is no `registerStubOpsIfAbsent` guard function — that name survives
only in a test name (`services/ec2/handler_route_server_test.go:258-274`,
`TestRegisterStubOpsIfAbsent_DoesNotShadowRealHandler`). The real mechanism
is **ordering, not an if-absent check**: `handler.go:517-525` builds
`h.ops = h.buildOps()` = `buildCoreOps()` (static baseline) then iterates
`opRegistrars()` (`handler.go:445-515`, ~55 registrar funcs of shape
`func(*Handler, map[string]ec2ActionFn)`) in a fixed order — a later
registrar's map write silently overwrites an earlier one. Inline comments
enforce the ordering (e.g. "registerAdvancedNetworkingOps must run last to
override stub entries"). **The invariant to preserve when de-stubbing: the
real handler's registrar must run after the stub's.** Get the order wrong
and the stub silently wins again.

`stubSupportedOperations()` in `handler_unimplemented_operations.go` lists
ops that fall back to a generic `stubResponse{XMLName xml.Name; RequestID;
Return bool}`. The XMLName is deliberately untagged so the runtime action
name wins on the wire — don't "fix" that by adding an explicit XMLName tag,
it would break every stub response's root element at once.

Two linters that actually bite during de-stub work: `fieldalignment`
(force-enabled via govet settings) and `goconst`. `nlreturn` and `lll` are
disabled — don't chase those. `unused` will catch an orphaned stub func you
forgot to delete.

## PARITY.md schema

Template: `services/_PARITY_TEMPLATE.md:1-37`. It's YAML-*shaped* but not
valid YAML — `note:` fields carry unquoted commas/braces — so it's parsed by
`cmd/gendocs/parser.go`'s tolerant line-based parser, never `yaml.Unmarshal`.
Don't "fix" it into strict YAML.

| field | meaning |
|---|---|
| `service`, `sdk_module: aws-sdk-go-v2/service/<name>@<version>`, `last_audit_commit`, `last_audit_date` | provenance |
| `overall: <A\|B>` (`A-` also appears) | **A = full integration-suite proof + every buildable gap closed. B = accurate but missing the SDK-driven integration suite.** |
| `ops:` | per-op `{wire, errors, state, persist}`, each `ok\|partial\|gap\|deferred`. wire=shape vs SDK, errors=code+HTTP status, state=real mutate/read, persist=in backendSnapshot |
| `families:` | same 4 axes, grouped when per-op tracking is impractical |
| `gaps:` | real, fixable-but-currently-unfixed divergences, each tagged `(bd: gopherstack-xxx)` |
| `structural_gaps:` | divergences that can **never** be fixed — no data source can exist in an emulator (no real traffic, ML engine, billing system, physical hardware). Does not block grade A |
| `deferred:` | consciously not audited this pass |
| `leaks: {status: clean\|found, note}` | |
| `## Notes` (body) | freeform protocol/wire-quirk notes for the next auditor |

### `gaps:` vs `structural_gaps:` — the judgment call people get wrong

`structural_gaps:` is **not an escape hatch**. The test is: could more
implementation effort, however large, produce real data here? If yes, it's a
`gaps:` entry (even if it'd take a week), tagged with a bd issue. If no —
because the thing being modeled requires actual network traffic, a real ML
model, a real billing system, or physical hardware that cannot exist inside
an in-memory emulator — it's `structural_gaps:`. Moving a hard-but-possible
gap into `structural_gaps:` to make the grade look better is exactly the
failure mode this distinction exists to catch. When unsure, default to
`gaps:` and open a bd issue.

Real A-grade PARITY.md files additionally carry: an implementation summary,
ARN verification against terraform-provider-aws source, per-op exception
tables, the cross-service validation mechanism, the chaos-transition
mechanism, a deliberately-simplified list, and a Tests section citing exact
`TestIntegration_*` names. Match that depth when writing a fresh A-grade file.

## sdkcheck completeness — the do-not-silence rule

`sdk_completeness_test.go` (158 services) calls
`sdkcheck.CheckCompleteness(t, &grafanasdk.Client{}, h.GetSupportedOperations(), []string{})`.
`pkgs/sdkcheck/check.go` reflects every exported method on `*<svc>sdk.Client{}`
(excluding `Options`) and asserts: no duplicates in either list, no overlap
between supported/unimplemented, no stale `notImplemented` entries for ops
renamed or removed from the SDK, no phantom `supportedOps` unless allowlisted
(`phantomAllowlist`, check.go:77-95, keyed by `fmt.Sprintf("%T", ptr)` e.g.
`"*s3.Client"`, for client-side-only helpers like presigners), and zero SDK
methods left unaccounted for.

**When an SDK bump surfaces new operations: implement them.** Do not silence
the failure by dumping the new op names into `notImplemented` — that's the
audit tool being defeated, not satisfied.

## Regenerating docs

```bash
make docs   # = go run ./cmd/gendocs
```
Reads every `services/*/PARITY.md`, parses the frontmatter, and regenerates
per-service `README.md`, the category-grouped table in root `README.md`, and
5 SVGs in `.badges/` (operations, services, parity, go, license — self-hosted,
no shields.io, because a shields.io outage once broke every badge). Expect
`README.md` + `.badges/*.svg` churn in your diff after editing any
PARITY.md — that's expected, not a mistake to revert. The generator is
idempotent: an identical PARITY.md corpus produces byte-identical SVGs, so if
`make docs` produces no diff, nothing changed. `.badges/parity.svg` reflects
`gradeDistribution()` — a tally of `normalizeGrade(doc.Overall)` (strips a
trailing `+`), e.g. "152 A".

## Wire verification

Don't restate the module-cache lookup recipe here — use `gopherstack-sdk-shape`
for every wire/error claim you put in PARITY.md's `ops:` block.
