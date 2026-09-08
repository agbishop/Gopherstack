---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: mwaa
sdk_module: aws-sdk-go-v2/service/mwaa@v1.43.4   # version audited against (go.mod pins this)
last_audit_commit: b0509bb19   # HEAD when this pass finished; see 2026-09-04 Notes entry
last_audit_date: 2026-09-04
overall: A                # 2026-09-04: one real bug found and fixed (PublishMetrics fabricated
                           # ResourceNotFoundException, an error its own op-level deserializer
                           # switch doesn't recognize; see 2026-09-04 Notes entry). Prior pass's
                           # two bugs (AirflowVersion valid-value set, LoggingConfiguration
                           # request/response type conflation) remain fixed; rest of the 12-op
                           # surface re-verified clean against mwaa@v1.43.4
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateEnvironment: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-20: AirflowVersion valid-value set corrected (see Notes); LoggingConfiguration request decode now uses a real request-shaped LoggingConfigurationInput/ModuleLoggingConfigurationInput (no CloudWatchLogGroupArn member), converted into the response's LoggingConfiguration on write -- see Notes. Prior pass's fixes re-verified unchanged: NetworkConfiguration enforced required with SubnetIds==2/SecurityGroupIds 1-5; EnvironmentClass includes mw1.micro; WebserverAccessMode includes PUBLIC_AND_PRIVATE; WorkerReplacementStrategy correctly absent from Create; duplicate-name conflict ValidationException/400; mw1.micro webserver defaults/bounds"}
  GetEnvironment: {wire: ok, errors: ok, state: ok, persist: ok, note: "Environment field list re-diffed member-by-member against types.Environment (v1.43.4): all 34 fields match by name and shape, none missing, none fabricated. Environment response no longer echoes a fabricated top-level WorkerReplacementStrategy field (real Environment has no such member -- only LastUpdate.WorkerReplacementStrategy is real)"}
  UpdateEnvironment: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-20: same AirflowVersion and LoggingConfiguration fixes as CreateEnvironment apply here (validateUpdateEnums and the update path share validAirflowVersions()/convertLoggingConfiguration()). Prior pass's fixes re-verified unchanged: WorkerReplacementStrategy enum values FORCED/GRACEFUL; WebserverAccessMode includes PUBLIC_AND_PRIVATE; NetworkConfiguration wire-shape (UpdateNetworkConfigurationInput has no SubnetIds); mw1.micro webserver-count restriction using the effective EnvironmentClass; a rejected update no longer silently mutates the stored environment's other fields first"}
  DeleteEnvironment: {wire: ok, errors: ok, state: ok, persist: ok}
  ListEnvironments: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified MaxResults/NextToken are httpQuery-bound (not body) against serializers.go -- matches"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCliToken: {wire: ok, errors: ok, state: ok, persist: n/a, note: "re-verified CliToken/WebServerHostname field names against CreateCliTokenOutput -- matches"}
  CreateWebLoginToken: {wire: partial, errors: ok, state: ok, persist: n/a, note: "AirflowIdentity/IamIdentity response fields still not populated (see gaps -- re-investigated this pass, confirmed genuinely not derivable, not just an unwired accessor)"}
  InvokeRestApi: {wire: partial, errors: ok, state: ok, persist: n/a, note: "now enforces the environment must be AVAILABLE (ResourceNotFoundException otherwise), matching CreateCliToken/CreateWebLoginToken -- the mock previously let InvokeRestApi succeed against a CREATING/DELETING/etc environment whose Airflow webserver doesn't exist yet; response is still always a synthesized 200 for an AVAILABLE env regardless of Path (see gaps -- re-investigated this pass with botocore's service-2.json, see gap note for why this is more nuanced than a simple 404/405 miss)"}
  PublishMetrics: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04: fixed a fabricated error code -- not-found returned ResourceNotFoundException, which this op's own deserializer switch doesn't recognize; now ValidationException/400 (see Notes)"}
families:
  environment_lifecycle: {status: ok, note: "EnvironmentStatus constant fixed: gopherstack used the fabricated string \"UPDATE_ROLLING_BACK\" for a transient rollback state; the real aws-sdk-go-v2/service/mwaa/types.EnvironmentStatus enum value is \"ROLLING_BACK\". Also removed an entirely invented \"ERROR\" status (not in the real 12-value enum, was unused except in one test). CREATING/UPDATING/etc transiently promote to AVAILABLE on next GetEnvironment observation (promoteTransientStatus); this remains a deliberate mock simplification, not a stuck-forever bug"}
  errors: {status: ok, note: "error taxonomy unchanged from the prior pass (7 real exception types, confirmed again against types/errors.go); ErrEnvironmentAlreadyExists's Go error message text no longer contains the literal string \"AlreadyExistsException\" (it was leaking the fabricated exception name into the wire response's \"message\" field even though \"__type\" was already correctly ValidationException)"}
persistence: {status: ok, note: "Snapshot/Restore round-trips verified unaffected by this pass's field/validation changes (NetworkConfiguration, status constants, WorkerReplacementStrategy removal all covered by existing + new tests); no persistence.go edits were needed since none of the fixed fields had bespoke DTO mapping"}
gaps:
  - CreateWebLoginToken does not populate AirflowIdentity/IamIdentity (real AWS returns the calling IAM identity's username/ARN). Re-investigated end-to-end this pass rather than assuming the blocker (the codedeploy/mgn `siblingServices`/`GetXHandler()` pattern exists and *CLI already exposes GetSTSHandler()/GetIAMHandler() at cli.go:1043-1046). The actual missing piece is upstream of any mwaa-specific wiring: gopherstack has NO per-request caller-identity plumbing anywhere in the codebase. Confirmed by grepping every `ctxval.NewKey` call site (repo-wide): only two context keys exist at all, `pkgs/awsmeta` (Account/Region/Partition/RequestID -- no principal/ARN field) and `pkgs/logger`. `pkgs/httputils/sigv4.go`'s SigV4Validator parses the Authorization header's Credential (which contains the access-key-id) purely to verify the signature and explicitly discards it afterward -- its own doc comment states "the access-key-id in the request is informational only -- gopherstack is a single-tenant simulator". So even though `services/sts.GetCallerIdentity(accessKeyID, sessionToken)` exists and could resolve an access-key-id to an ARN, no access-key-id is ever threaded from the request into a handler's context anywhere in gopherstack today. Making IamIdentity real requires NEW cross-cutting plumbing (a context key carrying the parsed Credential access-key-id, populated by an Echo middleware, consumed via a services/mwaa/cross_service.go siblingServices accessor into STS) -- not a mwaa-local fix. AirflowIdentity is a second, independent gap on top of that: it requires mapping the resolved IAM principal to an Airflow RBAC username, which AWS derives from environment-specific IdP/role-mapping configuration that has no field anywhere in mwaa's Environment model (verified: models.go has no such member). Populating either field with a fabricated value would violate the no-fabricated-data rule, so both are left absent.
  - InvokeRestApi always synthesizes a 200 success with an empty RestApiResponse for any AVAILABLE environment, regardless of the caller-supplied Path/Method. Re-investigated this pass against botocore's mwaa/2020-07-01/service-2.json (not just the Go SDK): the operation's HTTP binding is `"responseCode": 200` for the success shape, so the AWS-transport-level 200 gopherstack already returns is not itself wrong -- real MWAA's InvokeRestApiOutput/RestApiClientException/RestApiServerException shapes ALL carry the same `RestApiStatusCode`/`RestApiResponse` pair (types/errors.go:94-150), meaning the *actual* downstream Airflow HTTP status (e.g. 404 for an unknown path, 405 for a wrong method) is meant to be surfaced as data inside that pair, not necessarily as a distinct SDK-visible exception -- and the SDK model does not document which of {success w/ non-2xx RestApiStatusCode, RestApiClientException, RestApiServerException} a given downstream failure maps to. Enumerating the real Apache Airflow REST API's actual path/method surface (which varies by AirflowVersion: /api/v1 for Airflow 2.x, /api/v2 for Airflow 3.x per the AWS user guide) to decide per-request which of those three shapes applies would mean inventing a route table gopherstack cannot verify -- exactly the fabrication class today's campaign already reverted once (an invented xray formula). Declined to implement path/method-based rejection for this reason; RestAPIStatusCode remains a fixed, documented mock simplification (see Notes) rather than a per-path guess.
  - MethodNotAllowedException (405) is used for HTTP-verb mismatches on matched MWAA path prefixes (e.g. GET /clitoken/{name}). This exception name is not part of the real MWAA API model, but the code path is unreachable by any conformant aws-sdk-go-v2 client (which always sends the correct verb per operation) -- and the same pattern is used consistently across 15+ other gopherstack services (apigatewayv2, pinpoint, lambda, opensearch, etc.), so it was left as-is rather than special-cased here.
deferred:
  - Chaos/fault-injection interaction with this pass's status-constant and NetworkConfiguration-validation changes (not re-audited; ChaosOperations() surface is GetSupportedOperations() minus nothing new -- it shrank by one entry this pass since GetMetrics was removed, see Notes).
leaks: {status: clean, note: "no goroutines/janitors in this service; existing leak_test.go/isolation_test.go untouched and still green"}
---

## Notes

**2026-08-15 (gopherstack-3gbe):** investigated whether MWAA shares Omics'
(gopherstack-keee) client-side host-prefix-rewrite reachability gap. It
does, and covers nearly this service's entire real surface: **12 of MWAA's
operations** carry a `req.URL.Host = "..." + req.URL.Host` rewrite from a
per-operation Smithy Finalize middleware, confirmed against the pinned
`mwaa@v1.43.4` module -- `api.` (8: CreateEnvironment
`api_op_CreateEnvironment.go:340`, GetEnvironment `:130`, DeleteEnvironment
`:126`, UpdateEnvironment `:312`, ListEnvironments `:219`, TagResource
`:135`, UntagResource `:133`, ListTagsForResource `:134`), `env.` (3:
CreateCliToken `api_op_CreateCliToken.go:134`, CreateWebLoginToken
`api_op_CreateWebLoginToken.go:142`, InvokeRestApi
`api_op_InvokeRestApi.go:159`), `ops.` (1: PublishMetrics
`api_op_PublishMetrics.go:140`) -- exactly matching gopherstack-3gbe's
filing (three literal prefixes using `.`, not `-`).

No routing/auth code needed changing. `Handler.RouteMatcher` (`handler.go:82`)
matches on `URL.Path` alone, gated on the SigV4 service name `"airflow"`
(already listed as SigV4-scoped and confirmed clean in
`services/_ROUTE_COLLISIONS.md`'s "hand-read this pass" section), and every
op already has a distinct path/method pair. The reachability gap is a pure
client-side DNS/dial failure, same as Omics -- confirmed live via
`host_prefix_reachability_test.go`'s before-fix test:
`dial tcp: lookup api.127.0.0.1 on 127.0.0.53:53: no such host`.

Before this pass, mwaa had **no real-SDK-client test at all** -- every
existing test drives the handler directly over a raw `httptest.Recorder`,
so the real-client reachability of this operation family had never been
exercised in either direction. Added
`host_prefix_reachability_test.go` following
`services/omics/host_prefix_reachability_test.go`'s before/after pattern
(real unmodified client fails to dial; a redial-to-the-real-listener
transport leaves the SDK's real, un-disabled rewrite intact on the wire and
the op succeeds with correctly decoded values), one representative op per
prefix. Gates green: build, vet, race, `go fix -diff` (no diff),
golangci-lint (0 findings; the one staticcheck SA1019 on the deliberately
deprecated-but-real `PublishMetrics` call is `//nolint:staticcheck`'d, same
convention as `services/directconnect/sdk_roundtrip_test.go`).

- **Protocol**: restjson1. Route prefixes unchanged from the prior pass, re-verified against
  aws-sdk-go-v2/service/mwaa@v1.43.4 serializers.go for every op: `/environments`
  (POST-less; GET=List), `/environments/{Name}` (GET/PUT/DELETE/PATCH =
  Get/Create/Delete/Update), `/clitoken/{Name}` (POST), `/webtoken/{Name}` (POST -- the
  real wire path is `/webtoken/`, NOT `/weblogintoken/` despite the operation being named
  CreateWebLoginToken), `/restapi/{Name}` (POST), `/tags/{ResourceArn}` (GET/POST/DELETE
  = List/Tag/Untag), `/metrics/environments/{EnvironmentName}` (POST=PublishMetrics; GET
  is intentionally unrouted, see the GetMetrics note below).

- **GetMetrics deleted from the wire surface** (was `GET /metrics/environments/{Name}`,
  advertised in `GetSupportedOperations()`/`ChaosOperations()` and dispatched by
  `handler.go`). Confirmed independently against
  aws-sdk-go-v2/service/mwaa@v1.43.4's exported `*mwaa.Client` methods: there is no
  `GetMetrics` method on the real SDK client at all -- only `PublishMetrics` exists on this
  path (documented "internal use only", used by the Airflow environment itself to push
  metrics to CloudWatch). The prior audit pass flagged this as an invented
  test-observability extension but left it wired up as if it were a real op, reasoning it
  was "harmless" since no real client would call it; that reasoning missed that
  `GetSupportedOperations()` feeds `ChaosOperations()` (presenting a fake op as
  fault-injectable) and is exactly the kind of drift `sdkcheck.CheckCompleteness` does NOT
  catch (it only verifies every *real* SDK method is accounted for, not that
  `GetSupportedOperations()` contains no extras). Fixed by removing the GET case from
  `extractMetricsOperation`/`dispatchMetrics` (GET now correctly falls through to
  `MethodNotAllowedException`/405, consistent with every other unsupported-verb-on-matched-path
  case in this handler) and deleting `handleGetMetrics`. The backend's
  `InMemoryBackend.GetMetrics` Go method is kept as internal, non-wire-exposed test
  introspection (tests assert `PublishMetrics`'s side effects by calling
  `h.Backend.GetMetrics(...)` directly, the same pattern as the `EnvironmentCount`/
  `MetricsCount` helpers in export_test.go) -- it is no longer presented as an AWS
  operation anywhere.

- **WorkerReplacementStrategy was fabricated on CreateEnvironment and on the Environment
  response shape.** Confirmed via aws-sdk-go-v2/service/mwaa@v1.43.4/api_op_CreateEnvironment.go's
  `CreateEnvironmentInput` struct (no `WorkerReplacementStrategy` member at all) and
  types/types.go's `Environment` struct (also no top-level `WorkerReplacementStrategy`
  member -- it exists ONLY on the nested `LastUpdate` struct, which real AWS uses to record
  just the most recent update call's setting, not a persistent environment-level value).
  gopherstack previously (a) accepted and validated `WorkerReplacementStrategy` in the
  Create request body, (b) copied it onto a fabricated top-level `Environment.WorkerReplacementStrategy`
  field on both Create and Update, and (c) emitted that fabricated field in every
  CreateEnvironment/GetEnvironment/UpdateEnvironment JSON response. Fixed by removing the
  field from `createEnvironmentRequest` and `Environment` entirely; it remains correctly
  present on `updateEnvironmentRequest` and `LastUpdate` (the only two real members).

- **WorkerReplacementStrategy's enum values were also wrong.** gopherstack accepted
  `FORCED`/`TERMINATION_WITH_DRAIN` and rejected `GRACEFUL`. The real
  `aws-sdk-go-v2/service/mwaa/types.WorkerReplacementStrategy` enum
  (types/enums.go) has exactly two values: `FORCED` and `GRACEFUL`.
  `TERMINATION_WITH_DRAIN` does not exist in the real API at all -- this was a double bug
  (accepting a fake value AND rejecting a real one). Fixed the constant and all test
  fixtures.

- **WebserverAccessMode was missing a real third value.** gopherstack's validator only
  accepted `PUBLIC_ONLY`/`PRIVATE_ONLY`. The real
  `aws-sdk-go-v2/service/mwaa/types.WebserverAccessMode` enum also has
  `PUBLIC_AND_PRIVATE`. This was a real functional bug (rejecting valid input, not just a
  permissive superset), fixed via a new shared `validateWebserverAccessMode` helper used by
  both Create and Update.

- **EnvironmentClass was missing `mw1.micro`.** gopherstack's `validEnvironmentClasses()`
  had small/medium/large/xlarge/2xlarge but not `mw1.micro`, which IS a documented valid
  value (aws-sdk-go-v2/service/mwaa@v1.43.4/types/types.go's EnvironmentClass field
  comment: "Valid values: mw1.micro, mw1.small, mw1.medium, mw1.large, mw1.xlarge, and
  mw1.2xlarge"). Fixed by adding it; see gaps for the still-unmodeled mw1.micro-specific
  webserver-count default.

- **EnvironmentStatus had a wrong value and an invented one.** gopherstack used
  `"UPDATE_ROLLING_BACK"` for the transient rollback status; the real
  `aws-sdk-go-v2/service/mwaa/types.EnvironmentStatus` enum value (types/enums.go) is
  `"ROLLING_BACK"` (no `UPDATE_` prefix). Also removed an `"ERROR"` status constant that
  does not exist anywhere in the real 12-value enum (`CREATING`, `CREATE_FAILED`,
  `AVAILABLE`, `UPDATING`, `DELETING`, `DELETED`, `UNAVAILABLE`, `UPDATE_FAILED`,
  `ROLLING_BACK`, `CREATING_SNAPSHOT`, `PENDING`, `MAINTENANCE`) and was unused except in
  one test's terminal-status list. `MAINTENANCE` remains unmodeled (gopherstack's mock
  never produces it, since there's no maintenance-window simulation) -- this is a
  pre-existing, low-risk simplification, not newly introduced.

- **NetworkConfiguration is now enforced required with real bounds on Create.**
  Confirmed via aws-sdk-go-v2/service/mwaa@v1.43.4/validators.go's generated
  `validateOpCreateEnvironmentInput` (client-side rejects a nil `NetworkConfiguration`
  before the request is even sent -- so real conformant clients can never omit it) and the
  live API docs for SubnetIds ("Fixed number: 2") / SecurityGroupIds ("1-5"), which have NO
  client-side validator (`validateNetworkConfigurationInput` does not exist in
  validators.go, unlike `validateUpdateNetworkConfigurationInput`) and so ARE genuinely
  reachable with a real client sending e.g. 1 subnet. The prior pass identified this gap
  but deferred it citing ~10+ tests relying on the lenient behavior; this pass did the
  test sweep (added a shared `testNetworkConfig()`/`newCreateReq()`/`seedEnv()`/
  `newIsoCreateReq()` fixture update covering ~80 call sites across both Go struct
  literals and HTTP JSON bodies) and landed the fix: `validateNetworkConfigCreate`
  requires non-nil, `len(SubnetIds) == 2`, `1 <= len(SecurityGroupIds) <= 5`.

- **InvokeRestApi now requires the environment to be AVAILABLE**, matching
  CreateCliToken/CreateWebLoginToken (the other two operations that reach into the
  environment's Airflow webserver process, which doesn't exist yet during
  CREATING/UPDATING/etc). Previously `InvokeRestAPI` only checked the environment existed,
  not its status, so it incorrectly succeeded against environments whose webserver isn't up.

- **The prior pass's standout bug** (UpdateEnvironment's NetworkConfiguration wire shape;
  see git history / prior manifest version) was re-verified unchanged and still correct
  this pass.

- **Error taxonomy**: unchanged from the prior pass -- MWAA's API model has exactly 7
  exception types (`AccessDeniedException`, `InternalServerException`,
  `ResourceNotFoundException`, `RestApiClientException`, `RestApiServerException`,
  `ServiceUnavailableException`, `ValidationException`), re-confirmed against
  `aws-sdk-go-v2/service/mwaa@v1.43.4/types/errors.go`. This pass additionally scrubbed
  `ErrEnvironmentAlreadyExists`'s Go error message text (previously
  `"AlreadyExistsException: environment already exists"`), which leaked the fabricated
  exception name into the wire response's `"message"` field even though `"__type"` was
  already correctly `ValidationException` -- now reads
  `"ValidationException: environment already exists"`.

- **mw1.micro's special-case MaxWebservers/MinWebservers default/bounds are now modeled.**
  `aws-sdk-go-v2/service/mwaa@v1.43.4/types/types.go`'s MaxWebservers/MinWebservers doc
  comments (identical text also in botocore's `mwaa/2020-07-01/service-2.json`, confirmed
  independently): "Valid values: For environments larger than mw1.micro, accepts values
  from 2 to 5. Defaults to 2 for all environment sizes except mw1.micro, which defaults to
  1." gopherstack previously applied the same default (2) and bounds (1-5) to every
  EnvironmentClass including mw1.micro -- more permissive than real AWS, which (per the
  quoted text explicitly scoping the 2-5 range to "environments larger than mw1.micro")
  only accepts 1 for that class. Fixed via a new `validateWebserversForClass` helper used
  by both CreateEnvironment (using the request's EnvironmentClass) and UpdateEnvironment
  (using the effective EnvironmentClass: the request's if it also changes class, else the
  persisted environment's). CreateEnvironment's default resolution now also picks 1 instead
  of 2 when EnvironmentClass resolves to mw1.micro and MaxWebservers/MinWebservers are
  unset. This does NOT model per-Environment reconfiguration edge cases beyond the
  documented default/range (e.g. AWS's exact behavior when downgrading EnvironmentClass to
  mw1.micro via Update while implicitly leaving a previously-set 2-5 value in place is not
  independently confirmed beyond "the effective class governs the check").

- **UpdateEnvironment no longer silently persists a rejected request's other fields.**
  `env` returned by the backend's `store.Table[Environment].Get` is the live stored
  pointer, not a copy. `UpdateEnvironment` previously called `applyUpdateScalars`/
  `applyUpdateS3Paths` (which mutate `env` in place) BEFORE checking
  `MinWorkers <= MaxWorkers`; a request that failed that check (or, before this pass, the
  mw1.micro webserver check) had already had its other fields (DagS3Path,
  ExecutionRoleArn, AirflowVersion, etc.) applied to the stored environment despite the API
  call returning an error to the caller -- a client retrying with valid worker counts would
  see fields it never successfully set. Fixed by computing the effective (post-update)
  MinWorkers/MaxWorkers from the request before any mutation and validating first;
  the mw1.micro webserver check added this pass follows the same pre-mutation-validation
  pattern.

**2026-08-20: wrapper-key/nested-shape wire-parity sweep.** Twelve ops
enumerated from `GetSupportedOperations()` and cross-checked against
`ls $(go env GOMODCACHE)/github.com/aws/aws-sdk-go-v2/service/mwaa@v1.43.4/api_op_*.go`
(9 files -- CreateCliToken, CreateEnvironment, CreateWebLoginToken,
DeleteEnvironment, GetEnvironment, InvokeRestApi, ListEnvironments,
ListTagsForResource, PublishMetrics, TagResource, UntagResource,
UpdateEnvironment -- matches). Protocol confirmed restjson1 from
`api_client.go`. Verified per-op, by reading the function BODY (not name/call
site) of every `awsRestjson1_deserializeOp<Op>` in deserializers.go: all 12
decode the response body flat at the top level via
`awsRestjson1_deserializeOpDocument<Op>Output`, and every one of those
`OpDocument` helpers is live (called directly, not dead code) and actually
performs a JSON decode into named fields -- no cnhp-trap variant found here
(no `(output, body, contentLength)` signature, no body-passthrough helper).

Two real bugs found and fixed:

1. **AirflowVersion valid-value set was wrong in both directions.**
   `validAirflowVersions()` (validation.go) accepted five stale values
   (`2.6.3`, `2.5.1`, `2.4.3`, `2.2.2`, `1.10.12`) that real AWS no longer
   offers, while rejecting three real, currently-offered values (`2.10.1`,
   `2.11.0`, `3.0.6`). Confirmed via the "Valid values: 2.7.2, 2.8.1, 2.9.2,
   2.10.1, 2.10.3, 2.11.0, and 3.0.6" doc comment repeated identically on
   `types.Environment.AirflowVersion` (types/types.go:41),
   `CreateEnvironmentInput.AirflowVersion` (api_op_CreateEnvironment.go:88),
   and `UpdateEnvironmentInput.AirflowVersion` (api_op_UpdateEnvironment.go:52).
   This is bug class (e), Right key wrong VALUE. Fixed the constant list;
   corrected `TestAirflowVersion_SupportedVersions`'s list (it had asserted
   success for the same five stale values gopherstack wrongly accepted) and
   added the five stale values plus one real one to
   `TestAirflowVersion_UnsupportedVersions`. `TestAirflowVersion_V1_SingleSchedulerOK`
   and `TestDefaults_SchedulersV1OnCreate` asserted `CreateEnvironment`
   succeeds with `AirflowVersion: "1.10.12"` -- that premise breaks once 1.x
   is correctly rejected, so both were corrected to expect rejection
   (renamed the first to `..._SingleSchedulerRejected`). This makes
   `validateSchedulers`'s `strings.HasPrefix(airflowVersion, "1.")` branch
   and `defaultSchedulersV1` genuinely unreachable via any currently-valid
   CreateEnvironment/UpdateEnvironment call -- left in place as harmless
   vestigial code (same category as the pre-existing unmodeled MAINTENANCE
   status) rather than removed, to keep this pass scoped to wire shape.
   Hand-revert (`cp` method) reproduced the exact symptom:
   `TestAirflowVersion_SupportedVersions` failed on 2.10.1/2.11.0/3.0.6 and
   `TestAirflowVersion_UnsupportedVersions` failed on 1.10.12/2.6.3 with the
   old list restored.

2. **LoggingConfiguration was used for BOTH request decode and response
   encode**, sharing one Go type (`ModuleLoggingConfiguration`, with a
   `CloudWatchLogGroupArn` field) across `createEnvironmentRequest`/
   `updateEnvironmentRequest` and the `Environment` response. Real AWS models
   these as two distinct shapes: `LoggingConfiguration`/
   `ModuleLoggingConfiguration` (response, HAS `CloudWatchLogGroupArn`,
   types/types.go:407-422) vs `LoggingConfigurationInput`/
   `ModuleLoggingConfigurationInput` (request, NO `CloudWatchLogGroupArn`,
   types/types.go:290-330) -- `CloudWatchLogGroupArn` is server-computed once
   a module's logs are enabled, never client-supplied. Grepped for readers
   before calling this cosmetic: `buildEnvironment` and `UpdateEnvironment`
   both did `env.LoggingConfiguration = req.LoggingConfiguration` verbatim,
   so a request body setting `CloudWatchLogGroupArn` was decoded, persisted,
   and echoed straight back in the response as if AWS-generated -- a
   response-only field leaking through the request path (the mirror of the
   request-only-field-in-a-response shape called out elsewhere this
   campaign). No real conformant SDK client can trigger this (its generated
   request marshaler has no such field to send), but gopherstack simulates
   arbitrary HTTP clients, not just the official SDK, so the request wire
   shape itself was inaccurate regardless of client conformance. Fixed by
   adding real `LoggingConfigurationInput`/`ModuleLoggingConfigurationInput`
   types (models.go) used only by the two request structs, and a
   `convertLoggingConfiguration`/`convertModuleLoggingConfiguration` mapping
   step (environments.go) that intentionally never sets
   `CloudWatchLogGroupArn` -- it stays an honest, disclosed gap (gopherstack
   never computes it) rather than a spoofable field. This is now a
   compile-time-enforced separation: hand-reverting `buildEnvironment` to
   assign `req.LoggingConfiguration` (the Input type) directly to
   `Environment.LoggingConfiguration` (the response type) fails to build
   (`cannot use req.LoggingConfiguration (variable of type
   *LoggingConfigurationInput) as *LoggingConfiguration value in struct
   literal`) -- a stronger proof than a runtime test. Also added
   `TestHTTP_LoggingConfig_CloudWatchLogGroupArn_NotEchoedFromRequest`
   (handler_environments_test.go), which POSTs a `CloudWatchLogGroupArn` in
   the request body and asserts it comes back empty in the GetEnvironment
   response, as a wire-level (not just Go-type-level) regression guard for
   any future handler that bypasses the typed request struct. Updated ~30
   struct-literal call sites in `environments_config_test.go` and
   `store_test.go` from `mwaa.LoggingConfiguration{...}` /
   `mwaa.ModuleLoggingConfiguration{...}` to the `...Input` variants (none of
   them set `CloudWatchLogGroupArn`, so this was a pure rename, no assertion
   changes); `handler_environments_test.go`/`handler_tags_test.go` construct
   requests as raw `map[string]any` JSON bodies and needed no changes.

Families re-verified CLEAN, no changes: `Environment` full 34-field diff
against `types.Environment` (see GetEnvironment note above) -- no missing, no
fabricated members. `ListEnvironments` returns bare `Environments []string` +
`NextToken` (handler_environments.go's `handleListEnvironments` builds
`map[string]any{"Environments": names}` from a `[]string`) -- confirmed
against `ListEnvironmentsOutput.Environments []string`
(api_op_ListEnvironments.go:45); gopherstack does not return objects there.
`CreateEnvironmentOutput`/`UpdateEnvironmentOutput` each have exactly one
member, `Arn *string` -- confirmed `writeEnvironmentResult` (handler.go:349)
returns only `{"Arn": env.ARN}`, not a full Environment (this looked like a
plausible wrapper-key bug going in; it is not). `CreateCliTokenOutput`
(`CliToken`, `WebServerHostname`) and `CreateWebLoginTokenOutput`
(`AirflowIdentity`, `IamIdentity`, `WebServerHostname`, `WebToken`) field
names re-verified byte-for-byte against the deserializer's `case` keys;
gopherstack's handler_cli_token.go/handler_web_login_token.go match, and the
still-unpopulated `AirflowIdentity`/`IamIdentity` gap (documented above) was
re-confirmed rather than re-derived. `InvokeRestApiOutput`
(`RestApiResponse` as `document.Interface`, `RestApiStatusCode *int32`)
matches gopherstack's `InvokeRestAPIResponse{RestAPIResponse any,
RestAPIStatusCode int32}`. Error taxonomy (7 exception types) unchanged, all
present in types/errors.go. `UntagResource`'s `tagKeys` query param name
(lowercase) matches serializers.go:1039. Enums checked both directions
against types/enums.go: `EndpointManagement` (CUSTOMER/SERVICE),
`WebserverAccessMode` (PRIVATE_ONLY/PUBLIC_ONLY/PUBLIC_AND_PRIVATE),
`WorkerReplacementStrategy` (FORCED/GRACEFUL), `LoggingLevel`
(CRITICAL/ERROR/WARNING/INFO/DEBUG), `EnvironmentStatus` (12 values),
`UpdateStatus` (SUCCESS/PENDING/FAILED) -- all match gopherstack's accepted
sets exactly; only `AirflowVersion`'s valid-value set (not a generated Go
enum, but an equally-authoritative documented discrete set) was wrong, see
bug 1 above. `Unit` is internal-only (PublishMetrics's deprecated
`MetricDatum.Unit`) and not independently re-verified this pass (26-value
list, unchanged from prior audits, low risk).

**Provenance finding.** The manifest's stamp
(`last_audit_commit: e15f163e+uncommitted`, `last_audit_date: 2026-07-23`)
had NOT advanced across two later, substantive passes that both touched this
exact file's content: `366717981` (2026-08-10, "fix(mq,mwaa): validate tag
targets that exist, and stop writing rejected updates" -- bumped
`sdk_module` from v1.40.1 to v1.43.4, added the mw1.micro
webserver-default/bounds fix, rewrote several `ops`/`gaps` notes) and
`d39bf33e4` (2026-08-11, "Chore/parity upgrade (#2414)" -- touched
`environments.go`, `store.go`, `tags.go`, `validation.go`,
`environments_micro_webservers_test.go`,
`environments_validation_test.go`). Both commits' diffs modify PARITY.md's
`ops`/`sdk_module` lines directly while leaving `last_audit_commit`/
`last_audit_date` completely untouched (confirmed via `git show <sha> --
services/mwaa/PARITY.md`) -- this is not the "commit merely touches the
directory" false-positive pattern the campaign warned about (four prior
false accusations); it is the stamp's own tracked fields failing to move
across edits to the very content they attest to. Separately,
`e15f163e` itself (2026-07-13) predates `last_audit_date` (2026-07-23) by
ten days, but that gap has an innocent explanation: `27f63288f`
("fix(mwaa): delete invented op/field, fix enums, require network config"),
the commit whose diff matches the 2026-07-23 manifest content verbatim, is
timestamped 2026-07-23T19:16:10 -- exactly the recorded date, consistent
with a branch opened at `e15f163e` and merged ten days later. Verdict: the
originally-stamped date was accurate for the pass it described; the stamp
simply stopped advancing afterward. This pass's fixes bring `sdk_module`,
`last_audit_commit`, and `last_audit_date` back in sync with actual HEAD.

### 2026-08-29: independent re-sweep, GENUINELY CLEAN (gopherstack-6flj/21my)

No code changes since `last_audit_commit`; `git log d5aaf8e79..HEAD --
services/mwaa/` shows only the already-recorded 2026-08-20
wrapper-key/AirflowVersion/LoggingConfiguration sweep and an unrelated
IAM-enforcement test addition. Re-derived member lists directly from
`types/types.go`/`api_op_*.go` rather than trusting the prior manifest's
counts, and checked write-only state both directions:

- **N of N member coverage, independently re-counted**: `Environment`
  27/27 real fields on gopherstack's struct match 34/34 wire-serialized
  members on the real `types.Environment` (the delta is `Environment`'s
  unexported `region` field, which carries no json tag and is never
  serialized -- not a wire gap); `CreateEnvironmentInput` 25/25 (24 body
  fields + `Name` bound from the path); `UpdateEnvironmentInput` 23/23 (22
  body fields + `Name` from the path, `KmsKey`/`EndpointManagement`
  correctly absent since the real `UpdateEnvironmentInput` has no such
  members).
- **FORWARD (accept-and-drop)**: re-read `createEnvironmentRequest`/
  `updateEnvironmentRequest` against `buildEnvironment`/
  `applyUpdateScalars`/`applyUpdateS3Paths` field-by-field; every accepted
  field is either stored on `Environment` or is real request-only
  plumbing with no response counterpart (none found this pass).
  `invokeRestAPIRequest.Body`/`.QueryParameters` are decoded and never
  read by `InvokeRestAPI` -- already investigated and disclosed as a gap
  (a per-path Airflow route table gopherstack cannot fabricate), not a
  fresh finding.
- **REVERSE (computable-but-unemitted)**: no stored field found without a
  reader; `LastUpdate.WorkerReplacementStrategy`,
  `NetworkConfiguration.SecurityGroupIds` (update-merge path),
  `LoggingConfiguration` (via `convertLoggingConfiguration`) all round-trip
  through `GetEnvironment`'s direct struct marshal.
- **Enums**: `EndpointManagement`, `EnvironmentStatus` (12 values, `MAINTENANCE`
  disclosed unmodeled), `WebserverAccessMode`, `WorkerReplacementStrategy`,
  `RestApiMethod`/`validRestAPIMethods()`, `LoggingLevel` all re-diffed
  against `types/enums.go` field-for-field; no invented or missing values
  found.
- Tools: `enumcheck` run repo-wide, zero findings for `services/mwaa/`.
  `go build`, `go vet ./...` (repo-wide), `go test -race -count=1
  ./services/mwaa/...`, `golangci-lint run ./services/mwaa/...` all clean,
  0 issues.

Verdict: no bugs found this pass. This is the second independent
confirmation (after 2026-08-20's sweep) that this service's wire shape is
correct in both directions.

### 2026-09-04: per-op error-switch audit (gopherstack-0h1)

Went beyond the prior passes' service-wide error-taxonomy check (7 exception
types exist) to verify, per op, that its own
`awsRestjson1_deserializeOpError<Op>` switch (deserializers.go) actually
recognizes every code gopherstack returns for it -- the prior passes checked
that a code was a real MWAA exception, not that the specific op's generated
client can decode it as one.

Found one mismatch: **`PublishMetrics` returned `ResourceNotFoundException`
(404) for an unknown environment, but
`awsRestjson1_deserializeOpErrorPublishMetrics` only has cases for
`InternalServerException` and `ValidationException`** (confirmed by reading
the function body directly, deserializers.go:1335-1425) -- unlike every
other not-found-capable op in this service (`GetEnvironment`,
`DeleteEnvironment`, `UpdateEnvironment`, `CreateWebLoginToken`,
`CreateCliToken`, `InvokeRestApi`, `ListTagsForResource`, `TagResource`,
`UntagResource`), all of which do carry a `ResourceNotFoundException` case.
Fixed `handler_metrics.go`'s `handlePublishMetrics` to map
`awserr.ErrNotFound` to `ValidationException`/400 instead, the same
"op doesn't model this exception" precedent already established for
`ErrEnvironmentAlreadyExists` in `writeEnvironmentResult`. Updated
`TestHandler_PublishMetrics`'s `env_not_found` case to expect 400, and added
`TestHandler_PublishMetrics_NotFound_ErrorType` asserting the wire `__type`
is `ValidationException` (a bare status-code check wouldn't have pinned the
exact `__type` string). Verified both tests fail against the unfixed code
(400/`ResourceNotFoundException` reverted to 404): `TestHandler_PublishMetrics/env_not_found`
and `TestHandler_PublishMetrics_NotFound_ErrorType` both fail with
`Not equal: expected: 400, actual: 404`.

All other ops re-checked clean: every `ResourceNotFoundException`,
`ValidationException`, `AccessDeniedException`,
`RestApiClientException`/`RestApiServerException` gopherstack currently
returns for an op is present in that same op's deserializer switch.
`CreateEnvironment`/`ListEnvironments` correctly never emit
`ResourceNotFoundException` (their switches don't have it either).

Also re-examined (no changes, findings below):
- **DeleteEnvironment precondition**: doc comment is exactly "Deletes an
  Amazon Managed Workflows for Apache Airflow (Amazon MWAA) environment." --
  no status precondition documented anywhere (api_op_DeleteEnvironment.go).
  `InMemoryBackend.DeleteEnvironment` (environments.go) deletes unconditionally
  regardless of `Status` (CREATING/UPDATING/etc included). Left as-is: the SDK
  is silent, so restricting delete-while-transitioning would be inventing a
  rule, not fixing a documented one -- cannot be determined from the SDK.
- **Ghost rows after delete**: not applicable here. Tags live inline on
  `Environment.Tags` (deleted with the row); `metrics` (store.go) is a plain
  `map[region]map[envName][]MetricDatum` and `DeleteEnvironment` does
  `delete(b.metricsStore(region), name)`; CLI/web-login tokens are stateless
  (`generateMWAAToken`, no side table at all). No hand-rolled map survives a
  delete-and-recreate under the same name.
- **InvokeRestApi's fixed 200/empty response regardless of Path/Method**:
  already investigated and disclosed in `gaps` (re-read, reasoning still
  holds -- the SDK model doesn't document which downstream Airflow HTTP
  status maps to which of {200 w/ non-2xx `RestApiStatusCode`,
  `RestApiClientException`, `RestApiServerException`}, and Airflow's own REST
  surface varies by `AirflowVersion` -- not re-touched this pass).

Gates: `go build ./services/mwaa/...`, `go test -race -count=1
./services/mwaa/...` (ok), `golangci-lint run ./services/mwaa/...` (0
issues), `go test -race -count=1 ./services/cloudformation/...` (ok,
dependent package).

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `RestApi`/`RestAPI` acronym
casing gives it 1 op/handler pair needing the ambiguous fold, a genuine
collision between an exported backend method and the real unexported
handler: `InvokeRestApi`.

Verified directly: ran the unpatched tool from `ef0eef041~1` five times and
diffed against the fixed tool at HEAD. `cmd/reqfieldscan` was byte-identical
across all 5 runs and HEAD -- zero damage. `cmd/reqfielddiff` was not: old
runs found 11 or 15 findings vs 11 at HEAD, with 4 fields flickering, all
only in old (misresolved) runs, never at HEAD: `InvokeRestApi.{Body, Method,
Path, QueryParameters}`. Read the source (handler_rest_api.go:14-26):
`invokeRestAPIRequest` is `json.Unmarshal`'d (a recognized decode verb) and
forwarded whole to `h.Backend.InvokeRestAPI`. Confirmed genuine -- not a bug.

Verdict: zero real bugs, safe direction only.

## 2026-09-08: writeErrorResponse nil-on-write fall-through audit (gopherstack-246v) -- clean

Part of the sweep following the elasticache fix (gopherstack-8haq): `writeErrorResponse`
(`handler.go:399`) writes the JSON error body and unconditionally `return nil`s, so any
helper that rejects via `return writeErrorResponse(...)` and is called by code doing
`err := helper(...); if err != nil { ... }` would get a silent `nil` back and fall through
past the rejection.

**Method (mechanical).** A `go/parser`/`go/ast` script over every non-test `.go` file in
this flat package (mwaa has no subdirectories) computed the fixed-point closure of every
function whose body contains a bare `return writeErrorResponse(...)`: seed the sink set
with `writeErrorResponse` itself, find every function with a direct `return <sink>(...)`,
add its name to the sink set, repeat until no growth. This closure doubles as the
dispatch-vs-non-dispatch cross-reference the issue asks for: `ServeHTTP` (the function
wired as `h.Handler()`, registered directly with echo) and all seven `dispatchXxx`
functions it calls fall into the set naturally, because each ends in a
`return writeErrorResponse(...)` default case -- so their own call sites got checked
alongside the rest, no separate partition needed.

The closure converged at 23 functions (`writeErrorResponse` plus 22 discovered wrappers:
`ServeHTTP`, the 7 `dispatchXxx` routers, and 14 `handleXxx`/`writeEnvironmentResult`/
`writeEnvironmentVoidResult` handlers). Every call site of every function in that set was
then re-walked and classified: 65 total call sites, of which 63 are `return <fn>(...)`
(direct, safe -- includes `ServeHTTP` itself, since `Handler()` just returns it, and echo
consumes its result directly). The remaining 2 (`handler.go:315,321`, inside
`decodeJSONBody`) store `writeErrorResponse`'s result but explicitly discard it
(`_ = writeErrorResponse(...)`) and signal via a `bool` return, not the discarded `error`
-- `decodeJSONBody`'s two callers (`handleCreateEnvironment`, `handleUpdateEnvironment`,
`handler_environments.go:16,50`) correctly check `if !decodeJSONBody(...) { return nil }`
before touching the backend. This is the same "checked-bool-helper" shape as elasticache's
`parsePaginationChecked`, just using `bool` instead of a sentinel error, and it is correct.

**No instance of the broken shape exists in mwaa.** No code changed as a result. Gates:
`GOTOOLCHAIN=go1.27.0 golangci-lint run ./services/mwaa/...` 0 issues;
`GOTOOLCHAIN=go1.27.0 go test -race ./services/mwaa/...` ok.
