---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: mediastoredata
sdk_module: aws-sdk-go-v2/service/mediastoredata@v1.32.4   # version audited against; unchanged from prior pass, confirmed still the go.mod pin
last_audit_commit: bce8159207
last_audit_date: 2026-09-04
overall: A            # this pass (gopherstack-5ce): PutObject accepted a body of ANY size -- PutObjectInput's doc comment (api_op_PutObject.go:13-14) states "Object sizes are limited to 25 MB for standard upload availability and 10 MB for streaming upload availability," and gopherstack enforced no such limit anywhere in the package. Fixed via a new ErrObjectTooLarge sentinel (errors.go, ValidationException wire type, same convention as ErrInvalidPath/ErrInvalidStorageClass/ErrInvalidUploadAvailability) and a size check in PutObject keyed on the (already-defaulted) uploadAvailability value. Covered by TestInMemoryBackend_PutObject_SizeLimit and TestMediaStoreData_ObjectSizeLimit. All other surface re-checked against the pinned v1.32.4 SDK this pass (field-level doc comments on all 5 *Input structs, types/errors.go's 4-exception list, types/enums.go) with no other discrepancies found -- see ops/families below.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  PutObject: {wire: ok, errors: ok, state: ok, persist: ok, note: "PutObjectOutput{ContentSHA256,ETag,StorageClass} JSON body fields verified against deserializeOpDocumentPutObjectOutput. errors: an earlier pass replaced 3 fabricated, non-existent exception names (InvalidPathException, InvalidStorageClassException, InvalidContentSHA256Exception) with real AWS error names -- ValidationException (path/storage-class/upload-availability validation, matching the AWS-wide/gopherstack-wide convention already used by this same handler's ListItems MaxResults check) and XAmzContentSHA256Mismatch (the real S3-family error for a declared X-Amz-Content-Sha256 that doesn't match the actual body, verified via AWS SDK GitHub issues across multiple language SDKs). Per the real deserializeOpErrorPutObject switch (re-confirmed this pass at the pinned v1.32.4 -- deserializers.go is byte-identical to v1.29.19), PutObject's OWN narrow modeled-error list is only {ContainerNotFoundException, InternalServerError} -- ValidationException/XAmzContentSHA256Mismatch are real AWS names but not officially enumerated for this specific op in the public smithy model (see gaps); this is the closest-to-correct choice achievable without live-AWS access, and a definite improvement over inventing new strings. BUG FIXED this pass: x-amz-upload-availability accepted and stored ANY string verbatim (no validation, no default) -- this is the exact 'emulator more permissive than the real service' inversion this pass was asked to hunt for: the real enum (types.UploadAvailability) has exactly 2 members (STANDARD, STREAMING) and PutObjectInput's doc comment states the default is 'standard' when omitted, matching the existing StorageClass empty->TEMPORAL default convention right above it in the same function. Now: empty defaults to STANDARD, unknown values are rejected with ValidationException via the new ErrInvalidUploadAvailability sentinel (errors.go), same as StorageClass. Covered by TestInMemoryBackend_UploadAvailability and TestMediaStoreData_UploadAvailabilityValidation. BUG FIXED this pass (2026-09-04, gopherstack-5ce): no object-size limit was enforced at all -- PutObjectInput's doc comment states a hard 25 MB (standard) / 10 MB (streaming) cap, evaluated against whichever upload availability the request resolves to (default or explicit). Fixed via ErrObjectTooLarge (ValidationException, same not-officially-enumerated-but-real-AWS-name convention as the other validation sentinels above) and a size check in InMemoryBackend.PutObject (objects.go) run after uploadAvailability's default/validation so the correct limit is selected. Covered by TestInMemoryBackend_PutObject_SizeLimit (backend-level, at/over both limits) and TestMediaStoreData_ObjectSizeLimit (wire-level, streaming limit, checks __type)."}
  GetObject: {wire: ok, errors: ok, state: ok, persist: ok, note: "body, Content-Type/Length/Range, ETag, Last-Modified, Cache-Control, X-Amz-Content-Sha256, Accept-Ranges all verified against deserializers.go httpBindings for GetObjectOutput. Range (single-range bytes=a-b, suffix, open) -> 206 + Content-Range; unsatisfiable -> 416. BUG FIXED this pass: the 416 response's __type was the fabricated \"InvalidRangeException\" even though the HTTP status (416) was already correct -- the real modeled name is \"RequestedRangeNotSatisfiableException\" (types.RequestedRangeNotSatisfiableException), confirmed via deserializeOpErrorGetObject's switch. A real client's errors.As(&types.RequestedRangeNotSatisfiableException{}) would NOT have matched gopherstack's old response; now it does. Conditional headers (If-Match/If-None-Match/If-Modified-Since/If-Unmodified-Since) implemented, not part of the modeled GetObjectInput but harmless/standard-HTTP. Per-op modeled errors confirmed via deserializeOpErrorGetObject: {ContainerNotFoundException, InternalServerError, ObjectNotFoundException, RequestedRangeNotSatisfiableException} -- all 4 real names, all correctly used."}
  DeleteObject: {wire: ok, errors: ok, state: ok, persist: ok, note: "Per-op modeled errors confirmed via deserializeOpErrorDeleteObject: {ContainerNotFoundException, InternalServerError, ObjectNotFoundException}. ObjectNotFoundException correctly used; DeleteObjectOutput is void (matches c.NoContent(200))."}
  DescribeObject: {wire: ok, errors: ok, state: ok, persist: ok, note: "HEAD /{Path+}; correctly does NOT support Range (DescribeObjectInput has no Range field in the real SDK) and does not set StatusCode (DescribeObjectOutput has no StatusCode field, unlike GetObjectOutput). Per-op modeled errors confirmed via deserializeOpErrorDescribeObject: {ContainerNotFoundException, InternalServerError, ObjectNotFoundException}. BUG FOUND AND FIXED this pass (2026-08-20): every error response (this op and all 4 others) carried its __type ONLY in the JSON body, never in the X-Amzn-ErrorType header that deserializeOpErrorDescribeObject (and every other op's deserializeOpError<Op>) checks FIRST, before ever attempting to decode a body. This was silently harmless for PUT/GET/DELETE (their error responses carry a body a real client CAN read) but a hard, 100%-reproducing break for DescribeObject specifically: DescribeObject is HEAD, and net/http's client transport unconditionally discards a HEAD response's body (RFC 7231 SS4.3.2) regardless of what the server wrote, so a real SDK caller could NEVER recover ObjectNotFoundException/InternalServerError/ContainerNotFoundException from a failed DescribeObject -- every failure silently degraded to an untyped smithy.GenericAPIError{Code:\"UnknownError\"}, which a caller's errors.As(&types.ObjectNotFoundException{}) (the normal, documented way to check for a 404) would never match. Reproduced against the REAL aws-sdk-go-v2 client via a new httptest+router round-trip (TestDescribeObject_SDKRoundTrip in wire_sdk_roundtrip_test.go, new this pass) and confirmed via hand-revert (commenting out just the header-Set line reproduces the exact original failure). Fixed via a new writeErrorJSON(c, status, code, msg) helper (handler.go) that sets X-Amzn-ErrorType alongside the existing JSON body, replacing all 7 c.JSON(status, errorResponse(...)) call sites in the package -- this also brings PutObject/GetObject/DeleteObject/ListItems's error responses one step closer to the real wire shape even though their bodies already worked."}
  ListItems: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET / always (Path/MaxResults/NextToken are query params, never part of the URL path -- confirmed via serializers.go: opPath is literally \"/\" for ListItems). Item{Name,Type,ContentLength,ContentType,ETag,LastModified} field set and JSON key names verified byte-for-byte against deserializeDocumentItem AND types.Item -- gopherstack's internal Item struct carries extra CacheControl/StorageClass/SHA256 fields but these correctly never reach the wire (handler.go's itemEntry struct only serializes the 6 real fields). LastModified emitted as epoch-seconds JSON number (matches ParseEpochSeconds), MaxResults bounded 1-1000, folder synthesis from path prefixes with object/folder name-collision dedup verified by TestInMemoryBackend_ListItems_NoNameCollision. Per-op modeled errors confirmed via deserializeOpErrorListItems: {ContainerNotFoundException, InternalServerError} only (no ObjectNotFoundException) -- gopherstack never returns ObjectNotFoundException from ListItems, correct."}
# Families audited as a group (when per-op is impractical):
families:
  routing: {status: ok, note: "RouteMatcher matches on the \"mediastoredata\" marker (msdMatchPriority=87 > S3's 0). Verified real aws-sdk-go-v2 UA marker is literally \"api/mediastoredata#<version>\" (api_client.go addClientUserAgent -> AddSDKAgentKeyValue(APIMetadata, \"mediastoredata\", ...)), so the substring match is correct and won't collide with plain \"mediastore\" (no data suffix). BUG FOUND AND FIXED this pass: the prior audit's \"verified correct\" claim above was true only for aws-sdk-go-v2 and did not hold for a browser. RouteMatcher checked only the User-Agent header; the Fetch spec forbids browser JS from setting User-Agent, so the AWS SDK for JavaScript in a browser puts its SDK identification exclusively in X-Amz-User-Agent instead -- every browser-originated MediaStore Data request (the dashboard's own UI, via @aws-sdk/client-mediastore-data) fell through to S3's priority-0 catch-all and was rejected with an S3 XML error the dashboard then failed to parse as JSON. Confirmed against the actual installed npm package (ui/node_modules/@aws-sdk/core's userAgentMiddleware: `if (options.runtime !== \"browser\") { headers[USER_AGENT] = ... } else { headers[X_AMZ_USER_AGENT] = ... }`) that the real browser marker is ALSO not simply \"mediastoredata\" in a different header: MediaStore Data's JS SDK serviceId is \"MediaStore Data\" (with a space), which the SDK's user-agent escaping turns into \"MediaStore-Data\" (hyphenated, PascalCase) -- a different literal string from aws-sdk-go-v2's module-path-derived \"mediastoredata\", not just a case difference. Fixed via the new pkgs/service.MatchesUserAgentMarker helper (shared with the same bug class in docdb/neptune/appsync), which checks both User-Agent and X-Amz-User-Agent case-insensitively; mediastoredata passes it both the \"mediastoredata\" and \"mediastore-data\" markers to cover both SDKs' spellings. ExtractOperation/Handler() dispatch on method (PUT/GET/DELETE/HEAD) then disambiguate GET ListItems-vs-GetObject purely by URL.Path == \"/\" -- this is CORRECT per the real SDK: ListItems always serializes to path \"/\" with Path as a query param, it is never a GET on a folder path (confirmed via awsRestjson1_serializeOpListItems: opPath, opQuery := httpbinding.SplitURI(\"/\")). GetObject/DescribeObject/PutObject/DeleteObject all serialize Path via {Path+} greedy URI capture, matched via r.URL.Path directly (no separate matcher regex to verify). TestSDKCompleteness confirms GetSupportedOperations() == exactly the 5 real SDK ops, no invented ops."
  persistence: {status: ok, note: "Handler.Snapshot/Restore delegate to backend; backendSnapshot versioned (mediastoredataSnapshotVersion=1), region-nested via store.Table[Object], round-tripped in persistence_test.go including all Object fields."}
gaps:
  - "ValidationException (used by PutObject/GetObject/DeleteObject/DescribeObject via ValidatePath, and separately by ListItems for an out-of-range MaxResults) and PutObject's XAmzContentSHA256Mismatch, while real AWS error names (unlike the fabricated names they replaced this pass), are not officially enumerated in ANY of mediastoredata's 5 per-op error models -- confirmed this pass by reading all 5 deserializeOpError* switches at the pinned v1.32.4: PutObject/ListItems={ContainerNotFoundException,InternalServerError}, DeleteObject/DescribeObject={+ObjectNotFoundException}, GetObject={+ObjectNotFoundException,RequestedRangeNotSatisfiableException} -- none list ValidationException. (The prior audit only checked/flagged this for PutObject and GetObject; it applies identically to DeleteObject, DescribeObject, and ListItems since they share the same ValidatePath/MaxResults-bound code paths.) All of these ARE reachable by a real (non-conformant-only) SDK caller: client-side validators.go on every op checks only that Path/Body != nil (never their content), and ListItemsInput has no client-side validator at all -- confirmed via `grep 'func validateOp' validators.go` returning only 4 hits (Delete/Describe/Get/PutObjectInput), so a malformed Path or out-of-range MaxResults always reaches the server. Without live-AWS access there is no way to confirm the exact wire name real AWS uses for these cases; ValidationException/XAmzContentSHA256Mismatch are the closest verified-real AWS error names and a strict improvement over the wholly-invented names they replaced. The empty-Path sub-case specifically IS unreachable via a real SDK client (client serializers reject Path==nil or len(Path)==0 with a local SerializationError before the request is ever sent), so that one branch can never be observed on the wire at all."
  - "x-amz-upload-availability STREAMING has no real chunked/progressive-download semantics (an object is only ever visible after PutObject fully returns) -- real MediaStore streams partial reads to STREAMING objects while still uploading and ignores the Range header for such objects mid-upload (api_op_GetObject.go:62-63: 'ignores this header for partially uploaded objects'). Verified this pass that this is a genuine, permanent absence, not a partial gap: neither GetObjectOutput, DescribeObjectOutput, nor Item (the ListItems entry) has an UploadAvailability field at all in the real SDK, so a COMPLETED object's read response is byte-identical regardless of how it was uploaded in real AWS too -- there is nothing for gopherstack to expose on a normal read either way. The only real difference is entirely inside the upload window (an in-flight PUT's partial bytes become visible to a concurrent GET), which requires a chunked/progressive PutObject this backend's atomic single-request model cannot produce. UploadAvailability is stored on Object (and persisted) purely as an internal record, never serialized onto any wire response -- not modeled elsewhere, would require a bigger feature (partial/chunked PutObject) to emulate faithfully."
  - "ContainerNotFoundException (a real modeled error, present in every op's model) is never returned by this handler -- mediastoredata has no notion of containers in gopherstack's per-region flat object store (containers are provisioned by the separate mediastore service, not mediastoredata). Confirmed structural, not just deferred-for-convenience: the real wire protocol has NO ContainerName-shaped field on any of the 5 operations (confirmed: `grep ContainerName` across the whole pinned SDK package returns nothing) -- real MediaStore Data identifies the target container purely by which per-container hostname the client's BaseEndpoint points at (endpoints.go:204 resolveBaseEndpoint; a container's Endpoint, e.g. https://<container>.data.mediastore.<region>.amazonaws.com, is obtained out-of-band from mediastore:DescribeContainer). gopherstack is single-origin and routes purely on SDK User-Agent marker (handler.go RouteMatcher), with no Host-based dispatch anywhere in the codebase (confirmed: no r.Host/Header.Get(\"Host\") use in pkgs/service or this package). Reaching ContainerNotFoundException would need BOTH: (1) route-level wiring to recover a container identifier from the request (most plausibly the Host header, since gopherstack's mediastore already synthesizes AWS-shaped per-container endpoint hostnames in containerEndpoint() -- services/mediastore/store.go:27), which touches RouteMatcher/cli.go and is out of scope here, AND (2) a cross-service read of services/mediastore's container registry from this handler. Deferred: cross-service + routing change, out of scope for a mediastoredata-only pass."
  - "InMemoryBackend.UpdateObjectMetadata (objects.go) is NOT a wire-routed AWS operation and has no aws-sdk-go-v2/service/mediastoredata counterpart (confirmed: only PutObject/GetObject/DeleteObject/DescribeObject/ListItems exist in the real SDK's api_op_*.go set) -- gopherstack-vxmb's premise that it was orphaned dead code was WRONG: it is the sole implementation behind dashboard/ui.go's registerMediaStoreDataUpdateMetadataRoute, which IS registered (called from setupSubRouter) and backs the live PATCH /dashboard/api/mediastoredata/objects dashboard-only endpoint. A service-directory-scoped grep for callers missed this cross-package (dashboard/) consumer and led to briefly (and incorrectly) deleting it 2026-08-07 before the dashboard build failure caught the mistake; restored with a doc comment recording that it is intentionally dashboard-internal, not a modeled AWS operation."
deferred:
  - cross-service container-existence validation against services/mediastore (see gaps)
leaks: {status: clean, note: "no goroutines/janitors; region map lazily allocated under Lock, read via non-allocating stateRO under RLock -- no torn-state risk found. Every b.mu.Lock/RLock in objects.go, items.go, store.go, persistence.go is immediately followed by a defer Unlock/RUnlock; no early-return bypasses found this pass."}
---

## Notes

- Protocol: restjson1. PutObject/GetObject/DeleteObject/DescribeObject use `/{Path+}`
  (greedy URI capture of the object path, no JSON request/response body except PutObject's
  raw byte stream and PutObject's small JSON output). ListItems is the only op with a
  JSON request-shape-like surface, and even it is GET `/` with everything as query params
  (Path, MaxResults, NextToken) -- it is NEVER a GET on a folder-shaped path. This
  confirms the codebase's `ExtractOperation`/`Handler()` disambiguation of ListItems vs
  GetObject via `r.URL.Path == "/"` is correct, not a bug (a plausible-looking bug that
  turned out to be right after checking the real serializer).

- **StorageClass has exactly one real value: `TEMPORAL`.** `STANDARD` is easily confused
  with it because it IS a valid value -- but of the unrelated `x-amz-upload-availability`
  header (`UploadAvailability` enum: `STANDARD` | `STREAMING`), not `x-amz-storage-class`
  (`StorageClass` enum: `TEMPORAL` only). Fixed in an earlier pass; still correct.

- `GetObjectOutput`/`DescribeObjectOutput` `LastModified` is an HTTP-date header
  (`smithytime.ParseHTTPDate`, RFC1123-ish `http.TimeFormat`), NOT epoch seconds --
  correctly implemented via `obj.LastModified.UTC().Format(http.TimeFormat)`. Contrast
  with `ListItems`' `Item.LastModified`, which IS epoch-seconds as a JSON number
  (`smithytime.ParseEpochSeconds` in the JSON-body deserializer) -- also correctly
  implemented (`float64(item.LastModified.Unix())`). Don't conflate the two: same field
  name, different wire representation depending on whether it's a header or a JSON body
  field.

- `PutObjectOutput.ETag` is JSON-body-only in the real SDK (no HTTP header binding
  exists for it in deserializers.go) -- gopherstack also sets an `ETag` response header
  on PutObject, which is extra/unused by the SDK but harmless.

- The 4 real error types (`ContainerNotFoundException`, `InternalServerError`,
  `ObjectNotFoundException`, `RequestedRangeNotSatisfiableException`) are all correctly
  wired for the paths that can actually produce them today (ObjectNotFoundException on
  missing object for Get/Describe/Delete, RequestedRangeNotSatisfiableException on bad
  Range -- fixed this pass, see ops.GetObject -- InternalServerError as the catch-all
  default). `ContainerNotFoundException` is unreachable because this service has no
  container-existence concept (see gaps).

- **This pass's fixes (2026-07-24):** the previous audit (2026-07-13) identified but left
  in place 3 fabricated exception names (`InvalidPathException`, `InvalidStorageClassException`,
  `InvalidContentSHA256Exception`) as a known gap, reasoning "no known correct replacement
  code exists in the model." This pass replaced them with real AWS error names instead of
  leaving them fabricated: `ValidationException` (the established AWS-wide/gopherstack-wide
  convention for parameter validation, already used elsewhere in this same handler for the
  ListItems MaxResults bound check -- so this also fixes a same-file internal
  inconsistency) and `XAmzContentSHA256Mismatch` (confirmed via AWS SDK issue trackers as
  the real error S3-family services return for a declared-vs-actual body hash mismatch;
  this is a generic SigV4-payload-integrity check, not mediastoredata app logic, so it's
  reasonable it isn't in mediastoredata's own narrow per-op model, and it is NOT redundant
  with `pkgs/httputils.SigV4Validator`, which only checks that the request signature is
  self-consistent with whatever hash the client declared -- it never independently
  recomputes the hash of the actually-received bytes). Separately, and NOT previously
  flagged: `GetObject`'s 416 response had the fabricated `InvalidRangeException` for its
  `__type` even though the HTTP status code (416) was already correct -- fixed to the real
  modeled `RequestedRangeNotSatisfiableException`, since a real client's typed error
  assertion depends on that exact string, not just the status code. All 3 error-related
  edits are locked in by new test assertions on the wire `__type` field (previously the
  tests only checked HTTP status, not the error body) in handler_test.go's
  TestMediaStoreData_PathValidation, TestMediaStoreData_StorageClassValidation,
  TestMediaStoreData_ContentSHA256Verification, and TestMediaStoreData_RangeRequests.

- **This pass's fix (2026-08-20):** wrapper-key/nested-shape/HTTP-binding sweep of all 5
  ops. Confirmed clean, against the pinned v1.32.4 deserializers.go/serializers.go, with no
  changes needed: every GetObject/DescribeObject header binding name and casing
  (CacheControl->Cache-Control, ContentLength->Content-Length, ContentRange->Content-Range,
  ContentType->Content-Type, ETag->ETag, LastModified->Last-Modified via
  smithytime.ParseHTTPDate NOT epoch), GetObject's StatusCode (200 vs 206, sourced straight
  from response.StatusCode, not hardcoded), PutObject's request-header bindings
  (CacheControl->Cache-Control, ContentType->Content-Type, StorageClass->X-Amz-Storage-Class,
  UploadAvailability->X-Amz-Upload-Availability) and JSON-body-only response
  (ContentSHA256/ETag/StorageClass -- confirmed via deserializeOpDocumentPutObjectOutput
  that GetObject's own body deserializer, deserializeOpDocumentGetObjectOutput, is the
  polly-shape single-line `v.Body = body` with no JSON decode at all), ListItems' GET "/"
  with Path/MaxResults/NextToken as query-only params (never a path segment) and its
  Item{ContentLength,ContentType,ETag,LastModified,Name,Type} field set (LastModified
  correctly epoch-seconds here, unlike the header form), the greedy `/{Path+}` routing
  (gopherstack's router has no Echo route pattern at all -- pkgs/service.Router matches on
  User-Agent and dispatches on the raw, already zero-loss r.URL.Path, so multi-segment
  paths were never at risk), and both ItemType/StorageClass/UploadAvailability enums
  checked both ways -- all found correct, all now covered by a real aws-sdk-go-v2
  round-trip test (`wire_sdk_roundtrip_test.go`, new this pass:
  TestPutGetObject_MultiSegmentPath_SDKRoundTrip, TestGetObject_Range_SDKRoundTrip,
  TestDescribeObject_SDKRoundTrip, TestListItems_SDKRoundTrip) that drives the real SDK
  client through the real `pkgs/service` router, not just this package's own JSON tags.
  ONE real bug found and fixed: no op's error response ever set the `X-Amzn-ErrorType`
  header, which `deserializeOpError<Op>` checks BEFORE attempting to decode a body --
  harmless for PUT/GET/DELETE (whose error bodies a real client can read) but a 100%
  reproducing break for DescribeObject, whose HEAD response body net/http's own transport
  always discards client-side regardless of what the server sent, so no DescribeObject
  error could ever deserialize into its real typed exception. See the DescribeObject `ops`
  entry above for the full repro/fix/test detail. Structurally unverifiable, as always: the
  actual bytes an object stores (this backend only proves shape and count, e.g. body
  round-trips through a fixed test payload -- it cannot exhaustively prove every possible
  byte sequence survives, though SHA-256-based ETag/ContentSHA256 gives strong indirect
  coverage).

- **This pass's fix (2026-09-04, gopherstack-5ce):** field-level doc-comment re-read of all 5
  *Input structs at the pinned v1.32.4. ONE real bug found and fixed: PutObjectInput's doc
  comment (`api_op_PutObject.go:13-14`) states "Object sizes are limited to 25 MB for
  standard upload availability and 10 MB for streaming upload availability," but gopherstack
  enforced no size limit at all -- an emulator-more-permissive-than-real-AWS gap in the same
  class as the earlier UploadAvailability accept-anything bug. Fixed via a new
  `ErrObjectTooLarge` sentinel (`errors.go`, wire `__type` `ValidationException` -- not
  officially enumerated in PutObject's own {ContainerNotFoundException, InternalServerError}
  modeled-error pair per `deserializeOpErrorPutObject`, same reasoning as the existing
  ValidationException uses) and a size check in `InMemoryBackend.PutObject` keyed on the
  already-defaulted `uploadAvailability` value (so an omitted header, which defaults to
  STANDARD, gets the 25 MB limit, not 10 MB). Regression tests:
  `TestInMemoryBackend_PutObject_SizeLimit` (backend-level, at/over both limits, plus a case
  proving a body under the 25 MB standard cap but over the 10 MB streaming cap is still
  rejected when STREAMING is requested) and `TestMediaStoreData_ObjectSizeLimit`
  (wire-level, checks the `ValidationException` `__type`). Both confirmed to fail without the
  fix via hand-revert. Everything else re-checked this pass (all 5 *Input doc comments,
  `types/errors.go`'s 4-exception list, `types/enums.go`, `persistence.go`/`store_setup.go`)
  found correct, no other changes needed.

## 2026-09-08: writeErrorJSON nil-on-write fall-through sweep (gopherstack-246v) -- clean

Part of the 12-service sweep for the elasticache class bug (gopherstack-8haq): a helper
that rejects a request via the local response writer and *returns* that writer's result
hands a caller doing `if err != nil { return err }` a `nil`, since the writer returns nil
after a successful write -- the rejection is silently skipped and the operation continues.

**Base writer**: `writeErrorJSON` (`handler.go:583`) returns `c.JSON(status,
errorResponse(...))` directly -- nil on a successful write. `(h *Handler) writeError`
(`handler.go:553`) wraps it, `return writeErrorJSON(...)` at every branch.

**Method (mechanical).** A `go/parser`/`go/ast` script over every non-test `.go` file (9
files) found every function with a `return`-statement whose result is a bare call to
`writeErrorJSON`, then fixed-point-expanded to any function bare-returning a call to an
already-found member -- 8 functions discovered: the `writeError` method and the 6
verb-named op handlers (`handlePutObject`, `handleGetObject`, `handleRangeGet`,
`handleDeleteObject`, `handleListItems`, `handleDescribeObject`), plus `Handler` itself.

**Dispatch verified, not assumed.** `Handler()` is a direct `switch r.Method` with one
`return h.handleXxx(c)` per case (`handler.go:130-165`) -- read in full, no intermediate
variable or `if err != nil` anywhere in the switch. All 6 op handlers are dispatch targets,
safe by construction; `writeError` (the method) is the only non-dispatch helper discovered.

Every call site of `writeErrorJSON` and the `writeError` method across the package (18
total) was enumerated and classified: **all 18 are direct `return writeErrorJSON(...)` /
`return writeError(...)` / `return h.handleXxx(c)` returns. Zero stored-then-checked sites,
zero `_ =` discards.** Independently confirmed by grepping every non-test-file occurrence
of `writeErrorJSON(`/`writeError(` outside their own definitions: every one is immediately
preceded by `return` on the same line.

**No instance of the broken shape exists in mediastoredata.** No code changed. Gates re-run
for the record: `GOTOOLCHAIN=go1.27.0 golangci-lint run ./services/mediastoredata/...` 0
issues; `GOTOOLCHAIN=go1.27.0 go test -race ./services/mediastoredata/...` ok.
