---
service: rolesanywhere
sdk_module: aws-sdk-go-v2/service/rolesanywhere@v1.26.3
last_audit_commit: e75a8cecd
last_audit_date: 2026-08-20
overall: A            # wrapper-key/nested-shape sweep this pass: zero bugs found, clean
ops:
  CreateTrustAnchor: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: no longer rejects duplicate names with an invented ConflictException (the real service has ZERO ConflictException shape anywhere in its model -- confirmed via botocore's rolesanywhere service-2.json, which lists only AccessDeniedException/ResourceNotFoundException/TooManyTagsException/ValidationException across all 27 operations); also fixed: now applies the request's notificationSettings at creation (previously silently dropped); also fixed: now validates source.sourceType is non-empty (required per CreateTrustAnchorInput); tags no longer stored on the TrustAnchor struct -- routed into the same ARN-keyed tags store TagResource/ListTagsForResource use (see families.tags_field_removed)"}
  GetTrustAnchor: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: response no longer includes an invented \"tags\" key -- real TrustAnchorDetail has no tags field at all (field-by-field diff against aws-sdk-go-v2 types.TrustAnchorDetail); tags are ListTagsForResource-only"}
  ListTrustAnchors: {wire: ok, errors: ok, state: ok, persist: ok, note: "same tags-field fix as GetTrustAnchor"}
  DeleteTrustAnchor: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now cascade-deletes the trust anchor's notification settings and tags (both live in separate ID/ARN-keyed maps, not on the TrustAnchor struct) -- previously left ghost rows keyed by a dead trust anchor ID/ARN"}
  UpdateTrustAnchor: {wire: ok, errors: ok, state: ok, persist: ok, note: "same tags-field fix as GetTrustAnchor"}
  EnableTrustAnchor: {wire: ok, errors: ok, state: ok, persist: ok, note: "same tags-field fix as GetTrustAnchor"}
  DisableTrustAnchor: {wire: ok, errors: ok, state: ok, persist: ok, note: "same tags-field fix as GetTrustAnchor"}
  CreateProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: no longer rejects duplicate names (see CreateTrustAnchor note -- no ConflictException in the real model); fixed: the request's enabled field was completely ignored (profile was always created enabled:true regardless of caller input) -- same ignored-input bug class as the CreateTrustAnchor.enabled bug fixed in the prior pass; now honors it, defaulting to true when omitted; added: acceptRoleSessionName (real CreateProfileInput field, was entirely unmodeled) and createdBy (real ProfileDetail field, populated with the backend's account ID) now round-trip; tags no longer stored on the Profile struct -- routed into the ARN-keyed tags store; fixed this pass: roleArns is now rejected when nil (CreateProfileInput.RoleArns is \"This member is required\" -- aws-sdk-go-v2@v1.26.3's validateOpCreateProfileInput checks v.RoleArns == nil, and botocore's CreateProfileRequest declares roleArns in its top-level required list); an explicitly empty slice is still accepted since the RoleArnList shape itself declares min:0, so the requirement is presence, not non-emptiness -- a prior gaps note had flagged this as deliberately left permissive; corrected 12 test call sites across profiles_test.go/store_test.go/crl_subject_test.go that had relied on nil roleArns silently succeeding"}
  GetProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: response no longer includes an invented \"tags\" key -- real ProfileDetail has no tags field at all (field-by-field diff); added acceptRoleSessionName/createdBy to the response"}
  ListProfiles: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fixes as GetProfile"}
  DeleteProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now cascade-deletes the profile's attribute mappings and tags (both live in separate ID/ARN-keyed maps, not on the Profile struct) -- previously left ghost rows keyed by a dead profile ID/ARN"}
  UpdateProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fixes as GetProfile; UpdateProfileInput.acceptRoleSessionName now applied"}
  EnableProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "same tags-field fix as GetProfile"}
  DisableProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "same tags-field fix as GetProfile"}
  ImportCrl: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: no longer rejects duplicate names (no ConflictException in the real model); fixed: now validates crlData and trustAnchorArn are non-empty (both required per ImportCrlInput, confirmed against validateOpImportCrlInput -- previously only name was checked, so a request missing crlData/trustAnchorArn silently created a malformed CRL)"}
  GetCrl: {wire: ok, errors: ok, state: ok, persist: ok}
  ListCrls: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCrl: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteCrl: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now cascade-deletes the CRL's tags (a separate ARN-keyed map, not on the Crl struct) -- previously left a ghost row keyed by a dead CRL ARN"}
  EnableCrl: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableCrl: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSubject: {wire: ok, errors: ok, state: partial, persist: ok, note: "store is never populated (see gaps: no CreateSession); confirmed this pass: an unknown subjectId correctly returns ResourceNotFoundException (matches botocore's GetSubject errors list), not a disguised empty success -- the empty-list gap is honest, not a broken operation"}
  ListSubjects: {wire: ok, errors: ok, state: partial, persist: ok, note: "store is never populated (see gaps: no CreateSession); confirmed this pass: pagination (maxResults) is validated via the shared parsePageParams path used by every List op in this service (non-numeric/negative maxResults -> ValidationException, per botocore's ListSubjects errors list), so the empty-list gap is honest, not a broken operation"}
  PutAttributeMapping: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: certificateField was accepted as any string (the real CertificateField shape is a 3-value enum: x509Subject/x509Issuer/x509SAN, confirmed via botocore); mappingRules was accepted as nil (required per validateOpPutAttributeMappingInput); each rule's specifier was accepted empty (required per validateMappingRule) -- all three now return ValidationException"}
  DeleteAttributeMapping: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: certificateField was accepted as any string -- same CertificateField enum validation added as PutAttributeMapping, confirmed via validateOpDeleteAttributeMappingInput's required check and botocore's shared CertificateField shape"}
  PutNotificationSettings: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: each resulting NotificationSettingDetail.configuredBy is now populated with the backend's account ID (real field, was entirely unmodeled -- gopherstack seeds no AWS-default settings, so every setting is customer-configured and configuredBy is always the account ID per AWS's documented semantics)"}
  ResetNotificationSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: HTTP response status was 200; real AWS's TagResource responds 201 Created per the service model's http.responseCode (confirmed against botocore's service-2.json -- every other void-result op in this service is genuinely 200, TagResource is the sole 201 exception); fixed: now returns ResourceNotFoundException when resourceArn matches no trust anchor/profile/CRL (previously happily wrote tags for any ARN, real or not); added: TooManyTagsException when the resulting tag count on a resource would exceed 200 (the real SDK's shared TagList shape's max:200 constraint, applied here as the per-resource total-tag limit since TagResource is the only op in the service model that declares TooManyTagsException); fixed this pass: the TooManyTagsException check merged the incoming batch into the tag store's own backing slice (via index assignment on an update-in-place path) before checking the resulting count, so a rejected over-limit batch could still leave an already-existing tag's value overwritten (state mutated before validation) -- now clones the stored slice before merging so a rejected batch leaves the store byte-for-byte unchanged"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed UntagResource declares NO ResourceNotFoundException in the real model (unlike TagResource/ListTagsForResource) -- left as a silent no-op against an unknown ARN, matching AWS"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now returns ResourceNotFoundException when resourceArn matches no trust anchor/profile/CRL (previously returned 200 with an empty list for ANY arn, including ones with no backing resource at all -- a disguised stub since the real op validates the resource exists)"}
families:
  trustAnchor_crud: {status: ok, note: "route matcher + PATCH/POST/GET/DELETE methods re-verified against botocore's service-2.json http.method/requestUri/responseCode for every op this pass; all match, including the 201 vs 200 distinction on TagResource"}
  profile_crud: {status: ok, note: "same verification as trustAnchor_crud"}
  crl_crud: {status: ok, note: "same verification as trustAnchor_crud"}
  tags: {status: ok, note: "this pass: removed the invented \"tags\" field from TrustAnchorDetail/ProfileDetail JSON (real AWS never returns one on either shape -- confirmed field-by-field against types.TrustAnchorDetail/types.ProfileDetail, neither has a Tags member), which also fixes the desync bug where creation-time tags (stored on the resource struct) permanently diverged from TagResource/UntagResource-mutated tags (stored in a separate ARN-keyed map) -- both now route through the same store; added ResourceNotFoundException validation to TagResource/ListTagsForResource (not UntagResource, which the real model doesn't declare it for) and TooManyTagsException to TagResource"}
  duplicate_name_rejection: {status: ok, note: "REMOVED this pass: CreateTrustAnchor/CreateProfile/ImportCrl each independently rejected duplicate names with a gopherstack-invented ConflictException/409. Cross-checked against botocore's rolesanywhere/2018-05-10/service-2.json: the service's shapes map contains exactly 4 exception shapes total (AccessDeniedException, ResourceNotFoundException, TooManyTagsException, ValidationException) across ALL 27 operations -- there is no ConflictException shape in the entire service model, so this was invented behavior with a fabricated error code, not a real AWS constraint. Real Roles Anywhere trust anchors/profiles/CRLs are identified by generated ID/ARN; names are not unique. Deleted ErrTrustAnchorAlreadyExists/ErrProfileAlreadyExists/ErrCrlAlreadyExists and their duplicate-check code paths; all three Create/Import ops now accept duplicate names, matching the real API."}
gaps:
  - "GetSubject/ListSubjects: subjects store is never populated -- there is no CreateSession endpoint in this service (AWS Roles Anywhere's session-vending API is a separate mTLS-authenticated data-plane API, not SigV4/control-plane, and remains out of scope). SubjectDetail's Credentials/InstanceProperties fields are also unmodeled. Would need its own audit pass if CreateSession is ever added to gopherstack."
  - "No AccessDeniedException path anywhere in this service -- gopherstack has no IAM policy evaluation engine to source it from; this is a cross-cutting infra gap common to every gopherstack service, not specific to rolesanywhere."
  - "FIXED this pass: CreateProfile now rejects a nil roleArns list with ValidationException, matching CreateProfileInput.RoleArns's \"This member is required\" marker (aws-sdk-go-v2@v1.26.3's validateOpCreateProfileInput checks v.RoleArns == nil) and botocore's CreateProfileRequest.required list. A prior pass's note framed this as deliberately left permissive 'to control blast radius' against existing tests -- that framing was backwards: the tests asserting nil-roleArns success were the bug, not a constraint to protect. An explicitly empty (non-nil) roleArns slice is still accepted, since the RoleArnList shape declares min:0 (requirement is presence, not non-emptiness). ImportCrlInput.CrlData/TrustAnchorArn and CreateTrustAnchorInput.Source were already validated by a prior pass."
  - "TrustAnchorDetail has no createdBy field in the real API (confirmed absent from types.TrustAnchorDetail) -- correctly NOT added to TrustAnchor's JSON output this pass (a prior gaps note incorrectly implied it should be); ProfileDetail DOES have createdBy and it is now implemented."
  - "gopherstack-i5ss (2026-09-06): ImportCrl does not validate TrustAnchorArn refers to an existing trust anchor, and DeleteTrustAnchor does not cascade to CRLs referencing it. Both left unimplemented -- see the dated section below for the sourced reasoning. Would be revisited if AWS ever adds ResourceNotFoundException to ImportCrl's modelled errors, or a doc revision states either behavior explicitly."
leaks: {status: clean, note: "no goroutines/janitors in this service; locking is via the shared lockmetrics.RWMutex per pkgs-catalog rule, single lock, no re-entrant locking (CreateTrustAnchor's notificationSettings-at-create path calls the new putNotificationSettingsLocked helper directly instead of re-entering PutNotificationSettings's own Lock). This pass's real find: DeleteTrustAnchor/DeleteProfile/DeleteCrl left ghost rows in the notificationSettings/attributeMappings/tags maps (keyed by the now-dead resource ID/ARN) -- all three Delete paths now cascade-delete their dependent maps under the same lock as the primary delete, closing the leak."}
---

## Notes

Protocol: restJson1. Re-verified route/method/path/**response status code** for every
operation directly against `botocore`'s `rolesanywhere/2018-05-10/service-2.json` (canonical
source for `http.method`/`http.requestUri`/`http.responseCode`, cross-checked against
`aws-sdk-go-v2/service/rolesanywhere@v1.23.0`'s `serializers.go`/`deserializers.go` for wire
field names) -- all of gopherstack's `parseRESTPath`/`parseEntityPath`/`parseTagPaths` switch
cases matched exactly, with one status-code exception fixed this pass: `TagResource` responds
`201 Created`, not `200 OK` (every other void-result op in this service genuinely is 200).

**Real bugs fixed this pass** (in addition to the ones summarized in the `ops`/`families`
frontmatter above):

1. **Invented `ConflictException` on duplicate names (`trust_anchors.go`, `profiles.go`,
   `crls.go`, `errors.go`, `handler.go`).** `CreateTrustAnchor`/`CreateProfile`/`ImportCrl`
   each scanned the region's existing resources for a name collision and returned a
   fabricated `ConflictException`/409. Cross-checked against botocore's service-2.json shapes
   map: the entire Roles Anywhere service model defines exactly 4 exception shapes
   (`AccessDeniedException`, `ResourceNotFoundException`, `TooManyTagsException`,
   `ValidationException`) -- **no `ConflictException` shape exists anywhere in the service**,
   confirming this was invented behavior with a fabricated error code, not a real AWS
   constraint. Deleted `ErrTrustAnchorAlreadyExists`/`ErrProfileAlreadyExists`/
   `ErrCrlAlreadyExists` and the duplicate-check loops entirely; all three ops now accept
   duplicate names (distinguished by generated ID/ARN), matching real AWS.

2. **Invented `"tags"` field on `TrustAnchorDetail`/`ProfileDetail` responses
   (`models.go`, `handler_trust_anchors.go`, `handler_profiles.go`).** Field-by-field diff
   against `aws-sdk-go-v2/service/rolesanywhere/types.TrustAnchorDetail` and
   `types.ProfileDetail` shows neither has a `tags` member at all -- tags are visible **only**
   via `ListTagsForResource`. A prior version stored creation-time tags directly on the
   `TrustAnchor`/`Profile` struct and serialized them into every Create/Get/List/Update/
   Enable/Disable response. This was doubly wrong: it invented a wire field no real client
   would ever see, AND it permanently desynced from the real tag state, since
   `TagResource`/`UntagResource` write to a wholly separate ARN-keyed map
   (`InMemoryBackend.tags`) that was never merged with the struct field -- a persistence_test.go
   comment even documented this desync as expected behavior. Removed `Tags` from both structs;
   `CreateTrustAnchor`/`CreateProfile` now route creation-time tags into the same ARN-keyed
   store `TagResource` uses (the pattern `ImportCrl` already followed correctly for CRLs,
   which have no `tags` field on `CrlDetail` either and were unaffected by this bug).

3. **`TagResource` HTTP status was 200, should be 201 (`handler_tags.go`).** Confirmed
   against botocore's `service-2.json`: `TagResource`'s `http.responseCode` is `201`, the only
   201 among this service's void-result ops (every other one -- `UntagResource`,
   `Enable*`/`Disable*`, etc. -- is genuinely 200).

4. **`TagResource`/`ListTagsForResource` never validated the resource exists
   (`tags.go`).** Both declare `ResourceNotFoundException` in the real model (confirmed via
   botocore's per-operation `errors` list), but the backend happily tagged/listed tags for any
   ARN string, including ones with no backing trust anchor/profile/CRL at all -- a disguised
   stub (accepting input without checking it refers to anything real). Added
   `resourceExistsLocked`, scanning the three region-indexed resource tables by ARN.
   `UntagResource` deliberately does **not** get this check: it declares no
   `ResourceNotFoundException` in the real model and is correctly left as a silent no-op
   against an unknown ARN.

5. **`TooManyTagsException` was entirely unimplemented (`tags.go`, `errors.go`,
   `handler.go`).** `TagResource` is the only operation in the service model that declares
   `TooManyTagsException`; added a 200-tag-per-resource cap (matching the real SDK's shared
   `TagList` shape's `max: 200` constraint) enforced atomically -- an over-limit batch is
   rejected without partially applying any of it.

6. **`CreateProfile` silently ignored the request's `enabled` field
   (`profiles.go`, `handler_profiles.go`).** `CreateProfileInput.Enabled` is a real, optional
   field (confirmed in the SDK's request struct); the backend hardcoded
   `Enabled: true` regardless of what the caller sent -- the same ignored-input bug class as
   the `CreateTrustAnchor.Enabled` bug a prior pass fixed. Now honors it, defaulting to true
   when nil.

7. **`CreateTrustAnchor` still dropped the request's `notificationSettings` field
   (`trust_anchors.go`).** Flagged but deliberately left unfixed in the prior pass's gaps
   list; fixed this pass by extracting `PutNotificationSettings`'s locked application logic
   into `putNotificationSettingsLocked` and calling it directly from `CreateTrustAnchor` under
   the same lock (avoiding re-entrant locking).

8. **`acceptRoleSessionName` was entirely unmodeled (`models.go`, `profiles.go`,
   `handler_profiles.go`).** Real `CreateProfileInput`/`UpdateProfileInput`/`ProfileDetail`
   all carry this field; added to the `Profile` struct and threaded through Create/Update/JSON
   output (following the same "omit when false" convention `requireInstanceProperties`
   already used).

9. **`ProfileDetail.createdBy` was unpopulated (`models.go`, `profiles.go`,
   `handler_profiles.go`).** Real field ("The Amazon Web Services account that created the
   profile"); now populated with the backend's account ID at creation. Note:
   `TrustAnchorDetail` has **no** `createdBy` field in the real API (confirmed absent from
   `types.TrustAnchorDetail`) -- the prior pass's gaps note conflated the two shapes; only
   `ProfileDetail` needed this fix.

10. **`NotificationSettingDetail.configuredBy` was unpopulated
    (`models.go`, `notification_settings.go`).** Real field naming "the principal that
    configured the notification setting"; gopherstack seeds no AWS-default settings, so every
    setting reaching a response is customer-configured and `configuredBy` is always the
    backend's account ID per AWS's documented semantics. Populated in
    `putNotificationSettingsLocked`.

11. **`ImportCrl` didn't validate `crlData`/`trustAnchorArn` as required (`crls.go`).**
    Both are required members of `ImportCrlInput` (confirmed against
    `validateOpImportCrlInput` in the SDK); only `name` was checked, so a request missing
    either silently created a malformed CRL. Added both checks.

12. **Cascade-delete leaks: `DeleteTrustAnchor`/`DeleteProfile`/`DeleteCrl` left ghost
    rows (`trust_anchors.go`, `profiles.go`, `crls.go`).** Notification settings,
    attribute mappings, and tags all live in separate ID/ARN-keyed maps, not on the
    resource structs themselves (`InMemoryBackend.notificationSettings`/
    `.attributeMappings`/`.tags`). None of the three Delete paths cleaned these up, so a
    deleted trust anchor/profile/CRL's dependent rows survived indefinitely under its old
    ID/ARN. All three Delete paths now `delete()` their dependent map entries under the same
    lock as the primary delete.

13. **`ListTagsForResource` never percent-decoded the `resourceArn` query
    parameter (`handler_tags.go`).** `ListTagsForResourceInput.ResourceArn` is an
    `httpQuery`-bound member (confirmed via `awsRestjson1_serializeOpHttpBindingsListTagsForResourceInput`
    in `aws-sdk-go-v2/service/rolesanywhere/serializers.go`, which calls
    `encoder.SetQuery("resourceArn").String(...)`), so every real SDK client sends it
    percent-encoded (`:` -> `%3A`, `/` -> `%2F`) in `URL.RawQuery`. The handler parsed the
    raw query string with a manual `strings.SplitSeq`/`CutPrefix` scan and used the
    still-percent-encoded substring directly as the lookup key, which never matched any
    stored (plain, unencoded) ARN -- every `ListTagsForResource` call 404'd with
    `ResourceNotFoundException` regardless of whether the resource existed, caught by
    `TestTerraform_RolesAnywhere` (the AWS Terraform provider's post-create tags refresh
    calls `ListTagsForResource` immediately after `CreateTrustAnchor`). Fixed: now goes
    through `net/url.ParseQuery`, which percent-decodes. `TagResource`/`UntagResource` take
    `resourceArn` from the JSON body, not the query string, so they were unaffected; the
    other manual query scans in this service (`nextToken`/`maxResults` in `handler.go`,
    `certificateField`/`specifiers` in `handler_attribute_mappings.go`) carry the same
    decode gap but were out of scope for this fix -- none of their real values are ever
    percent-encoded by a real SDK client (opaque pagination tokens/counts/enum names, no
    `:` or `/`), so the gap is latent, not test-visible.

**Traps for the next auditor (looks-wrong-but-correct):**

- Timestamps use `time.RFC3339` (via `Format(time.RFC3339)`), not the SDK's exact
  millisecond output format (`2006-01-02T15:04:05.999Z`, per `smithy-go/time.
  FormatDateTime`). This is NOT a bug: restJson1's default timestamp trait is ISO-8601
  (confirmed in `deserializers.go`: `awsRestjson1_deserializeDocumentTrustAnchorDetail` parses
  `createdAt` via `smithytime.ParseDateTime` on a JSON *string*, never an epoch-seconds
  number) -- this service is NOT in the epoch-seconds bug class (unlike sagemaker/glue/ssm/
  iot/cloudtrail). `smithytime.ParseDateTime` (the client-side parser) also accepts
  `time.RFC3339`/`time.RFC3339Nano` as fallbacks, so gopherstack's output round-trips fine.
- `errBody` returns `{"__type": ..., "message": ...}` with no `X-Amzn-ErrorType` header.
  This is also NOT a bug: the SDK's error deserializer (`awsRestjson1_deserializeOpError*`)
  falls back to the JSON body's type field via `restjson.GetErrorInfo` when the header is
  absent.
- `go.Blob` fields (`crlData`) are plain `[]byte` in the Go struct/JSON tag; Go's
  `encoding/json` already base64-encodes `[]byte` on marshal, matching the SDK's
  base64-string wire representation with zero extra code.
- `TrustAnchorType`'s real enum has a third value, `SELF_SIGNED_REPOSITORY`, beyond
  `AWS_ACM_PCA`/`CERTIFICATE_BUNDLE` (confirmed in `types/enums.go`) -- it has no
  corresponding `SourceData` union member (`SourceData` only has `AcmPcaArn`/
  `X509CertificateData`), so it needs no external data. gopherstack's `TrustAnchorSource`
  already accepts any `sourceType` string generically (no enum-value allowlist), so this
  requires no code change -- just noting it so nobody "fixes" the SourceType validation to
  reject it.
- `AcceptRoleSessionName`/`RequireInstanceProperties` are both bool (not `*bool`) on the
  `Profile` struct and use "omit from JSON when false" instead of true three-state
  (unset/true/false) semantics. This matches `requireInstanceProperties`'s pre-existing
  convention in this codebase and is a reasonable simplification (the real API's
  `*bool` distinguishes "not provided" from "explicitly false", but neither is meaningfully
  observable by a client here since gopherstack has no separate CreateSession data plane
  where the distinction would matter).

## 2026-08-20 wrapper-key / nested-shape sweep (zero bugs found)

Dedicated pass hunting the bug class from the 27-service sweep session (wrong wrapper
key, wrong nesting level, wrong JSON type, right-key-wrong-value/invented enum) across
all 30 ops. Result: **clean, zero new bugs.**

**Protocol/flat-vs-wrapped, confirmed per op.** rolesanywhere is restJson1
(`awsRestjson1_*` prefix in `deserializers.go`, `aws-sdk-go-v2/service/rolesanywhere@v1.26.3`).
For every one of the 28 ops with a body (`TagResource`/`UntagResource` have no output
members, so no `deserializeOpDocument*Output` is generated for them at all -- correctly
void), grepped each op's own `HandleDeserialize` and confirmed
`awsRestjson1_deserializeOpDocument<Op>Output(&output, shape)` is genuinely **called**
on the JSON-decoded body, not dead code shadowed by an `httpPayload`-bound raw-body
assignment (the appmesh/glacier trap this session's brief warned about). rolesanywhere
has no `httpPayload`-bound output member on any op, so every op is wrapped, never flat.
Verified the wrapper key match for all 28 against gopherstack's `keyTrustAnchor`/
`keyProfile`/`keyCrl`/`keySubject`/`keyTrustAnchors`/`keyProfiles`/`keyCrls`/
`keySubjects`/`keyTags` constants (`handler.go`) and each handler's `map[string]any{key...}`
response construction -- all correct, matching the SDK's own `case "..."` in each
`awsRestjson1_deserializeOpDocument<Op>Output` switch.

**Per-shape field audit against the live deserializer functions** (not sibling
inference): `TrustAnchorDetail` (8/8 fields present, including the `source`/`sourceData`
union -- see below), `ProfileDetail` (14/14 fields present), `CrlDetail` (8/8 fields
present), `SubjectSummary` (7/7 fields present, used correctly for `ListSubjects`),
`NotificationSettingDetail` (5/5 fields present, including `configuredBy`),
`AttributeMapping`/`MappingRule` (complete). All enum VALUES gopherstack ever
constructs itself (none are self-generated -- every enum-typed field on this service's
wire is a client-echoed value, not a gopherstack-invented constant) checked against
`types/enums.go`: `TrustAnchorType` (3 values), `CertificateField` (3 values),
`NotificationChannel` (1 value, `ALL`), `NotificationEvent` (2 values) -- no fabricated
constants found.

**`sourceData` union** (`types.SourceData`, `deserializers.go`
`awsRestjson1_deserializeDocumentSourceData`): the two discriminator keys are
`acmPcaArn` and `x509CertificateData`, decoded via a single-iteration `for...break loop`
that resolves to whichever member key is present in the JSON object (real AWS unions
carry exactly one member). gopherstack's `TrustAnchorSource.SourceData` is a
`map[string]string` tagged `json:"sourceData,omitempty"` that round-trips the client's
own map verbatim (request-echo, not gopherstack-constructed) -- structurally
JSON-compatible with the union wire shape in both directions as long as the caller
supplies at most one key, which is the real API's own contract, not something
gopherstack needs to enforce for wire-shape correctness.

**`NotificationSetting` vs `NotificationSettingDetail` vs `NotificationSettingKey`**:
gopherstack collapses the SDK's two output-adjacent shapes (`NotificationSetting`,
the `PutNotificationSettingsInput` request member with no `configuredBy`, and
`NotificationSettingDetail`, the response member that adds it) into one Go struct
(`models.go` `NotificationSetting`, with `ConfiguredBy string json:"configuredBy,omitempty"`).
This is wire-safe in both directions: on request decode the extra tag is simply never
populated by a real client and is ignored; on response encode all 5 real
`NotificationSettingDetail` fields (`channel`, `configuredBy`, `enabled`, `event`,
`threshold`) are present. `NotificationSettingKey` (`event`+`channel`, used only by
`ResetNotificationSettings`'s request) is its own separate struct, correctly not
conflated with the other two. No bug.

**`SubjectDetail` vs `SubjectSummary`**: gopherstack uses one function
(`subjectToJSON`, `handler_subjects.go`) and one struct (`Subject`, `models.go`) for
both `GetSubject` (real shape: `SubjectDetail`, 9 fields incl. `credentials`/
`instanceProperties`) and `ListSubjects` (real shape: `SubjectSummary`, 7 fields,
neither of those two). The 7 fields gopherstack emits are exactly `SubjectSummary`'s
7 -- so `ListSubjects` is wire-complete. `GetSubject` is short two fields
(`credentials`, `instanceProperties`), which is the **already-recorded, out-of-scope**
gap (`gopherstack-fccd`: "SubjectDetail's Credentials/InstanceProperties fields are
also unmodeled") -- not a new finding, and structural: neither field can ever be
populated without the CreateSession mTLS data plane this emulator doesn't model, so
adding empty-slice stubs for them would be worse, not better.

**Existing tests**: grepped every `_test.go` assertion against a response wrapper key
(`resp["trustAnchor"]`, `["profile"]`, `["crl"]`, `["subject"]`, etc.) -- all assert the
real key. No wrong-key/wrong-nesting test found; none corrected this pass.

**`gopherstack-fccd` gap re-check**: of its three recorded gaps, two still hold exactly
as described (`GetSubject`/`ListSubjects` never populated -- no CreateSession data
plane; no `AccessDeniedException` path -- no IAM policy-eval engine). The third,
"CreateProfile.RoleArns nil/empty not rejected," **no longer holds** -- re-derived
against `validateOpCreateProfileInput` in `aws-sdk-go-v2/service/rolesanywhere@v1.26.3/
validators.go:779-780` (`RoleArns == nil` -> `ErrParamRequired`) and against current
`profiles.go:41` (`if name == "" || roleArns == nil { return nil, ErrValidation }`):
the check is present and correct, matching the frontmatter's `CreateProfile` note and
the 2026-08-10 `gaps` entry that already recorded it "FIXED this pass." The bd issue's
title still reads "not rejected" and was last updated 2026-08-13, after the fix landed
(commit `903d74b67`, 2026-08-10) -- the issue text is stale, not the code.

**Provenance / citation check**: `last_audit_commit` (prior value `cf439a0b1`, dated
2026-08-10 via `git show -s --format=%ad`) matched `last_audit_date` (2026-08-10) --
no backdating red flag. However, the manifest's own prose (former "Notes" section,
paragraph 1) cited `aws-sdk-go-v2/service/rolesanywhere@v1.23.0` for the
serializers.go/deserializers.go cross-check, while the `sdk_module` header says
`v1.26.3` (the version actually pinned in `go.mod`) -- a real header/prose mismatch of
the kind this sweep session was told to watch for. Diffed `deserializers.go`,
`types/types.go`, and `types/enums.go` between the two cached SDK versions
(`v1.23.0` and `v1.26.3`): byte-identical for all three files, so the mismatch is a
citation slip with no correctness impact -- this pass re-verified everything directly
against the correctly-pinned `v1.26.3` source regardless. No "FIXED" claim in the prior
manifest failed re-derivation.

**Gates**: `go build`, `go vet`, `go fix -diff` (empty), `gofmt -l` (empty),
`go test -race` (pass), `golangci-lint run` (0 issues) -- all clean, no code changed
this pass.

## Equality-matched-cursor restart sweep (2026-08-30)

All four paginated listings in this service (`ListTrustAnchors`, `ListProfiles`,
`ListCrls`, `ListSubjects`, all routed through `store.go`'s shared `listByRegionIndex`
+ `paginate[T]`) resumed a `pageToken` by scanning for the item whose ID equalled the
token and left `start` at 0 on no match -- deleting the resource a cursor named (or a
forged token) restarted pagination at page one instead of truncating.

Checked for the compounding non-total-sort trap this class is known to hit (quicksight's
tied-name bug) before choosing a fix: `listByRegionIndex` sorts by a display `Name`
(`sortKey`) that is *not* the same field as `getID` (`TrustAnchorID`/`ProfileID`/
`CrlID`) for three of the four callers -- `ListSubjects` is the exception, where both
are `SubjectID`. Real RolesAnywhere doesn't enforce trust-anchor/profile/CRL name
uniqueness, so `Name` genuinely admits ties. A dedicated test
(`TestHandler_ListTrustAnchors_Pagination_TiedNamesTotalOrder`, 6 same-named trust
anchors paginated across 3 pages) confirmed ties do **not** actually reorder between
calls here, unlike the map-range-sourced bug this class usually compounds with: sorted:
`store.Index.Get()` (the source `listByRegionIndex` sorts) returns a slice in insertion
order, not a raw Go map iteration, so repeated `sort.Slice` calls on the same
underlying slice are deterministically reproducible run-to-run absent a concurrent
insert/delete. No tiebreak was added; adding one would have been extra unproven surface
against a bug that doesn't manifest here.

Since `sortKey` != cursor field (`getID`) for 3 of 4 callers, a threshold search on the
cursor field isn't valid against a Name-sorted list. Fixed by defaulting an unresolved
token to the end of the collection (`paginate[T]` in store.go) instead -- correct for
all four callers regardless of which sort/cursor-field relationship they use.

New tests (`handler_pagination_restart_test.go`):
`TestHandler_ListTrustAnchors_Pagination_DeletedMidPage` (confirmed failing pre-fix) and
`TestHandler_ListTrustAnchors_Pagination_TiedNamesTotalOrder` (passed even pre-fix --
see tie-order note above; kept as a regression guard). `TestHandler_ListTrustAnchors_
Pagination` (existing) only ever exercised page sizes, never followed a `nextToken` into
a second page.

**Gates**: `go build ./services/rolesanywhere/...`, `go vet ./services/rolesanywhere/...`,
`go test -race -count=1 ./services/rolesanywhere/...` all pass; `golangci-lint run
./services/rolesanywhere/...` reports 0 issues.

## 2026-09-06: gopherstack-i5ss -- ImportCrl trust-anchor validation and DeleteTrustAnchor->CRL cascade (no code change)

The 2026-09-04 audit declined to invent either behavior for lack of a doc sentence or
fitting modelled error. This pass sourced both questions against
`aws-sdk-go-v2/service/rolesanywhere@v1.26.3` instead of overturning on hunch.

**Error-taxonomy extraction** (`awk "/deserializeOpError<Op>\(/,/^}/" deserializers.go`):

```
ImportCrl:          UnknownError, AccessDeniedException, ValidationException
DeleteTrustAnchor:  UnknownError, AccessDeniedException, ResourceNotFoundException
GetTrustAnchor:     UnknownError, AccessDeniedException, ResourceNotFoundException, ValidationException
DeleteCrl:          UnknownError, AccessDeniedException, ResourceNotFoundException
GetCrl:             UnknownError, ResourceNotFoundException
```

**Question 1 -- does ImportCrl validate the trust anchor exists?** Verdict: **evidence
implies AWS does not.** `ImportCrl` is the only one of these five ops without
`ResourceNotFoundException`; every sibling op that resolves an existing resource by ID/ARN
(`GetTrustAnchor`, `DeleteTrustAnchor`, `DeleteCrl`, `GetCrl`) declares it. `ValidationException`
has no doc comment at all in `types/errors.go` (a bare struct, no description) -- it cannot be
stretched to cover a missing trust anchor without inventing semantics; every other
existence-style check in this service already goes through `ResourceNotFoundException`
(`GetSubject`/`TagResource`/`ListTagsForResource`, per the frontmatter above), never
`ValidationException`. `ImportCrlInput.TrustAnchorArn`'s own doc comment only says "This
member is required" (presence, not existence). This is not merely absence of evidence: the
same declare-it-where-a-check-happens/omit-it-where-it-doesn't pattern is already the
precedent this file uses elsewhere -- `UntagResource` (line ~104 above) was left a silent
no-op specifically because it "declares no `ResourceNotFoundException` in the real model,"
and `services/acm/PARITY.md`'s `CreateAcmeExternalAccountBinding` (an owned-child create,
the closest analogue to `ImportCrl`) explicitly *does* declare `ResourceNotFoundException`
for its owning-endpoint FK and gopherstack validates it there. `ImportCrl`'s omission, next
to that established contrast, reads as deliberate, not silent. Not implemented.

**Question 2 -- does DeleteTrustAnchor cascade to CRLs?** Verdict: **evidence implies AWS
does not**, on weaker (structural, not error-taxonomy) footing than Question 1.
`DeleteTrustAnchor`'s doc comment is exactly one sentence -- "Deletes a trust anchor." --
with no mention of CRLs. `types.CrlDetail.TrustAnchorArn` is a one-directional data field
(`TrustAnchorDetail` carries no reverse reference to its CRLs). `ListCrlsInput` has exactly
two members, `NextToken`/`PageSize` -- no trust-anchor filter of any kind, so AWS's own API
gives no operational tool to enumerate "CRLs belonging to trust anchor X," which a
maintained cascade relationship would typically need. More decisively: grepping every
`api_op_*.go` in the module for a `TrustAnchorId` parameter shows it appears only on
trust-anchor-specific ops and `{Put,Reset}NotificationSettings` -- **no CRL operation
(`ImportCrl`, `GetCrl`, `UpdateCrl`, `DeleteCrl`, `EnableCrl`, `DisableCrl`, `ListCrls`)
ever takes a `TrustAnchorId`**, and `UpdateCrlInput` cannot even change `TrustAnchorArn`
once set (immutable after `ImportCrl`). Every CRL lifecycle operation in the real API is
addressed and scoped purely by `CrlId` -- contrast `services/acm/PARITY.md`'s
`DeleteAcmeEndpoint` cascade, which is justified because its children
(`AcmeExternalAccountBinding`/`AcmeDomainValidation`) are *structurally* owned: created
with a required, existence-checked `AcmeEndpointArn` FK and listed only in an
endpoint-scoped page (`ListAcmeDomainValidations`/`ListAcmeExternalAccountBindings`, "paginated
per-endpoint"), i.e. inaccessible except through the parent. CRLs have none of that: no FK
existence check on creation (Question 1), no endpoint-scoped listing, and a top-level
`ListCrls`/`GetCrl`/`DeleteCrl` surface structurally identical to `ListTrustAnchors`'s own --
siblings, not parent/child, in the API's own shape. This is silence plus multiple converging
structural signals, not a bare absence, so it's recorded as "implies not" rather than
"genuinely silent" -- but it rests on inference across several fields, not a single
authoritative declared-error contrast the way Question 1 does, hence weaker. Not implemented.

**What would change either answer**: for Question 1, AWS adding `ResourceNotFoundException`
to `ImportCrl`'s modelled errors, or a doc revision stating the ARN is validated. For
Question 2, `ListCrls` gaining a `TrustAnchorId`/`TrustAnchorArn` filter parameter, any CRL
op gaining a `TrustAnchorId` parameter, or a doc revision to `DeleteTrustAnchor` naming CRLs
explicitly.

No files under `services/rolesanywhere/` changed behavior this pass. `PARITY.md` is the only
diff.

**Gates**: `GOTOOLCHAIN=go1.26.6 go test -race ./services/rolesanywhere/...` and
`GOTOOLCHAIN=go1.26.6 golangci-lint run services/rolesanywhere/...` both pass, unchanged from
before this pass (no code touched).
