---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: servicediscovery
sdk_module: aws-sdk-go-v2/service/servicediscovery@v1.43.4   # version audited against; matches go.mod (verified)
botocore_model: servicediscovery/2017-03-14/service-2.json (botocore 1.43.56)  # for shape constraints not carried into the Go SDK comments
last_audit_commit: e50f52dce                      # this pass (2026-08-28, write-only-state sweep)
last_audit_date: 2026-08-28
overall: A            # write-only-state sweep pass (2026-08-28). No wire_field_fixes_test.go
                       # existed yet for this service despite the prior pass's extensive
                       # "audited and confirmed correct" notes below -- per this campaign's
                       # protocol, treated those as claims to verify, not proof. Ran the
                       # write-only-state method (what does each backend persist, what real op
                       # reads it back) across every namespace/service/instance Update op. Found
                       # one real bug: UpdatePrivateDnsNamespace/UpdatePublicDnsNamespace's
                       # Namespace.Properties.DnsProperties.SOA.TTL (types.
                       # PrivateDnsNamespaceChange/PublicDnsNamespaceChange, types.go:923-1033)
                       # was entirely absent from the wire-decode struct -- only Description was
                       # read -- so a real client's documented way to change a namespace's SOA
                       # TTL after creation was silently dropped. Fixed, see UpdatePrivateDNSNamespace/
                       # UpdatePublicDNSNamespace rows and wire_field_fixes_test.go (new file, two
                       # round-trip tests). UpdateHTTPNamespace unaffected (HttpNamespaceChange has
                       # only Description, confirmed against types.go:408-416). enumcheck: 0
                       # findings in this service. Protocol re-confirmed: X-Amz-Target
                       # Route53AutoNaming_v20170314.<Op>, POST /, awsjson1.1-shaped (handler_sdk_
                       # route_table_test.go). Also examined UpdateService's DnsConfig/
                       # HealthCheckConfig omission semantics (real AWS: "If you omit any existing
                       # DnsRecords or HealthCheckConfig configurations from an UpdateService
                       # request, the configurations are deleted from the service" per
                       # api_op_UpdateService.go's doc comment) -- this backend does NOT delete an
                       # existing HealthCheckConfig/DnsConfig.DnsRecords when omitted from an
                       # UpdateService call, a real but distinct (not write-only-state) behavioral
                       # gap; left unfixed this pass, see gaps.
                       # ---- prior pass's note follows ----
                       # real bugs found and fixed this pass (follow-up to gopherstack-bq50)
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateHttpNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePrivateDnsNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePublicDnsNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  GetNamespace: {wire: fixed, errors: ok, state: ok, persist: ok, note: "response included a Tags field; real types.Namespace has none (tags only via ListTagsForResource) -- fixed, see Notes"}
  ListNamespaces: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "Tags field removed (see GetNamespace); Filters now implement TYPE/NAME/HTTP_NAME/RESOURCE_OWNER with EQ/BEGINS_WITH -- fixed, see Notes"}
  DeleteNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateHttpNamespace: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed no SOA/DnsProperties surface exists to update -- HttpNamespaceChange has only Description, types.go:408-416"}
  UpdatePrivateDnsNamespace: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "write-only-state bug (this pass): Namespace.Properties.DnsProperties.SOA.TTL (types.PrivateDnsNamespaceChange, types.go:923-975) was entirely absent from the wire-decode struct -- only Description was read, so a real client's documented way to change the SOA TTL after creation was silently dropped. Fixed; round-trip test in wire_field_fixes_test.go."}
  UpdatePublicDnsNamespace: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "same SOA TTL wire-decode gap as UpdatePrivateDnsNamespace (types.PublicDnsNamespaceChange, types.go:981-1033) -- fixed, same pass"}
  CreateService: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "Tags field removed from response; ServiceAlreadyExists now enforced (case-insensitive within DNS namespaces, case-sensitive within HTTP namespaces); DnsConfig.RoutingPolicy/DnsRecords[].Type and HealthCheckConfig.Type now validated against their closed enums (see gopherstack-bq50 Notes) -- fixed"}
  GetService: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Tags field removed (see CreateService) -- fixed"}
  ListServices: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-tuh5: was reusing serviceToMap (the full GetService converter) unscoped, leaking a top-level NamespaceId that types.ServiceSummary does not declare (confirmed against awsAwsjson11_deserializeDocumentServiceSummary; the nested, deprecated DnsConfig.NamespaceId is a distinct field on both shapes and is unaffected). namespaceToMap in this same file was checked and is clean (types.NamespaceSummary matches exactly). serviceToMap now delegates to a dedicated serviceSummaryToMap plus the one extra field. Regression: raw-body assertion (an SDK client discards unrecognised keys and can't observe an over-wide response). Prior pass: Filters now implement NAMESPACE_ID/RESOURCE_OWNER -- fixed, see Notes"}
  DeleteService: {wire: ok, errors: ok, state: ok, persist: ok, note: "was silently auto-deregistering instances instead of failing ResourceInUse -- fixed prior pass"}
  UpdateService: {wire: ok, errors: fixed, state: ok, persist: ok, note: "DnsConfig.RoutingPolicy/DnsRecords[].Type and HealthCheckConfig.Type now validated (see CreateService) -- fixed"}
  GetServiceAttributes: {wire: fixed, errors: ok, state: ok, persist: ok, note: "response emitted the generic keyArn (\"Arn\") for ServiceAttributes.ServiceArn; real key is \"ServiceArn\" (deserializers.go:6001), distinct from Service/Namespace which really do use \"Arn\" -- fixed 2026-08-23"}
  UpdateServiceAttributes: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "REQUEST decode struct read wire key \"ServiceArn\"; real key is \"ServiceId\" (serializers.go:3040-3043, accepts ID or ARN) -- every real client call failed \"ServiceArn is required\". Fixed 2026-08-23, see Notes. Also: botocore model DOES carry the quota (shape ServiceAttributesMap{max:30,min:1}, ServiceAttributeKey{max:255}, ServiceAttributeValue{max:1024}); the prior pass's 'no documented numbers' excuse was wrong -- ServiceAttributesLimitExceededException and InvalidInput now enforced, see gopherstack-bq50 Notes"}
  DeleteServiceAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  RegisterInstance: {wire: ok, errors: ok, state: fixed, persist: ok, note: "custom-attribute quota (30 count/255 key/1024 value/5000 total, documented) and AWS_INIT_HEALTH_STATUS seeding were unenforced/unimplemented -- fixed, see Notes"}
  DeregisterInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  GetInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  ListInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  DiscoverInstances: {wire: ok, errors: ok, state: fixed, persist: n/a, note: "HEALTHY_OR_ELSE_ALL fixed prior pass; OptionalParameters was parsed but never applied -- fixed this pass, see Notes"}
  DiscoverInstancesRevision: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetInstancesHealthStatus: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "explicitly-requested Instances IDs not registered to the service were silently dropped from the response instead of failing InstanceNotFound (one of GetInstancesHealthStatus's 3 documented errors) -- fixed, see gopherstack-bq50 Notes. HealthStatus=UNKNOWN still never returned -- see gaps (structural, unlike the InstanceNotFound precondition)"}
  UpdateInstanceCustomHealthStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  GetOperation: {wire: ok, errors: ok, state: ok, persist: ok}
  ListOperations: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "response used the full Operation shape (Type/CreateDate/UpdateDate/Targets/ErrorCode/ErrorMessage); real ListOperationsOutput.Operations is []types.OperationSummary{Id,Status} only -- fixed. Filters now implement NAMESPACE_ID/SERVICE_ID/STATUS/TYPE/UPDATE_DATE with EQ/IN/BETWEEN -- fixed, see Notes"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: n/a}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  route_matcher: {status: ok, note: "X-Amz-Target prefix Route53AutoNaming_v20170314. verified byte-for-byte against serializers.go"}
  timestamps: {status: ok, note: "CreateDate/UpdateDate use pkgs/awstime.Epoch() (float64, sub-second precision), matching AWS's fractional Unix-timestamp wire format; fixed prior pass"}
  tags_wire_shape: {status: fixed, note: "GetNamespace/ListNamespaces/CreateService/GetService/ListServices all included a Tags field; real types.Namespace, types.NamespaceSummary, types.Service, types.ServiceSummary have none -- tags are ONLY returned by ListTagsForResource. Fixed by removing Tags from namespaceToMap/serviceToMap; see Notes"}
  filters: {status: fixed, note: "ListNamespaces/ListServices/ListOperations Filters now honor Condition (EQ default, BEGINS_WITH, IN, BETWEEN for UPDATE_DATE) and every documented Name value including RESOURCE_OWNER (single-account model: SELF matches everything, OTHER_ACCOUNTS matches nothing) -- fixed, see Notes"}
  service_name_uniqueness: {status: fixed, note: "CreateService now enforces the documented same-namespace name-collision rule (case-insensitive for DNS namespaces, case-sensitive for HTTP namespaces) and returns ServiceAlreadyExists -- fixed, see Notes"}
  persistence: {status: ok, note: "Handler.Snapshot/Restore delegate to backend; backendSnapshot covers all 4 store.Table-backed resources plus the two raw maps (serviceAttributes, instanceHealthStatuses); versioned and tested (persistence_test.go)"}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "GetInstancesHealthStatus/DiscoverInstances never surface HealthStatus=UNKNOWN. The enum value itself IS present in the source (types.HealthStatusUnknown, aws-sdk-go-v2/service/servicediscovery@v1.43.4/types/enums.go:74) -- this is NOT a source-level wire gap. Real Cloud Map instances backed by an AWS-managed HealthCheckConfig start UNKNOWN until the Route53 health check propagates; gopherstack has no Route53 health-check subsystem to drive that transition, so all instances are HEALTHY until explicitly marked UNHEALTHY via UpdateInstanceCustomHealthStatus. Confirmed structural (would require simulating real endpoint health evaluation); the precondition bug found alongside this claim (explicitly-requested unknown instance IDs silently omitted instead of erroring) WAS fixable and has been fixed, see gopherstack-bq50 Notes"
  - "DuplicateRequest ('operation is already in progress', returned by CreateHttpNamespace/CreatePrivateDnsNamespace/CreatePublicDnsNamespace/DeleteNamespace/DeregisterInstance/RegisterInstance/UpdateHttpNamespace/UpdatePrivateDnsNamespace/UpdatePublicDnsNamespace/UpdateService per strings.EqualFold(\"DuplicateRequest\", errorCode) in the vendored deserializers.go -- re-verified this pass, the operation list is one op fewer than a prior audit missed adding UpdateService/the three UpdateXNamespace ops) has no genuine trigger path: every op completes synchronously under the backend's coarse write lock, so there is never an observable in-flight/PENDING window for a concurrent duplicate request to collide with. Checked the narrower question this pass -- is there a *synchronous* duplicate AWS refuses that this backend accepts? Registering the same service+instance ID twice is upsert semantics in real AWS too (no error); creating a duplicate-name service is already caught by ServiceAlreadyExists, a different exception. No synchronous trigger found; sentinel intentionally not added (would be dead code with no real trigger)"
  - "ResourceLimitExceeded (CreateHttpNamespace/CreatePrivateDnsNamespace/CreatePublicDnsNamespace/CreateService/RegisterInstance) and RequestLimitExceeded (account-wide API throttling quota) are real SDK error types with no quota numbers documented anywhere in the vendored SDK source or the botocore model (only external doc links, e.g. cloud-map-limits.html) -- left unenforced rather than guessing at unverified thresholds"
  - "UpdateService does not implement the documented omit-to-delete semantics: api_op_UpdateService.go's doc comment states 'If you omit any existing DnsRecords or HealthCheckConfig configurations from an UpdateService request, the configurations are deleted from the service.' This backend's UpdateService only applies DnsConfig.DnsRecords[].TTL/HealthCheckConfig when present in the request and leaves the existing stored config untouched when omitted, instead of deleting it. Found this pass via the write-only-state method but not fixed (distinct bug class from the SOA TTL fix -- a real client can still express every value it wants stored, it just can't clear one to unset via omission); needs its own bd issue and a design call on how 'explicitly send nothing' is distinguished from 'field omitted for brevity' in this handler's json.Unmarshal-based decoding"
  - "FIXED 2026-09-06 (bd gopherstack-chmx), but note the residual fallback: DNS_PRIVATE/DNS_PUBLIC namespace Properties.DnsProperties.HostedZoneId now comes from a real route53.CreateHostedZone call when Route 53 is wired in (SetHostedZoneCreator, cli.go's wireServiceDiscoveryRoute53). When Route 53 is NOT wired (e.g. every test backend built via NewInMemoryBackend directly, and any real composition that omits the wiring call) it still falls back to the pre-fix synthetic 'Z'+randAlnum ID that matches no real zone -- this keeps namespace creation permissive rather than failing it, per this repo's unwired-hook convention, but callers relying on Route 53 lookups against that ID in an unwired deployment will still find nothing. See TestCreatePrivateDNSNamespace_UsesRealHostedZone/TestCreatePublicDNSNamespace_UsesRealHostedZone (wired) and TestCreateDNSNamespace_UnwiredRoute53StaysPermissive (unwired fallback), store_test.go."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "Full cross-account/shared-namespace support (OwnerAccount request param, ARN-as-Id acceptance for namespace/service ID fields, real per-resource ResourceOwner tracking) -- not emulated; single-account model throughout. The RESOURCE_OWNER *filter* itself IS now handled this pass (coarse SELF-always-true/OTHER_ACCOUNTS-always-false semantics matching a single-account backend), but that's filtering only, not the underlying sharing model. Re-confirmed structural this pass: gopherstack has no per-request account concept anywhere in the codebase -- pkgs/arn hardcodes a single fake account ID (000000000000) repo-wide -- so a second account to share a namespace with doesn't exist to model against. A partial cross-account model confined to this one service would be fake work, not emulation"
leaks: {status: clean, note: "no goroutines/janitors in this service; all state is plain maps/store.Table guarded by lockmetrics.RWMutex"}
---

## Notes

**2026-08-15 (gopherstack-3gbe):** investigated whether Cloud Map shares
Omics' (gopherstack-keee) client-side host-prefix-rewrite reachability gap.
It does: **2 ops, one literal prefix, `data-`** (DiscoverInstances
`api_op_DiscoverInstances.go:186`, DiscoverInstancesRevision
`api_op_DiscoverInstancesRevision.go:146`), confirmed against the pinned
`servicediscovery@v1.43.4` module, exactly matching gopherstack-3gbe's
filing.

No routing/auth code needed changing. `Handler.RouteMatcher`
(`handler.go:151`) matches on the `X-Amz-Target` header prefix
`"Route53AutoNaming_v20170314."`, never `Host` or `Path`, so header-based
dispatch is structurally immune to the path-collision class this bug family
could otherwise cause. The reachability gap is a pure client-side DNS/dial
failure, same as Omics.

servicediscovery already had a real-SDK-client round trip
(`handler_create_tags_test.go`), but it never exercised DiscoverInstances or
DiscoverInstancesRevision, so this family's real-client reachability had
never been proven either way. Added `host_prefix_reachability_test.go`
following `services/omics/host_prefix_reachability_test.go`'s before/after
pattern: a before-fix test proving the unmodified client can't dial, and an
after-fix test that drives CreateHttpNamespace -> CreateService ->
RegisterInstance -> DiscoverInstances/DiscoverInstancesRevision through a
redial-to-the-real-listener transport, leaving the SDK's real, un-disabled
`data-` rewrite intact on the wire, and asserts the full round trip succeeds
with correctly decoded values. Gates green: build, vet, race, `go fix -diff`
(no diff), golangci-lint (0 findings).

**Route matcher**: `X-Amz-Target: Route53AutoNaming_v20170314.<Op>` confirmed against
`aws-sdk-go-v2/service/servicediscovery@v1.43.4/serializers.go` (`SetHeader("X-Amz-Target").String("Route53AutoNaming_v20170314.<Op>")`
for every operation; re-verified against the currently pinned v1.43.4 this pass -- a prior
pass's note cited a stale v1.39.24, now corrected). No bug here.

**Bugs fixed this pass** (all real, verified against the vendored SDK source, not against
gopherstack's own output):

1. **GetNamespace/ListNamespaces/CreateService/GetService/ListServices returned a Tags field
   that doesn't exist on the real wire shape** (`handler_namespaces.go`'s `namespaceToMap`,
   `handler_services.go`'s `serviceToMap`). Real `types.Namespace` (returned by GetNamespace),
   `types.NamespaceSummary` (ListNamespaces), `types.Service` (CreateService/GetService), and
   `types.ServiceSummary` (ListServices) were all field-diffed against `types/types.go` --
   *none* of them declare a `Tags` field. Tags are only ever returned by
   `ListTagsForResource`. Because JSON unmarshaling silently drops unrecognized fields, this
   extra key was invisible to the real aws-sdk-go-v2 client and would only surface against a
   stricter/custom deserializer -- a real, if subtle, wire-shape divergence. Fixed by dropping
   `Tags` from both `*ToMap` functions; tests that asserted `Tags` presence in these responses
   were rewritten to assert its *absence* and to fetch tags via `ListTagsForResource` instead
   (`TestHandler_NamespaceTagsViaListTagsForResource`, `TestHandler_ServiceTagsViaListTagsForResource`,
   `TestMapToTagEntriesSorted`).

2. **ListOperations used the full `Operation` response shape instead of the real
   `OperationSummary` shape** (`handler_operations.go`). Real
   `ListOperationsOutput.Operations` is `[]types.OperationSummary`, which per
   `types/types.go` has exactly two fields: `Id` and `Status`. Gopherstack's `operationToMap`
   (built for `GetOperation`'s full `Operation` shape) was reused for `ListOperations` too,
   silently leaking `Type`/`CreateDate`/`UpdateDate`/`Targets`/`ErrorCode`/`ErrorMessage` into
   every list-operations response. Fixed by adding a dedicated `operationSummaryToMap`
   (Id+Status only) for `ListOperations`, leaving `GetOperation`'s full-shape `operationToMap`
   untouched. `TestListOperations_SummaryShape` locks the exact key set.

3. **CreateService never enforced the documented same-namespace name-collision rule** (was
   `deferred` in the prior audit pass; now resolved). `api_op_CreateService.go`'s doc comment
   is explicit: "For services that are accessible by DNS queries, you can't create multiple
   services with names that differ only by case (such as EXAMPLE and example) ... However, if
   you use a namespace that's only accessible by API calls [HTTP], then you can create
   services that with names that differ only by case." Added `checkServiceNameAvailable`
   (`services.go`): within a DNS namespace (`DNS_PRIVATE`/`DNS_PUBLIC`), a new service name is
   compared case-insensitively against existing services in the same namespace; within an
   HTTP namespace, case-sensitively. A collision returns the new `ErrServiceAlreadyExists`
   sentinel, wired to the real `ServiceAlreadyExists` error code. Services with no
   `NamespaceId` (`""`) are exempt -- there's no namespace to scope uniqueness to, matching
   `NamespaceId`'s optional status on `CreateServiceInput` (not in `validateOpCreateServiceInput`'s
   required-field list). The prior pass's "traps for the next auditor" note about
   `DiscoverInstances`' last-write-wins lookup is now stale (new creates can no longer produce
   namespace+name duplicates) but the defensive `svcMatches[len(svcMatches)-1]` lookup was left
   in place as a harmless safety net for any duplicates a pre-fix snapshot might still contain.

4. **DiscoverInstances silently ignored OptionalParameters** (`discovery.go`). The request
   struct already parsed `OptionalParameters` (`handler_discovery.go`'s
   `discoverInstancesRequest`) but never passed it to the backend. Real
   `DiscoverInstancesInput.OptionalParameters` doc comment: "Opportunistic filters to scope
   the results based on custom attributes. If there are instances that match both the filters
   specified in both the QueryParameters parameter and this parameter, all of these instances
   are returned. Otherwise, the filters are ignored, and only instances that match the filters
   that are specified in the QueryParameters parameter are returned." Added
   `narrowByOptionalParams`, applied after `QueryParameters` filtering and before health-status
   filtering, implementing exactly that fall-back semantic.

5. **RegisterInstance had no custom-attribute quota enforcement and ignored
   AWS_INIT_HEALTH_STATUS** (`handler.go`'s new `validateInstanceAttributes`, `instances.go`'s
   `RegisterInstance`). `RegisterInstanceInput.Attributes` doc comment: "You can add up to 30
   custom attributes. For each key-value pair, the maximum length of the attribute name is 255
   characters, and the maximum length of the attribute value is 1,024 characters. The total
   size of all provided attributes (sum of all keys and values) must not exceed 5,000
   characters" -- added as `InvalidInput` validation, mirroring the existing `validateTags`
   pattern. Separately: "AWS_INIT_HEALTH_STATUS If the service configuration includes
   HealthCheckCustomConfig, you can optionally use AWS_INIT_HEALTH_STATUS to specify the
   initial status of the custom health check, HEALTHY or UNHEALTHY. If you don't specify a
   value ..., the initial status is HEALTHY." Gopherstack already defaulted unset statuses to
   HEALTHY but never read this attribute at all, so `AWS_INIT_HEALTH_STATUS: UNHEALTHY` was
   silently ignored. `RegisterInstance` now seeds `instanceHealthStatuses` from this attribute
   when the target service has `HealthCheckCustomConfig`.

6. **ListNamespaces/ListServices/ListOperations Filters only implemented the single most
   common `Name` value per filter, always as implicit `EQ`** (was a `gap` in the prior audit
   pass; now resolved). Field-diffed every `Name`/`Condition` combination documented on
   `types.NamespaceFilter`, `types.ServiceFilter`, and `types.OperationFilter`. Added a shared
   `FilterValue` type (`models.go`, `Values []string` + `Condition string`) with a `matches`
   method supporting the real `EQ` (default)/`BEGINS_WITH`/`IN` operators, plus a
   `resourceOwnerMatches` helper for the `RESOURCE_OWNER` filter (this single-account backend
   treats every resource as always `SELF`-owned: `SELF` matches everything, `OTHER_ACCOUNTS`
   matches nothing -- no data is fabricated for a sharing model that doesn't exist here).
   `ListNamespaces` now honors `TYPE`/`NAME`/`HTTP_NAME`/`RESOURCE_OWNER`; `ListServices`
   honors `NAMESPACE_ID`/`RESOURCE_OWNER`; `ListOperations` honors
   `NAMESPACE_ID`/`SERVICE_ID`/`STATUS`/`TYPE`/`UPDATE_DATE`, the last via a new
   `parseEpochSeconds` helper decoding the `BETWEEN` start/end pair (Unix-seconds strings, per
   `OperationFilter`'s doc comment) into a `[start, end]` range matched against `UpdateDate`.

**Bugs fixed this follow-up pass** (gopherstack-bq50; all verified against the pinned
`aws-sdk-go-v2/service/servicediscovery@v1.43.4` and the botocore model
`servicediscovery/2017-03-14/service-2.json`, botocore 1.43.56):

7. **UpdateServiceAttributes had no quota enforcement, and the prior pass's "no documented
   numbers" excuse was wrong.** The Go SDK's `api_op_UpdateServiceAttributes.go` and
   `validators.go` carry no length/count constraints (Go SDK client validation only checks
   `nil`-ness), but the botocore JSON model does: shape `ServiceAttributesMap{max:30,min:1}`,
   `ServiceAttributeKey{max:255}`, `ServiceAttributeValue{max:1024}` -- the same numbers
   `ServiceAttributes.Attributes`'s doc comment states in prose ("You can specify a total of
   30 attributes"). Added `validateServiceAttributeShape` (`handler.go`) for the per-request
   shape (non-empty map, key/value length caps, `InvalidInput`) and a post-merge count check
   in `InMemoryBackend.UpdateServiceAttributes` (`services.go`, checked before any mutation)
   returning the new `ErrServiceAttributesLimitExceeded` sentinel
   (`ServiceAttributesLimitExceededException`, matching the exception's doc comment: "you've
   exceeded the quota for the number of attributes you can add to a service"). Table-driven
   coverage: `TestHandler_UpdateServiceAttributesQuota`.

8. **GetInstancesHealthStatus silently dropped explicitly-requested instance IDs that weren't
   registered to the service, instead of failing.** `InstanceNotFound`'s doc comment: "No
   instance exists with the specified ID..."; `GetInstancesHealthStatus` lists
   `InstanceNotFound` as one of its 3 documented errors (`service-2.json` operation entry).
   `InMemoryBackend.GetInstancesHealthStatus` (`instances.go`) now validates every ID in a
   non-empty `Instances` filter against the service's registered instances before building the
   response, returning `ErrInstanceNotFound` on the first miss. `HealthStatus=UNKNOWN` itself
   remains unimplemented and is now correctly recorded as an evidenced *structural* gap (the
   enum value IS present in the source, `types.HealthStatusUnknown`,
   `types/enums.go:74` -- it's the Route53 health-check propagation gopherstack can't
   simulate, not a source-level omission). Coverage:
   `TestHandler_GetInstancesHealthStatusUnknownInstanceID`.

9. **CreateService/UpdateService accepted any string for `DnsConfig.RoutingPolicy`,
   `DnsConfig.DnsRecords[].Type`, and `HealthCheckConfig.Type`** -- more permissive than real
   AWS. All three are closed enums in the botocore model (`RoutingPolicy{enum:[MULTIVALUE,
   WEIGHTED]}`, `RecordType{enum:[SRV,A,AAAA,CNAME]}`, `HealthCheckType{enum:[HTTP,HTTPS,
   TCP]}`); the Go SDK types are plain `string` aliases with no client-side enum check, so
   this is server-side-only validation missing from gopherstack. Added
   `validateDNSConfigEnums`/`validateHealthCheckConfigEnum` (`handler_services.go`), wired into
   both `handleCreateService` and `handleUpdateService`. `RoutingPolicy` is optional per
   `DnsConfig`'s `required` list (empty string allowed); `DnsRecord.Type` is required.
   Coverage: `TestHandler_CreateServiceInvalidEnums`.

**Re-examined and left unchanged** (gopherstack-bq50's other two items): `DuplicateRequest`
and cross-account/shared-namespace support were re-verified with fresh evidence (see `gaps`/
`deferred` above) and remain genuinely structural -- no fix made.

**Traps for the next auditor** (looks-wrong-but-correct):

- The Phase 3.3 `store.Table`/`store.Index` migration (`store_setup.go`) already makes
  service/instance lookups exact-key, not prefix-based, so the old class of "prefix
  collision" bugs that `TestCascadeDeleteUsesCorrectPrefix` was originally guarding against
  can no longer occur through that path; the test was kept for its DeleteService-ordering
  coverage.
- `DiscoverInstances`' `svcMatches[len(svcMatches)-1].ID` "last write wins" lookup is now
  unreachable via any *new* `CreateService` call (name collisions within a namespace now fail
  with `ServiceAlreadyExists`), but was deliberately left in place rather than simplified to
  `svcMatches[0].ID` -- it's a no-op safety net for restoring a persistence snapshot captured
  before this pass, and removing it buys nothing.
- `checkServiceNameAvailable`'s HTTP-namespace case-sensitive / DNS-namespace
  case-insensitive split is *intentional*, not a bug -- it's a direct, deliberate reading of
  `api_op_CreateService.go`'s doc comment distinguishing DNS-discoverable namespaces (where
  case-only name variants would produce ambiguous DNS records) from HTTP-only namespaces
  (which have no DNS record to collide over).

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors

`Handler()`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.
servicediscovery is awsjson1.1 (confirmed from `servicediscovery@v1.43.4`
deserializers.go's `awsAwsjson11_deserializeOpError*` prefix); its
per-operation deserializer still calls `restjson.GetErrorInfo` to
JSON-decode `__type`/`message`, so plain text doesn't decode -- a real
client got `*json.SyntaxError`, not even `UnknownError`.

Fixed by routing the ReadBody error through this handler's own
`handleError(c, err)`: it checks `sentinelErrorCodes()` and
`isMalformedRequest(err)` (which only matches `errInvalidRequest`,
`json.SyntaxError`, `json.UnmarshalTypeError`) -- a `*http.MaxBytesError`/
read error matches neither, so it falls through to the pre-existing
default (`__type: "InternalServiceError"`, 500).

NOTE (pre-existing, NOT fixed by this pass): `"InternalServiceError"` does
not appear in `servicediscovery@v1.43.4` `types/errors.go`'s modeled list
(`CustomHealthNotFound`, `DuplicateRequest`, `InstanceNotFound`,
`InvalidInput`, `NamespaceAlreadyExists`, `NamespaceNotFound`,
`OperationNotFound`, `RequestLimitExceeded`, `ResourceInUse`,
`ResourceLimitExceeded`, `ResourceNotFoundException`,
`ServiceAlreadyExists`, `ServiceAttributesLimitExceededException`,
`ServiceNotFound`, `TooManyTagsException`) -- it falls through to the
client's generic `smithy.GenericAPIError` branch rather than a modeled
struct, which still surfaces the correct `ErrorCode()` but is a possible
pre-existing wire-code mismatch in the genuine per-operation default, out
of this ticket's ReadBody-only scope.

CONFIRMED (documented "left untyped" decision distinct from the above):
this file's Notes elsewhere record a deliberate choice not to simplify a
validation branch that looks redundant at a glance (line ~237) -- a
different kind of gap (modeling ambiguity, not error-typing) that this fix
does not touch.

Proven with a real `aws-sdk-go-v2/service/servicediscovery` client's
`CreateService`, whose `Description` field alone exceeds
`httputils.MaxRequestBodyBytes` (16 MiB).
`TestHandler_OversizedBodySurfacesInternalServiceError`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalServiceError"`; confirmed it fails pre-fix with
`*json.SyntaxError` (hand-reverted, byte-identical restore after).

## gopherstack-o7gx follow-up (2026-08-22): default error path fixed to InternalFailure

The NOTE above flagged `"InternalServiceError"` as not matching
`servicediscovery@v1.43.4`'s `types/errors.go`. Confirmed:
`servicediscovery@v1.43.4` models zero 5xx/internal-fault exceptions at all
(its 15 modeled types are all 4xx client faults -- `CustomHealthNotFound`,
`DuplicateRequest`, `InstanceNotFound`, `InvalidInput`,
`NamespaceAlreadyExists`, `NamespaceNotFound`, `OperationNotFound`,
`RequestLimitExceeded`, `ResourceInUse`, `ResourceLimitExceeded`,
`ResourceNotFoundException`, `ServiceAlreadyExists`,
`ServiceAttributesLimitExceededException`, `ServiceNotFound`,
`TooManyTagsException` -- confirmed via `types/errors.go` in full). So no
replacement code maps to a modeled type here either way; per the
mediapackage/sagemaker precedent (prefer a generic AWS-wide code over
reusing another service's specific modeled exception name, and never invent
one), fixed `handler.go`'s `handleError` default to `errType =
"InternalFailure"` -- the same generic fallback already used by 7+ other
gopherstack services with no modeled internal fault (`backup`, `memorydb`,
`identitystore`, `accessanalyzer`, `elbv2`, `ssoadmin`, `rds`).
`"InternalServiceError"` was not a fabricated code (it's the real modeled
type for `secretsmanager`/`transfer`), just not this service's.

`TestHandler_OversizedBodySurfacesInternalFailure`
(renamed from `...InternalServiceError`) now asserts `apiErr.ErrorCode() ==
"InternalFailure"`; confirmed it fails pre-fix with the old
`"InternalServiceError"` code (hand-reverted, byte-identical restore
after).

## 2026-08-23: UpdateServiceAttributes request decoded the wrong wire key

Request-side sweep (accept-and-drop class, first audit of the request side
rather than the response side). `handler_services.go`'s
`updateServiceAttributesRequest` decoded `json:"ServiceArn"`. The real
`UpdateServiceAttributesInput` has no `ServiceArn` member at all --
serializers.go:3040-3043 always emits the identifier under key `"ServiceId"`
(which accepts either the service ID or its ARN, per the SDK doc comment).
Since servicediscovery is JSON-RPC 1.1 (case-sensitive, no fallback key),
every real client call left the identifier empty and failed validation with
"ServiceArn is required" -- this operation was 100% non-functional for any
real caller. The existing unit/persistence tests all passed because they
hand-built request bodies using the same wrong key, matching the bug
instead of catching it (`services_test.go`, `persistence_test.go`).

Fixed: decode struct now reads `json:"ServiceId"`; the backend method
(`UpdateServiceAttributes`, `services.go`) now resolves its argument as
either a bare ID (`b.services.Has`) or an ARN (`b.servicesByARN.Get`),
matching the doc'd "ID or ARN" semantics -- previously it only looked up by
ARN, so even a corrected wire key would have broken most real callers
(which send the plain ID).

While proving this end-to-end with a real SDK client, found a second, entangled
bug: `GetServiceAttributes`'s response emitted the ARN under the shared
`keyArn` constant ("Arn"), but `ServiceAttributes.ServiceArn` (unlike
`Service`/`Namespace`) really is named "ServiceArn" on the wire
(deserializers.go:6001) -- a real client always saw an empty ARN back. Fixed
alongside since it blocked proving the request-side fix round-trips.

Proof: `TestUpdateServiceAttributes_RoundTrip`
(`wire_update_service_attributes_test.go`) drives the real
`aws-sdk-go-v2/service/servicediscovery` client through
CreateService -> UpdateServiceAttributes -> GetServiceAttributes, by both ID
and ARN. Confirmed it fails pre-fix with exactly `InvalidInput: invalid
request: ServiceArn is required` (hand-reverted via `cp`, md5sum-identical
restore after). Existing tests (`services_test.go`,
`persistence_test.go`) updated to send the real `ServiceId` key instead of
the bug-matching `ServiceArn` key.

**2026-08-30 (negative-continuation-token sweep)**: `handler.go`'s `decodeCursor` base64-decoded
a token and `fmt.Sscanf`'d it to an int with no bounds check; all 5 callers
(`handler_operations.go`, `handler_namespaces.go`, `handler_instances.go` x2,
`handler_services.go`) only check `offset >= len(items)`, which does not catch a negative
offset, so `items[offset:end]` panicked given a token base64-decoding to `-5`. Fixed at the
decode site: `decodeCursor` now returns 0 for a negative offset, so all 5 callers inherit the
fix. No existing test supplied a hostile token for any of these listings before this pass.

Proof: `TestApplyPaginationNamespaces_NegativeOffsetToken` and
`TestApplyPaginationInstances_NegativeOffsetToken` (new file
`pagination_negative_token_internal_test.go`) confirmed panicking pre-fix, pass now. Gates:
`go build ./services/servicediscovery/...`, `go vet ./services/servicediscovery/...`, `go test
-race -count=1 ./services/servicediscovery/...`, `golangci-lint run
./services/servicediscovery/...` (0 issues). Work left uncommitted per this pass's
instructions.

## 2026-08-30 (request-field axis sweep, gopherstack-4shm's class)

Ran `cmd/reqfieldscan -dir servicediscovery`: dispatch table 30/30 resolved
(100%, all via the literal-decode path -- this service never uses
`service.JSONOpFunc`/`service.WrapOp`), 8 unread fields flagged, all
`CreatorRequestId`/`UpdaterRequestId`: `createHTTPNamespaceRequest`,
`createPrivateDNSNamespaceRequest`, `createPublicDNSNamespaceRequest`,
`createServiceRequest`, `registerInstanceRequest` (CreatorRequestID), and
`updateHTTPNamespaceRequest`, `updatePrivateDNSNamespaceRequest`,
`updatePublicDNSNamespaceRequest` (UpdaterRequestID).

**Real, verified gap; deliberately not fixed this pass (layer-boundary).**
All 8 are real SDK fields (`*string`, confirmed in each `api_op_*.go`), but
none is client-side-required (no `validators.go` entry for any of them --
distinct from managedblockchain's `ClientRequestToken`, which the same sweep
found *required* and fixed there). Their documented purpose is retry-safety:
"allows failed Create<X>/Update<X> requests to be retried without the risk
of running the operation twice." gopherstack decodes them and does nothing
else -- no dedup, so a client retry with the same token after, say, a
chaos-injected mid-request failure would create/re-update a second resource
instead of returning the original result.

This repo has a precedent for exactly this pattern
(`services/acm`'s `idempotencyMap`, `services/acmpca`'s
`lookupIdempotentCert`/`idempotentResourceARN`), so it is not structurally
unfixable -- but replicating it here means a new per-resource-type
(namespace x3 shapes, service, instance) dedup store plus persistence
wiring across 8 call sites, a real feature-sized change, not a field-read
fix. Left undone and recorded here rather than fabricated or silently
dropped, per gopherstack-4shm's restraint principle.

No code changes this pass -- PARITY.md documentation only. Gates unaffected.

## 2026-08-30 value-semantics filter sweep (gopherstack-uox6's class)

This file already noted (2026-08-30, request-field axis sweep) that
"filter semantics are unexamined" beyond the 30/30 field-presence scan.
This pass reads every `NamespaceFilter`/`ServiceFilter`/`OperationFilter`
Condition/Values semantic, the `HealthStatus`/`OptionalParameters` rules on
`DiscoverInstances`, and every List op's `MaxResults` default against
`aws-sdk-go-v2/service/servicediscovery@v1.43.4`'s own doc comments.

### 2 bugs found and fixed

**1. `ListServices`' `NAMESPACE_ID` filter rejected the documented ARN
form.** `types.ServiceFilter`'s doc comment: "NAMESPACE_ID: Specify one
namespace ID or ARN. Specify the namespace ARN for namespaces that are
shared with your Amazon Web Services account." `ListServices` (`services.go`)
compared the filter's raw value directly against `svc.NamespaceID` (the
bare ID field) via the shared `FilterValue.matches` helper — a real client
filtering by the namespace ARN (the documented, and for shared namespaces
the *only*, way to do it) matched nothing, even though the namespace and
its services existed. Same shape as the RDS ARN-form-filter bug found
earlier in this campaign on a sibling service.

Fixed: added `resolveNamespaceIDFilter` (`services.go`), which rewrites a
NAMESPACE_ID filter's values from ARN to bare ID via the existing
`namespacesByARN` index (already used by `ListTagsForResource`/
`TagResource`/`UntagResource`) before matching, leaving bare-ID filters
(the common case) untouched. `ServiceFilter`'s `RESOURCE_OWNER` name was
already correctly routed through the dedicated `resourceOwnerMatches`
special-case rather than the shared matcher, so it was not affected by (or
in need of) this fix. `OperationFilter`'s `NAMESPACE_ID`/`SERVICE_ID` doc
comments say only "Specify one namespace/service ID" (no ARN form
documented there), so `ListOperations` was checked and correctly left
alone.

Test: `TestBackend_ListServices_FilterByNamespaceARN` (new,
`services_test.go`), hand-confirmed failing against unmodified code (0
matches instead of 1) before the fix.

**2. `DiscoverInstances`' `HealthStatus` filter wasn't ignored for a
service with no health check at all.** `DiscoverInstancesInput.HealthStatus`'s
doc comment: "This parameter is ignored for services that don't have a
health check configured, and all instances are returned." A service
created with neither `HealthCheckConfig` nor `HealthCheckCustomConfig` can
never have an instance move to `UNHEALTHY` in this backend (only
`UpdateInstanceCustomHealthStatus` sets it, and that call itself already
requires `HealthCheckCustomConfig` — see `instances.go:210`, an existing,
correct rejection). So a `HealthStatus=UNHEALTHY` filter against such a
service was narrowing to zero results instead of being ignored and
returning everything, per the doc'd override. (`HEALTHY`/`ALL`/
`HEALTHY_OR_ELSE_ALL`/empty were already unaffected, since every instance
defaults to `HEALTHY` regardless — this only bit the `UNHEALTHY` case.)

Fixed: `filterInstancesByHealth` (`discovery.go`) now checks whether the
service has neither `HealthCheckConfig` nor `HealthCheckCustomConfig` set
and, if so, returns every candidate unconditionally regardless of the
requested `HealthStatus`. This is the one sub-case of the doc'd override
this backend can implement precisely without simulating real health
evaluation — the existing 2026-08-30 (or earlier) note above about
`HealthStatus=UNKNOWN` being structurally absent (no Route53 health-check
subsystem) covers the DNS-health-check case, which remains unfixable and
untouched.

Test: `TestHandler_DiscoverInstancesHealthStatusIgnoredWithoutHealthCheck`
(new, `discovery_test.go`), hand-confirmed failing against unmodified code
(0 instances instead of 1) before the fix. All existing `DiscoverInstances`
health-status tests (including the `HealthCheckCustomConfig`-gated
`UNHEALTHY` case) still pass unchanged.

### Other filters/defaults checked, no bug

- `MaxResults` default (documented "up to 100" on `ListNamespaces`,
  `ListServices`, `ListInstances`, `ListOperations`, and
  `GetInstancesHealthStatus`) matches this service's uniform
  `maxResultsDefault = 100` on all five — no per-op discrepancy like kms
  had.
- `NamespaceFilter`'s `Condition` semantics (`EQ` default/single-value,
  `BEGINS_WITH` single-value prefix, documented only for `TYPE`/`NAME`/
  `HTTP_NAME` — never `RESOURCE_OWNER`) are correct: `RESOURCE_OWNER` is
  routed through the dedicated `resourceOwnerMatches`, not the shared
  `BEGINS_WITH`-capable matcher, so the undocumented combination can't
  occur for that field.
- `OperationFilter`'s `UPDATE_DATE`/`BETWEEN` (start ≤ value ≤ end,
  inclusive both ends — the conventional reading of "specify a start date
  and an end date") and its epoch-seconds wire parsing are correct; `IN`
  for `STATUS`/`TYPE` and `EQ` for all four scalar names are correct.
- `GetInstancesHealthStatus`' `Instances` filter (absent ⇒ "Cloud Map
  returns the health status for all the instances", per doc) correctly
  returns everything when empty (`instances.go`).
- `DiscoverInstances`' `OptionalParameters`-vs-`QueryParameters` combining
  rule (opportunistic narrowing that falls back to the
  `QueryParameters`-only result when nothing matches both) was already
  correct and precisely comment-documented in `discovery.go` before this
  pass — re-verified against the doc's own wording, not re-derived as new.

**Discrimination kept separate, not folded in as this class's bug:**
`ServiceFilter`'s `Condition` doc only lists `EQ` as valid ("EQ is the
default condition and can be omitted" — no other value is documented for
this filter type at all); the shared `FilterValue.matches` would still
accept `BEGINS_WITH`/`IN` for a `ServiceFilter` entry if a raw HTTP caller
sent one, since the matcher is generic across all three filter families.
This is a **missing rejection** (an undocumented `Condition` value is
silently accepted rather than rejected) — validation-shaped, and the same
already-established "unrecognized condition matches everything" convention
this codebase uses deliberately elsewhere (see `FilterValue.matches`'s own
doc comment). Recorded, not fixed, to keep the missing-rejection axis
separate from wrong-algorithm semantics.

No web pages fetched this pass — everything resolved from the pinned
`aws-sdk-go-v2/service/servicediscovery@v1.43.4` module cache doc comments.

Gates: `go build ./services/servicediscovery/...`, `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/servicediscovery/...`,
`golangci-lint run ./services/servicediscovery/...` (0 issues). Work left
uncommitted per this pass's instructions.

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `Http`/`HTTP` and
`PrivateDns`/`PublicDns` vs `PrivateDNS`/`PublicDNS` acronym casing gives it
6 op/handler pairs needing the ambiguous fold, all genuine collisions
between an exported backend method and the real unexported handler:
`CreateHttpNamespace`, `CreatePrivateDnsNamespace`, `CreatePublicDnsNamespace`,
`UpdateHttpNamespace`, `UpdatePrivateDnsNamespace`, `UpdatePublicDnsNamespace`.

Verified directly: ran the unpatched tool from `ef0eef041~1` five times and
diffed against the fixed tool at HEAD. `cmd/reqfieldscan` was byte-identical
across all 5 runs and HEAD -- zero damage. (That identical output includes
8 pre-existing "declared but never read" findings for `CreatorRequestId`/
`UpdaterRequestId` across the Create/Update/RegisterInstance handlers in
this package -- AWS's idempotency-token field, which this emulator has
never modeled deduplication for. Unchanged before and after this defect's
fix, so out of scope for this sweep: noted, not fixed.)

`cmd/reqfielddiff` was the largest swing measured in this batch: old runs
found between 10 and 15 findings (3 distinct counts across 5 runs) vs 4 at
HEAD, with 14 fields flickering, all in the safe direction (present in some
old misresolved run, never at HEAD): `CreatePublicDnsNamespace.{CreatorRequestId,
Description, Name, Properties, Tags}`, `UpdateHttpNamespace.{Id, Namespace,
UpdaterRequestId}`, `UpdatePrivateDnsNamespace.{Id, Namespace,
UpdaterRequestId}`, `UpdatePublicDnsNamespace.{Id, Namespace,
UpdaterRequestId}`. Read the source (handler_namespaces.go:84-119,
286-390): `Id`/`Name`/`Description`/`Properties`/`Tags`/`Namespace` are all
genuinely declared and threaded to the backend on every one of these ops.
`CreatorRequestId`/`UpdaterRequestId` are declared but genuinely unread --
the same pre-existing idempotency-token gap `reqfieldscan` already flags
identically before and after this fix, not something this defect hid.

Verdict: zero bugs caused or hidden by the collision defect. One
pre-existing, already-flagged, out-of-scope gap noted (idempotency token is
a no-op across 8 create/update/register operations in this package).
