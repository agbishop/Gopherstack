---
name: gopherstack-service-op
description: Add a new AWS operation to an existing gopherstack service, or finish an incomplete one, following this repo's file layout, routing style, locking, error-wiring, persistence, and test conventions instead of re-deriving them from scratch. Trigger whenever asked to "implement <Operation> for <service>", "wire up" an AWS API call, fix a not-implemented/stub operation, or add a handler/backend method/wire struct in any services/<svc>/ directory.
---

# gopherstack-service-op

## No-stub rule first

Never ship a stub: no-op handlers, `&StubOutput{}`, fabricated IDs, or an op
that returns success without touching real state. Every routed operation must
mutate/read real backend state, return AWS-accurate shapes and error codes,
and persist when persistence is on. If an operation genuinely cannot be
implemented (no real AWS data source can exist in an emulator), say so
explicitly and record it as a structural gap in PARITY.md — never ship a
half-working stub silently. This is the rule most likely to slip under time
pressure; check your own diff against it before calling the op done.

## File anatomy

| file | role |
|---|---|
| `handler.go` | Handler struct, `NewHandler`, `service.Registerable` methods (Name, GetSupportedOperations, Reset, RouteMatcher, MatchPriority, ExtractOperation, ExtractResource), route dispatch, `Handler()` echo entrypoint, handleError/classifyError |
| `handler_<family>.go` | HTTP handler funcs for one resource family: decode → call backend → marshal. Pure translation, no business logic |
| `<family>.go` (e.g. `workspaces.go`) | backend methods `(b *InMemoryBackend) CreateWorkspace(...)`: validate, lock, mutate |
| `store.go` | InMemoryBackend struct, NewInMemoryBackend, Reset, ID generators, ARN builders |
| `store_setup.go` | `registerAllTables` — `store.Register(b.registry, "name", store.New(keyFn))` per collection + `AddIndex` |
| `wire.go` | wire DTO structs mirroring SDK field names/casing, doc-comment citing the SDK source that confirms the shape |
| `wire_convert.go` | toXWire/fromXWire mappers |
| `errors.go` | sentinel errors, `apiError` struct, constructors (notFoundError/conflictError/validationError/quotaError) |
| `consts.go` | path-segment + JSON-key literals reused 3+ times |
| `provider.go` | Provider implementing `service.Provider` (Name, Init) — what `cli.go` registers |
| `persistence.go` | `backendSnapshot` struct + Snapshot/Restore |
| `tags.go` / `handler_tags.go` | TagResource/UntagResource/ListTagsForResource/TaggedResources |
| `cross_service.go` | lazy sibling-service lookups via a structurally-matched `siblingServices` interface |
| `chaos_transitions.go` | fault-injection hooks for async state transitions |

Older services may instead have `interfaces.go` (StorageBackend interface) +
a flat `operationHandlers` map — same idea, different plumbing.

## Which routing shape does this service use?

Grep `handler.go` for `routeRequest`, `routeTable`, or `operationHandlers` to
tell which of the three shapes you're editing before you write the dispatch line.

**(a) Nested path-segment switch** (grafana style, `services/grafana/handler.go:163-321`):
```go
func (h *Handler) routeRequest(r *http.Request) (string, dispatchFunc) {
	segs := rawPathSegments(r)
	switch segs[0] {
	case segWorkspaces: return h.routeWorkspaces(segs, r.Method)
	}
	return "", nil
}
```
Add a `case` inside the relevant `route<Family>` func.

**(b) Declarative route table** (networkmanager style, `services/networkmanager/handler.go:130-403`):
```go
type route struct { fn dispatchFunc; op string; method string; pattern []string } // ":Param" = wildcard capture
func (h *Handler) routeTable() []route { return concatRoutes(h.globalNetworksRoutes(), ...) }
```
Add a `route{}` entry to the relevant `<family>Routes()` func in `handler_<family>.go`.

**(c) Fixed `POST /OperationName`** (account style, `services/account/handler.go:55-175`):
```go
operationNames map[string]string
operationHandlers map[string]handlerFunc // method expressions: (*Handler).handleGetContactInformation
```
Add one key to each map. This shape keeps cyclomatic complexity flat as ops
grow — do not convert it to an if/else chain.

All three funnel into `Handler.Handler() echo.HandlerFunc`, which reads the
body, dispatches, and calls `handleError` on failure.

## Backend method shape

```go
func (b *InMemoryBackend) CreateWorkspace(...) (*Workspace, error) {
	// 1. validate (incl. cross-service checks) BEFORE taking the lock
	if err := validate(...); err != nil { return nil, err }
	b.mu.Lock("CreateWorkspace")
	defer b.mu.Unlock()
	// 2. mutate the store.Table
	// 3. return a COPY, never a live pointer into backend state
	cp := *w
	return &cp, nil
}
```
Reference: `services/grafana/workspaces.go:97-176`.

`store.Table[V]`/`store.Index[V]` (`pkgs/store/table.go`) do **no locking
themselves** — the backend's single coarse `lockmetrics.RWMutex` (`b.mu`) is
the lock boundary. Lock granularity follows *invariant* granularity, not
data-structure granularity: AWS ops are cross-map transactions (FK validation
+ write + index update atomically; Snapshot needs one consistent view), so
one op = one lock acquisition across every table it touches. Never scatter
raw `sync.Mutex`. `safemap.Map[K,V]` is only for genuinely isolated
single-map state (caches, token stores) — never a backend resource map.
(`services/account/store.go` still uses a plain `sync.RWMutex`; that's a
pre-convention holdout, not a pattern to copy in new/maintained services.)

New collection → register it in `store_setup.go`:
```go
b.workspaces = store.Register(b.registry, "workspaces", store.New(workspaceKeyFn))
b.apiKeysByWorkspace = b.apiKeys.AddIndex("byWorkspace", apiKeyWorkspaceIndexKeyFn)
```

## Error wiring — do not skip this

Two coexisting patterns exist; match whichever the service already uses.

**Switch-based** (grafana/networkmanager, newer): sentinels + `apiError{cause,
message, resourceType, resourceID, ...}`, matched with `errors.Is`/`errors.As`
in `handleError`/`classifyError` (`services/networkmanager/errors.go:21-85`,
`handler.go:213-254`).

**`errCodeLookup` table** (older: memorydb, rds, opensearch, codecommit):
```go
var errCodeLookup = []errCodeEntry{
	{sentinel: ErrNotFound, code: http.StatusNotFound, errType: "RepositoryDoesNotExistException"},
}
```
(`services/codecommit/handler.go:349-424`)

**Known failure mode**: a missing `errCodeLookup` entry (or missing switch
case) makes a legitimate not-found surface as a **500 InternalFailure**
instead of the correct 4xx + exception type. Always return errors via the
sentinel constructor (`notFoundError(resourceType, id)`), never a bare
`errors.New`, so the classifier has something to match on — and add the
matching table/switch entry in the same change.

## Persistence

`backendSnapshot` = `Version int` + `Tables map[string]json.RawMessage`
(from `b.registry.SnapshotAll()`) + any scalar backend fields not in a table
(AccountID, Region, counters). A new `store.Table` registered via
`registerAllTables` is picked up **automatically** — no backendSnapshot change
needed. A bare scalar/counter needs: a field added to `backendSnapshot`,
populated in `Snapshot`, restored in `Restore`, and a bump of
`<service>SnapshotVersion` (Restore *discards*, not partial-decodes, on a
version mismatch — `services/grafana/persistence.go:17-89`). Non-JSON-safe
fields (e.g. `tags.Tags`) need a `restoreXTagsLocked` rebuild after
RestoreAll (`services/grafana/persistence.go:91-108`).

## Tagging

If the resource is taggable, use the same `tags.Tags` field its other ops
already use, created via `tags.New("<service>.<resource>.<id>.tags")`.
`pkgs/tags` has its own internal locking, separate from `b.mu`.
`handler_tags.go` is the thin HTTP wrapper; `TaggedResources()` feeds the
resourcegroupstaggingapi cross-service integration (wired in `cli.go`, e.g.
`wireTaggingGrafana`).

## Wire shapes

Don't guess field names or error codes — use `gopherstack-sdk-shape` to pull
them from the pinned aws-sdk-go-v2 source before writing `wire.go`.

## Tests to add

- `<family>_test.go` — backend unit tests, same package (white-box).
- `handler_<family>_test.go` — handler-level.
- `sdk_completeness_test.go` — asserts `GetSupportedOperations()` covers every
  SDK client method via `sdkcheck.CheckCompleteness`. Add your op name here if
  it isn't auto-covered by the route table.
- `sdk_roundtrip_helper_test.go`'s `newRoundTripClient`/`newTestHandlerAndClient`
  — stands up an `httptest.Server` running the real router and drives it with
  the real AWS SDK client. This is what actually proves wire compatibility
  (percent-encoded ARNs, epoch-seconds timestamps) — write a round-trip test
  for any new op, not just a handler unit test.
- `test/integration/<service>_test.go` — full lifecycle against the real
  Docker-built binary; use `require.Eventually` for async state (never
  `time.Sleep`), `t.Cleanup` for teardown, real `smithy.APIError` code
  assertions.

## End-to-end checklist

1. Add the op name to `GetSupportedOperations()` (or the `<family>Routes()` entry).
2. Wire the dispatch: a `case` in the segment switch, a `route{}` entry, or an
   `operationHandlers` key (match the service's existing shape, above).
3. Add `wire.go` request/response structs verified against the real SDK
   serializers, not your own output — cite the source in the doc comment.
   Use `gopherstack-sdk-shape` for this.
4. Add `wire_convert.go` mappers if the wire shape differs from the domain type.
5. Write the `handler_<family>.go` func: decode → backend → marshal.
6. Write the backend method in `<family>.go`: validate → `b.mu.Lock(opName)` →
   mutate/read the Table → return a copy.
7. If it can fail: add/extend the sentinel + `errCodeLookup` entry (or switch
   case) so it surfaces as the right status + exception type, not a 500.
8. New persisted state → backendSnapshot + version bump (new Tables are automatic).
9. Taggable → reuse the resource's existing `tags.Tags` field.
10. Touch `sdk_completeness_test.go`'s exception list only for a deliberate,
    documented gap — never to silence a real missing op.
11. Write unit tests + extend `test/integration/<service>_test.go` for the SDK
    round-trip proof.
12. Update PARITY.md's `ops:` block (`wire/errors/state/persist: ok`) — see
    `gopherstack-parity-audit` for the schema.
