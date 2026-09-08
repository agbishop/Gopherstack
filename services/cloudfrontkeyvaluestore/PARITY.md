---
service: cloudfrontkeyvaluestore
sdk_module: aws-sdk-go-v2/service/cloudfrontkeyvaluestore@v1.15.4
last_audit_commit: 37229aaf1
last_audit_date: 2026-09-04
overall: B
ops:
  DescribeKeyValueStore: {wire: ok, errors: ok, state: ok, persist: ok, note: "ItemCount/TotalSizeInBytes computed from real per-store data; see gaps for the byte-accounting approximation"}
  GetKey: {wire: ok, errors: ok, state: ok, persist: ok}
  PutKey: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteKey: {wire: ok, errors: ok, state: ok, persist: ok}
  ListKeys: {wire: ok, errors: ok, state: ok, persist: ok, note: "MaxResults/NextToken pagination via pkgs/page"}
  UpdateKeys: {wire: ok, errors: ok, state: ok, persist: ok, note: "all-or-nothing per the real API is NOT modeled -- see gaps"}
gaps:
  - "TotalSizeInBytes is len(key)+len(value) summed per item. AWS's real byte accounting includes undocumented per-item overhead this emulator cannot replicate exactly; the number is real and deterministic (derived from actual stored data, not fabricated) but will not byte-for-byte match a real account. (bd: gopherstack-4ara)"
  - "UpdateKeys is not transactional: puts and deletes apply sequentially against the shared InMemoryBackend lock rather than as a single all-or-nothing batch. A backend error partway through (never currently possible, since PutKVSValue/DeleteKVSValue on an already-validated store/ETag cannot fail mid-batch) would leave a partial result. (bd: gopherstack-4ara)"
  - "No per-store size/count quotas enforced and no AccessDeniedException path (no IAM enforcement in this emulator) -- see errors.go's doc comment. The AWS Developer Guide's 'Quotas on key value stores' table (docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/cloudfront-limits.html#limits-keyvaluestores; not stated in the SDK doc comments themselves) documents: max key size 512 Bytes, max value size 1 KB, max UpdateKeys batch 50 keys or 3 MB payload, max individual store size 5 MB, max key value stores per account 200. None of these are enforced here, so ServiceQuotaExceededException/AccessDeniedException are never returned, though both are in the real client's exception set for several ops. (bd: gopherstack-4ara)"
structural_gaps:
  - "None. Every op here reads or mutates real per-KVS-store key/value state (services/cloudfront's keyValueStoreData/keyValueDataETags) -- there is no billing/ML/hardware dependency that would make any of these ops structurally unimplementable."
deferred: []
leaks: {status: clean, note: "Handler owns no goroutines, janitors, or independent maps -- see Handler's doc comment and persistence_test.go's TestHandler_OwnsNoState guard."}
---

## Notes

**Why this package exists** (gopherstack-4ara): AWS splits CloudFront's
KeyValueStore surface across two SDK clients/protocols. `cloudfront.Client`
(services/cloudfront) owns the *control plane*
(Create/Get/List/Delete/UpdateKeyValueStore, path
`/2020-05-31/key-value-store/...`, REST-XML). `cloudfrontkeyvaluestore.Client`
(this package) owns the *data plane* -- the actual key/value pairs inside a
store -- at an entirely different, unversioned path family
(`/key-value-stores/{KvsARN}/...`, REST-JSON, `ServiceID = "CloudFront
KeyValueStore"`, its own `AWS_ENDPOINT_URL_CLOUDFRONT_KEYVALUESTORE` env var).
A prior gopherstack pass implemented GetKey/PutKey/DeleteKey/ListKeys/
UpdateKeys as handlers *inside* services/cloudfront, routed under the
REST-XML `/2020-05-31/` prefix -- reachable by nothing, since no real
`cloudfrontkeyvaluestore` client request ever carries that prefix. The
underlying backend state (services/cloudfront's `keyValueStoreData`/
`keyValueDataETags` maps and their `GetKVSValue`/`PutKVSValue`/
`DeleteKVSValue`/`ListKVSValues`/`UpdateKVSValues` methods) was real, not
fabricated -- only the HTTP-layer routing and wire shape were wrong. This
package replaces the dead handlers with correct routing and wire shape, wired
(cli.go's `wireCloudFrontKeyValueStore`) directly to that same
`*cloudfront.InMemoryBackend`, mirroring how services/dynamodbstreams borrows
services/dynamodb's backend rather than owning a duplicate store.

**Wire shape, verified against cloudfrontkeyvaluestore@v1.15.4
serializers.go/deserializers.go directly** (not assumed from the prior dead
code, which got several of these wrong):

- JSON field names are **PascalCase** (`Key`, `Value`, `ItemCount`,
  `TotalSizeInBytes`, `KvsARN`, `Status`, `Created`, `LastModified`), not
  lowerCamelCase like most other restJson1 services in this repo.
- `KvsARN` and `Key` are URI path segments, percent-encoded by the real
  client (the ARN contains `:` and `/`). Decoding must happen per-segment,
  not on the whole decoded path, or the ARN's embedded `/` fragments the
  route -- same "ARN-in-path route-matching trap" as services/grafana and
  services/s3tables; `rawPathSegments` in handler.go is the same fix.
- `ETag` is **never** a JSON body field. It is an `ETag` response header on
  PutKey/DeleteKey/UpdateKeys/DescribeKeyValueStore outputs, and does not
  exist at all on GetKey/ListKeys outputs (verified: no
  `awsRestjson1_deserializeOpHttpBindings{GetKey,ListKeys}Output` function
  exists in the SDK).
- `DescribeKeyValueStoreOutput.ETag` is the **data-plane** ETag (the same
  value `ListKVSValues` returns), not the KeyValueStore resource's own
  control-plane ETag from services/cloudfront -- PutKeyInput's IfMatch doc
  comment says so explicitly ("which you can get using
  DescribeKeyValueStore"). Getting this wrong breaks the real
  Describe-then-Put/Delete/UpdateKeys workflow every real client uses, since
  IfMatch is a *required* field on Put/Delete/UpdateKeys.
- `Created`/`LastModified` are **epoch-seconds** JSON numbers
  (`smithytime.ParseEpochSeconds`), unlike services/cloudfront's own
  REST-XML API, which uses RFC3339 strings for the same underlying
  `KeyValueStore.CreatedTime`/`LastModifiedTime` fields -- `epochSeconds()`
  converts.
- `UpdateKeysInput.Deletes` is `[]{"Key": "..."}` objects, not a bare string
  array, despite carrying only a key.
- Error body is `{"message": "..."}` plus an `X-Amzn-Errortype` header
  naming the exception (`AccessDeniedException`, `ConflictException`,
  `InternalServerException`, `ResourceNotFoundException`,
  `ServiceQuotaExceededException`, `ValidationException` -- verified against
  each op's own `awsRestjson1_deserializeOpError<Op>` switch in
  deserializers.go, not assumed). **ETag mismatches map to `ConflictException`
  (409)**, not the HTTP 412 the removed services/cloudfront handlers used to
  send -- 412 does not appear anywhere in this SDK's error model.

**Testing this SDK client requires overriding `EndpointResolverV2`, not just
`BaseEndpoint`**: this service's endpoint ruleset derives a per-account-ID
virtual host from the `KvsARN` input, which `BaseEndpoint` alone does not
suppress -- see handler_test.go's `staticEndpointResolver`. Skipping this
makes every SDK-driven test fail with a DNS lookup on
`<accountID>.<host>`, not a gopherstack bug.

**services/cloudfront changes made alongside this package** (same commit):
added `KeyValueStore.CreatedTime` (needed for DescribeKeyValueStore's
required `Created` field, previously untracked) and persisted
`keyValueStoreData`/`keyValueDataETags` in `backendSnapshot`
(`cloudfrontSnapshotVersion` bumped 1 -> 2) -- the KVS data-plane key/value
pairs were silently dropped across a restart before this. Removed the dead
`/2020-05-31/key-value-store/{id}/keys/...` handlers, op constants, and
routing from services/cloudfront (kept the backend methods, now called by
this package instead).

## 2026-08-21: gopherstack-r80d batch 21 -- required-output-member cut, 0 bugs

Tied with sesv2 at 18 required output fields (5 ops, all 5 with >=1) per a
fresh `cmd/requiredoutputfields` run cross-checked against
`services/_REQUIRED_OUTPUT_CANDIDATES.md`; taken first (alphabetical tie).
Instrument cross-checked three ways (character-level brace matcher,
`go/parser` AST walk, raw `grep -c`) across `types/types.go` + every
`api_op_*.go` file -- all three agree at 52 total required fields / 22
structs (only 6 ops total in this SDK, so the surface is small and mostly
input-only).

`types/types.go`'s `ListKeysResponseListItem` (`Key`/`Value`, both
required) is the one nested-domain-struct undercount case here: it backs
`ListKeysOutput.Items`, which is itself NOT required (so `ListKeys` never
appeared in the flat per-op required-field list at all) -- checked anyway.
`wire.go`'s `keyValuePairJSON.Key`/`.Value` are plain, non-omitempty
strings, always present.

Every other required member is present on every real-client-reachable
path: `getKeyOutput`/`mutateKeyOutput` (`ItemCount`/`TotalSizeInBytes`/
`Key`/`Value`) are plain non-pointer fields with no `omitempty`.
`describeKeyValueStoreOutput`'s `KvsARN`/`Status`/`Created`/`ItemCount`/
`TotalSizeInBytes` are likewise always-present plain fields. `ETag`
(required on `DeleteKey`/`PutKey`/`UpdateKeys`/`DescribeKeyValueStore`) is
served as an HTTP header, not a body field -- confirmed the real
deserializer (`awsRestjson1_deserializeOpHttpBindingsPutKeyOutput`) only
sets it `if len(headerValues) != 0`, i.e. silently leaves it nil if the
transport ever omitted the header -- but `handler.go` sets the `ETag`
header unconditionally on all four ops, and the backing
`services/cloudfront`'s `kvsDataETag`/`PutKVSValue`/`DeleteKVSValue`/
`UpdateKVSValues` always generate a non-empty `uuid.NewString()`, so the
header is never actually absent or empty on a real client's request. No
code changes; see `services/_REQUIRED_OUTPUT_CANDIDATES.md`'s
settled-services table.

## 2026-09-04: parity sweep -- missing IfMatch silently bypassed the ETag check

`IfMatch` is a required member on `PutKeyInput`/`DeleteKeyInput`/
`UpdateKeysInput` (validators.go's
`validateOp{PutKey,DeleteKey,UpdateKeys}Input` in
cloudfrontkeyvaluestore@v1.15.4 each add
`smithy.NewErrParamRequired("IfMatch")` when unset; the field's own doc
comment says "This member is required."). `services/cloudfront`'s
`PutKVSValue`/`DeleteKVSValue`/`UpdateKVSValues` implement the comparison as
`if ifMatch != "" && ifMatch != currentETag`, which means an absent
If-Match header was treated as "skip the check" rather than "reject the
request" -- the same silently-bypassed-when-omitted bug class this
campaign found in wafv2's LockToken, codeartifact's policyRevision, and
s3tables's versionToken. A raw HTTP client (or any non-generated-SDK
caller) could mutate a store with no concurrency token at all.

Fixed in this package's handler.go (not services/cloudfront, since that
backend's relaxed contract is only ever reached via this package's three
mutating handlers in production -- `PutKVSValue`/`DeleteKVSValue`/
`UpdateKVSValues` have no other callers, confirmed by grep): added
`requireIfMatch`, called before each of `handlePutKey`/`handleDeleteKey`/
`handleUpdateKeys` invokes the backend, returning `ValidationException`
(400) when the `If-Match` header is empty. Regression test
`TestHandler_MutationsRequireIfMatch` in handler_test.go drives the
handler directly with raw HTTP requests (not the SDK client, which
validates this client-side and would never send such a request).
