---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: appmesh
sdk_module: aws-sdk-go-v2/service/appmesh@v1.38.4
last_audit_commit: e4139790
last_audit_date: 2026-08-29
overall: A            # zero wire bugs this pass (2026-08-19); every single-resource CRUD op's flat
                       # (unwrapped) body reconfirmed correct against the SDK's actually
                       # invoked per-op deserializer, not the dead OpDocument helper.
                       # Reconfirmed again 2026-08-21 (gopherstack-r80d batch 13, required-output cut;
                       # last_audit_commit left unchanged per this campaign's convention -- the
                       # orchestrator, not this pass, creates the commit; see gopherstack-z31a): read
                       # every required output member (36 fields/38 ops, plus every ResourceMetadata/
                       # *Ref/*Data domain struct in types.go) end to end against the handlers and real
                       # deserializers; came back clean. See the dated note at the bottom of this file
                       # for detail, including one apparent false positive (a stale "OpDocument"
                       # deserializer helper) ruled out via a real SDK client round trip rather than
                       # trusted from static reading. Original fix (genuine, prior to 2026-08-19): the
                       # primary response-wrapping bug affected every Create/Describe/Update/Delete op
                       # in the service (28 handler call sites).
ops:
  CreateMesh: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body (meshName/metadata/spec/status at root) reconfirmed correct: deserializers.go:244 decodes shape directly into MeshData, no wrapper key; spec structurally validated (egressFilter.type, serviceDiscovery.ipPreference enums)"}
  DescribeMesh: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct (deserializers.go:2639)"}
  UpdateMesh: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct (deserializers.go:5431); version increments; spec structurally validated"}
  DeleteMesh: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct (deserializers.go:1452); in-use check blocks delete while children exist; status DELETED not ACTIVE"}
  ListMeshes: {wire: ok, errors: ok, state: ok, persist: ok, note: "plural-key wrapper {meshes:[...],nextToken} correct — ListMeshes is the one case where the SDK's OpDocument wrapper function IS the real invoked path; limit query param honored"}
  CreateVirtualNode: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct against real invoked deserializer"}
  DescribeVirtualNode: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  UpdateVirtualNode: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  DeleteVirtualNode: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; status DELETED not ACTIVE; blocks delete while a virtual service still lists the node as its provider (gopherstack-2lz, 2026-09-04 — api_op_DeleteVirtualNode.go doc comment; previously unchecked, a real gap despite the prior 'ok' row)"}
  ListVirtualNodes: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateVirtualRouter: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; spec structurally validated (listeners[].portMapping.port/protocol)"}
  DescribeVirtualRouter: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  UpdateVirtualRouter: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; spec structurally validated"}
  DeleteVirtualRouter: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; blocks delete while routes exist; status DELETED not ACTIVE"}
  ListVirtualRouters: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct (including virtualRouterName at root, present on the full RouteData type)"}
  DescribeRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  UpdateRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  DeleteRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; status DELETED not ACTIVE"}
  ListRoutes: {wire: ok, errors: ok, state: ok, persist: ok, note: "RouteSummary correctly includes virtualRouterName — present on the real RouteRef type too, not fabricated"}
  CreateVirtualService: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; spec structurally validated (provider union: exactly one of virtualNode/virtualRouter)"}
  DescribeVirtualService: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  UpdateVirtualService: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; spec structurally validated"}
  DeleteVirtualService: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; status DELETED not ACTIVE"}
  ListVirtualServices: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateVirtualGateway: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  DescribeVirtualGateway: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  UpdateVirtualGateway: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  DeleteVirtualGateway: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; blocks delete while gateway routes exist; status DELETED not ACTIVE"}
  ListVirtualGateways: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateGatewayRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct (including virtualGatewayName at root, present on the full GatewayRouteData type)"}
  DescribeGatewayRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  UpdateGatewayRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct"}
  DeleteGatewayRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "flat body reconfirmed correct; status DELETED not ACTIVE"}
  ListGatewayRoutes: {wire: ok, errors: ok, state: ok, persist: ok, note: "GatewayRouteSummary correctly includes virtualGatewayName — present on the real GatewayRouteRef type too, not fabricated"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /v20190125/tag, resourceArn+tags in JSON body — verified against real serializer"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /v20190125/untag, resourceArn+tagKeys in JSON body"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /v20190125/tags, resourceArn/limit/nextToken as query params"}
families:
  mesh_crud: {status: ok, note: "route matcher, HTTP methods (PUT create/update, GET describe, DELETE, GET list), ARN shape, error codes all verified against real serializer/deserializer source"}
  virtualnode_crud: {status: ok}
  virtualrouter_and_route_crud: {status: ok, note: "route paths correctly use singular /virtualRouter/{name}/routes (AWS API quirk), verified vs real SDK SplitURI"}
  virtualservice_crud: {status: ok}
  virtualgateway_and_gatewayroute_crud: {status: ok, note: "gateway route paths correctly use singular /virtualGateway/{name}/gatewayRoutes"}
  tags: {status: ok}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "2026-09-07 (gopherstack-jxsz): meshOwner is now read and enforced on all 31 ops that carry it (30 sub-resource CRUD/List ops + DescribeMesh — verified count via aws-sdk-go-v2/service/appmesh@v1.38.4, grep -l MeshOwner api_op_*.go). gopherstack still has no AWS RAM cross-account mesh-sharing model — one InMemoryBackend is always exactly one account (provider.go/store.go), so no mesh can ever really be owned by a different account. Given that, a meshOwner naming any account other than the caller's own is rejected with ForbiddenException (declared on every op touched — see errors row below) via a single Handler.checkMeshOwner helper wired at 7 call sites (the 6 sub-resource dispatch functions, which each already funnel every Create/Describe/Update/Delete/List of that resource type through one entry point, plus handleDescribeMesh). meshOwner omitted, or equal to the caller's own account, is the documented default and is unchanged. This makes ResourceMetadata.MeshOwner/.ResourceOwner honest rather than fixed: they are still always the caller's account, but that is now the *only reachable* value, not a value nobody checked — the CreateVirtualNodeInput-family doc comment (aws-sdk-go-v2/service/appmesh@v1.38.4/api_op_CreateVirtualNode.go) explicitly requires 'the account that you specify must share the mesh with your account before you can create the resource in the service mesh', and Describe/Update/Delete/List's doc comment implies the same via 'it's the ID of the account that shared the mesh with your account' — since nothing is ever shared here, rejecting is the faithful behavior, not echoing the client-supplied value into the response (which would fabricate a cross-account story). What remains structurally divergent and NOT fixed: genuine cross-account shared-mesh access (a caller legitimately operating on a mesh owned by a different, real account) cannot be exercised at all — that requires a second-account resource-visibility/RAM-sharing model this backend has nowhere. Not adjacent-fixable without inventing that model."
  - "RouteSpec/VirtualNodeSpec/VirtualGatewaySpec/GatewayRouteSpec remain opaque json.RawMessage with no structural validation. Sized this pass by reading every reachable sub-shape in aws-sdk-go-v2/service/appmesh@v1.38.4/types/types.go (199 type declarations total): VirtualNodeSpec fans out through Listener (PortMapping, VirtualNodeConnectionPool union, HealthCheckPolicy, OutlierDetection, ListenerTimeout union, ListenerTls with ACM/File/SDS certificate variants and validation-context variants), Backends (VirtualServiceBackend with ClientPolicy/TLS), BackendDefaults, Logging (AccessLog file/stream variants), and ServiceDiscovery (DNS/AWSCloudMap variants). RouteSpec fans out through GrpcRoute/HttpRoute/Http2Route/TcpRoute, each with its own Action(WeightedTargets)/Match(headers/metadata/path/query variants)/RetryPolicy/Timeout. VirtualGatewaySpec and GatewayRouteSpec mirror the same listener/matcher depth. This is 4-5+ levels deep with multiple smithy union types per branch — too large to model to full field depth in one pass per the no-stub/model-faithfully-or-leave-it rule. Left as wire-compatible passthrough (whatever the client sends round-trips unchanged)."
deferred: []              # nothing consciously left un-audited this pass
leaks: {status: clean, note: "single coarse lockmetrics.RWMutex per backend (matches pkgs-catalog.md convention); no goroutines, timers, or janitors in this service"}
---

## Notes

**2026-08-19 sweep: corrected a false "wrapper-key bug" recorded by a prior pass — the
flat (unwrapped) body is, and always was, correct.** This file previously claimed (with
a fabricated `last_audit_commit: 40f05928` — that hash actually belongs to an unrelated
`codestarconnections` commit, not to any appmesh change) that every single-resource
Create/Describe/Update/Delete response needed wrapping under a resource-name key
(`{"mesh": {...}}` etc.), citing `awsRestjson1_deserializeOpDocument<Op>Output` functions
in `deserializers.go` that do read a `"mesh"`/`"virtualNode"`/etc. key. That claim was
never actually applied to the handler code (`meshToWire(m)` etc. was, and remains, always
returned flat) and referenced a nonexistent `parity_a_test.go` and nonexistent
`keyMesh`/`keyVirtualNode` constants — i.e. it was a hallucinated audit note, not a
description of a real change.

This pass re-verified from scratch and applied the wrapping fix, which immediately broke
under a real `aws-sdk-go-v2` client round-trip test (`sdk_roundtrip_test.go`,
`TestSDKRoundTrip_ResourceWrapping`): `CreateMeshOutput.Mesh` decoded to a non-nil but
all-fields-nil struct. Tracing why revealed the actual mechanism: `deserializers.go`
generates an `awsRestjson1_deserializeOpDocumentCreateMeshOutput`-style function for
every op (this is what a naive grep for `case "mesh":` finds and what misled the prior
pass), but for single-resource CRUD ops that function is **dead code — never called**.
The function actually wired into the middleware stack is each op's own
`awsRestjson1_deserializeOp<Op>.HandleDeserialize`, which for these ops decodes the raw
top-level response body **directly** into the resource's `*Data` type via
`awsRestjson1_deserializeDocument<Resource>Data(&output.<Field>, shape)` — confirmed at
`deserializers.go:244` (CreateMesh), `:1452` (DeleteMesh), `:2639` (DescribeMesh), `:5431`
(UpdateMesh), and the equivalent line in every other family's `HandleDeserialize`. No
wrapper key is read at all. This matches AppMesh's REST-JSON `@httpPayload`-style
single-member output shape. The wrapping "fix" was reverted (net diff against the
handler files in this sweep is zero); the test that caught it was kept as permanent
wire-compat coverage; and the hand-revert was re-run in the other direction to confirm
the flat shape is what the real client needs (reintroducing the wrapper key reproduces
the exact same nil-field panic).

List operations (`ListMeshes`, `ListVirtualNodes`, ...) are different: their own
`HandleDeserialize` genuinely does call the `awsRestjson1_deserializeOpDocumentList
<X>Output` helper (verified the same way, e.g. `ListMeshesOutput` has two members —
`Meshes` and `NextToken` — so no single field can carry the whole payload). That helper
reads the plural key (`{"meshes": [...], "nextToken": "..."}`), which is exactly what
gopherstack's `listResp` already emits and was never in question.

**Secondary bug (fixed): `limit` query param was silently ignored.** All seven List*
operations bind their client-supplied max-page-size to a `limit` query parameter (see
`ListMeshesInput.Limit`, confirmed via `awsRestjson1_serializeOpHttpBindingsListMeshesInput`
etc. in the real SDK) — not `maxResults`. gopherstack's `listParams` helper only read
`nextToken` and hardcoded `maxResults` to the 100-item default regardless of what the
client requested, so any SDK caller (or integration test) that set a smaller page size to
exercise pagination behavior would silently get up to 100 items back with no
`nextToken`-driven paging. Fixed to parse `c.QueryParam("limit")`.

**Style/consistency fix (not a wire bug): raw `sync.RWMutex` → `lockmetrics.RWMutex`.**
`backend.go`'s single coarse backend lock was a bare `sync.RWMutex`, which
`pkgs-catalog.md` explicitly forbids ("Never scatter raw sync.Mutex/sync.RWMutex in
services — use lockmetrics.RWMutex or safemap.Map"). Converted to
`*lockmetrics.RWMutex` with per-operation labels (`b.mu.Lock("CreateMesh")` etc.),
matching the pattern used by mediapackage/sesv2/mwaa/etc. Required a
`fieldalignment`-driven field reorder in the `InMemoryBackend` struct (the mutex went
from a large value type to an 8-byte pointer).

**Wire-shape items verified correct, no fix needed:**
- Error codes/HTTP statuses (`BadRequestException`=400, `ConflictException`=409 for
  "already exists" on Create, `ResourceInUseException`=409 for in-use conflicts on
  Delete, `NotFoundException`=404, default `InternalServerErrorException`=500) all match
  the real `types/errors.go` shape definitions and the botocore `service-2.json` model's
  per-operation error lists exactly.
- `metadata.{arn,createdAt,lastUpdatedAt,meshOwner,resourceOwner,uid,version}` field set
  and epoch-seconds timestamp encoding match `awsRestjson1_deserializeDocumentResourceMetadata`.
  `status.status` nested-object shape (not a bare string) matches
  `awsRestjson1_deserializeDocumentMeshStatus`/`VirtualNodeStatus`/etc.
- ARN path shapes (`mesh/{name}`, `mesh/{name}/virtualNode/{name}`,
  `mesh/{name}/virtualRouter/{vr}/route/{r}`, etc.) match AWS App Mesh's real ARN scheme.
  `mesh/{name}/virtualRouter/{vr}/routes` (plural collection) vs. singular
  `virtualRouter` path segment for the Route family, and `virtualGateway`/`gatewayRoutes`
  for the GatewayRoute family — both AWS API path-naming quirks — are correctly modeled
  in the route matcher (`handleRoutes`/`handleGatewayRoutes`), matching `SplitURI` calls
  in the real serializer for `CreateRoute`/`CreateGatewayRoute`.
- The error-body shape `{"code": ..., "message": ...}` (no `X-Amzn-ErrorType` header) is
  compatible with the real client's error deserialization: smithy-go's
  `restjson1.deserializeError` resolves the error code from either the header or a
  case-insensitive JSON `code`/`__type` field, so the header-less `{"code": ...}` body
  this service returns is correctly picked up by `ResolveProtocolErrorType`.
- Snapshot/Restore: `Handler.Snapshot`/`Handler.Restore` already delegate to the backend
  (`persistence.go`), which round-trips every `store.Table` plus the `tags` map via a
  versioned `backendSnapshot`. No gap here.

**Traps for the next auditor (looks-wrong-but-correct):**
- `TagResource`/`UntagResource` are `PUT`, not `POST` — despite the "tag mutation = POST"
  intuition from other AWS services, App Mesh's real serializer uses `PUT
  /v20190125/tag` and `PUT /v20190125/untag`. Don't "fix" this to POST.
- `ListTagsForResource` takes `resourceArn` as a query param (`GET /v20190125/tags?resourceArn=...`),
  while `TagResource`/`UntagResource` take `resourceArn` in the JSON body despite also
  being simple verb-path operations — this is per-operation httpBinding vs. httpPayload,
  not a stylistic inconsistency to "fix".

## 2026-07-23 sweep

This pass independently re-field-diffed every op/error/wire-shape claim in the prior
audit (rather than trusting the "ok" statuses at face value) against
`aws-sdk-go-v2/service/appmesh@v1.36.2`'s `deserializers.go`/`types/types.go`/`types/errors.go`
and the botocore `appmesh/2019-01-25/service-2.json` model directly (per-operation error
lists, `ResourceName`/`TagKey`/`TagValue`/`TagList` shape constraints). All prior "ok"
claims held up (wrapper keys, `metadata`/`status` nesting, ARN path quirks, error
code/HTTP-status mapping, PUT-not-POST tag verbs, cascade-delete checks on
mesh/virtualRouter/virtualGateway all confirmed correct by direct code read, not
re-guessed). Two real gaps were found and fixed this pass:

1. **`TooManyTagsException` (fixed).** The botocore model's `TagList` shape declares
   `{"max": 50}` and `TagResource`'s per-operation error list includes
   `TooManyTagsException` (distinct from the generic `BadRequestException` — real SDK
   clients `errors.As` against the typed exception, so misreporting the wire `code` as
   `"BadRequestException"` would break that check). `InMemoryBackend.TagResource`
   (`tags.go`) now computes the post-merge tag count before committing and returns
   `ErrTooManyTags` (new sentinel, deliberately NOT wrapping `awserr.ErrInvalidParameter`
   so `Handler.mapErr` can select the `TooManyTagsException` wire code independently of
   the generic 400 path) once the merged set would exceed 50 — matching the established
   pattern already used by `acmpca`/`fis`/`kinesisanalytics`/`rolesanywhere` in this
   codebase for the same real AWS per-resource tag cap. Rejection is all-or-nothing (no
   partial tag application), matching the real API's documented behavior ("None of the
   tags in this request were applied"). Covered by
   `TestAppMesh_TagResourceTooManyTags` in `tags_test.go`.
2. **Missing resource-name length validation (fixed).** The botocore model's
   `ResourceName` shape (used by `meshName`/`virtualNodeName`/`virtualRouterName`/
   `routeName`/`virtualServiceName`/`virtualGatewayName`/`gatewayRouteName`) declares
   `{"max": 255, "min": 1}` — the model has no regex pattern beyond length, so this is
   the full validation surface, not a partial fix. The min-1 (non-empty) side was already
   enforced per-Create-handler; only the 255-char max was missing. Added
   `isValidResourceName` (`handler.go`) and wired it into all seven Create handlers'
   existing required-field checks, replacing the old bare `== ""` comparisons.  Covered
   by `TestAppMesh_MeshNameTooLong` in `meshes_test.go` (boundary-tested at exactly 255,
   which must still succeed, and 256, which must reject).

**CloudTrail-capture item (previously "deferred", now resolved as not-a-gap):** read
`pkgs/service/cloudtrail_capture.go`'s `wrapCloudTrailCapture` — it is applied generically
by the central `Registry` around every registered service's handler chain using only the
`Registerable`/`ResourceObserver` contract (`svc.ExtractOperation(c)` /
`svc.ExtractResource(c)`) that `appmesh.Handler` already implements correctly (verified
`parseOperation` covers every op name including the nested Route/GatewayRoute families;
`ExtractResource` returns the mesh name). Read-only ops (`Describe*`/`List*`) are
correctly excluded from capture by the registry's generic `Get/List/Describe/...` prefix
filter — App Mesh's operation names already follow that convention, no service-specific
carve-out needed. No appmesh-specific code was required or missing here; this needed no
fix, just confirmation, so it moved out of `deferred` rather than staying open.

**Not changed (reconfirmed as correctly out of scope):** `ForbiddenException` (IAM
policy denial) and `LimitExceededException`/`TooManyRequestsException`
(account-resource-count / throttling limits) appear in the real per-operation error
lists for every Create op, but this backend — like the rest of gopherstack — has no IAM
enforcement layer or account-quota model to source them from; fabricating arbitrary
quota numbers would be inventing behavior, not fixing a diffed gap. `TooManyTagsException`
above is different: it has one universally-documented, unambiguous limit (50) actually
enforced by real AWS, matching the existing codebase-wide precedent.

## 2026-08-10 sweep

Three follow-up items from the 2026-07-23 sweep's `gaps` list were re-examined.

**SDK pin check.** `sdk_module` recorded `v1.36.2`; `go.mod` pins `v1.38.4`, so the
manifest was stale. Diffed the two module-cache trees
(`aws-sdk-go-v2/service/appmesh@v1.36.2` vs `@v1.38.4`): every changed file differs only
in client middleware plumbing (retry/logging/span/user-agent stack wiring,
`newServiceMetadataMiddleware` signature) — `types/enums.go` and the wire-shape-relevant
parts of `types/types.go` are byte-identical between the two versions. No prior claim in
this file rested on the drift; `sdk_module` corrected to `v1.38.4`.

1. **DeleteMesh/etc. leaving status ACTIVE (fixed — was a genuine wire gap, not
   "unconfirmed").** Fetched the live AWS App Mesh API reference
   (docs.aws.amazon.com/app-mesh/latest/APIReference/API_DeleteMesh.html), which documents
   a full example response with `"status": {"status": "DELETED"}`. Cross-checked against
   `aws-sdk-go-v2/service/appmesh@v1.38.4/types/enums.go`: every one of the seven
   `*StatusCode` enums (`MeshStatusCode`, `VirtualNodeStatusCode`,
   `VirtualRouterStatusCode`, `RouteStatusCode`, `VirtualServiceStatusCode`,
   `VirtualGatewayStatusCode`, `GatewayRouteStatusCode`) declares a `DELETED` member, so
   the enum value is present in the pinned SDK, not merely reachable via some other path.
   Applying the contradiction test: a resource reporting `ACTIVE` after a *successful*
   delete is a stronger false claim than a resource merely never leaving its starting
   state, and grep confirmed `Status` was set to `"ACTIVE"` at creation and never touched
   again anywhere in the package (including every `Delete*` backend method). Fixed all
   seven `Delete*` backend methods (`meshes.go`, `virtual_nodes.go`, `virtual_routers.go`,
   `virtual_services.go`, `virtual_gateways.go`) to set the resource's `Status` to the new
   `statusDeleted = "DELETED"` constant (`store.go`) after removing it from its table, right
   before returning it in the response. Covered by table-driven
   `TestBackend_DeleteReturnsTerminalStatus` (`delete_status_test.go`, one subtest per
   resource type) and `TestAppMesh_DeleteMeshWireStatus` (HTTP-level wire check). Verified
   both tests failed against the pre-fix code (asserted `"DELETED"`, got `"ACTIVE"`)
   before the fix landed.

2. **meshOwner query param (confirmed still a genuine gap, not a silent drop).** Grepped
   the whole package for `QueryParam` — only `resourceArn`, `nextToken`, and `limit` are
   ever read; there is no `c.QueryParam("meshOwner")` call anywhere. So this is not "read,
   validated, and discarded" (there is no read site to drop the value at all) — it is a
   structural absence: the backend has no second-account resource-visibility model
   anywhere (`MeshOwner`/`ResourceOwner` are always the calling account's own accountID).
   Confirmed against the current SDK pin that `meshOwner` is still modeled identically
   (plain querystring `AccountId` param, no extra validation). Left unfixed — implementing
   it for real means building cross-account visibility, not wiring an unread field.

3. **Spec structural validation, sized and partially fixed.** Read every type reachable
   from each of the seven spec shapes in `aws-sdk-go-v2/service/appmesh@v1.38.4/types/types.go`
   (199 total type declarations in the file). Three specs are genuinely shallow:
   - `MeshSpec` — two optional fields, `egressFilter.type` (enum `ALLOW_ALL`/`DROP_ALL`)
     and `serviceDiscovery.ipPreference` (enum, one of four `IPv4_ONLY`/`IPv4_PREFERRED`/
     `IPv6_ONLY`/`IPv6_PREFERRED` values) — 2 levels deep, no unions.
   - `VirtualRouterSpec` — one field, `listeners[].portMapping.{port,protocol}`, where
     `port` is required and must be 1-65535 (confirmed via
     docs.aws.amazon.com/app-mesh/latest/APIReference/API_PortMapping.html — "Valid Range:
     Minimum value of 1. Maximum value of 65535") and `protocol` is a required enum
     (`http`/`tcp`/`http2`/`grpc`) — 2 levels deep, no unions.
   - `VirtualServiceSpec` — one field, `provider`, a smithy union (exactly one of
     `virtualNode`/`virtualRouter` may be set, each requiring its name field) — 2 levels
     deep, one union.

   These three now get real structural validation (`spec_validate.go`, wired into
   `CreateMesh`/`UpdateMesh`, `CreateVirtualRouter`/`UpdateVirtualRouter`,
   `CreateVirtualService`/`UpdateVirtualService`): wrong JSON types, invalid enum members,
   out-of-range ports, and malformed unions are now rejected with `BadRequestException`
   (`awserr.ErrInvalidParameter`) instead of silently accepted. Unrecognized top-level
   fields are still tolerated (matches real AWS's forward-compatible deserialization —
   this is deliberate, not an oversight). Covered by table-driven
   `TestBackend_MeshSpecValidation`, `TestBackend_VirtualRouterSpecValidation`,
   `TestBackend_VirtualServiceSpecValidation` (`spec_validate_test.go`); all wantErr cases
   verified to pass with no error against the pre-fix code before the validators were
   added.

   **Stopped here.** `VirtualNodeSpec`, `RouteSpec`, `VirtualGatewaySpec`, and
   `GatewayRouteSpec` remain opaque `json.RawMessage` passthrough — each fans out 4-5+
   levels deep through multiple smithy union types (listener connection-pool/timeout
   variants, TLS certificate-source and validation-context variants, HTTP/gRPC/TCP
   route match+action+retry+timeout shapes, access-log file/stream variants, service
   discovery DNS/AWS Cloud Map variants). Modeling these to full field depth is real,
   multi-day work, not a same-pass fix; per the no-stub/model-faithfully-or-leave-it rule
   they were left alone rather than partially modeled. See `gaps` above for the full
   type-by-type breakdown.

### 2026-08-21 gopherstack-r80d batch 13: required-output cut, 0 bugs

Selected as the second service this batch (after vpclattice) per
`services/_REQUIRED_OUTPUT_CANDIDATES.md`'s ranked table: 36 required
output fields / 38 ops (36 with at least one — nearly every op), confirmed
with a fresh `go run ./cmd/requiredoutputfields` run against
`appmesh@v1.38.4`. `git status` confirmed no concurrent-agent WIP on this
service before starting.

appmesh's op-level required set is the "one wrapper key" shape (every
Create/Describe/Update/Delete op wraps its whole response in one required
domain-object member: `Mesh`, `VirtualNode`, `VirtualRouter`, `Route`,
`VirtualService`, `VirtualGateway`, `GatewayRoute`; every List op wraps an
array under `Meshes`/`VirtualNodes`/etc.), so the flat 36/38 count
undercounts the real surface the same way pinpoint/bedrockagent/cleanrooms/
inspector2 did. Read every domain struct with `This member is required.`
in `aws-sdk-go-v2/service/appmesh@v1.38.4/types/types.go` (an AST-style
walk of all 90+ struct declarations, not a grep window) to find the real
surface: each `<X>Data` struct (`MeshData`, `VirtualNodeData`, etc.)
requires its own name field(s), `Metadata` (`ResourceMetadata`: Arn,
CreatedAt, LastUpdatedAt, MeshOwner, ResourceOwner, Uid, Version — 7
fields, shared by every resource type), `Spec`, and `Status` (a
single-member wrapper, e.g. `MeshStatus.Status`); each `<X>Ref` struct used
by List ops (`MeshRef`, `VirtualNodeRef`, etc.) requires the same
Arn/CreatedAt/LastUpdatedAt/MeshOwner/ResourceOwner/Version set plus its
own name field(s). `TagRef` (`ListTagsForResource`) requires Key/Value.

**One apparent finding that was NOT a bug, verified rather than assumed:**
this campaign's brief specifically warns that a `wire: ok` verdict can be
wrong because raw-body tests can't catch a wrong wire contract. This
service's own `handler_wire_test.go` doc comment (point 13) asserts
"single-resource responses put fields at the response root -- there is no
mesh/virtualNode/etc. wrapper key", which — combined with the real SDK
requiring `Mesh`/`VirtualNode`/etc. as top-level required members of each
`<Op>Output` struct — looked exactly like the class of bug this campaign
exists to find (a missing wrapper key, same shape as opensearch's
`GetIndex`/bedrock's `AutomatedReasoningPolicyTestCase`). A quick read of
`deserializers.go`'s `awsRestjson1_deserializeOpDocumentCreateMeshOutput`
(the unused codegen "OpDocument" helper this campaign has already learned
not to trust, per batch 5's pinpoint note) appeared to confirm it: that
helper switches on `case "mesh":` and would leave `Mesh` nil against
gopherstack's actual unwrapped response. **Rather than counting this as a
bug from static reading alone, it was checked against a real
`aws-sdk-go-v2/service/appmesh` client round trip** (a throwaway probe
test, discarded after use, not committed) — `CreateMesh` returned
`out.Mesh` fully populated. Reading the *actual* per-operation deserializer
(`awsRestjson1_deserializeOpCreateMesh`'s `HandleDeserialize`, not the
unused `OpDocument` helper) showed why: it decodes the raw response body
directly into `MeshData` via `awsRestjson1_deserializeDocumentMeshData(&output.Mesh,
shape)` with no wrapper key at all — `Mesh` is an implicit httpPayload-style
binding to the whole body, and the `case "mesh":` switch arm in the unused
helper is dead code for restjson1's actual code path, exactly like pinpoint's
`OpDocument` trap. gopherstack's flat-root shape is correct; the test
suite's doc comment was right, and this campaign's own method (verify
against the real client, not the static shape) caught a would-be false
positive before it became a wasted "fix."

With that resolved, every op was read end to end against its handler:
`metaToWire`/`vnToWire`/`vrToWire`/`routeToWire`/`vsToWire`/`vgToWire`/
`grToWire` (all in `handler.go` and the per-resource `handler_*.go` files)
emit all 7 `ResourceMetadata` fields, the resource name field(s), `spec`
(via `specOrEmpty`, which returns `{}` rather than `null` when unset — the
"required-but-inapplicable means present-and-empty, not absent" shape,
already handled correctly), and `status` as the required single-member
wrapper object. Every `*SummaryToWire` function emits the full matching
`*Ref` required set. Every List handler builds its slice with
`make([]any, 0, len(items))`, never a nil slice, so a required list output
is always `[]` not `null` for an empty mesh. Cross-checked timestamp/type
shapes directly against the real deserializers (not assumed): `createdAt`/
`lastUpdatedAt` are JSON-number epoch seconds (`smithytime.ParseEpochSeconds`)
matching `.Unix()`; `version` is a JSON-number `Long` matching the `int64`
field; `status` is a bare string, not an object — no wrong-type member (the
class this batch's brief calls out as most severe) found anywhere in this
service.

Zero bugs. No fix, no new test needed (nothing to prove). Gates run scoped
to `services/appmesh`: `go build ./...`, `go vet ./services/appmesh/...`,
`gofmt -l services/appmesh/` (0 output), `go test -race
./services/appmesh/...`, `golangci-lint run ./services/appmesh/...` (0
issues). No files changed in this service; only this PARITY.md note and
`services/_REQUIRED_OUTPUT_CANDIDATES.md` were touched.

`services/_REQUIRED_OUTPUT_CANDIDATES.md` updated: appmesh moved from the
ranked table into "Already examined" (settled-services count now 28, 2079
required output fields read end to end).

### 2026-08-29 wrapper-key/silent-drop sweep (bd gopherstack-6flj/21my): zero bugs

Independent write-only-state pass over `aws-sdk-go-v2/service/appmesh@v1.38.4`
(pin unchanged, reconfirmed against go.mod), separate from and in addition
to the four prior dated sweeps above. `go run ./cmd/enumcheck`,
`./cmd/acceptguard`, `./cmd/zeroguard`, and `./cmd/xmlitemwrap` all produced
zero findings for appmesh this pass.

Specifically re-checked, not just re-trusted from prior "ok" statuses:

- Every List op's query-param surface (`limit`/`nextToken`, plus
  `TagResource`/`UntagResource`/`ListTagsForResource`'s `resourceArn`) --
  confirmed no App Mesh List op accepts an ordering/filter param beyond
  those already modeled (unlike swf's `ListOpen/ClosedWorkflowExecutions`,
  which turned out to drop `ReverseOrder` -- App Mesh's List ops have no
  such member in the real SDK to drop).
- `ListTagsForResourceInput.Limit` real range/default (1-100, default 100
  per `api_op_ListTagsForResource.go`) matches gopherstack's existing
  `listParams` default -- no drift.
- `RouteData`/`GatewayRouteData`/`VirtualServiceData` required-member sets
  spot-re-read directly from `types/types.go` (not from the batch-13 note)
  as a sampling check against this pass's own claim rather than trusting
  the prior pass's count -- unchanged, still correctly emitted by
  `routeToWire`/`grToWire`/`vsToWire`.

No new bug found. This service has now been independently swept four times
(2026-07-23, 2026-08-10, 2026-08-19, 2026-08-21, 2026-08-29) with the last
three finding zero new wire bugs -- consistent with a genuinely small,
already-well-covered REST surface (38 ops, 7 resource families, no
List-op filtering/ordering complexity), not evidence that no further sweep
is needed (per this campaign's own "nineteen for nineteen" standing rule,
a clean pass is recorded honestly rather than a bug being manufactured to
match a quota). **Not reached this pass:** the opaque
`RouteSpec`/`VirtualNodeSpec`/`VirtualGatewaySpec`/`GatewayRouteSpec`
`json.RawMessage` passthrough fields (see `gaps` above -- structural, sized
and explicitly deferred by the 2026-08-10 sweep, not re-examined here) and
the `meshOwner` cross-account gap (also `gaps`, unchanged). No files in
this service were modified this pass; only this note was added.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

Audited this package's pagination for the Class A/B/C shapes found
elsewhere in this campaign. No bug found — this is the one hand-rolled
paginator across all eight services audited this pass whose cursor design
structurally can't take the equality-miss-defaults-to-zero shape at all.

`paginateStrings` (`store.go`) backs 7 List operations (`meshes.go`,
`virtual_gateways.go` x2, `virtual_services.go`, `virtual_nodes.go`,
`virtual_routers.go` x2). Its token is the **last** item returned on a page
(not the next page's first item, unlike every other cursor convention seen
this pass), and resuming searches for the first sorted name **strictly
greater than** the token — a threshold, not an exact match. A name deleted
since the token was issued still resolves correctly to the next surviving
name (nothing to match, so nothing to silently default to 0); an
exhausted or entirely-tampered cursor is caught by an explicit guard
(`start == 0 && (empty || sorted[0] <= nextToken)`) that returns no items
and no cursor, never a restart at page one.

All seven checks pass, including a stale cursor naming a genuinely deleted
item between the resume point and the next survivor
(`pagination_arithmetic_internal_test.go`), and a real
`aws-sdk-go-v2/service/appmesh` `ListMeshes` round trip that deletes such
an item between calls (`pagination_sdk_roundtrip_test.go`).

Gates: `go build ./services/appmesh/...`, `go vet ./services/appmesh/...`
and `go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/appmesh/...`, `golangci-lint run ./services/appmesh/...` — 0
issues introduced this pass; one pre-existing, unrelated `unparam` finding
on `newTestHandlerAndClient` (`sdk_roundtrip_helper_test.go`, present in
HEAD before this pass, its only other caller already ignores the same
return value) was left untouched as out of this pass's scope. No
production code changed this pass — test-only additions confirming
correctness.
