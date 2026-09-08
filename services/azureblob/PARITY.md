---
service: azureblob
sdk_module: azure-sdk-for-go/sdk/storage/azblob@v1.8.0
last_audit_commit: f1427114b
last_audit_date: 2026-09-03
overall: C
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  ListContainers: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /<account>?comp=list. No prefix/marker/maxresults pagination support yet -- returns all containers in one page with an always-empty NextMarker."}
  CreateContainer: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /<account>/<container>?restype=container. No metadata (x-ms-meta-*) or public-access-level support."}
  DeleteContainer: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<account>/<container>?restype=container. No lease/If-Match conditional support."}
  ListBlobs: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<account>/<container>?restype=container&comp=list. Flat listing only -- no prefix/delimiter/marker/maxresults, no snapshot/version/metadata inclusion flags."}
  PutBlob: {wire: partial, errors: ok, state: ok, persist: ok, note: "PUT /<account>/<container>/<blob>, BlockBlob only (x-ms-blob-type required and validated). Whole-body single-request PUT only -- Put Block/Put Block List (large-object multipart upload) is not implemented, see gaps."}
  GetBlob: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<account>/<container>/<blob>, single-range Range or x-ms-range header supported (start-end, open-ended, and suffix forms; x-ms-range takes precedence when both are present, matching real Azure); multi-range requests are rejected as unsatisfiable rather than served. x-ms-range support added in M4 after the cross-SDK smoke test (AZURE.md section 7) showed the Python (azure-storage-blob) and JS (@azure/storage-blob) Get Blob clients send only x-ms-range, never Range -- Range-only support made single-range downloads unreachable from those SDKs' default download path even though the Go SDK (which sends Range) passed."}
  GetBlobProperties: {wire: ok, errors: ok, state: ok, persist: n/a, note: "HEAD /<account>/<container>/<blob>. Returns ETag/Last-Modified/Content-Length/Content-Type/x-ms-blob-type; no x-ms-meta-* or lease-state headers."}
  DeleteBlob: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<account>/<container>/<blob>. No snapshot/version-scoped delete, no soft-delete."}
families:
  auth: {status: partial, note: "pkgs/azureauth (SharedKey/SharedKeyLite header parsing + canonicalization + HMAC signing/verification) has landed and is wired in: checkAuth parses a present Authorization header via azureauth.ParseAuthorizationHeader. Verification (azureauth.VerifySharedKey) is implemented in pkgs/azureauth but not yet called from checkAuth -- enforcement is deliberately deferred past M0, matching services/s3's PresignSecret-opt-in philosophy. An absent or invalid header is still accepted."}
  blob_body_headers: {status: ok, note: "x-ms-version, x-ms-request-id, and Date are set on every response (success and error paths) via setCommonHeaders, so azure-sdk-for-go's response parsing does not error on missing headers."}
  routing_isolation: {status: ok, note: "Runs on its own dedicated *http.Server, bound synchronously in StartWorker to a fixed port (default 10000 via --azure-blob-port/AZURE_BLOB_PORT, no fallback pool -- fails fast if unavailable, mirroring services/iot's MQTT broker), never registered into the shared AWS single-port Router -- see provider.go's Provider doc comment and AZURE.md section 4 for the full rationale. cli.go's reserveFixedServicePorts additionally reserves this port in the shared PortAlloc pool (pkgs/portalloc.Allocator.Reserve) at startup, since 10000 sits inside --port-range-start/--port-range-end's own default range and would otherwise be handed to an unrelated Acquire caller (fixed in M0 review)."}
  observability: {status: ok, note: "StartWorker wraps its Echo handler with telemetry.WrapEchoHandler so ExtractOperation/ExtractResource feed Prometheus metrics, and derives its listener logger via logger.WithWorker(ctx, \"azureblob\", \"listener\"). InMemoryBackend and the server-lifecycle mutex both use *lockmetrics.RWMutex instead of raw sync.RWMutex/Mutex, matching repo convention."}
gaps:
  - "Put Block / Put Block List (large-object multipart upload) is not implemented -- Put Blob only accepts a single whole-body BlockBlob PUT. Deliberate M0 scope per AZURE.md; not currently assigned to a later milestone (see AZURE.md section 8's M0 entry for the full deferred-gaps list)."
  - "No ACL / container public-access-level support (x-ms-blob-public-access, Set/Get Container ACL are unimplemented)."
  - "No blob or container metadata (x-ms-meta-* headers) -- neither stored on PUT/Create nor returned on GET/HEAD/List."
  - "No conditional-header support (If-Match/If-None-Match/If-Modified-Since/If-Unmodified-Since) on any operation -- every write unconditionally overwrites, every read unconditionally succeeds regardless of ETag/date preconditions."
  - "No Copy Blob (server-side or cross-account) support."
  - "No snapshot, versioning, soft-delete, lease, or tier (hot/cool/archive) support."
  - "List Containers / List Blobs return every result in one page; no prefix/marker/maxresults pagination."
  - "Auth verification is not enforced -- see families.auth. pkgs/azureauth.VerifySharedKey exists and is unit-tested but checkAuth does not call it yet."
  All gaps above are intentional MVP scope per AZURE.md's M0 entry, not oversights; see AZURE.md sections 2 and 8 for the milestone plan.
deferred:
  - "Initial implementation pass (2026-09-02): seeded this service from scratch per AZURE.md M0. No prior audit history to reconcile."
  - "M0 review pass (2026-09-03): pkgs/azureauth, cli.go registration, and test/integration/azureblob_test.go all landed in the same PR as this service (see AZURE.md's M0 entry) -- the file was previously drafted assuming a multi-PR sequence that did not happen. sdk_module bumped to the azblob v1.8.0 actually pinned in go.mod (used by the integration test)."
  - "Dead-code sweep (2026-09-06, gopherstack-rxdr): removed errors.go's ErrInvalidBlobType/ErrInvalidRange sentinels -- unlike every other sentinel in this file (ErrContainerNotFound/ErrContainerAlreadyExists/ErrBlobNotFound, all returned by store.go and consumed via errors.Is at the handler.go boundary, matching the repo-wide convention seen in services/s3, services/sqs, services/dynamodb), these two were never returned by anything: putBlob's x-ms-blob-type check and getBlob's parseRange check are pure handler-local HTTP validation that never crosses into the backend layer. grep confirmed zero references anywhere in the repo outside their own declaration. TestPutBlob_RequiresBlockBlobType and TestGetBlob_RangeHeaderPartialRead/unsatisfiable assert on HTTP status/body strings, not the sentinels, and pass unchanged."
leaks: {status: clean, note: "No background goroutines, tickers, or janitor: InMemoryBackend is pure in-memory maps guarded by one *lockmetrics.RWMutex, with no TTL/expiry sweep in this MVP scope. The dedicated *http.Server started by StartWorker is stopped by Shutdown via srv.Shutdown(ctx) (falling back to srv.Close() on a graceful-shutdown error, both logged), mirroring cli.go's own top-level server lifecycle."}
---

## Notes

### Why Azure Blob gets its own port instead of the shared AWS router
Every other gopherstack service registers a `RouteMatcher` into the shared
single-port AWS `Router` (`pkgs/service/router.go`), which disambiguates
services by header (`X-Amz-Target`) or distinctive path/form shape. Azure
Blob's REST path shape (`/<account>/<container>/<blob>`) has no such
service-identifying header, and colliding with Azure Queue/Table's identical
`/<account>/<resource>` shape (once those land) would be exactly the
ambiguity the AWS router avoids by construction. Instead, the returned
`*Handler` implements `service.BackgroundWorker`, and `StartWorker`
synchronously binds a fixed port (default 10000, Azurite's own Blob-service
default, overridable via `--azure-blob-port`/`AZURE_BLOB_PORT`) before
standing up its own `*echo.Echo` + `*http.Server` to serve on that same
listener -- there is no probe-then-close window for another process to steal
the port between "we checked it was free" and "we're listening on it", and
no fallback into a different port if the bind fails (StartWorker returns the
bind error directly instead). This mirrors `services/iot`'s MQTT broker
(`services/iot/broker.go`), gopherstack's existing precedent for a
fixed-port service, rather than drawing from the shared `--port-range-start`/
`--port-range-end` `PortAlloc` pool used for on-demand ephemeral resources.
See `provider.go`'s `Provider` doc comment and AZURE.md section 4 for the
full rationale.

### Auth
The `Authorization` header is parsed via `pkgs/azureauth.ParseAuthorizationHeader`
(proving a real Azure SDK's header round-trips through this package), but a
malformed or absent header is still accepted -- matching this repo's
permissive-by-default philosophy (`services/s3/sigv4.go`'s
`PresignSecret`-opt-in pattern). `pkgs/azureauth.VerifySharedKey` implements
real SharedKey HMAC verification and is unit-tested, but `handler.go`'s
`checkAuth` does not call it yet; enforcing it is deliberately deferred past
this milestone.

### Blob names with slashes
Azure blob names may contain `/` as a virtual-directory separator (e.g.
`logs/2026/09/02.txt`). `splitPath` only ever splits the URL into three
pieces (`account`, `container`, everything else as `blob`), so a blob name's
internal slashes are preserved intact rather than being mistaken for
additional path segments.

### Range reads
`Get Blob` supports the standard `Range: bytes=start-end`, open-ended
(`bytes=N-`), and suffix (`bytes=-N`) forms, returning `206 Partial Content`
with `Content-Range` set. `x-ms-range` is also accepted, taking precedence
over `Range` when both are present, matching real Azure Blob -- the Python
(`azure-storage-blob`) and JS (`@azure/storage-blob`) generated clients send
`x-ms-range` exclusively for `Get Blob`, never
`Range`, so this was a real wire-compatibility gap until M4's cross-SDK
smoke test (`test/e2e/azure_crosssdk_test.go`) caught it -- the Go SDK's own
generated client sends `Range`, so unit/integration tests using it never
exercised this path. Multi-range requests (`bytes=0-1,3-4`) are rejected
with `416 Requested Range Not Satisfiable` rather than served -- Azure's own
Get Blob does not support multi-range either.

### ETags
Blob ETags are derived from the body *and* a per-backend monotonically
increasing counter incremented on every `Put Blob` (see `store.go`'s
`computeBlobETag`/`etagSeq`), not from the body alone -- overwriting a blob
with byte-identical content still produces a new ETag, matching real Azure
Blob semantics and keeping `If-Match`/`If-None-Match` concurrency checks
meaningful (those headers are not yet enforced -- see gaps -- but the ETags
themselves are now correct). Container listing ETags (`List Containers`) use
a separate, simpler hash with no mutation-sequence component, since
containers have no mutable properties in this MVP.

## More

- [Full parity audit](PARITY.md)
- [All services](../../README.md#services)
