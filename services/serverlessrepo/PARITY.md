---
service: serverlessrepo
sdk_module: aws-sdk-go-v2/service/serverlessapplicationrepository@v1.33.4
last_audit_commit: af89d3e6f
last_audit_date: 2026-08-20
overall: A            # zero known gaps; every op field-diffed against real serializers/deserializers/model this pass
ops:
  CreateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "201 Created via errHTTP201 sentinel; optionally creates the first version in the same call when semanticVersion + one of sourceCodeUrl/sourceCodeArchiveUrl/templateUrl are given, matching real API behavior. FIXED 2026-08-20: top-level sourceCodeUrl leaked onto the response root -- see Notes"}
  GetApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "embeds current/queried Version; explicit ?semanticVersion=X 404s if missing, implicit default silently omits Version if app has none. FIXED 2026-08-20: top-level sourceCodeUrl leaked onto the response root -- see Notes"}
  UpdateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH; labels replaced only when the JSON key is present (nil vs [] distinguished). FIXED this pass: readmeBody was silently dropped -- see Notes. FIXED 2026-08-20: top-level sourceCodeUrl leaked onto the response root -- see Notes"}
  DeleteApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "204 No Content; cascades to versions/templates/changesets/policy/dependencies"}
  ListApplications: {wire: ok, errors: ok, state: ok, persist: ok, note: "nextToken = exclusive cursor on last-seen application Name, matching Table's Name-ascending key order"}
  CreateApplicationVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /applications/{id}/versions/{semanticVersion}, 201 Created; synthesizes templateUrl when only sourceCodeUrl/sourceCodeArchiveUrl given. FIXED this pass: templateBody was silently dropped -- see Notes. FIXED this pass: unknown applicationId returned NotFoundException (404), but this op models no NotFoundException -- now BadRequestException (400)"}
  ListApplicationVersions: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: summaries no longer include the invented non-wire 'resourcesSupported' key -- real VersionSummary shape is exactly applicationId/creationTime/semanticVersion/sourceCodeUrl, see Notes"}
  CreateCloudFormationTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "status ACTIVE->EXPIRED computed dynamically off ExpirationTime at read time, not stuck PREPARING"}
  GetCloudFormationTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCloudFormationChangeSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "TemplateId request field is cross-validated against a prior CreateCloudFormationTemplate call for the same application; an unknown or wrong-application applicationId/templateId is a BadRequestException (400), not NotFoundException -- this op's modelled error set has no NotFoundException, see Notes"}
  GetApplicationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutApplicationPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "action allow-list matches the real 8 documented SAR policy actions (fixed in a prior pass)"}
  ListApplicationDependencies: {wire: ok, errors: ok, state: ok, persist: ok}
  UnshareApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "204 No Content; organizationId is validated as required but not otherwise checked against PutApplicationPolicy's PrincipalOrgIDs -- acceptable emulation simplification"}
families:
  route_matcher: {status: ok, note: "every op's HTTP method + path template cross-checked against aws-sdk-go-v2 serializers.go (POST /applications, PUT .../versions/{v}, PATCH .../{id}, DELETE .../{id}, PUT/GET .../policy, POST .../changesets, POST .../templates, GET .../templates/{id}, GET .../dependencies, POST .../unshare) -- all match; ExtractOperation dispatch table is exhaustive and correct"}
  error_shapes: {status: ok, note: "BadRequestException(400)/ConflictException(409)/NotFoundException(404)/InternalServerErrorException(500) __type strings and status codes verified against types/errors.go and api-2.json httpStatusCode traits. ForbiddenException(403)/TooManyRequestsException(429) are declared on every real operation but intentionally unimplemented: gopherstack has no IAM-authorization or rate-limiting subsystem to derive them from (no other service in this codebase synthesizes these either), so there is no state to key a 403/429 off of; this is a systemic emulator scope decision, not a service-specific gap."}
gaps: []
deferred: []
leaks: {status: clean, note: "coarse lockmetrics.RWMutex guards all backend maps; store.Table/Index used throughout (no raw sync.Mutex, no per-map locks); Snapshot/Restore round-trip all state including the 3 dirty tables (appVersions/cfTemplates/cfChangeSets) via an ephemeral DTO registry and the 2 plain maps (appPolicies/appDependencies) directly"}
---

## Notes

Protocol: **restjson1** (`aws-sdk-go-v2/service/serverlessapplicationrepository`, generated
from `models/apis/serverlessrepo/2017-09-08/api-2.json` in `aws-sdk-go@v1.55.5`). All
timestamp fields (`creationTime`, `expirationTime`) are modeled as plain `string`, not a
`timestamp` shape -- there is no epoch-vs-ISO8601 wire trap here the way there is in
JSON-1.0/1.1 services; gopherstack's `isoTimestamp` (RFC3339 UTC) is a reasonable, real-AWS-
compatible string format and does not need `pkgs/awstime.Epoch`.

### This pass (2026-08-20)

Wrapper-key / nested-shape wire-parity sweep: all 13 real ops (`GetSupportedOperations`
vs. the 13 `api_op_*.go` files under `serverlessapplicationrepository@v1.33.4` -- both
enumerations agree) field-diffed member-by-member against their own live
`awsRestjson1_deserializeOpDocument<Op>Output`/`awsRestjson1_deserializeDocument<Type>`
switch statements in `deserializers.go`, not against sibling ops or gopherstack's own
JSON tags. Protocol confirmed restjson1 (no `X-Amz-Target`, `SplitURI`-based routing).
The `awsRestjson1_deserializeOpDocument<Op>Output` cnhp trap does not apply here: every
op with a body has that helper both defined AND called from its `HandleDeserialize`
(verified via `grep -c` returning 2 for all 11 non-void ops); `DeleteApplication`/
`UnshareApplication` are genuinely void (204, no body).

One real bug found and fixed:

1. **`applicationResponse` (shared by `CreateApplication`/`GetApplication`/
   `UpdateApplication`) carried a fabricated top-level `sourceCodeUrl` field.** The real
   `CreateApplicationOutput`/`GetApplicationOutput`/`UpdateApplicationOutput` shapes
   (`api_op_CreateApplication.go`, `api_op_GetApplication.go`,
   `api_op_UpdateApplication.go`) have no `SourceCodeUrl` member at the response root at
   all -- confirmed both by the struct field list and by each op's own
   `deserializeOpDocument*Output` case statement, none of which has a `case
   "sourceCodeUrl"` at the top level. `sourceCodeUrl` exists only nested under the
   optional `Version` sub-object (`types.Version.SourceCodeUrl`, case `"sourceCodeUrl"`
   inside `awsRestjson1_deserializeDocumentVersion`). `CreateApplicationInput` *does*
   carry a request-only `SourceCodeUrl` (used to seed the app's first version) -- this is
   the brief's "a REQUEST-ONLY field appearing in a RESPONSE" bug class: gopherstack's
   `toApplicationResponse` (`handler_applications.go`) was copying
   `Application.SourceCodeURL` onto `applicationResponse.SourceCodeURL` at the response
   root in addition to (correctly) embedding it under `resp.Version.SourceCodeURL`. Real
   `aws-sdk-go-v2` clients silently ignore unknown JSON keys, so this was not
   client-breaking for the Go SDK, but is a real wire-shape inaccuracy for any strict
   consumer (schema validators, other-language SDKs). Fixed by removing the
   `SourceCodeURL` field from `applicationResponse` and its assignment in
   `toApplicationResponse` (`handler_applications.go`). No existing test asserted the
   wrong top-level key (grepped `resp["sourceCodeUrl"]`/`response["sourceCodeUrl"]`
   across all `_test.go` -- zero hits), so nothing needed correcting; new regression
   coverage: `TestCreateApplication_NoTopLevelSourceCodeURL_SDKRoundTrip`
   (`wire_sdk_roundtrip_test.go`), which drives the real `aws-sdk-go-v2` client through
   `pkgs/service`'s router and additionally inspects the raw wire body (the SDK client
   itself can't detect an extraneous ignored key). Hand-revert (`cp` a pre-fix copy back
   over `handler_applications.go`, no git) reproduced the symptom: the round-trip test's
   `require.False(t, hasTopLevelSourceCodeURL, ...)` failed with the field present in the
   raw JSON body; restoring the fix made it pass again.

All other families confirmed clean this pass, each verified against its own
deserializer/type independently (not against a sibling):

- `Application` (`toApplicationResponse`) vs. `ApplicationSummary`
  (`applicationSummary`/`toApplicationResponse`'s list counterpart): the full type's 13
  wire fields (`applicationId`, `author`, `creationTime`, `description`, `homePageUrl`,
  `isVerifiedAuthor`, `labels`, `licenseUrl`, `name`, `readmeUrl`, `spdxLicenseId`,
  `verifiedAuthorUrl`, `version`) match `GetApplicationOutput`/`CreateApplicationOutput`/
  `UpdateApplicationOutput` exactly (after the fix above); the summary's narrower 8 fields
  (`applicationId`, `author`, `creationTime`, `description`, `homePageUrl`, `labels`,
  `name`, `spdxLicenseId`) match `types.ApplicationSummary`/
  `awsRestjson1_deserializeDocumentApplicationSummary` exactly -- correctly omits
  `isVerifiedAuthor`/`licenseUrl`/`readmeUrl`/`verifiedAuthorUrl`/`version`, none of which
  the real summary type carries.
- `Version`/`ParameterDefinition`/`ApplicationPolicyStatement`/`ParameterValue`: `Version`
  (`versionResponse`, 9 fields) matches `types.Version` exactly, including the
  request-only-vs-response-nested distinction confirmed above -- `CreateApplicationVersion`
  is the one op where `Version`'s fields are genuinely flat at the response root
  (`CreateApplicationVersionOutput` itself has no `version` wrapper key), which
  `toVersionResponse` already modeled correctly. `ParameterDefinition` (`models.go`, 13
  fields) matches `types.ParameterDefinition` exactly (all 6 optional numeric/pattern
  fields present, not just the 7 already-carried ones). `ApplicationPolicyStatement` (4
  fields: `actions`, `principalOrgIDs`, `principals`, `statementId`) matches
  `types.ApplicationPolicyStatement` exactly, including the case-sensitive
  `principalOrgIDs` spelling (capital ID, confirmed against both serializer and
  deserializer).
- `CreateCloudFormationChangeSet`/`CreateCloudFormationTemplate`/
  `GetCloudFormationTemplate`: response shapes (4 fields; 7 fields; 7 fields
  respectively) match `CreateCloudFormationChangeSetOutput`/
  `CreateCloudFormationTemplateOutput`/`GetCloudFormationTemplateOutput` exactly.
  `Status` enum usage (`ACTIVE`/`EXPIRED`, dynamically computed) is a strict subset of
  the real 3-value `types.Status` enum (`PREPARING`/`ACTIVE`/`EXPIRED`) -- gopherstack
  never emits `PREPARING` since template creation is synchronous, which is narrower-but-
  valid, not wrong.
- The three list ops' three item types verified separately, each against its own
  deserializer: `ListApplications` → `ApplicationSummary` (8 fields, see above);
  `ListApplicationVersions` → `VersionSummary` (4 fields: `applicationId`,
  `creationTime`, `semanticVersion`, `sourceCodeUrl` -- correctly excludes
  `resourcesSupported`, already caught and documented by the prior pass);
  `ListApplicationDependencies` → `ApplicationDependencySummary` (2 fields:
  `applicationId`, `semanticVersion`). All three gopherstack response-builders
  (`handleListApplications`, `handleListApplicationVersions`,
  `handleListApplicationDependencies`) use a distinct shape per op; none cross-copies
  another list op's fields.
- `Capability` enum checked both directions: gopherstack's 4-value switch
  (`cloud_formation.go`) exactly matches `types.Capability.Values()`
  (`CAPABILITY_IAM`/`CAPABILITY_NAMED_IAM`/`CAPABILITY_AUTO_EXPAND`/
  `CAPABILITY_RESOURCE_POLICY`) -- no missing value, no invented extra constant.
- Routing (Layer 1): all 13 ops' HTTP method + path template
  (`handler_sdk_route_table_test.go`'s `sdkRouteCases`, itself re-verified this pass
  against every `request.Method`/`httpbinding.SplitURI(...)` pair in `serializers.go`)
  match exactly, no collisions beyond the two already-disambiguated-by-method pairs
  (`GetApplication`/`UpdateApplication`/`DeleteApplication` on
  `/applications/{id}`; `GetApplicationPolicy`/`PutApplicationPolicy` on
  `/applications/{id}/policy`).

Provenance check: `last_audit_commit: e98f13133` → `git show -s --format=%ad e98f13133`
= `Fri Jul 24 12:09:14 2026 -0500`, matching `last_audit_date: 2026-07-24` exactly (same
day, no gap) -- the prior stamp was trustworthy, no false-audit correction needed.
Refreshed to current HEAD (`af89d3e6f`, 2026-08-20) above.

### This pass (2026-07-24)

Four real field-diff bugs were found and fixed, all wire-shape-vs-real-SDK (not
self-consistency) issues per `models.go`/handler request structs vs.
`aws-sdk-go-v2/service/serverlessapplicationrepository`'s generated `serializers.go`/`types`:

1. **`CreateApplication`/`CreateApplicationVersion`/`UpdateApplication` silently dropped the
   `licenseBody`/`readmeBody`/`templateBody` wire fields.** The real
   `CreateApplicationInput`/`CreateApplicationVersionInput`/`UpdateApplicationInput` types all
   serialize these as raw JSON string fields (confirmed in `serializers.go`'s
   `awsRestjson1_serializeOpDocument*Input` functions: `object.Key("licenseBody")`,
   `object.Key("readmeBody")`, `object.Key("templateBody")`) alongside their `*Url`
   counterparts, with the AWS doc stating "You can specify only one of X and Y; otherwise, an
   error results." gopherstack's request DTOs had no field to receive them at all, so a real
   SDK client passing `LicenseBody`/`ReadmeBody`/`TemplateBody` (as opposed to a pre-uploaded
   `*Url`) had that content silently discarded by `json.Unmarshal` (unknown JSON keys are
   ignored) -- the resulting application/version would end up with an empty `licenseUrl`/
   `readmeUrl`/`templateUrl` instead of the URL real AWS generates after uploading the body
   content to S3. Fixed by adding the three `*Body` fields to `createApplicationRequest`,
   `createApplicationVersionRequest`, and `updateApplicationRequest`; added mutual-exclusivity
   validation (`ErrValidation` / 400) matching the documented constraint; and added
   `synthesizeLicenseURL`/`synthesizeReadmeURL`/`synthesizeTemplateURL` (`models.go`) to
   produce a deterministic S3-style URL when only the `*Body` form is given, mirroring the
   emulation convention gopherstack already used for deriving `templateUrl` from a bare
   `sourceCodeUrl`.

2. **`ListApplicationVersions` summaries emitted an invented `resourcesSupported` field.** The
   real `VersionSummary` shape (`types.VersionSummary`) has exactly four fields --
   `applicationId`, `creationTime`, `semanticVersion`, `sourceCodeUrl` -- and does **not**
   include `resourcesSupported` (that field exists only on the full `Version` shape returned by
   `GetApplication`/`CreateApplication`/`CreateApplicationVersion`, which gopherstack still
   emits correctly). A prior pass identified this and left it in place because 3 existing tests
   asserted its presence -- exactly the "unit tests are not parity proof" trap the project's
   `parity-principles.md` calls out. Per this task's explicit invented-field rule, removed the
   key from `handleListApplicationVersions`'s summary map and rewrote
   `TestListApplicationVersions_ResourcesSupported` as
   `TestListApplicationVersions_SummaryShape`, which now asserts the field is **absent**.

3. **`CreateCloudFormationChangeSet`'s `templateId` was parsed but never forwarded to the
   backend**, so it was accepted on the wire yet had zero effect -- not even the "accepted
   without validation" behavior the prior audit's gap note described. Added `TemplateID` to
   `CreateCloudFormationChangeSetOptions`, wired it through from
   `handleCreateCloudFormationChangeSet`, and added real cross-validation in
   `CreateCloudFormationChangeSetWithOptions`: an unknown `templateId`, or one belonging to a
   different application, now rejects with `BadRequestException` (400) instead of being
   silently accepted. A later pass found the first cut of this cross-validation, and the
   pre-existing `applicationId`-not-found check in the same function, both returned
   `NotFoundException` (404) -- `CreateCloudFormationChangeSet`'s modelled error set
   (`deserializers.go` `awsRestjson1_deserializeOpErrorCreateCloudFormationChangeSet`) has no
   `NotFoundException`, only `BadRequestException`/`ForbiddenException`/
   `InternalServerErrorException`/`TooManyRequestsException`, so 404 was an error code this op
   can never actually emit. `CreateApplicationVersion` had the same bug for its
   `applicationId`-not-found check (its modelled set also has no `NotFoundException`); both are
   now `BadRequestException`.

4. **`ParameterDefinition` was missing 6 of 13 real fields** (`AllowedPattern`,
   `ConstraintDescription`, `MaxLength`, `MaxValue`, `MinLength`, `MinValue` vs.
   `types.ParameterDefinition`). Added them for full field-accuracy; functionally inert today
   since gopherstack never derives non-empty parameter definitions (that requires parsing an
   AWS SAM template body, out of scope), but the shape is now correct for any caller seeding
   state directly.

Carried forward from the prior pass (2026-07-13), still verified correct:

- `handler.go`'s default error branch emits `"InternalServerErrorException"` (not the
  fabricated `"InternalServerException"`), matching `types.InternalServerErrorException` and
  the `error` trait's `httpStatusCode: 500` in `api-2.json`. Regression test:
  `TestHandler_UnexpectedError_ReturnsInternalServerErrorException` in `handler_test.go`.
- `validPolicyActionsSet()`'s 8-action allow-list for `PutApplicationPolicy` matches AWS's
  published "Application Permissions" table exactly (`GetApplication`,
  `CreateCloudFormationChangeSet`, `CreateCloudFormationTemplate`, `ListApplicationVersions`,
  `ListApplicationDependencies`, `SearchApplications`, `Deploy`, `UnshareApplication`).

"Looks-wrong-but-correct" traps for the next auditor:
- The `AppName` field on `ApplicationVersion`/`CloudFormationTemplate`/`CloudFormationChangeSet`
  is `json:"-"` and exists purely to key/index the flattened `store.Table`s (see
  `store_setup.go`'s file doc comment) -- it is intentionally absent from wire responses.
- `RouteMatcher()` gates only on SigV4 service name + `/applications` path prefix (not
  method); `ExtractOperation()` is what actually derives the operation from method + path
  depth, and uses `URL.RawPath` (falling back to `URL.Path`) specifically so ARN-form
  application IDs containing a literal `/` (percent-encoded as `%2F`) route correctly --
  this is intentional, not a routing bug.
- `GetApplicationPolicy`/`ListApplicationDependencies`/`ListApplications` etc. all
  deliberately return non-nil empty slices/maps (never `null`) to match AWS always returning
  `[]`/`{}` for empty collections.
- `ParameterDefinition` (`models.go`) is field-complete against `types.ParameterDefinition` but
  is never populated with non-empty values by any code path -- gopherstack does not parse AWS
  SAM template bodies, so `ParameterDefinitions` on every `Version`/`ApplicationVersion` is
  always `[]`. This is intentional scope, not a stub: the field is still real and always
  present (never omitted) on the wire, exactly matching what AWS returns for an application
  with no template-declared parameters.
- `synthesizeLicenseURL`/`synthesizeReadmeURL`/`synthesizeTemplateURL` (`models.go`) produce
  deterministic, not random, S3-style URLs from the caller-supplied name/semanticVersion. Real
  AWS generates opaque, unpredictable S3 keys for uploaded `*Body` content; gopherstack's
  determinism is an intentional emulation simplification (stable, greppable URLs in tests/
  snapshots) and is not meant to byte-for-byte match a real AWS-generated URL.

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors

`Handler()`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.
serverlessrepo (SDK package name
`serverlessapplicationrepository`) is restjson1 (confirmed from
`serverlessapplicationrepository@v1.33.4` deserializers.go's
`awsRestjson1_deserializeOpError*` prefix); plain text doesn't decode
through `aws/protocol/restjson.GetErrorInfo`, so a real client got
`*json.SyntaxError`, not even `UnknownError`.

Fixed by routing the ReadBody error through this handler's own
`handleError(c, err)`: none of its typed `case`s (`awserr.ErrNotFound`,
`awserr.ErrConflict`, `awserr.ErrInvalidParameter`, `errInvalidRequest`,
`errUnknownAction`, syntax/type errors) match a `*http.MaxBytesError`/read
error, so it falls through to the pre-existing default -- already
documented in this file's own comment as matching the real
`InternalServerErrorException` `__type`
(`serverlessapplicationrepository@v1.33.4` `types/errors.go:105`).

Proven with a real `aws-sdk-go-v2/service/serverlessapplicationrepository`
client's `CreateApplication`, whose `Description` field alone exceeds
`httputils.MaxRequestBodyBytes` (16 MiB).
`TestHandler_OversizedBodySurfacesInternalServerErrorException`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalServerErrorException"`; confirmed it fails pre-fix with
`*json.SyntaxError` (hand-reverted, byte-identical restore after).

## Equality-matched-cursor restart sweep (2026-08-30)

All three paginated listings in this service (`ListApplications`,
`ListApplicationVersions`, `ListApplicationDependencies`) resumed a `nextToken` by
scanning for the item whose key equalled the token and left `start` at 0 on no match --
deleting the resource a cursor named (or a forged token) restarted pagination at page
one instead of truncating.

`ListApplications` (sorted by `Name`, `store.Table`'s own unique key -- see the
existing `ops:` note above) and `ListApplicationVersions` (sorted by `SemanticVersion`,
unique per app) are each sorted by exactly the field their own cursor carries, so both
were converted to a threshold search: resume at the first item whose key is strictly
greater than the token. `ListApplicationDependencies` is different: its collection is
sorted by `(ApplicationID, SemanticVersion)`, but the cursor carries only
`ApplicationID`, which is **not** unique within that sort (`collectDependencies`
dedupes only on the `ApplicationID+"@"+SemanticVersion` pair, so the same dependency
`ApplicationID` can legitimately appear at multiple semantic versions) -- a threshold
search on the bare `ApplicationID` would skip same-ID entries at other versions. Fixed
by defaulting an unresolved token to the end of the collection there instead.

Real AWS SAR has no per-version delete operation, so `ListApplicationVersions`'s
hostile test forges an unresolvable token; dependency entries are derived, not
independently deletable, so `ListApplicationDependencies`'s hostile test does the same.
`ListApplications` genuinely deletes the cursor's application mid-page
(`DeleteApplication` exists).

New tests (`handler_pagination_restart_test.go`, all confirmed failing pre-fix):
`TestListApplications_Pagination_DeletedMidPage`,
`TestListApplicationVersions_Pagination_StaleTokenDoesNotRestart`,
`TestListApplicationDependencies_Pagination_StaleTokenDoesNotRestart`. Prior pagination
coverage (`TestListApplications_Pagination_NextToken`,
`TestListApplicationVersions_PaginationNextToken`,
`TestListApplicationDependencies_Pagination`) only ever exercised the happy path where
every named cursor still resolves.

**Gates**: `go build ./services/serverlessrepo/...`, `go vet ./services/serverlessrepo/...`,
`go test -race -count=1 ./services/serverlessrepo/...` all pass; `golangci-lint run
./services/serverlessrepo/...` reports 0 issues.
