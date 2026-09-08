---
service: mediapackage
sdk_module: aws-sdk-go-v2/service/mediapackage@v1.42.4
last_audit_commit: cb5dac6ff
last_audit_date: 2026-08-29
overall: A            # 2026-08-29: independent re-sweep, deliberately NOT using this campaign's
                      # known bug-class list (see comprehend's PARITY.md same-date entry for the
                      # sibling audit that found real bugs there via this method). Re-derived
                      # HTTP bindings (path/method/query params for all 19 ops), List filter
                      # params (ListOriginEndpoints.ChannelId, ListHarvestJobs.IncludeChannelId/
                      # IncludeStatus), Tags-at-create vs Tags-only-via-TagResource split
                      # (CreateChannelInput/CreateOriginEndpointInput have Tags,
                      # CreateHarvestJobInput/UpdateChannelInput/UpdateOriginEndpointInput do
                      # not), UntagResource's TagKeys-as-repeated-query-param (not body) wire
                      # shape, and both enum types this service emits (Origination/Status) --
                      # all independently re-confirmed correct, zero new bugs found. Genuinely
                      # clean; see Notes below for exact coverage and method.
                      # wrapper-key/nested-shape sweep: zero bugs found, prior audit's claims re-verified against SDK source
ops:
  CreateChannel: {wire: ok, errors: ok, state: ok, persist: ok, note: "added missing createdAt"}
  DescribeChannel: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateChannel: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteChannel: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades origin endpoints, matches AWS"}
  ListChannels: {wire: ok, errors: ok, state: ok, persist: ok}
  ConfigureLogs: {wire: ok, errors: ok, state: ok, persist: ok, note: "was a disguised no-op -- fixed, see Notes"}
  RotateChannelCredentials: {wire: ok, errors: ok, state: ok, persist: ok, note: "route path AND rotation semantics were wrong -- fixed, see Notes"}
  RotateIngestEndpointCredentials: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateOriginEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "Authorization/MssPackage now typed+validated; Hls/Dash/Cmaf remain opaque passthrough -- see Notes"}
  DescribeOriginEndpoint: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateOriginEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "Authorization/MssPackage now typed+validated; Hls/Dash/Cmaf remain opaque passthrough -- see Notes"}
  DeleteOriginEndpoint: {wire: ok, errors: ok, state: ok, persist: ok}
  ListOriginEndpoints: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateHarvestJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "now starts IN_PROGRESS (was synchronously SUCCEEDED) and never transitions further -- see Notes; validates all 5 SDK-required members (Id/OriginEndpointId/StartTime/EndTime/S3Destination.{BucketName,ManifestKey,RoleArn})"}
  DescribeHarvestJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListHarvestJobs: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  Channel: {status: ok, note: "CreatedAt, EgressAccessLogs/IngressAccessLogs added; RotateChannelCredentials route+semantics fixed"}
  OriginEndpoint: {status: ok, note: "CreatedAt added; Authorization/MssPackage fully typed+SPEKE-validated; Hls/Dash/Cmaf remain opaque passthrough (see Notes)"}
  HarvestJob: {status: ok, note: "CreateHarvestJob now starts IN_PROGRESS instead of synchronously SUCCEEDED -- see Notes"}
  Tags: {status: ok, note: "no changes this pass; prior sweep already wired tag<->resource sync"}
deferred:
  - "HlsPackage/DashPackage/CmafPackage remain an opaque map[string]any passthrough -- NOT semantically validated (no ad-marker/encryption logic for these three). Sized this pass: HlsPackage has 12 leaf fields across 3 enum types plus a nested HlsEncryption/SpekeKeyProvider chain; DashPackage has 14 leaf fields across 5 enum types plus DashEncryption/SpekeKeyProvider; CmafPackage additionally nests a *list* of HlsManifest sub-resources (~10 fields each) -- a materially larger, separate unit of work from Authorization/MssPackage (2 and ~11 leaf fields, single level of nesting, no repeated sub-resources), which this pass modeled to full depth instead of shaving fields to fit. Next pass: model Hls/Dash/Cmaf to the same full depth if a consumer needs it."
leaks: {status: clean, note: "no goroutines/timers introduced; all ops are synchronous map operations under the existing lockmetrics.RWMutex"}
---

## Notes

### 2026-08-29 constraint-not-honoured sweep (gopherstack-wksw, same day as the audit above)

Independent pass for a different bug class than the sweep above: a parameter that
constrains a result (filter/page-limit) present in the real Input but silently unapplied,
read wrong, or applied to the wrong baseline. All 3 collection-returning ops re-checked
against their own `api_op_List*.go` in `mediapackage@v1.42.4`, confirmed query-bound (REST-
JSON, `serializeOpHttpBindingsList*Input` -- `maxResults`/`nextToken`/`includeChannelId`/
`includeStatus`/`channelId` all `encoder.SetQuery`, no JSON body member for any of them):

- `ListChannels` (`MaxResults`/`NextToken` only, no filter): `handler_channels.go:120-138`
  reads both query params, `channels.go:116-130` passes through to the shared
  `pkgs/page.New` helper. Correct.
- `ListHarvestJobs` (`IncludeChannelId`/`IncludeStatus`/`MaxResults`/`NextToken`):
  `handler_harvest_jobs.go:83-106` reads all four; `harvest_jobs.go:104-135` applies
  `IncludeChannelId`/`IncludeStatus` as exact-match filters before paginating. Correct --
  and `IncludeStatus` has no real SDK enum type (plain `*string` in both
  `ListHarvestJobsInput` and `HarvestJob.Status`, confirmed no `HarvestJobStatus` type
  exists in `types/`), so exact string comparison is the whole contract, not a
  case-folding or partial-match question.
- `ListOriginEndpoints` (`ChannelId`/`MaxResults`/`NextToken`): `handler_origin_endpoints.go:
  249-269` reads all three; `origin_endpoints.go:237-261` uses the `channelId`-indexed
  `originEndpointsByChannel` map when set, falls back to the full snapshot when empty.
  Correct; a `ChannelId` naming a channel with no endpoints correctly returns empty rather
  than erroring (AWS documents no channel-existence check for this filter).

**Documented-default check**: none of the three ops' `MaxResults` doc comments in the
pinned SDK state a numeric default (`"Upper bound on number of records to return."` /
`"The upper bound on the number of records to return."` -- no number given, unlike
kinesis/sns's ops which do). `store.go:10`'s `defaultMaxResults = 20` is therefore an
internal choice, not a violation of a documented contract -- nothing to fix here per this
sweep's "take semantics from the SDK's own doc comment, never invent them" rule; a missing
number in the doc isn't license to assert AWS's real default from outside knowledge either.
All three ops share one `pkgs/page.New` call site each, so a default-size bug would have
hit identically everywhere; confirmed no such bug in `page.New` itself (`limit <= 0 ->
defaultLimit`, correct fallback, no max-clamp because none is documented to clamp to).

**0 bugs found.** Genuinely clean for this class -- reconfirms, from a completely
different angle (parameter-by-parameter against each op's own doc comment) than the
same-date audit above (HTTP-binding/wire-shape re-derivation), that this service's List
surface is small (3 ops, 5 total constraining parameters excluding NextToken) and already
correct.

## Notes

### 2026-08-20: wrapper-key / nested-shape wire-parity sweep (zero bugs found)

Full re-verification of every op's wire shape against
`aws-sdk-go-v2/service/mediapackage@v1.42.4` source (not against
gopherstack's own handler output), per the campaign's five bug-class hunt
(generalized/missing members, wrong nesting level, wrong JSON type,
case-sensitive key near-misses, right-key-wrong-value/invented enums, and
request-only fields leaking into responses).

**Provenance correction**: the prior stamp cited `last_audit_commit
f942b4d6b9d0353bd693cc733196bc7228ededd9` (`git show -s --format=%ad`:
2026-07-24) against `last_audit_date: 2026-08-10` -- a 17-day gap with the
sha predating the date, despite 12 intervening commits on `main` in that
window (including two full-repo parity sweeps, #2402/#2404/#2406). The sha
was stale/copied forward rather than reflecting HEAD at the actual 2026-08-10
write time. Content-wise the 2026-08-10 entry's claims all independently
re-verified true this pass (see below) -- this is a stamp-hygiene fix, not a
retraction of that audit's findings. Refreshed to current HEAD
(`711100b0006aeb09a8422f1e6c09a400068f27ee`) + today.

**Full-field-list diff, every op, optional members included**: `Channel`
(`types.go:29-56`), `OriginEndpoint` (`592-654`), `HarvestJob` (`262-297`),
`HlsIngest`/`IngestEndpoint` (`324-331,536-551`), `EgressAccessLogs`/
`IngressAccessLogs` (`228-235,553-560`), `Authorization` (`10-26`),
`MssPackage`/`MssEncryption`/`SpekeKeyProvider`/
`EncryptionContractConfiguration`/`StreamSelection` (`563-590,681-736`), and
`S3Destination` (`656-677`) each diffed member-by-member against
`interfaces.go`/`models.go`/the `*Output` structs in `handler_channels.go`,
`handler_origin_endpoints.go`, `handler_harvest_jobs.go`. No member missing
in either direction; no extra/fabricated member. `CmafPackage`/`DashPackage`/
`HlsPackage`/`HlsManifest`/`HlsManifestCreateOrUpdateParameters` remain
outside this diff -- see `deferred` (unchanged from the prior pass).

**Every field's JSON key name** re-confirmed against both
`awsRestjson1_serializeDocument*` (`serializers.go`) and
`awsRestjson1_deserializeDocument*` (`deserializers.go`) for `Channel`,
`OriginEndpoint`, `Authorization`, `MssPackage` chain, `HarvestJob`,
`S3Destination`, and the `List*Output` wrapper keys (`channels`,
`originEndpoints`, `harvestJobs`, `nextToken`, `tags`) -- all match
gopherstack's `json:` tags exactly, case included. No case-sensitive
near-miss found.

**Enum check, both directions**: `originationAllow = "ALLOW"` and
`harvestJobStatusInProgress = "IN_PROGRESS"` are the only two enum-typed
string constants gopherstack emits for this service (`store.go`) -- both are
real `types.Origination`/`types.Status` values (`enums.go:158-175,302-321`).
No gopherstack-invented enum constant exists anywhere in the package.
Direction 2 (every SDK enum value representable): moot for `Origination`/
`Status` since both are passed through as bare strings without a fixed
allowlist; all fifteen other enums (`AdMarkers`, `AdsOnDeliveryRestrictions`,
`CmafEncryptionMethod`, `EncryptionMethod`, `ManifestLayout`,
`PeriodTriggersElement`, `PlaylistType`, `PresetSpeke20Audio`,
`PresetSpeke20Video`, `Profile`, `SegmentTemplateFormat`, `StreamOrder`,
`UtcTiming`, `AdTriggersElement`) live entirely inside the opaque
`CmafPackage`/`DashPackage`/`HlsPackage` passthrough blobs or as unvalidated
bare strings (`StreamSelection.StreamOrder`) -- gopherstack never declares Go
constants for them, so it structurally cannot invent a value the SDK
doesn't have.

**The four package types / four encryption types**: `HlsPackage`,
`DashPackage`, `CmafPackage` remain opaque `map[string]any` (deferred, sized
and justified in the 2026-08-10 entry below -- unchanged this pass); only
`MssPackage`/`MssEncryption` are modeled, and were re-verified field-by-field
against `serializers.go:2156-2222`/`deserializers.go:5547-5650` this pass
with a new real-SDK round-trip test (see below). No cross-contamination
found between the modeled `MssPackage` and the opaque blocks -- they are
distinct fields on `OriginEndpoint`/`PackagingConfig`, never merged.

**`HlsManifest` vs `HlsManifestCreateOrUpdateParameters`**: no leak possible
-- gopherstack has no structured `HlsManifest` type at all (confirmed by
`grep -rn HlsManifest services/mediapackage/*.go`: the only hit is a doc
comment). Both real-SDK variants live entirely inside the opaque `CmafPackage`
blob, which is stored and echoed verbatim per-request; a consequence
(pre-existing, not new this pass) is that gopherstack's echoed `CmafPackage`
never gains the server-computed `HlsManifest.Url` field the real API adds on
top of what `HlsManifestCreateOrUpdateParameters` sends -- in scope of the
same `deferred` opaque-passthrough limitation already on file, not a new gap.

**New test**: `wire_sdk_roundtrip_test.go` --
`TestCreateOriginEndpoint_AuthorizationMssPackage_SDKRoundTrip` drives
`CreateOriginEndpoint`/`DescribeOriginEndpoint` through the real
`aws-sdk-go-v2` client for every leaf field of `Authorization` and the full
`MssPackage`->`MssEncryption`->`SpekeKeyProvider`->
`EncryptionContractConfiguration`/`StreamSelection` chain. Verified the test
actually catches a real bug (not a tautology): hand-reverted
`SpekeKeyProvider.ResourceID`'s json tag from `resourceId` to `resourceld`
in `interfaces.go` via `cp` (never git), reran the test -- failed exactly as
predicted (`ResourceId` deserialized empty, since the real SDK's
`awsRestjson1_deserializeDocumentSpekeKeyProvider` only recognizes
`resourceId`), then restored `interfaces.go` from the `cp` copy and confirmed
byte-identical via `md5sum` before and after.

**No bugs found this pass.** Gates: `go build`, `go vet`, `go fix -diff`
(empty), `gofmt -l` (empty), `go test -race` (pass), `golangci-lint run`
(0 issues) all clean on `services/mediapackage/...`.

---

### 2026-08-10 audit (prior pass, content re-verified above)

**Protocol**: REST-JSON (restjson1), matching the real SDK's `awsRestjson1_*`
serializers/deserializers in `aws-sdk-go-v2/service/mediapackage@v1.42.4`.
Very few timestamps (createdAt/harvest-job start/end) and they are all
ISO8601 *string* wire values, not epoch numbers -- confirmed against the
SDK's `deserializeOpDocument*` functions, which decode `createdAt` etc. as
plain JSON strings via `ptr.String(jtv)`.

### SDK pin correction

The audit frontmatter above previously cited `v1.39.25` while `go.mod` pins
`v1.42.4` -- a stale-pin mismatch found this pass (the same class flagged
across the campaign today). All wire claims in this file are now verified
against `v1.42.4`; the `Status` enum, `HlsManifest`/`SpekeKeyProvider`/
`EncryptionContractConfiguration` shapes, and field counts cited below are
all from that version's `types/types.go` and `types/enums.go`. No API surface
changed between the two versions for this service (still 19 ops, same field
sets on every type touched this pass).

### CreateHarvestJob: IN_PROGRESS instead of synchronous SUCCEEDED

`CreateHarvestJob` set `Status=SUCCEEDED` synchronously on every create. Real
MediaPackage harvest jobs start `IN_PROGRESS` and transition to
`SUCCEEDED`/`FAILED` asynchronously as content is actually copied to the
target S3 bucket (`types.HarvestJob.Status` doc comment: "Consider setting up
a CloudWatch Event to listen for HarvestJobs as they succeed or fail" --
`types/types.go:291-294`). This backend never performs that S3 copy, so
claiming `SUCCEEDED` on create asserted work had completed that never
happened -- a stronger, false claim than a job that simply never reaches a
terminal state.

This differs from the emrserverless/elasticsearch/kinesisanalytics async-op
simplifications audited today, which start in a legitimate non-terminal state
and never transition further -- self-consistent because no other field in
their wire shape claims otherwise. This service had inverted that pattern:
it jumped straight to a terminal state instead of stopping short of one.
Fixed to match the same *shape* of simplification those three use: `Status`
now starts `IN_PROGRESS` (`harvestJobStatusInProgress`, `harvest_jobs.go`)
and this backend never transitions it further -- no goroutine or timer
introduced, so no new leak surface, and every other field (`CreatedAt`,
`S3Destination`, etc.) remains internally consistent with "submitted, not yet
complete."

The enum itself was also incomplete: `types.Status` defines `IN_PROGRESS`,
`SUCCEEDED`, and `FAILED` (`types/enums.go:302-317`), but gopherstack only
had a `harvestJobStatusSucceeded` constant -- `IN_PROGRESS` did not exist as
a Go value anywhere in the package, which is a wire-completeness gap
independent of whether this backend ever reaches that state. Now declared
and used as the initial (and only) status this backend sets; `SUCCEEDED`/
`FAILED` are documented in a comment rather than declared as unused
identifiers (this backend never reaches either).

### Packaging protocol blocks: Authorization and MssPackage modeled to full depth

Per the modeling standard (full real-SDK depth or leave it and say why -- no
field-shaving), each opaque `map[string]any` packaging block was sized
against `types/types.go` in `aws-sdk-go-v2/service/mediapackage@v1.42.4`
before deciding what to model:

- **Authorization** (`types.go:10-26`): 2 required string fields
  (`CdnIdentifierSecret`, `SecretsRoleArn`), no nesting. Modeled fully as a
  typed `Authorization` struct (`interfaces.go`); `CreateOriginEndpoint`/
  `UpdateOriginEndpoint` now 422 if the block is present but either field is
  empty (previously any partial/malformed authorization map was silently
  accepted).
- **MssPackage** (`types.go:575-590`): 4 top-level fields, one level of
  nesting via `MssEncryption`→`SpekeKeyProvider` (`types.go:563-572,
  681-721`, 5 fields + nested `EncryptionContractConfiguration`,
  `types.go:246-259`, 2 fields) and `StreamSelection` (`types.go:724-736`, 3
  fields) -- 11 leaf fields total, no repeated sub-resources. Modeled fully
  as typed `MssPackage`/`MssEncryption`/`SpekeKeyProvider`/
  `EncryptionContractConfiguration`/`StreamSelection` structs; validates the
  SPEKE required-together chain (`SpekeKeyProvider` required if
  `Encryption` is present; `ResourceId`/`RoleArn`/`SystemIds`/`Url` required
  if `SpekeKeyProvider` is present; `PresetSpeke20Audio`/`PresetSpeke20Video`
  required together if `EncryptionContractConfiguration` is present) --
  closing the "no SPEKE handling" gap for this block.
- **HlsPackage/DashPackage/CmafPackage** are left as opaque passthrough --
  see `deferred` in the frontmatter for exact sizes. These are materially
  larger (12-16 leaf fields each across several enum types, and CmafPackage
  additionally nests a *list* of `HlsManifest` sub-resources, ~10 fields
  each) -- a distinct, larger unit of work than Authorization/MssPackage, not
  something to partially model and call done.

`PackagingConfig`/`OriginEndpoint`/`storedOriginEndpoint`/
`originEndpointOutput` all changed `Authorization`/`MssPackage` from
`map[string]any` to the new typed pointers; the JSON field names are
unchanged (`cdnIdentifierSecret`, `secretsRoleArn`, `resourceId`, `roleArn`,
`url`, `systemIds`, `certificateArn`, `encryptionContractConfiguration`,
`presetSpeke20Audio`, `presetSpeke20Video`, `manifestWindowSeconds`,
`segmentDurationSeconds`, `streamSelection`, `maxVideoBitsPerSecond`,
`minVideoBitsPerSecond`, `streamOrder`, `encryption`, `spekeKeyProvider`),
verified against both `serializers.go` and `deserializers.go` for symmetry,
so this is additive/compatible for any snapshot already using real field
names; the snapshot version was not bumped.

### Bugs found and fixed this pass

1. **RotateChannelCredentials was unreachable at its real path (route-matcher
   bug class).** `classifyChannelPath` matched `POST
   /channels/{id}/ingest_endpoints/credentials` for this op, but the real SDK
   sends `PUT /channels/{id}/credentials`
   (`awsRestjson1_serializeOpHttpBindingsRotateChannelCredentialsInput`, SDK
   `serializers.go:1179`). The wrong path was even baked into a unit test
   (`handler_audit1_test.go`), which is exactly why unit tests alone aren't
   parity proof -- they exercised the classifier via `h.Handler()` directly
   with a hand-picked path that a real client never sends. A genuine
   `RotateChannelCredentials` call would have 404'd as "unknown operation."
   Fixed the route (`handler.go`) and updated the two tests that referenced
   the fictional path.

2. **RotateChannelCredentials also had wrong rotation semantics.** The SDK
   doc comment says "Changes the Channel's first IngestEndpoint's username
   and password" (deprecated in favor of
   RotateIngestEndpointCredentials-by-ID), but the backend regenerated
   *both* ingest endpoints from scratch with brand-new IDs and URLs. Real AWS
   only rotates `ingestEndpoints[0]`'s credentials and leaves ID/URL (and the
   second endpoint) untouched. Fixed in `backend.go`.

3. **ConfigureLogs was a disguised no-op.** It accepted
   `egressAccessLogs`/`ingressAccessLogs` (each carrying `logGroupName`),
   discarded them with `_ = egressLogGroup; _ = ingressLogGroup`, and always
   returned the channel completely unchanged -- a textbook stub matching
   parity-principles rule #1 (never ship an op that reads/mutates nothing).
   Real MediaPackage's Channel/CreateChannelOutput/ConfigureLogsOutput always
   carry `egressAccessLogs`/`ingressAccessLogs` (each `{logGroupName:
   string}`), which `channelOutput` didn't even have fields for. Fixed:
   `storedChannel` now has `EgressLogGroupName`/`IngressLogGroupName
   *string`; `ConfigureLogs(id, egressLogGroup, ingressLogGroup *string)`
   only overwrites a side when the caller's request included that key (nil
   pointer means "leave existing config alone", matching AWS's
   independently-optional members); `channelOutput` gained
   `egressAccessLogs`/`ingressAccessLogs` (omitted when unset, present when
   configured).

4. **CreateOriginEndpoint/UpdateOriginEndpoint discarded the packaging
   protocol config entirely.** Real MediaPackage's OriginEndpoint carries
   `authorization`, `hlsPackage`, `dashPackage`, `cmafPackage`, and
   `mssPackage` (confirmed field names against
   `awsRestjson1_serializeOpDocumentCreateOriginEndpointInput` /
   `UpdateOriginEndpointInput` in the SDK). gopherstack's handler never even
   parsed these keys out of the request body, so any Terraform/CDK
   OriginEndpoint configured with e.g. `hls_manifest` blocks would silently
   lose that configuration on create -- a "create that discards config"
   no-op per parity-principles. Fixed: added a `PackagingConfig` struct
   (`Authorization`/`CmafPackage`/`DashPackage`/`HlsPackage`/`MssPackage`,
   each `map[string]any`) threaded through `CreateOriginEndpoint`/
   `UpdateOriginEndpoint`; each block is stored and echoed back verbatim on
   Describe/List (see "deferred" above for the scope of this fix -- it's an
   opaque passthrough, not semantic modeling of encryption/ad-marker
   config).

5. **CreatedAt was missing from both Channel and OriginEndpoint entirely.**
   Real AWS always returns this field (confirmed in both types' Describe
   deserializers). Added `CreatedAt string` to both, set at creation time.

### Bugs looked for but NOT found (already correct)

- ARN shapes (`arn:aws:mediapackage:<region>:<account>:channels/<id>` and
  `.../origin_endpoints/<id>`) match AWS's pattern.
- Error status codes: NotFoundException->404, UnprocessableEntityException->422
  (used for both "already exists" and "invalid parameter", matching the real
  SDK's error type set -- there is no ConflictException in this API).
  `__type` is included on error bodies for SDK exception classification.
- DeleteChannel/DeleteOriginEndpoint correctly return 202 Accepted with an
  empty body.
- List* pagination (`nextToken`, `maxResults`) uses `pkgs/page` uniformly.
- Tag<->resource sync (TagResource/UntagResource updating the resource's own
  `Tags` field, not just the separate ARN-keyed tag store) was already
  correct from a prior sweep.
- RotateIngestEndpointCredentials (the newer, ID-scoped op) already had the
  correct `PUT /channels/{id}/ingest_endpoints/{ingestId}/credentials` route
  and semantics.

### Invented ops deleted this pass

`CreatePackagingConfiguration`/`DescribePackagingConfiguration`/
`DeletePackagingConfiguration`/`ListPackagingConfigurations` and
`PutChannelLifecyclePolicy`/`GetChannelLifecyclePolicy` were registered and
routed in this service but **do not correspond to any operation in the real
`aws-sdk-go-v2/service/mediapackage` client** (v1.39.25) -- confirmed by
listing `api_op_*.go` in the downloaded module source: there are exactly 19
files, matching the 19 ops in the `ops:` table above, with no
`api_op_CreatePackagingConfiguration.go` / `api_op_PutChannelLifecyclePolicy.go`
etc., and no `PackagingConfiguration`/`PackagingGroup` type in `types/types.go`
either. PackagingConfiguration/PackagingGroup belong to MediaPackage VOD, a
separate AWS service with its own SigV4 signing name and REST surface (not a
dependency of this repo's go.mod). No real `aws-sdk-go-v2/service/mediapackage`
client will ever call these paths -- a prior audit pass flagged this but left
it in place; this pass deletes it outright, per the "delete gopherstack-invented
surface not in the real SDK" rule. This wasn't caught by `TestSDKCompleteness`
because that check only flags SDK ops *missing* from `GetSupportedOperations()`,
not extra ops beyond the SDK's surface.

Removed: the `/packaging_configurations` route family and `lifecycle_policy`
channel sub-route (`handler.go`); `handler_packaging_configurations.go` and
its test file; `packaging_configurations.go` (backend CRUD); the
`PackagingConfiguration`/`storedPackagingConfiguration` types and
`CreatePackagingConfiguration`/`Describe`/`Delete`/`List` +
`PutChannelLifecyclePolicy`/`GetChannelLifecyclePolicy` methods from
`StorageBackend` and `InMemoryBackend`; the `packagingConfigurations`
`store.Table` and its ARN builder; `storedChannel.LifecyclePolicy`; the
`PackagingConfigCount` test helper; and every test referencing any of the
above (`handler_packaging_configurations_test.go`, the `packagingConfigurations`
snapshot-restore subtest, the four `TestLifecyclePolicy*`/`TestChannelLifecyclePolicy*`
tests in `handler_channels_test.go`, and the packaging-config case in
`TestNotFound_ErrorType`). Not touched: `PackagingConfig` (no trailing "uration") --
that is a real, distinct type holding the Authorization/CmafPackage/DashPackage/
HlsPackage/MssPackage opaque blocks on the legitimate `OriginEndpoint` resource,
confirmed against `types.OriginEndpoint` in the real SDK; it was not renamed or
removed.

### CreateHarvestJob required-field validation added

The real SDK's `CreateHarvestJobInput` marks `EndTime`, `Id`, `OriginEndpointId`,
`S3Destination`, and `StartTime` all `// This member is required.`, and
`types.S3Destination` itself requires `BucketName`/`ManifestKey`/`RoleArn`
(confirmed in `api_op_CreateHarvestJob.go` and `types/types.go`). A previous
pass only validated `Id`/`OriginEndpointId` and flagged the rest as a gap;
this pass closes it: `CreateHarvestJob` (`harvest_jobs.go`) now 422s
(`ErrInvalidParameter`) when `StartTime`, `EndTime`, or any of the three
`S3Destination` fields is empty, matching what a real client would have
already validated client-side before the request ever reached the server.

## 2026-08-22 gopherstack-wlo1: error envelope is wire shape too

`mapError`'s NotFound branch was the only one of four that set `__type` in
the response body (via `jsonErrorTyped`); AlreadyExists, InvalidParameter,
and the default (internal-error) branches all called the untyped `jsonError`
helper, which wrote only `Message`. mediapackage doesn't use the
X-Amzn-Errortype header approach (no `amznErrorTypeHeader` const exists in
this package) -- it relies entirely on the body `__type` field -- so these
three branches produced a body restjson.GetErrorInfo could not classify:
every conflict, validation failure, and internal error decoded client-side
as `smithy.GenericAPIError{Code:"UnknownError"}`, not the three-quarters of
this service's error surface it should have been. Separately, the two
handleREST dispatch-failure sites ("invalid JSON body" 400, "unknown
operation" 404) had the same gap. Confirmed against
aws-sdk-go-v2/service/mediapackage@v1.42.4 deserializers.go's
`awsRestjson1_deserializeOpErrorCreateChannel`, which models exactly
`ForbiddenException`, `InternalServerErrorException`, `NotFoundException`,
`ServiceUnavailableException`, `TooManyRequestsException`, and
`UnprocessableEntityException` -- notably no 400-class exception at all, so
the malformed-body path now reuses `UnprocessableEntityException` (the
closest modeled "bad input" type; the deserializer classifies purely by
`__type`/header, not by matching the HTTP status returned).

Fixed: `mapError`'s three untyped branches now call `jsonErrorTyped` with
`ErrConflict.Error()` ("UnprocessableEntityException"),
`ErrInvalidParameter.Error()` (same), and a literal
`"InternalServerErrorException"` for the default case; the untyped
`jsonError` helper is removed (dead after the fix). The two dispatch sites
now include `"__type": "UnprocessableEntityException"` and
`"__type": "NotFoundException"` respectively, alongside the existing
`Message` key.

Proof (`handler_error_type_test.go`, new): `TestCreateChannel_DuplicateIDSurfacesUnprocessableEntityException`
and `TestCreateChannel_EmptyIDSurfacesUnprocessableEntityException` drive
the AlreadyExists and InvalidParameter branches through a real
`mediapackagesdk.Client` with no middleware needed (the empty-Id case
relies on `validateOpCreateChannelInput` only rejecting a nil Id, not an
empty string). `TestCreateChannel_MalformedBodySurfacesUnprocessableEntityException`
and `TestCreateChannel_UnrecognisedRouteSurfacesNotFoundException` use the
same request-corrupting smithy middleware technique as the sibling
medialive/mediatailor fixes to reach the two dispatch-failure sites. All
four confirmed failing against the unfixed `handler.go` (asserted
"UnknownError") via hand-revert, then restored byte-identical
(md5sum-verified). Same bug class as gopherstack-wlo1's medialive,
mediatailor, and vpclattice fixes, and the s3control/iot instances that
opened the issue.

## 2026-08-29: independent re-sweep, no bug-class checklist (clean)

Given the same brief as comprehend's same-date sweep (see that service's
PARITY.md for the method and what it found there): compare from first
principles against the pinned SDK, without using this campaign's own list of
previously-found bug classes, specifically to test whether the campaign's
rising clean-result rate reflects real correctness or checklist blindness.
mediapackage had already been swept three times (2026-08-10, 2026-08-20,
2026-08-22) at real depth, so this pass deliberately targeted areas those
sweeps' own stated scope (wrapper keys/nesting, error-envelope typing)
would not have emphasized:

- **HTTP-level request shape, all 19 ops**: extracted every
  `awsRestjson1_serializeOpHttpBindings<Op>Input` function's path
  (`httpbinding.SplitURI`) and HTTP method directly from `serializers.go`,
  independent of `handler.go`'s own routing table, then cross-checked
  `classifyPath`/`classifyChannelPath`/`classifyOriginEndpointPath`/
  `classifyHarvestJobPath`/`classifyTagPath` against that extracted list.
  19 of 19 paths+methods match exactly, including the two previously-fixed
  ops (`RotateChannelCredentials` at `PUT /channels/{Id}/credentials`,
  `RotateIngestEndpointCredentials` at
  `PUT /channels/{Id}/ingest_endpoints/{IngestEndpointId}/credentials`).
- **List query-parameter filters**: `ListOriginEndpointsInput.ChannelId`,
  `ListHarvestJobsInput.IncludeChannelId`/`IncludeStatus` (both from
  `serializers.go`'s own binding functions, not assumed) are read and
  applied by `handleListOriginEndpoints`/`handleListHarvestJobs` and
  correctly filter in the backend (`origin_endpoints.go`/`harvest_jobs.go`).
  `ListChannelsInput` genuinely has no filter fields on the real SDK (only
  `MaxResults`/`NextToken`) -- confirmed, not a gap.
- **`UntagResourceInput.TagKeys` is a repeated query parameter
  (`encoder.AddQuery("tagKeys")`), NOT a JSON body field** -- easy to get
  backwards (`TagResourceInput.Tags` IS a body field on the same op family).
  `handleUntagResource` correctly reads `c.QueryParams()["tagKeys"]`.
- **Tags-at-create vs Tags-only-via-TagResource, field-diffed per op**:
  `CreateChannelInput`/`CreateOriginEndpointInput` both have a `Tags` field
  (handled: `handleCreateChannel`/`handleCreateOriginEndpoint` both call
  `extractTags`); `CreateHarvestJobInput` has NO `Tags` field at all
  (correctly not extracted in `handleCreateHarvestJob`);
  `UpdateChannelInput` has only `Id`+`Description` (no `Tags`, matches
  `handleUpdateChannel`); `UpdateOriginEndpointInput` likewise has no
  `Tags` (matches `handleUpdateOriginEndpoint`).
- **Both enum types this service actually emits**: `types.Origination`
  (`ALLOW`/`DENY`) and `types.Status` (`IN_PROGRESS`/`SUCCEEDED`/`FAILED`,
  used only for `HarvestJob.Status`) -- gopherstack's `originationAllow`/
  `harvestJobStatusInProgress` constants match exactly; this is the same
  enum-VALUE check that found real bugs in comprehend, run here too and
  came back clean. No other status/enum-shaped field exists on this
  service's wire surface (`Channel` has no `Status` field on the real API
  at all).
- **List ordering**: `store.Table.Snapshot()` returns items sorted by key
  ascending (`pkgs/store/table.go:184-201`), deterministic but AWS itself
  documents no particular order for these List ops -- not a client-
  observable divergence, since no real client can assert on order here.
- **Client-side required-field/required-together validators**
  (`validators.go`): re-diffed `validateAuthorization`/`validateMssPackage`/
  `validateMssEncryption`/`validateSpekeKeyProvider`/
  `validateEncryptionContractConfiguration` against
  `origin_endpoints.go`'s `validatePackagingConfig` -- matches exactly,
  including the "required only if the parent block is present" nesting
  (confirms the 2026-08-10 entry's claims independently, not just trusting
  the prior stamp).

**No bugs found.** Direction verified: both request (HTTP bindings, filter
params, tag-field presence) and response (enum values, list ordering).
Gates: `go build`, `go vet` (repo-wide), `go test -race -count=1`,
`golangci-lint run --fix` all clean on `services/mediapackage/...`; no file
in this service was modified this pass.

**Not covered this pass**: a fresh full member-by-member diff of
`Channel`/`OriginEndpoint`/`HarvestJob` (already done exhaustively
2026-08-20, unchanged since); the opaque `HlsPackage`/`DashPackage`/
`CmafPackage` passthrough (unchanged, already disclosed in `deferred`).

### 2026-08-31 (gopherstack-uox6, value-semantics-of-a-correctly-read-field pass)

`covledger -service mediapackage` reported no rows for every class, and
`git log --oneline -- services/mediapackage/` shows no prior pass targeting
this specific class (wrong algorithm applied to a correctly-read field, as
opposed to the wire-shape axis this file otherwise tracks). Checked every
List/Describe filter field this service declares against its own doc
comment in `aws-sdk-go-v2/service/mediapackage@v1.42.4`:

- `ListHarvestJobs.IncludeChannelId`/`.IncludeStatus`: plain equality,
  matches "When specified, the request will return only ... associated
  with/in the given ...". Both correctly skip the comparison when the
  filter is empty (absence means no filter, not the AWS doc stating any
  other default). `harvest_jobs.go:116-122`.
- `ListOriginEndpoints.ChannelId`: same shape, `origin_endpoints.go:246-250`.
- Neither service has an operator grammar, wildcard, negation, case-
  insensitivity, or range/bound filter documented anywhere in its pinned
  SDK -- `grep -in "wildcard|case.sensitiv|regex|negat|prefix|substring"`
  over every `api_op_List*.go`/`api_op_Describe*.go` found nothing. No
  `MaxResults` doc comment on any of the 3 paginated List ops states a
  specific default number, so the narrowing/widening-default sub-shape
  that hit shield/ecs/kms has no surface here either -- the internal
  `defaultMaxResults` cap contradicts nothing documented.
- `IncludeStatus`'s stored field (`storedHarvestJob.Status`) is the same
  `Status` enum family used elsewhere in this file's wire audit
  (`IN_PROGRESS`/`SUCCEEDED`/`FAILED`) -- re-confirmed, not assumed.

Zero bugs found; this is a genuine clean result on this axis; the service
is structurally too small (2 filter parameters total across 3 List ops) to
carry most of this class's known sub-shapes.

One test-quality gap found and fixed: `TestHarvestJob_List`'s "filter by
channel" case only ever seeded one channel, so it could not distinguish
"filtered correctly" from "filter ignored, matched everything" -- the
exact weakness this bd issue warns about. Added a second channel with its
own harvest job; the channel-filtered case now asserts the count (3, not
just non-empty) and that the other channel's job is absent. Proved the new
assertion can fail: temporarily changed `harvest_jobs.go`'s
`includeChannelID != ""` guard to `false`, watched
`TestHarvestJob_List/filter_by_channel_returns_subset,_excludes_other_channel`
fail with the other channel's job leaking through, then restored the file
byte-identical (`diff` empty, `git status --short` clean before/after).
`list all jobs returns all` updated from 3 to 4 to account for the new
seed job; assertion count otherwise unchanged. No production code changed.

### 2026-09-04 audit: ghost-tags-after-delete (already fixed on branch), a hot-path O(n) tag lookup fixed

Full re-run of the campaign's two cheap high-yield checks plus the yield-order
list: never-returned sentinels (all three of `ErrNotFound`/`ErrConflict`/
`ErrInvalidParameter` are reachable, `errors.go` vs `git grep`, no gap);
parsed-then-dropped fields (`PackagingConfig`'s five blocks all round-trip
Create->Describe, re-confirmed against `handler_origin_endpoints.go`); Delete
preconditions (`DeleteChannel`/`DeleteOriginEndpoint` doc comments in
`api_op_Delete{Channel,OriginEndpoint}.go` state no "must first" constraint,
so the existing cascade-delete is a legitimate simplification, not a missing
guard -- confirmed, not just assumed); `UpdateOriginEndpoint`/`UpdateChannel`
partial-update direction (no field-level "replaces the entire configuration"
sentence in `api_op_Update{OriginEndpoint,Channel}.go`, so the existing
leave-unspecified-fields-unchanged semantics is not contradicted by any
doc text); ghost rows after delete; fabricated error codes; unreachable enum
values -- all reconfirmed clean, per PARITY.md's own recent history.

**Ghost tags after DeleteChannel/DeleteOriginEndpoint**: already fixed on
this branch immediately prior to this pass (`b8484292f`, this repo's commit
`b8484292f`) -- `channels.go`/`origin_endpoints.go` now `delete(b.tags, ...)`
on both delete paths, with `TestBackend_DeleteChannel_ClearsTagsOnRecreate`
proving it fail-before/pass-after. No further action needed; noted here so
this file's own audit trail records it was independently re-checked, not
missed.

**Performance bug found and fixed**: `TagResource`/`UntagResource`
(`tags.go`) called `findChannelByARN`/`findOriginEndpointByARN` on every
single call, each doing a full `store.Table.Range` scan (O(n) over every
channel, then every origin endpoint) under the coarse write lock, to find
the one row matching a resource ARN the caller already supplied -- the
`store` package doc (`pkgs/store/table.go`) states `Get` is the O(1) primary
lookup and `Range`/`All` are for genuine full scans, so this was the "clone
whole map to read one entry" class flagged in this audit's brief. Since
MediaPackage ARNs are built as `arn:<partition>:mediapackage:<region>:
<account>:<resourceType>/<id>` (`buildChannelARN`/`buildOriginEndpointARN`,
`store.go`), the ID can be read directly out of the ARN's trailing segment
and looked up with `Table.Get` (O(1)) instead of scanning. Fixed:
`splitMediaPackageResourceARN` extracts `(resourceType, id)` from the ARN;
`findChannelByARN`/`findOriginEndpointByARN` now do a direct `Get(id)` plus
an ARN-equality check (preserving the old behavior of refusing a same-ID,
different-account/region ARN collision) instead of a linear `Range`.

Proof: `TestTags_TagResourceTargetsCorrectResourceAmongMany`
(`handler_tags_test.go`) creates two channels and two origin endpoints,
tags one of each by ARN, and asserts only that resource's `Tags` field
changed. Neutered `findChannelByARN` in place (looked up `b.channels.Get
(resourceType)` -- the wrong key -- instead of `Get(id)`), reran: failed with
`Not equal: expected: string("team-a") actual: <nil>(<nil>)` at
`handler_tags_test.go:253` (the tagged channel's own `Tags` field never
synced). Restored `tags.go` from a `cp` copy, confirmed byte-identical, reran
-- passes. Gates: `go build`, `go test -race -count=1`, `golangci-lint run`
all clean on `services/mediapackage/...` (HEAD `4d7407a11`).

No other bugs found this pass.
