---
service: apprunner
sdk_module: aws-sdk-go-v2/service/apprunner@v1.42.4
last_audit_commit:                                # unknown: pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-09-03
overall: A            # full field-diff sweep: closed every gaps/deferred item from the 2026-07-13 audit,
                       # plus the wrapper-key/nested-shape sweep (2026-08-19, one fabricated-field bug fixed);
                       # 2026-08-23: closed the four member-never-emitted items disclosed 2026-08-19 (see Notes)
ops:
  CreateService: {wire: fixed, errors: ok, state: ok, persist: ok, note: "immediate RUNNING (no OPERATION_IN_PROGRESS poll-forever trap); full field set now threaded: InstanceConfiguration (Cpu/Memory/InstanceRoleArn), SourceConfiguration (ImageRepository incl. ImageConfiguration, CodeRepository incl. SourceCodeVersion/CodeConfiguration, AuthenticationConfiguration, AutoDeploymentsEnabled with real default), AutoScalingConfigurationArn (resolved-or-default, HasAssociatedService bookkeeping), NetworkConfiguration (Egress/IngressConfiguration, IpAddressType, real defaults), HealthCheckConfiguration (real defaults), EncryptionConfiguration, ObservabilityConfiguration. Service response now includes the previously-missing required AutoScalingConfigurationSummary and NetworkConfiguration fields. FIXED 2026-08-21 (bd gopherstack-r80d, batch 10; fixed but NOT counted -- see Notes): validateSourceConfig checked CodeRepository.RepositoryUrl but never SourceCodeVersion (types.go:245-263, both required on CodeRepository) -- an omitted SourceCodeVersion was silently accepted and then dropped from codeRepositoryOutput entirely. Added the same required-field check already used for RepositoryUrl/ImageIdentifier. Not counted: the real aws-sdk-go-v2 client's own generated validateCodeRepository (validators.go:792-806) already rejects a nil SourceCodeVersion client-side, so no real Go SDK client can ever reach gopherstack in the buggy state -- proven instead via a raw request bypassing that client-side check, which is real for any other caller (raw HTTP, a non-Go SDK) but not provable via this campaign's real-SDK-client round-trip standard."}
  DescribeService: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateService: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects update unless status RUNNING, matches InvalidStateException; rejects switching between image/code source types (InvalidRequestException, matching the real op's documented restriction); all new CreateService fields are independently patchable (nil/empty = no change)"}
  DeleteService: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "now cascade-cleans the service's customDomains map entry and recomputes the old AutoScalingConfiguration's HasAssociatedService (see leaks). FIXED 2026-08-23: Service.DeletedAt (deserializers.go:6615) was entirely absent from storedService and Service -- added the field, set on DeleteService before the row is evicted from the store, emitted as an omitempty pointer (only DeleteService's own response can ever observe it, since ListServices/DescribeService can no longer see the service after eviction). FIXED 2026-09-03 (gopherstack-9vv): now rejects (InvalidStateException) deleting a service with an active VpcIngressConnection still referencing it, matching the op's own doc sentence -- see Notes."}
  ListServices: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23: ServiceSummary.UpdatedAt (deserializers.go:6939) was omitted from the wire struct even though storedService.UpdatedAt was already tracked and current -- emit-only fix, no backend logic change."}
  PauseService: {wire: ok, errors: ok, state: ok, persist: ok}
  ResumeService: {wire: ok, errors: ok, state: ok, persist: ok}
  StartDeployment: {wire: ok, errors: fixed, state: ok, persist: ok, note: "records a real operation; completes immediately (SUCCEEDED) rather than modeling OPERATION_IN_PROGRESS. FIXED 2026-08-23 (gopherstack-wlo1): a non-running service reported InvalidStateException, a code StartDeployment's own deserializeOpError switch cannot type (only InternalServiceErrorException/InvalidRequestException/ResourceNotFoundException are modeled for this op, unlike UpdateService/PauseService/ResumeService which do model InvalidStateException) -- now reports InvalidRequestException."}
  ListOperations: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed -- OperationSummary now includes UpdatedAt (set equal to StartedAt/EndedAt since operations complete immediately in this backend's simplified state machine)"}
  CreateAutoScalingConfiguration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23: AutoScalingConfiguration.Latest (deserializers.go:4692) and .DeletedAt (deserializers.go:4660) were both untracked/unemitted. Latest is now computed the same way ObservabilityConfiguration.Latest already was -- b.asgByName[name] tracks revisions in creation order; the new revision flips the prior last entry's Latest to false and sets its own to true. DeletedAt was already tracked on storedAutoScalingConfiguration/AutoScalingConfiguration (DeleteAutoScalingConfiguration already set it) but never surfaced on the wire; emit-only fix, omitempty pointer."}
  DescribeAutoScalingConfiguration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same Latest/DeletedAt fix as CreateAutoScalingConfiguration."}
  DeleteAutoScalingConfiguration: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "same Latest/DeletedAt fix as CreateAutoScalingConfiguration; on delete, the remaining highest-revision sibling (if any) gets Latest promoted to true, mirroring DeleteObservabilityConfiguration's existing convention. FIXED 2026-09-03 (gopherstack-9vv): now rejects (InvalidRequestException) deleting the account default configuration or one still associated with a service, using the cfg.IsDefault/HasAssociatedService fields this backend already tracked but never checked on delete -- see Notes."}
  ListAutoScalingConfigurations: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed -- summary now includes real HasAssociatedService, recomputed from live CreateService/UpdateService/DeleteService association state"}
  UpdateDefaultAutoScalingConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  ListServicesForAutoScalingConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed -- now returns real associated service ARNs; CreateService threads AutoScalingConfigurationArn (explicit, name-only-ARN, or the account's always-present seeded default) into a real association tracked on every service"}
  CreateConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConnection: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-09-03 (gopherstack-9vv): now rejects (InvalidRequestException) deleting a connection still referenced by a service's SourceConfiguration.AuthenticationConfiguration.ConnectionArn, matching the op's own doc sentence -- see Notes."}
  ListConnections: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateObservabilityConfiguration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (bd gopherstack-r80d, batch 10; fixed but NOT counted toward the required-field tally -- TraceConfiguration itself is optional per types.go:601, only its nested Vendor is required-when-present): TracingVendor was captured from CreateObservabilityConfigurationInput and stored, but observabilityConfigurationOutput had no TraceConfiguration field at all, so it was silently dropped on every response. Added, present only when TracingVendor != \"\" (real AWS: absent means tracing isn't enabled -- not fabricating a vendor when none was configured)."}
  DescribeObservabilityConfiguration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (bd gopherstack-r80d, batch 10): same TraceConfiguration gap and fix as CreateObservabilityConfiguration above."}
  DeleteObservabilityConfiguration: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-09-03 (gopherstack-9vv): now rejects (InvalidRequestException) deleting a configuration still enabled on a service, matching the op's own doc sentence -- see Notes."}
  ListObservabilityConfigurations: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2026-08-19 -- summary entries were emitting fabricated Status/Latest/CreatedAt keys that have no case in the real types.ObservabilityConfigurationSummary document deserializer (deserializers.go:6215-6270); a real client would silently drop them. Now emits only ObservabilityConfigurationArn/Name/Revision, matching the narrower summary type exactly (types/types.go:613-628)"}
  CreateVpcConnector: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23: VpcConnector.DeletedAt (deserializers.go:7299) was already tracked on storedVpcConnector/VpcConnector (DeleteVpcConnector already set it) but never surfaced on the wire; emit-only fix, omitempty pointer."}
  DescribeVpcConnector: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same DeletedAt fix as CreateVpcConnector."}
  DeleteVpcConnector: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "same DeletedAt fix as CreateVpcConnector. FIXED 2026-09-03 (gopherstack-9vv): now rejects (InvalidRequestException) deleting a connector still referenced by a service's NetworkConfiguration.EgressConfiguration.VpcConnectorArn, matching the op's own doc sentence -- see Notes."}
  ListVpcConnectors: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateVpcIngressConnection: {wire: ok, errors: ok, state: partial, persist: ok, note: "doesn't validate ServiceArn refers to an existing service (dangling ref allowed); matches real op's documented error set which has no ResourceNotFoundException, so not a wire bug -- see gaps"}
  DescribeVpcIngressConnection: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23: VpcIngressConnection.DeletedAt (deserializers.go:7547) was already tracked on storedVpcIngressConnection/VpcIngressConnection (DeleteVpcIngressConnection already set it) but never surfaced on the wire; emit-only fix, omitempty pointer."}
  DeleteVpcIngressConnection: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same DeletedAt fix as DescribeVpcIngressConnection."}
  ListVpcIngressConnections: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateVpcIngressConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateCustomDomain: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed to use InvalidRequestException (not ResourceNotFoundException) for unknown ServiceArn, matching this op's documented error set. FIXED 2026-08-21 (bd gopherstack-r80d, batch 10): required vpcDNSTargets (api_op_AssociateCustomDomain.go, required; deserializers.go:7705-7763) had no struct field on associateCustomDomainOutput at all -- DescribeCustomDomains (identical required set) already emitted it correctly as []. Added, always []any{} (this backend doesn't model per-domain VPC ingress DNS targets, so empty is the honest value, not fabricated). Originally logged 2026-08-19 as a separate duplicate entry describing the same fix; merged 2026-08-23 (gopherstack-fg0u)."}
  DisassociateCustomDomain: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (bd gopherstack-r80d, batch 10): same vpcDNSTargets gap and fix as AssociateCustomDomain above (deserializers.go:8462-8520). Originally logged 2026-08-19 as a separate duplicate entry describing the same fix; merged 2026-08-23 (gopherstack-fg0u)."}
  DescribeCustomDomains: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  error_taxonomy: {status: ok, note: "was systemically broken across all 35 ops -- see Notes; fixed 2026-07-13"}
gaps:
  - "CreateVpcIngressConnection doesn't validate that ServiceArn refers to an existing service, allowing a dangling reference. Left as-is because CreateVpcIngressConnection's documented error set has no ResourceNotFoundException -- adding validation would need a new InvalidRequestException-mapped check, not a NotFound one, to stay wire-correct; low traffic op, deferred. Re-verified 2026-07-23: still the correct call, not a bug."
  - "2026-08-19 (Layer 3, disclosed not fixed): CLOSED 2026-08-23 -- ListServices's ServiceSummary.UpdatedAt (deserializers.go:6939, emit-only: storedService.UpdatedAt was already tracked and current), Service.DeletedAt (deserializers.go:6615, needed a new storedService field plus a DeleteService write since real AWS keeps returning it on the DeleteService response even though this backend evicts the row from the store immediately after), VpcConnector.DeletedAt (deserializers.go:7299, emit-only) and VpcIngressConnection.DeletedAt (deserializers.go:7547, emit-only) all now round-trip through a real aws-sdk-go-v2 client. AutoScalingConfiguration.Latest (deserializers.go:4692) is now computed the same way ObservabilityConfiguration.Latest already was, via a b.asgByName[name] revision-order list; AutoScalingConfiguration.DeletedAt (deserializers.go:4660, emit-only -- also already tracked internally) closed alongside it. See ops table above for per-op detail. Still open: CustomDomain omits CertificateValidationRecords (deserializers.go:4899, 5381), a genuine backend gap since no cert validation flow is modeled -- not touched, no internal tracking exists to surface."
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this backend; existing leak_test.go covers handler/backend lifecycle. 2026-07-23: found and fixed one real leak -- DeleteService left its b.customDomains[serviceArn] entry behind forever (unreachable once the service is gone, since DescribeCustomDomains 404s on a deleted ServiceArn); now cascade-deleted, covered by TestDeleteService_CascadesCustomDomains. New AutoScalingConfiguration HasAssociatedService bookkeeping (CreateService/UpdateService/DeleteService) stays entirely inside the existing b.mu critical sections, no new lock paths or goroutines introduced."}
---

## Notes

**Fixed: systemic wrong exception-type names (the real bug this sweep found).** App Runner's
error model (`aws-sdk-go-v2/service/apprunner/types/errors.go`) has exactly five exception
types: `InternalServiceErrorException`, `InvalidRequestException`, `InvalidStateException`,
`ResourceNotFoundException`, `ServiceQuotaExceededException`. Before this fix, gopherstack's
`handleError` (handler.go) and the backend's sentinel wrappers (backend.go) used three
different, *wrong* type strings:

- Not-found errors (`awserr.ErrNotFound`) returned `__type: "InvalidParameterException"` --
  this exception **does not exist** anywhere in App Runner's model. Every
  Describe/Delete/Update/Pause/Resume/Tag/List op that can 404 was affected; the real SDK
  deserializer (`awsAwsjson10_deserializeOpError*` in deserializers.go) only recognizes
  `ResourceNotFoundException` for these ops, so a real client would get an untyped
  `smithy.GenericAPIError` instead of `types.ResourceNotFoundException`, breaking any
  `errors.As(err, &types.ResourceNotFoundException{})` check callers rely on.
- Already-exists conflicts (`awserr.ErrAlreadyExists`) returned
  `__type: "ServiceQuotaExceededException"`. That type IS valid for CreateService /
  CreateConnection / CreateVpcIngressConnection (verified against their deserializer
  switches), but is semantically about *quota*, not name/domain conflicts, and is **not**
  in AssociateCustomDomain's documented error set at all (only
  `InternalServiceErrorException`, `InvalidRequestException`, `InvalidStateException` --
  confirmed via the AWS docs page, not just the Go SDK). `AssociateCustomDomain` returning
  `ServiceQuotaExceededException` for a duplicate domain association would deserialize as
  untyped on a real client. Fixed to `InvalidRequestException`, which is valid for every
  App Runner operation with no exceptions.
- The catch-all/internal-fault case returned `__type: "InternalServiceError"` (missing the
  `Exception` suffix) instead of `InternalServiceErrorException`. `strings.EqualFold`
  comparisons in the SDK's error switch require an exact string match, so this also always
  fell through to a generic untyped error client-side, even though the HTTP status (500)
  was already correct.

Verified case-by-case against real per-op error sets (not just the model's global exception
list) by walking each `awsAwsjson10_deserializeOpError<Op>` function's `strings.EqualFold`
switch in `aws-sdk-go-v2/service/apprunner@v1.40.2/deserializers.go`. One op-specific
subtlety was fixed as a result: `AssociateCustomDomain`'s "service not found" branch in
`backend.go` now wraps `ErrInvalidParameter` (-> `InvalidRequestException`) instead of the
package-wide `ErrNotFound` (-> `ResourceNotFoundException`), since that op's real error set
has no `ResourceNotFoundException`. `DisassociateCustomDomain` and `DescribeCustomDomains`
*do* accept `ResourceNotFoundException` and were left using `ErrNotFound` unchanged.

HTTP status codes were already correct throughout (400 for all four client-fault exceptions,
500 for the internal-fault default) -- only the `__type` string was wrong.

**Confirmed correct (don't re-flag):**
- Protocol is `awsjson1.0` (`Content-Type: application/x-amz-json-1.0`,
  `X-Amz-Target: AppRunner.<Op>`) -- matches `handler.go`'s `apprunnerTargetPrefix`/
  `contentType` constants and the SDK's `awsAwsjson10_*` serializer/deserializer naming.
- Timestamps: `CreatedAt`/`UpdatedAt`/`StartedAt`/`EndedAt` are epoch-seconds JSON numbers
  on the wire (`smithytime.ParseEpochSeconds` in the real deserializer); gopherstack emits
  these as `int64` via `.Unix()`, which is wire-compatible (a JSON number, same as the
  SDK's `float64` epoch parse expects) even though it doesn't route through
  `pkgs/awstime.Epoch`.
- Field names for `InstanceConfiguration` (`Cpu`/`Memory`/`InstanceRoleArn`),
  `SourceConfiguration.ImageRepository` (`ImageIdentifier`/`ImageRepositoryType`), and all
  `*SummaryList`/`NextToken` list envelopes match the real serializers exactly.
- Status enums used (`RUNNING`/`PAUSED`/`DELETED` for Service; `ACTIVE`/`INACTIVE` for
  ASG/Observability/VpcConnector; `AVAILABLE`/`DELETED` for Connection/VpcIngressConnection)
  are all valid real enum members. Services/configs transition to terminal states
  immediately on create/pause/resume/delete rather than sitting in
  `OPERATION_IN_PROGRESS`/`PENDING_CREATION` -- this is a deliberate simplification (avoids
  the "client polls DescribeService forever" trap called out in parity-principles.md), not a
  disguised no-op: state actually mutates and persists correctly.
- `Handler.Snapshot`/`Restore` delegate to the backend correctly (`persistence.go`); the
  doc comment there notes this delegation itself was a previously-fixed dead-wiring bug
  (Handler had no Snapshot/Restore before Phase 3.3, so App Runner was silently never
  persisted) -- already fixed prior to this sweep, left as historical context.

## Session log

- 2026-07-13 (911ff167): Fresh audit. Fixed the three exception-type-name bugs and the
  AssociateCustomDomain not-found mapping described above (backend.go, handler.go). Added
  `invalidStateType`/`internalServiceErrorType` consts so `handleError` no longer duplicates
  wire-type strings as separate literals from backend.go's sentinel-wrapping consts (the
  literal/const divergence is exactly how the `InternalServiceError` vs
  `InternalServiceErrorException` typo happened in the first place). No existing tests
  asserted the old (wrong) `__type` strings, so no test updates were needed; full existing
  suite plus `go vet`/`go fix -diff`/`golangci-lint` all green. ~30 LOC changed.

- 2026-07-23: Closed every `gaps`/`deferred` item from the 2026-07-13 audit by field-diffing
  `CreateServiceInput`/`UpdateServiceInput`/`Service`/`OperationSummary`/
  `AutoScalingConfigurationSummary` against `aws-sdk-go-v2/service/apprunner@v1.40.2/types`
  and implementing what was missing for real (no stubs):
  - **AutoScalingConfigurationArn association** (the root cause of three separate gaps).
    `CreateService`/`UpdateService` now resolve the ARN (full ARN, name-only ARN, or bare
    name -- both formats `CreateServiceInput`'s doc comment describes) via
    `resolveASG`/`resolveOrDefaultASG` (`service_associations.go`), or fall back to the
    account's default when omitted. `ensureDefaultAutoScalingConfiguration` seeds App
    Runner's real always-present `DefaultConfiguration` revision 1 (real accounts have this
    before any `CreateAutoScalingConfiguration` call) at backend construction, `Reset`, and
    both `Restore` paths. `HasAssociatedService` is now real, recomputed by
    `recomputeASGAssociation` on every association change (create/update/delete) by scanning
    live services rather than a hand-tracked counter (simplicity over micro-perf; table sizes
    are emulator-scale). This closes: `Service.AutoScalingConfigurationSummary` (previously
    always missing, a documented-required field), `ListAutoScalingConfigurations`'s
    `HasAssociatedService` (previously hardcoded false), and
    `ListServicesForAutoScalingConfiguration` (previously always empty).
  - **`Service.NetworkConfiguration`** (previously entirely missing, also a documented-required
    field): `CreateService`/`UpdateService` accept `NetworkConfiguration` (Egress/
    IngressConfiguration, IpAddressType), validate `EgressType: VPC`'s `VpcConnectorArn`
    against the real `vpcConnectors` table (`InvalidRequestException` if unresolvable --
    `CreateService`'s error set has no `ResourceNotFoundException`, verified against
    `awsAwsjson10_deserializeOpErrorCreateService`'s switch), and apply App Runner's
    documented defaults (`DEFAULT` egress, publicly accessible, `IPV4`) when omitted.
  - **`OperationSummary.UpdatedAt`**: added to `storedOperation`/`addOperation` (set equal to
    `StartedAt`/`EndedAt` since operations complete immediately in this backend's simplified
    state machine, matching the existing `SUCCEEDED`-on-create pattern) and threaded through
    `ListOperations`'s wire output.
  - **De-deferred `HealthCheckConfiguration`/`EncryptionConfiguration`/`CodeRepository`
    sub-shapes** (previously silently accepted-and-ignored, per parity-principles.md's
    de-stub-hygiene concern about disguised no-ops): `HealthCheckConfiguration` now stores
    Protocol/Path/Interval/Timeout/HealthyThreshold/UnhealthyThreshold with App Runner's real
    defaults (`TCP`, `/`, 5s, 2s, 1, 5). `EncryptionConfiguration.KmsKey` round-trips and is
    only returned when a customer key was actually provided (App Runner omits it for the
    default managed-key case). `SourceConfiguration.CodeRepository` (RepositoryUrl,
    SourceCodeVersion, CodeConfiguration/CodeConfigurationValues) and
    `AuthenticationConfiguration` (AccessRoleArn, ConnectionArn -- validated against the real
    `connections` table when present) now round-trip field-for-field.
    `AutoDeploymentsEnabled` applies App Runner's documented default (false for an ECR Public
    image source, true otherwise) when the caller doesn't specify it.
    `InstanceConfiguration.InstanceRoleArn` now round-trips (was silently dropped).
    `ServiceObservabilityConfiguration` (ObservabilityEnabled + ObservabilityConfigurationArn,
    validated against the real `observabilityConfigs` table) now round-trips.
  - **`UpdateService`** additionally now rejects switching a service between image and code
    sources (`InvalidRequestException`), matching the real op's documented restriction ("you
    must provide the same structure member... that you originally included when you created
    the service") -- previously unenforced since `CodeRepository` didn't exist at all.
  - **Leak fix**: `DeleteService` was leaving its `b.customDomains[serviceArn]` entry behind
    forever after delete (unreachable dead state, since `DescribeCustomDomains` 404s on a
    deleted `ServiceArn`); now cascade-deleted alongside the existing tags cleanup.
  - Backend `CreateService`/`UpdateService` signatures changed from long positional-primitive
    argument lists to `CreateServiceParams`/`UpdateServiceParams` structs (internal to this
    package -- `StorageBackend` has no external implementers besides `InMemoryBackend`, and no
    caller outside this package touches backend method signatures directly, only
    `NewInMemoryBackend`/`NewHandler`/`Provider` -- verified via repo-wide grep before making
    the change).
  - No new goroutines/tickers/janitors introduced; all new bookkeeping stays inside the
    existing `b.mu` critical sections.
  - Added `service_associations.go` (resolution/validation/normalization helpers) and 15 new
    test functions across `handler_services_test.go` (13, covering every new behavior above)
    and `leak_test.go` (the customDomains cascade-delete fix), plus updated pre-existing
    `ListAutoScalingConfigurations` count assertions and `AutoScalingConfigCount` expectations
    in `handler_auto_scaling_configurations_test.go`/`persistence_test.go` for the new
    always-present `DefaultConfiguration` seed. `go build`/`go vet`/`go test -race`/
    `gofmt -l`/`golangci-lint` all green; zero `cyclop`/`gocyclo`/`gocognit`/`funlen` nolints
    before or after.

## 2026-08-21 pass (bd gopherstack-r80d, batch 10): required OUTPUT members never populated

Second service this batch, after stepfunctions. `cmd/requiredoutputfields`
puts apprunner at 44 required output fields across 32 ops-with-required (37
ops total).

**Wire shape**: not "one wrapper key around a nested domain object"
(pinpoint/bedrockagent/cleanrooms) or `map[string]any` literals (s3tables/
codecommit) -- responses are tagged structs, and most ops' own top-level
required members are flat scalars. But an AST-style walk of every
`type X struct { ... }` block in `apprunner@v1.42.4/types/types.go` (not a
grep window) found only `Service` and its nested source-config family
(`CodeConfiguration`, `CodeConfigurationValues`, `CodeRepository`,
`CustomDomain`, `EncryptionConfiguration`, `ImageRepository`,
`ServiceObservabilityConfiguration`, `SourceCodeVersion`,
`TraceConfiguration`) carry any required fields at all --
`AutoScalingConfiguration`/`Connection`/`ObservabilityConfiguration`/
`VpcConnector`/`VpcIngressConnection` and every one of their `*Summary` list
siblings declare **zero** required fields. That made this pass narrower
than stepfunctions': read all 32 ops' own required members plus every one
of those 9 nested types against their handlers.

### 1 bug found and fixed, proven via a real `aws-sdk-go-v2/service/apprunner`
### client round-trip test (`wire_output_required_r80d_test.go`)

- **`AssociateCustomDomain`/`DisassociateCustomDomain`'s `VpcDNSTargets`**
  (both ops' `Output`, required). `DescribeCustomDomains` -- the sibling op
  with the identical required set (`CustomDomain`, `DNSTarget`, `ServiceArn`,
  `VpcDNSTargets`) -- already emitted `VpcDNSTargets: []any{}` correctly, but
  `associateCustomDomainOutput`/`disassociateCustomDomainOutput` had no
  `VpcDNSTargets` field at all, so the key was entirely absent on both ops.
  Fixed by adding the field to both structs, always `[]any{}` (this backend
  doesn't model per-domain VPC-ingress DNS targets, matching
  `DescribeCustomDomains`'s existing honest-empty convention -- not
  fabricated). Proven via `Test_SDKRoundTrip_CustomDomain_VpcDNSTargets`,
  hand-reverted/confirmed-failing/restored, `md5sum`-verified byte-identical.

### 2 fixed but NOT counted

- **`CodeRepository.SourceCodeVersion`** (types.go:245-263, required
  alongside `RepositoryUrl`). `validateSourceConfig` (services.go) checked
  `RepositoryURL != ""` but never checked `SourceCodeVersionType`, so an
  omitted `SourceCodeVersion` was silently accepted and then dropped from
  `codeRepositoryOutput` (`toCodeRepositoryOutput` only sets it
  `if cs.SourceCodeVersionType != ""`). Fixed by adding the same required
  check already used for `RepositoryUrl`/`ImageIdentifier`. **Not counted**:
  the real `aws-sdk-go-v2` client's own generated `validateCodeRepository`
  (`validators.go:792-806`) already rejects a nil `SourceCodeVersion`
  client-side before any request is sent -- no real Go SDK client can ever
  reach gopherstack in the buggy state, so this campaign's real-SDK-client
  round-trip proof standard cannot apply here even though the underlying gap
  is real for any other caller (raw HTTP, a non-Go SDK, or a Go client with
  validation disabled). Proven instead via a raw request through this
  package's own `doRequest`/`newTestHandler` test helpers
  (`Test_CodeRepository_SourceCodeVersion_Required`), which bypass the Go
  SDK's client-side check the same way those other callers would; hand-
  reverted/confirmed-failing/restored, `md5sum`-verified byte-identical.
- **`ObservabilityConfiguration.TraceConfiguration`** (types.go:601,
  optional -- "If not specified, tracing isn't enabled"; not a Smithy-
  required member, so outside this cut's precise target class even though
  it is a real, provable bug). `CreateObservabilityConfiguration` captured
  `TracingVendor` from the request and stored it, but
  `observabilityConfigurationOutput` had no `TraceConfiguration` field at
  all -- a configured vendor was silently dropped on every
  Create/DescribeObservabilityConfiguration response. Fixed by adding the
  field, present only when `TracingVendor != ""` (matching the real "absent
  means not enabled" semantics, not fabricating a vendor for an
  unconfigured one). Proven via
  `Test_SDKRoundTrip_ObservabilityConfiguration_TraceConfiguration`
  (a genuine real-client round trip -- this one has no client-side
  validation blocking it, unlike `SourceCodeVersion` above), hand-reverted/
  confirmed-failing/restored, `md5sum`-verified byte-identical.

### Reviewed, not a bug / out of scope

- **`ImageRepository.ImageRepositoryType`** (required) is passed through
  unvalidated on `CreateService` (only `ImageIdentifier` is checked), but
  `imageRepositoryOutput.ImageRepositoryType` has no `omitempty` -- an
  omitted value is emitted as a present-but-empty string, not a dropped
  key. Different bug class from this cut's target (wrong/invalid content
  on a present field, not an absent required field) -- disclosed, not
  fixed.
- **`AutoScalingConfiguration`/`Connection`/`ObservabilityConfiguration`/
  `VpcConnector`/`VpcIngressConnection`** and all their `*Summary` siblings:
  confirmed via the same AST-style walk that none declare any required
  field in `types.go` -- there is nothing for this bug class to violate on
  any of them.
- **`Service`**'s own 10 required top-level fields
  (`AutoScalingConfigurationSummary`, `CreatedAt`, `InstanceConfiguration`,
  `NetworkConfiguration`, `ServiceArn`, `ServiceId`, `ServiceName`,
  `SourceConfiguration`, `Status`, `UpdatedAt`) and `CodeConfiguration`/
  `CodeConfigurationValues`'s required fields (`ConfigurationSource`/
  `Runtime`) were all already correctly guarded -- each nested object is
  only constructed (and thus its own required fields only ever set) when a
  real, non-empty upstream value exists, matching this campaign's
  "required-but-inapplicable means present-and-empty, not absent"
  principle by construction. `EncryptionConfiguration.KmsKey` similarly
  guarded (`if svc.EncryptionKmsKey != ""`).
- **`StopExecutionOutput`-style `*float64`/list-field patterns** don't
  apply here; all List ops already build their summary slices via
  `make(..., 0, len(...))` or an explicit nil-guard before assignment,
  confirmed for all 6 List ops that carry a required list field
  (`ListAutoScalingConfigurations`, `ListConnections`,
  `ListObservabilityConfigurations`, `ListServices`,
  `ListServicesForAutoScalingConfiguration`, `ListVpcConnectors`,
  `ListVpcIngressConnections`).

Total for apprunner this pass: 44 required output fields plus all 9 nested
required-bearing types read end to end across 32 ops with required output
fields, 1 counted bug, 2 fixed-but-not-counted findings, 1 disclosed
(wrong-content, not absent-field) finding.
- 2026-08-19: Wrapper-key/nested-shape wire-parity sweep (all 37 ops enumerated from the
  pinned SDK's `api_op_*.go` files and `GetSupportedOperations()`, both agree). Protocol
  reconfirmed as JSON-RPC 1.0 (`awsAwsjson10_*` in `deserializers.go`, exact-match protocol
  -- not tolerant of casing). Read every op's own `awsAwsjson10_deserializeOpDocument<Op>Output`
  and, for every nested/summary type actually emitted, its own `deserializeDocument<Type>`
  case list.
  - **Bug found and fixed**: `ListObservabilityConfigurations`'s summary entries emitted
    three fabricated keys -- `Status`, `Latest`, `CreatedAt` -- that have no case at all in
    the real `types.ObservabilityConfigurationSummary` document deserializer
    (`deserializers.go:6215-6270`; the real struct at `types/types.go:613-628` only has
    `ObservabilityConfigurationArn`/`Name`/`Revision`). A real SDK client would silently drop
    them (this is the same fabricated-summary-field bug class the sibling `fis` sweep found
    this session). Fixed in `handler_observability_configurations.go` by narrowing
    `observabilityConfigurationSummaryOutput` to the 3 real fields. New test
    `TestListObservabilityConfigurations_SummaryHasNoFabricatedFields`
    (`observability_configuration_summary_wire_test.go`) does a raw-body assertion (the real
    SDK type can't observe a leaked key, so this is the only instrument that can), plus
    `TestListObservabilityConfigurations_RealClientSeesSummaryFields` proves the 3 real
    fields still round-trip through the real client. Hand-revert reproduced the exact leaked
    keys in the raw JSON body; restored fix verified byte-identical to the pre-revert file.
  - **Incidental Layer-3 fix** (one only, per this sweep's scope -- member-never-emitted
    hunting is otherwise out of scope): `AssociateCustomDomain`/`DisassociateCustomDomain`
    were missing the `VpcDNSTargets` key present in the real deserializer
    (`deserializers.go:7705-7763`, `8462-8520`) and already emitted (as an always-empty list)
    by the sibling `DescribeCustomDomains` op in the same file. Added the same
    `VpcDNSTargets: []any{}` convention to both. New test
    `TestAssociateDisassociateCustomDomain_VpcDNSTargetsPresent`
    (`custom_domain_vpc_dns_targets_test.go`) proves via the real SDK client that
    `out.VpcDNSTargets` is now non-nil (the real
    `awsAwsjson10_deserializeDocumentVpcDNSTargetList` only runs when the JSON key is
    present, so omitting the key leaves the real client's field `nil` instead of an empty
    slice -- confirmed by hand-revert reproducing exactly that `nil` before restoring).
  - **Families verified clean** (correct wrapper key + correct nesting, summary types
    checked against their own narrower case list, no fabricated fields): CreateService/
    UpdateService/DeleteService/DescribeService/PauseService/ResumeService/StartDeployment
    (`Service`, `serviceOutput` in `handler_services.go`, all nested sub-shapes --
    SourceConfiguration/CodeRepository/ImageRepository/NetworkConfiguration/
    HealthCheckConfiguration/EncryptionConfiguration/ServiceObservabilityConfiguration --
    field-for-field against their own deserializer case lists); ListServices
    (`ServiceSummary`, correctly narrow vs. full `Service`, aside from the disclosed
    `UpdatedAt` gap below); the `AutoScalingConfigurationSummary` embedded in every `Service`
    response (correctly narrow -- 7 fields, no `MaxConcurrency`/`MaxSize`/`MinSize`, matching
    the real embedded-summary shape, not the full `AutoScalingConfiguration`);
    Create/Describe/Delete/UpdateDefaultAutoScalingConfiguration (full type, 10/12 real
    fields -- see gaps for the 2 omitted); ListAutoScalingConfigurations (`Summary`, matches
    the real 7-field narrow type exactly); Create/Delete/ListConnections (`Connection`/
    `ConnectionSummary` -- identical field sets on the real types too, so no over-emission
    risk there); ListOperations (`OperationSummary`, all 7 real fields present, including
    `UpdatedAt` which a prior sweep already fixed); Create/Describe/Delete/
    ListVpcConnectors (`VpcConnector`, no separate summary type in the real SDK, confirmed);
    Create/Describe/Delete/List/UpdateVpcIngressConnection (`VpcIngressConnection` full type,
    and `VpcIngressConnectionSummary` -- correctly narrow at just 2 fields,
    `VpcIngressConnectionArn`/`ServiceArn`, matching the real type exactly);
    ListServicesForAutoScalingConfiguration (plain `ServiceArnList`); Tag/Untag/
    ListTagsForResource (`Tag`: `Key`/`Value`, `TagResourceOutput`/`UntagResourceOutput`
    correctly empty). `DescribeCustomDomains` itself already emitted `VpcDNSTargets`
    correctly (the two sibling ops fixed above didn't).
  - Gates: `go build`, `go vet`, `go fix -diff`, `gofmt -l`, `go test -race`,
    `golangci-lint run` all clean on `./services/apprunner/...` after the fixes (see report
    for verbatim output). Zero `cyclop`/`gocyclo`/`gocognit`/`funlen` nolints introduced.
    No existing test asserted a wrong key for either bug, so none needed correcting -- only
    new tests were added.

- 2026-08-23: Closed the four member-never-emitted items the 2026-08-19 Layer 3 sweep
  disclosed but explicitly left unfixed (member-never-emitted hunting was out of scope that
  pass). Verified each against `aws-sdk-go-v2/service/apprunner@v1.42.4/deserializers.go`
  (the go.mod-pinned version; the 2026-08-19 note cited line numbers from the same tag) before
  touching code:
  - `ServiceSummary.UpdatedAt` (deserializers.go:6939): emit-only -- `storedService.UpdatedAt`
    was already tracked and current, `toSummary()`/`serviceSummaryOutput` just never carried it.
  - `Service.DeletedAt` (deserializers.go:6615): NOT emit-only -- `storedService` had no
    `DeletedAt` field at all, and `DeleteService` evicts the row from the store (`b.services.
    Delete`) rather than soft-deleting it, so no later Describe/List can ever observe it; only
    `DeleteService`'s own response can. Added the field, set it right before eviction.
  - `VpcConnector.DeletedAt` (deserializers.go:7299) / `VpcIngressConnection.DeletedAt`
    (deserializers.go:7547): emit-only -- both were already tracked internally (confirmed:
    `DeleteVpcConnector`/`DeleteVpcIngressConnection` already set them) but `vpcConnectorOutput`/
    `vpcIngressConnectionOutput` never carried the key.
  - `AutoScalingConfiguration.Latest` (deserializers.go:4692): NOT emit-only -- unlike the note's
    other three items, `Latest` was not tracked anywhere in the domain model. Computed the same
    way `ObservabilityConfiguration.Latest` already was (this codebase's own established pattern
    for the identical "is this the highest revision of this name" fact): `b.asgByName[name]`
    already held revisions in creation order for `latestOnly` list filtering; `Create`/
    `DeleteAutoScalingConfiguration` now flip the prior/new last entry's `Latest` bit the same
    way `Create`/`DeleteObservabilityConfiguration` do. `AutoScalingConfiguration.DeletedAt`
    (deserializers.go:4660) was bundled into this fix too -- same already-tracked-but-unemitted
    gap as VpcConnector/VpcIngressConnection, on the same struct.
  - Not touched: `CustomDomain.CertificateValidationRecords` (deserializers.go:4899, 5381) --
    still a genuine backend gap, no cert-validation flow modeled, nothing internal to surface.
  - Persisted-struct changes: `storedService` gained `DeletedAt time.Time`;
    `storedAutoScalingConfiguration` gained `Latest bool`. Both are additive
    (`encoding/json` zero-values a missing key on restore of an older snapshot: `DeletedAt`
    correctly defaults to "not deleted"; a pre-fix snapshot's `Latest` defaults to `false` for
    every revision of a name until the next Create/Delete on that name recomputes it -- a
    cosmetic gap on the very first read after upgrade, not a data-loss risk). Snapshot version
    was NOT bumped (correctly -- see pkgs/persistence's `TestSnapshotVersionGuard`, which exists
    specifically to catch a reflexive bump on an additive change). `go test ./pkgs/persistence/...
    -run TestSnapshotVersionGuard` reports apprunner's golden is out of date (additive-only diff,
    needs `-update`) -- refresh deferred to the orchestrator per this campaign's standing rule.
  - Proof: `wire_output_required_r80d_gapclosure_test.go` adds 5 real-`aws-sdk-go-v2`-client
    round-trip tests, one per fixed member (`TestListServices_SummaryHasUpdatedAt`,
    `TestDeleteService_ResponseHasDeletedAt`, `TestDeleteVpcConnector_ResponseHasDeletedAt`,
    `TestDeleteVpcIngressConnection_ResponseHasDeletedAt`, `TestAutoScalingConfiguration_Latest`).
    All 5 hand-verified to fail against the pre-fix code (nil-pointer/false-value assertions),
    confirmed to pass against the fix, then the fix was hand-reverted via `cp` to the 9 touched
    source files and restored, `md5sum`-verified identical to the fixed state post-restore.
  - Introduced-then-fixed side effects: adding `*int64`/`bool` fields to `serviceOutput`,
    `autoScalingConfigurationOutput`, `vpcConnectorOutput`, `vpcIngressConnectionOutput` tripped
    `govet fieldalignment` (9 findings) -- fixed by hand-reordering fields in just those 4 wire
    structs (verified the target layout using the `fieldalignment` binary as a read-only oracle
    against an isolated scratch file, per this campaign's "no package-wide/fieldalignment -fix"
    rule -- the actual repo files were never run through an automated fixer). Mirroring
    `DeleteObservabilityConfiguration`'s `Latest`-promotion logic in `DeleteAutoScalingConfiguration`
    also tripped `dupl` (the two functions are now near-identical by design); moved the existing
    `//nolint:dupl` comments from the two `List*` functions (no longer duplicates of each other
    after this change, so the old directives went stale/unused per `nolintlint`) onto the two
    `Delete*` functions where the real duplication now lives.
  - Gates: `go build`, `go vet`, `go fix -diff`, `gofmt -l`, `go test -race`, `golangci-lint run`
    all clean on `./services/apprunner/...` (0 issues). Full existing suite (`go test
    ./services/apprunner/...`) green throughout -- no existing test asserted the old
    (missing-field) shape, so none needed correcting.

## Notes (2026-08-30 pass — pagination map-order audit)

Audited every `pkgs/page.New` call site in this service (9 call sites: `vpc_ingress_
connections.go`, `services.go`, `operations.go`, `observability_configurations.go`,
`auto_scaling_configurations.go` x2, `custom_domains.go`, `vpc_connectors.go`,
`connections.go`) for the class of bug confirmed in `services/opsworks`: a paginator
consuming an unspecified-order Go map walk (`pkgs/store.Table.All()`/`.Range()`)
with no total sort.

Verdict: 0 bugs. Every call site sources its pre-pagination slice from one of three
safe mechanisms, none of which is a raw map walk:
- `Table.Snapshot()` (`ListVpcIngressConnections`, `ListServices`,
  `ListObservabilityConfigurations`, `ListAutoScalingConfigurations`,
  `ListServicesForAutoScalingConfiguration`, `ListVpcConnectors`, `ListConnections`)
  -- per `pkgs/store.Table.Snapshot`'s doc comment this is already sorted by the
  table's own (definitionally unique) primary key, unlike `Table.All()`;
- a plain append-only Go slice, not a `Table` at all (`ListOperations` reads
  `svc.Operations []*storedOperation`, bounded to 200 and only ever grown via
  `append`; `DescribeCustomDomains` reads `b.customDomains[serviceArn]`, same
  append/splice-only shape) -- deterministic order requires no sort;
- filtering (`nameFilter`/`latestOnly`/ARN match) is applied to the
  already-deterministic `Snapshot()`/slice output and always precedes the
  `page.New` call -- no filter-after-pagination bug found.

Empirically proved the `Table.Snapshot()` mechanism (the most novel of the three,
new since the opsworks fix predates `pkgs/store`) with a full-walk test rather than
trusting the doc comment alone: added `pagination_full_walk_test.go`'s
`TestListServices_FullWalk_NoDropsOrDuplicates`, seeding 25 services via the real
`aws-sdk-go-v2` client, walking `ListServices` to completion at `MaxResults=5`, and
asserting the union of every page is exactly the seed set with no drop or
duplicate. Passed 10/10 runs under `-race -count=10`.

No sort found non-total on a map-walk-sourced call site (none of the 9 sites
touch a map walk at all); no MaxResults/NextToken-accepting op found that
silently returns everything untruncated. Gates on `./services/apprunner/...`:
`go build`, `go vet`, `go test -race -count=1` (all pass), `golangci-lint run`
(0 issues).

## 2026-08-31 (value-semantics pass, gopherstack-uox6): two bugs, filter/default
surface otherwise clean

Scope: every optional filter and boolean default across all 14 List/Describe
input structs (`aws-sdk-go-v2/service/apprunner@v1.42.4 api_op_List*.go`/
`api_op_Describe*.go`), read field-by-field against the pinned SDK's own doc
comments -- the class this campaign has been sweeping other services for
(bd `gopherstack-uox6`): a filter that is read and applied but implements the
wrong semantics, invisible to every shape/enum-based scanner.

**Bug 1 -- a documented `Default: true` collapsed to Go's `bool` zero value
(false).** `ListAutoScalingConfigurations` and `ListObservabilityConfigurations`
both document `LatestOnly`: "Set to true to list only the latest revision...
Set to false to list all revisions... **Default: true**." Both handlers
decoded it as a plain `bool` (`json:"LatestOnly"`), so an omitted key -- the
*only* wire form any conformant client can produce, since the pinned SDK's
own serializer (`serializers.go`: `if v.LatestOnly { ok.Boolean(...) }`) never
puts the key on the wire for a false/unset value -- decoded to Go's zero
value `false` and fell into this backend's `else` branch: "return every
revision." The documented default is the *opposite* -- latest-only -- so
every unfiltered `List*ScalingConfigurations`/`List*ObservabilityConfigurations`
call returned every revision of every configuration instead of one row per
name. Fixed by changing both request fields to `*bool` (nil means "key
absent" and now resolves to the documented default `true`; a decoded `false`
or `true` is honoured explicitly) -- `handler_auto_scaling_configurations.go`,
`handler_observability_configurations.go`. `TestAutoScalingConfigurationRevisions`
(`handler_auto_scaling_configurations_test.go`) was asserting the bug
directly (empty body expected 3 rows, i.e. every revision); corrected to
expect 2 (latest-only, matching the explicit-`LatestOnly:true` case
immediately below it) and a new explicit-`false` case added to keep the
"list all" branch under test. Added
`TestObservabilityConfigurationRevisionsLatestOnlyDefault`
(`handler_observability_configurations_test.go`) from scratch --
`TestObservabilityConfigurationDescribeDeleteList`'s existing list check only
ever seeded one revision, so the omitted-`LatestOnly` case was never
distinguishable from the bug there. Both new/changed assertions hand-verified
failing against the unmodified code before the fix (bare `bool` still in
place), then passing after.

**Bug 2 -- a wire key that doesn't exist on the real type.**
`ListVpcIngressConnections`'s `Filter` decoded a
`VpcIngressConnectionArn` member that `types.ListVpcIngressConnectionsFilter`
(`aws-sdk-go-v2/service/apprunner@v1.42.4 types/types.go`) does not have --
the real second member is `VpcEndpointId` (confirmed against
`serializers.go`'s `awsAwsjson10_serializeDocumentListVpcIngressConnectionsFilter`,
which serializes exactly `ServiceArn`/`VpcEndpointId` and nothing named
`VpcIngressConnectionArn`). The mismatched key meant this filter was
permanently empty regardless of what a real client sent, and an empty filter
value fell through this backend's `!= ""` no-filter case -- so a
`VpcEndpointId` filter silently matched every connection instead of
narrowing to the one requested. Same shape as the CloudWatch instance in this
class's twelfth pass: a wrong wire key feeding an otherwise-correct
empty-means-no-filter default, so each half looks fine in isolation and only
the combination is wrong. Fixed the field name/JSON tag
(`handler_vpc_ingress_connections.go`) and renamed the filter through
`vpc_ingress_connections.go`/`interfaces.go` to match against
`VpcIngressConnection.VpcEndpointID`, which this backend already tracks on
the full record (just never on the filter path). Added two new subtests to
`TestVpcIngressConnectionDescribeDeleteListUpdate`
(`handler_vpc_ingress_connections_test.go`): a matching-`VpcEndpointId`
filter (passed even against the bug, since the filter was a no-op) and a
non-matching one (hand-verified failing against the unmodified code -- it
returned the one seeded connection instead of an empty list -- then passing
after the fix).

**Everything else checked, clean.** Every other List/Describe input across
both services was read against its own doc comment, not assumed from a
sibling: `ConnectionName`/`AutoScalingConfigurationName`/
`ObservabilityConfigurationName` (`nameFilter`) all correctly treat an
absent value as "not filtered by name", matching each op's own prose;
`ListFirewallRuleGroupAssociations`' `Status`/`Priority`/`VpcId`/
`FirewallRuleGroupId` (this pass also swept `route53resolver`'s firewall
family for the same class -- see that service's entry below) and
`ListVpcIngressConnections`' `ServiceArn` all correctly no-op when absent;
`ListServicesForAutoScalingConfiguration`'s partial-ARN-or-name resolution
(`resolveASG`) already accepts both forms. No range/bound/date filter,
operator grammar, wildcard, or negation syntax exists anywhere in this
service's request surface -- every filter here is plain scalar equality, so
those sub-shapes of this bug class (boundary inclusivity, unit mismatch,
operator mishandling) are structurally absent, not merely unaudited.

Gates: `go build`, `go vet ./...` (repo-wide, no other caller of the two
changed backend interface methods), `go test -race -count=1
./services/apprunner/...`, `golangci-lint run ./services/apprunner/...` (0
issues; `fieldalignment` checked via a scratch-directory oracle per this
repo's no-automated-fixer convention, hand-applied to both changed structs).

## 2026-09-03 pass (bd gopherstack-9vv): 5 missing delete-precondition bugs, referential integrity

Targeted sweep of the "missing delete precondition / referential integrity"
class this campaign has repeatedly found across services: every `Delete*`
op's own doc comment in `aws-sdk-go-v2/service/apprunner@v1.42.4
api_op_Delete*.go`, read in full, against what the corresponding backend
method actually checks before mutating state.

**5 bugs found and fixed, all backed by an explicit doc sentence (not just a
modelled error) and all proven via a hand-revert/confirmed-failing/restored
regression test:**

- **`DeleteConnection`** (`api_op_DeleteConnection.go`): "You must first
  ensure that there are no running App Runner services that use this
  connection. If there are any, the DeleteConnection action fails." Never
  checked -- `connections.go`'s `DeleteConnection` deleted unconditionally.
  Fixed: rejects (`InvalidRequestException` -- this op's error set has no
  `InvalidStateException`, confirmed against
  `awsAwsjson10_deserializeOpErrorDeleteConnection`'s switch) when any live
  service's `SourceConfiguration.AuthenticationConfiguration.ConnectionArn`
  still references it (`serviceUsesConnection`, `service_associations.go`).
  Test: `TestDeleteConnection_RejectsWhenServiceUsesIt`
  (`handler_connections_test.go`).
- **`DeleteAutoScalingConfiguration`** (`api_op_DeleteAutoScalingConfiguration.go`):
  "You can't delete the default auto scaling configuration or a
  configuration that's used by one or more App Runner services." Never
  checked, despite this backend already tracking both exact booleans
  (`cfg.IsDefault`, `cfg.HasAssociatedService`) for other purposes
  (`ListAutoScalingConfigurations`'s summary, `UpdateDefaultAutoScalingConfiguration`).
  Fixed: two guards in `DeleteAutoScalingConfiguration`
  (`auto_scaling_configurations.go`), each independently proven load-bearing
  by neutering one at a time. Test:
  `TestDeleteAutoScalingConfiguration_RejectsDefaultAndInUse`
  (`handler_auto_scaling_configurations_test.go`, two subtests).
- **`DeleteVpcConnector`** (`api_op_DeleteVpcConnector.go`): "You can't
  delete a connector that's used by one or more App Runner services." Never
  checked. Fixed: rejects (`InvalidRequestException`) when any live
  service's `NetworkConfiguration.EgressConfiguration.VpcConnectorArn` still
  references it (`serviceUsesVpcConnector`). Test:
  `TestDeleteVpcConnector_RejectsWhenServiceUsesIt`
  (`handler_vpc_connectors_test.go`).
- **`DeleteObservabilityConfiguration`** (`api_op_DeleteObservabilityConfiguration.go`):
  "You can't delete a configuration that's used by one or more App Runner
  services." Never checked. Fixed: rejects (`InvalidRequestException`) when
  any live service has it enabled
  (`Observability.Enabled && Observability.ConfigurationArn == obsArn`,
  `serviceUsesObservabilityConfig`). Test:
  `TestDeleteObservabilityConfiguration_RejectsWhenServiceUsesIt`
  (`handler_observability_configurations_test.go`).
- **`DeleteService`** (`api_op_DeleteService.go`): "Make sure that you don't
  have any active VPCIngressConnections associated with the service you want
  to delete." Never checked. Unlike the four resource-config deletes above,
  `DeleteService`'s error set *does* model `InvalidStateException`
  (`awsAwsjson10_deserializeOpErrorDeleteService`), the same mechanism
  `UpdateService`/`PauseService`/`ResumeService` already use for their own
  state preconditions, so this fix uses `ErrInvalidState` rather than
  `ErrInvalidParameter`. Fixed: rejects when any VPC ingress connection still
  references the service (`hasActiveVpcIngressConnections` --
  `DeleteVpcIngressConnection` already removes its row from the table on
  delete, so any entry found is inherently active; no status filter needed).
  Test: `TestDeleteService_RejectsWhenActiveVpcIngressConnectionExists`
  (`handler_services_test.go`).

All 4 new cross-resource-scan helpers (`serviceUsesConnection`,
`serviceUsesVpcConnector`, `serviceUsesObservabilityConfig`,
`hasActiveVpcIngressConnections`) live in `service_associations.go` next to
the pre-existing `recomputeASGAssociation`/`validateNetworkConfig`/
`validateObservability`/`validateSourceAuth` helpers they mirror in style
(scan `b.services`/`b.vpcIngressConnections` under the already-held lock, no
new locking or goroutines).

Every fix hand-verified to fail without it (temporarily reverted the guard
via `cp`-backup/edit/restore, ran the specific new test, confirmed it failed
with the exact expected-vs-actual mismatch, then restored and reconfirmed
green) -- see bd gopherstack-9vv for the verbatim failure output. No existing
test asserted the old (permissive) behavior, so none needed correcting; full
existing suite was green throughout.

**`dupl` fallout**: `DeleteAutoScalingConfiguration`/`DeleteObservabilityConfiguration`
diverged enough (different in-use checks) that they're no longer
near-duplicates of each other, making their existing `//nolint:dupl`
directives stale (flagged by `nolintlint`) while `ListAutoScalingConfigurations`/
`ListObservabilityConfigurations` (unchanged, always structurally identical)
became the new `dupl` match. Moved the `//nolint:dupl` directives from the
two `Delete*` functions to the two `List*` functions accordingly -- same
"duplication moves, directive follows it" pattern the 2026-08-23 pass
documented for these same two function pairs.

**Not touched / out of scope this pass**: `DeleteVpcIngressConnection`'s own
documented state-machine precondition ("must be in AVAILABLE,
FAILED_CREATION, FAILED_UPDATE, or FAILED_DELETION") and `UpdateVpcIngressConnection`'s
("AVAILABLE, FAILED_CREATION, or FAILED_UPDATE") are structurally
unreachable in this backend: VPC ingress connections only ever have
`AVAILABLE` (live, in `b.vpcIngressConnections`) or removed-from-table
(post-delete) states -- the `FAILED_*`/`PENDING_*` transitional states are
never modeled anywhere in this service (a deliberate simplification this
campaign has repeatedly confirmed elsewhere: services/configs transition to
terminal states immediately). Adding a guard against an unreachable state
would be dead code, not a fix.

Gates: `go build`, `go vet`, `go fix -diff`, `gofmt -l`, `go test -race
-count=1 ./services/apprunner/...`, `golangci-lint run
./services/apprunner/...` (0 issues) all clean. `go test -race -count=1
./services/cloudformation/...` (unrelated dependent sanity check per this
campaign's standing instruction) also green. No `StorageBackend` interface
method signatures changed, so no root-package run was needed.
