---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: elasticsearch
sdk_module: aws-sdk-go-v2/service/elasticsearchservice@v1.45.4
last_audit_commit: 8dc21e834
last_audit_date: 2026-08-15
overall: A            # gopherstack-6flj pass (2026-08-15): the outbound cross-cluster-search-connection
                       # family -- adjacent territory none of the 6 prior audits' notes mention -- had 3 real
                       # bugs: CreateOutboundCrossClusterSearchConnection's request/response used
                       # LocalDomainInfo/RemoteDomainInfo (copied from this package's own internal struct)
                       # instead of the real SourceDomainInfo/DestinationDomainInfo (sibling InboundConnection
                       # already had it right); CreateOutboundCrossClusterSearchConnectionOutput was wrapped
                       # in {"CrossClusterSearchConnection": ...} like its Delete/Accept/Reject siblings, but
                       # the real Create output is flat at the response root; and the top-level route matcher
                       # used `path == elasticsearchCCSOutbound` (exact match) instead of a prefix check like
                       # its Inbound sibling, so DescribeOutboundCrossClusterSearchConnections and
                       # DeleteOutboundCrossClusterSearchConnection were unroutable by any real client -- a
                       # 404 before the handler ever ran. All three fixed; see Notes. Prior gopherstack-p2mx
                       # pass: fixed CancelDomainConfigChange's borrowed-shape response and
                       # CreateVpcEndpoint/UpdateVpcEndpoint's VpcOptions map[string]string that made every
                       # real-SDK-client request 400 -- see Notes. Route audit (51/51) reconfirmed, no new
                       # routing bugs. VPCOptions VPCId/AvailabilityZones and Processing remain documented gaps
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateElasticsearchDomain: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-10 pass added AdvancedSecurityOptions.SAMLOptions, AutoTuneOptions.MaintenanceSchedules, and DeploymentStrategyOptions -- previously accepted-but-dropped or entirely unmodeled; see Notes"}
  DescribeElasticsearchDomain: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeElasticsearchDomains: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteElasticsearchDomain: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED (2026-09-04 pass) -- did not remove the deleted domain from packageAssociationsStore, leaving a ghost row visible via ListDomainsForPackage/ListPackagesForDomain. See Notes and ListPackagesForDomain row."}
  ListDomainNames: {wire: ok, errors: ok, state: ok, persist: ok, note: "route bug fixed this pass -- was served at the wrong path; see Notes"}
  UpdateElasticsearchDomainConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-10 pass added AdvancedSecurityOptions.SAMLOptions, AutoTuneOptions.MaintenanceSchedules, and DeploymentStrategyOptions; see Notes"}
  DescribeElasticsearchDomainConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-10 pass fixed AutoTuneOptions.Options/Status to use their real distinct shapes (types.AutoTuneOptions/types.AutoTuneStatus, not the DomainStatus response's AutoTuneOptionsOutput/generic OptionStatus) and added MaintenanceSchedules + DeploymentStrategyOptions; see Notes"}
  CancelDomainConfigChange: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-p2mx) -- was echoing DescribeElasticsearchDomainConfig's DomainConfig-wrapped body (a borrowed shape, wrong operation entirely) instead of CancelDomainConfigChangeOutput's own {CancelledChangeIds,CancelledChangeProperties,DryRun}; DryRun was also never read from the request. Now returns the real shape: empty CancelledChangeIds/CancelledChangeProperties (this backend has no pending-change queue -- every config change already applied synchronously, so there is truly nothing to report as cancelled) and DryRun echoed from the request. Prior wire: ok was false; the old unit test asserted the wrong (bug-matching) shape and was corrected alongside the fix"}
  AddTags: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED (2026-09-04 pass) -- handler discarded the backend's ErrDomainNotFound and always wrote 200 OK for an unknown ARN; a non-empty TagList also risked a nil-map panic (maps.Copy into the nil map ListTags returns for an unknown ARN). Now returns ValidationException (400) -- AddTags's deserializer has no ResourceNotFoundException case, matching services/opensearch's identical fix. See Notes."}
  RemoveTags: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED (2026-09-04 pass) -- same discarded-error/always-200 bug as AddTags above; same ValidationException fix. See Notes."}
  ListTags: {wire: ok, errors: ok, state: ok, persist: ok}
  StartElasticsearchServiceSoftwareUpdate: {wire: ok, errors: ok, state: ok, persist: ok, note: "route bug fixed this pass -- see Notes"}
  CancelElasticsearchServiceSoftwareUpdate: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteElasticsearchServiceRole: {wire: ok, errors: ok, state: ok, persist: n/a}
  UpgradeElasticsearchDomain: {wire: ok, errors: ok, state: ok, persist: ok}
  GetUpgradeHistory: {wire: ok, errors: ok, state: ok, persist: n/a, note: "no upgrade-history state tracked; always returns empty list"}
  GetUpgradeStatus: {wire: ok, errors: ok, state: ok, persist: n/a, note: "always reports SUCCEEDED; no async upgrade state. Disclosed gap (gopherstack-6flj): real UpgradeName (*string, optional, api_op_GetUpgradeStatus.go) is never emitted -- this backend has no upgrade-name/upgrade-history state at all (GetUpgradeHistory always returns empty), so there is no honest value to source it from; a fabricated 'Upgrade to X' string would be invented state. Not fixed -- see gaps"}
  DescribeDomainAutoTunes: {wire: ok, errors: ok, state: ok, persist: n/a, note: "always empty; no auto-tune state modeled"}
  DescribeDomainChangeProgress: {wire: ok, errors: ok, state: ok, persist: n/a, note: "always COMPLETED; changes apply synchronously"}
  GetCompatibleElasticsearchVersions: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListElasticsearchVersions: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListElasticsearchInstanceTypes: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeElasticsearchInstanceTypeLimits: {wire: ok, errors: ok, state: ok, persist: n/a}
  CreatePackage: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass added required PackageSource (S3BucketName/S3Key) validation; also deleted invented ZIP-PLUGIN package type -- see Notes. 2026-08-10: added CreatedAt/LastUpdatedAt"}
  DescribePackages: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-10: response now includes CreatedAt/LastUpdatedAt; ErrorDetails always omitted (no COPY_FAILED state modeled)"}
  UpdatePackage: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-10: LastUpdatedAt now advances on update"}
  DeletePackage: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociatePackage: {wire: ok, errors: ok, state: ok, persist: ok}
  DissociatePackage: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (cmd/enumcheck sweep, 1d6e40d1a): DomainPackageStatus was the non-member string \"DISSOCIATED\" -- types.DomainPackageStatus only has ASSOCIATING/ASSOCIATION_FAILED/ACTIVE/DISSOCIATING/DISSOCIATION_FAILED (types/enums.go:189-198), no terminal DISSOCIATED. Now emits DISSOCIATING (the transitional state a real client sees on a successful call; this backend completes the removal synchronously, but that is an implementation detail, not a wire value). See TestDissociatePackage_DomainPackageStatus_RealSDKClient (wire_field_fixes_test.go)."}
  GetPackageVersionHistory: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListDomainsForPackage: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListPackagesForDomain: {wire: ok, errors: fixed, state: fixed, persist: n/a, note: "FIXED (2026-09-04 pass) -- never validated the domain existed (ResourceNotFoundException is modelled but never returned); also DeleteElasticsearchDomain never removed the domain from packageAssociationsStore, so a deleted domain remained a ghost row forever in both ListDomainsForPackage and this op. Now 404s for an unknown/deleted domain, and DeleteElasticsearchDomain cleans the association map on delete. See Notes."}
  CreateVpcEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-p2mx) -- request/response VpcOptions was map[string]string; real wire shape is types.VPCOptions/{SecurityGroupIds,SubnetIds} (request) and types.VPCDerivedInfo (response, same two fields plus unmodeled AvailabilityZones/VPCId -- matches the identical domain-level VPCOptions simplification). A real SDK client always serializes VpcOptions as {SecurityGroupIds:[...],SubnetIds:[...]}, so json.Unmarshal into map[string]string failed on every real call with a security group or subnet -- CreateVpcEndpoint 400'd unconditionally for any non-toy client. Reused the already-correct vpcOptionsRequestJSON/vpcDerivedInfoJSON/toVPCDerivedInfoJSON machinery built for domain-level VPCOptions (handler_domains.go) -- CreateVpcEndpointInput.VpcOptions is the literal same SDK type. Prior wire: ok was false; existing unit tests asserted the broken shape (flat VpcId/SubnetId keys) and were corrected. Proven via a real aws-sdk-go-v2 client round-trip (handler_sdk_roundtrip_test.go), verified to fail against the unfixed code by hand-revert"}
  DescribeVpcEndpoints: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListVpcEndpoints: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-lx5h) — dropped required NextToken (ListVpcEndpointsOutput, deserializers.go). Single-page emulator (never truncated) so no data is lost, but a required pointer left nil could panic a client that dereferences it unconditionally; now always emitted as an empty string. Prior wire: ok was false. gopherstack-4gzs: CORRECTED (this pass) — see the 'Not a bug' note below (now removed), which argued returning the full vpcEndpointJSON shape (Endpoint/VpcOptions included) was harmless. Now emits vpcEndpointSummaryJSON via toVpcEndpointSummariesJSON (handler_vpc_endpoints.go)."}
  ListVpcEndpointsForDomain: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-lx5h) — same required-NextToken gap and fix as ListVpcEndpoints above. Prior wire: ok was false. gopherstack-4gzs: CORRECTED (this pass) — same vpcEndpointSummaryJSON fix as ListVpcEndpoints above."}
  UpdateVpcEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-p2mx) -- same VpcOptions map[string]string bug and fix as CreateVpcEndpoint above. Prior wire: ok was false"}
  DeleteVpcEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-4gzs: CORRECTED (this pass) — DeleteVpcEndpointOutput.VpcEndpointSummary is *types.VpcEndpointSummary (api_op_DeleteVpcEndpoint.go:41-53), same narrower shape as the List ops; was emitting the full vpcEndpointJSON. Now emits vpcEndpointSummaryJSON via toVpcEndpointSummaryJSON."}
  AuthorizeVpcEndpointAccess: {wire: ok, errors: ok, state: ok, persist: ok}
  RevokeVpcEndpointAccess: {wire: ok, errors: ok, state: ok, persist: ok}
  ListVpcEndpointAccess: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-lx5h) — same required-NextToken gap and fix as ListVpcEndpoints (ListVpcEndpointAccessOutput, deserializers.go). Prior wire: ok was false"}
  CreateOutboundCrossClusterSearchConnection: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) -- request/response SourceDomainInfo/DestinationDomainInfo were tagged LocalDomainInfo/RemoteDomainInfo (sibling-copy from this package's own internal OutboundConnection struct); both are required request members with no matching wire key, so a real client's connection was always created with empty domain info on both ends. ALSO -- the response was wrapped in {CrossClusterSearchConnection: ...} like Delete/Accept/Reject, but CreateOutboundCrossClusterSearchConnectionOutput is genuinely flat at the response root (api_op_CreateOutboundCrossClusterSearchConnection.go/deserializers.go:1253); every field was nested one level too deep to ever decode. Prior wire: ok was false on both counts. Sibling InboundConnection already used the correct SourceDomainInfo/DestinationDomainInfo names throughout -- report per this issue's sibling-check instruction"}
  DescribeOutboundCrossClusterSearchConnections: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-6flj) -- same SourceDomainInfo/DestinationDomainInfo rename as Create above. ALSO a routing bug: matchElasticsearchCorePaths used `path == elasticsearchCCSOutbound` (exact match against the bare path), so the real op's path (.../outboundConnection/search) never matched and every real client's call 404'd before reaching the handler at all -- unlike the correctly prefix-matched Inbound sibling. Now `strings.HasPrefix`, matching Inbound's pattern. Prior wire: ok was false"}
  DeleteOutboundCrossClusterSearchConnection: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) -- same SourceDomainInfo/DestinationDomainInfo rename and same routing-prefix fix as the two rows above (.../outboundConnection/{id} also never matched the exact-match core-path check). Prior wire: ok was false"}
  AcceptInboundCrossClusterSearchConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  RejectInboundCrossClusterSearchConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteInboundCrossClusterSearchConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeInboundCrossClusterSearchConnections: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeReservedElasticsearchInstanceOfferings: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeReservedElasticsearchInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  PurchaseReservedElasticsearchInstanceOffering: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "FIXED (2026-09-04 pass) -- never validated ReservedElasticsearchInstanceOfferingId against the known offering; an unknown offering ID silently created a reservation with zero-value InstanceType/FixedPrice/UsagePrice/Duration and 200 OK instead of the modelled ResourceNotFoundException. See Notes."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "GetUpgradeStatus.UpgradeName (gopherstack-6flj, 2026-08-15): real, optional *string member \
     never emitted -- no upgrade-name/upgrade-history state is tracked anywhere in this backend \
     (GetUpgradeHistory always returns empty), so there is no honest source value; fabricating a \
     plausible name would be invented state, not parity."
  - "PackageDetails.AvailablePackageVersion and DomainPackageDetails.PackageVersion/ReferencePath/ \
     LastUpdated (gopherstack-6flj, 2026-08-15): real members with no backing state at all in this \
     backend's Package model (models.go) -- a structural modeling gap, not a value the backend \
     already holds and fails to emit. ErrorDetails on both types already handled the same way \
     (see packageJSON's doc comment)."
  - "Domains never transition through a Processing/creating state -- CreateElasticsearchDomain \
     returns Processing=false / DomainProcessingStatus=Active immediately, and Endpoint is \
     populated synchronously too, so every field a real client would poll on (Processing, \
     DomainProcessingStatus, Endpoint, and DescribeElasticsearchDomainConfig's per-field \
     OptionStatus.State) is self-consistently 'already done'. Re-verified 2026-08-10 \
     (gopherstack-toz8): checked whether any client-visible action (Create, \
     UpdateElasticsearchDomainConfig, Delete) should visibly flip Processing to true -- this \
     backend applies all three synchronously with no async work to represent, so there is \
     nothing for a transient Processing=true to model faithfully; a fake timed delay would be \
     invented state, not parity. Confirmed deliberate simplification, not a stub -- SDK callers \
     that poll DescribeElasticsearchDomain waiting for Processing==false succeed immediately \
     instead of spinning. Separately (not in scope this pass): ElasticsearchDomainStatus.Created/ \
     Deleted (types.go:958-966) are not modeled at all, unlike Processing/DomainProcessingStatus \
     which are (see toDomainStatusJSON)."
  - "VPCOptions.VPCId and .AvailabilityZones are never populated on Describe/domain-status \
     responses -- deriving them would require a cross-service EC2 subnet/VPC lookup this \
     backend does not perform (SubnetIds/SecurityGroupIds are correctly modeled and echoed). \
     Matches services/opensearch's identical, already-accepted simplification. Needs cli.go \
     wiring to close: this service has no reference to any EC2 backend today (grep confirms no \
     ec2 import in services/elasticsearch), so VPCId/AvailabilityZones would need either (a) an \
     EC2 lookup interface (mirroring how services/elasticsearch already takes a DNSRegistrar \
     interface, store_setup.go) that cli.go wires to the real services/ec2 backend when both \
     services are registered, or (b) a shared pkgs/ helper cli.go injects both backends into. \
     Either way the wiring decision belongs in cli.go, which this pass does not touch."
  - "AutoTuneOptions.RollbackOnDisable (types.AutoTuneOptions, Update-only -- it is not a \
     member of the Create-only types.AutoTuneOptionsInput) is not modeled. Not filed as a bd \
     issue this pass: this backend has no rollback state machine to act on it, and it is a \
     narrower field than the two this pass targeted (SAMLOptions/MaintenanceSchedules)."
deferred: []              # this pass's target deferred item (DescribeElasticsearchDomainConfig per-field OptionStatus) is now implemented; remaining edges tracked under gaps above
leaks: {status: clean, note: "no goroutines/janitors in this service; Snapshot/Restore close domain Tags before replacing state (verified in persistence.go). This pass also fixed domainCopy (store.go) to deep-clone AdvancedOptions/VPCOptions/CognitoOptions/AdvancedSecurityOptions/AutoTuneOptions/LogPublishingOptions -- previously AdvancedOptions (and now the five new option fields) were shallow-copied, so a caller mutating the map/slice on a DescribeDomain result would have silently mutated the backend's stored state. Not a resource leak, but a real aliasing bug fixed alongside the new fields it would otherwise have applied to as well. 2026-08-10: extended the same deep-clone treatment to AdvancedSecurityOptions.SAMLOptions (and its Idp pointer) and AutoTuneOptions.MaintenanceSchedules (and each element's Duration pointer), which would otherwise have reintroduced the identical aliasing bug for the newly-added nested pointers/slices."}
---

## Notes

Protocol: **restjson1**. Base path prefix `/2015-01-01/`.

### 2026-08-13 pass (gopherstack-p2mx): first full audit + two real bugs found and fixed

This issue was filed on the premise that `services/elasticsearch` had **no** PARITY.md
at all. That premise was false by the time this pass started: the file already existed
(added by the 2026-07-12/07-24/08-10 passes above, `last_audit_commit: 59ab8f6a`,
graded A) and was current -- it already reflected the `0190c00b0`
NextToken fix on `ListVpcEndpoints`/`ListVpcEndpointAccess`/`ListVpcEndpointsForDomain`.
Rather than skip the pass on that basis, this session treated the existing file as a
baseline to re-verify (per parity-principles.md's own re-audit protocol) instead of
trusting it blind, and found two real bugs the prior passes' route-level and
field-level checks had missed:

1. **`CancelDomainConfigChange` was returning the wrong operation's response shape**
   (`handler_domain_config.go`, previously line ~447) -- a textbook instance of the
   "operation reimplemented as a different operation entirely" bug class. The handler
   called `buildDomainConfigOutput(d)`, which builds `{"DomainConfig": {...}}` --
   the `DescribeElasticsearchDomainConfig`/`UpdateElasticsearchDomainConfig` response
   shape. Real `CancelDomainConfigChangeOutput`
   (confirmed via `awsRestjson1_deserializeOpDocumentCancelDomainConfigChangeOutput`,
   deserializers.go:747) is `{CancelledChangeIds: []string, CancelledChangeProperties:
   []types.CancelledChangeProperty, DryRun: *bool}` -- an entirely different shape with
   no overlapping keys. None of the three real fields are required, so a real SDK
   client wouldn't panic (restjson1 ignores unknown response keys and leaves absent
   optional fields nil) -- but it would silently get `CancelledChangeIds`,
   `CancelledChangeProperties`, and `DryRun` as permanently nil/false regardless of
   what it asked for, and the request's own `DryRun` member was never read at all. The
   existing unit test (`TestElasticsearchHandler_CancelDomainConfigChange`) asserted
   the wrong (bug-matching) shape -- `wantContains: []string{"DomainConfig",
   "ElasticsearchVersion"}` -- so it passed green against the bug, another instance of
   parity-principles.md rule 3. Fixed: the handler now reads `DryRun` from the request
   body and returns the real shape with empty `CancelledChangeIds`/
   `CancelledChangeProperties` (this backend has no pending-config-change queue --
   every change already applies synchronously, so "nothing is ever pending to cancel"
   is the honest answer, not a stub) and `DryRun` echoed from the request. Test
   corrected to assert the real keys; a new `Test_SDKRoundTrip_CancelDomainConfigChange`
   (`handler_sdk_roundtrip_test.go`) drives the real SDK client and asserts `DryRun`
   round-trips, verified to fail against the unfixed code by hand-revert.

2. **`CreateVpcEndpoint`/`UpdateVpcEndpoint`'s `VpcOptions` was `map[string]string`**
   (`models.go`, `vpc_endpoints.go`, `handler_vpc_endpoints.go`) instead of the real
   `types.VPCOptions` shape (`{SecurityGroupIds: []string, SubnetIds: []string}`,
   confirmed via `awsRestjson1_serializeDocumentVPCOptions`, serializers.go:5038, and
   `awsRestjson1_deserializeDocumentVPCDerivedInfo` for the response side,
   deserializers.go:15133). This is a client-breaking bug, not a cosmetic one: a real
   `aws-sdk-go-v2` client always serializes `VpcOptions` as
   `{"SecurityGroupIds":["sg-..."],"SubnetIds":["subnet-..."]}` -- decoding that into
   `map[string]string` fails outright (`json: cannot unmarshal array into Go value of
   type string`), so `CreateVpcEndpoint` 400'd with `ValidationException: invalid JSON
   body` for *every* real caller that supplied a security group or subnet, which is to
   say every real caller. `test/integration/elasticsearch_test.go`'s
   `TestIntegration_Elasticsearch_VpcEndpointList_NextToken` even carried a comment
   noting this as a known, out-of-scope, unfixed bug ("gopherstack-elsewhere") -- it
   was in scope for this pass and is now fixed. The fix reuses the
   `vpcOptionsRequestJSON`/`vpcDerivedInfoJSON`/`toVPCDerivedInfoJSON` machinery
   `handler_domains.go` already built for domain-level `VPCOptions`, since
   `CreateVpcEndpointInput.VpcOptions` is the literal same SDK type
   (`*types.VPCOptions`) -- no new wire-shape modeling was needed, just correcting
   which existing shape this operation used. `models.go`'s `VpcEndpoint.VpcOptions`
   field changed from `map[string]string` to the existing `VPCOptions` model type;
   `vpc_endpoints.go`'s deep-copy helper and `store.go`'s `vpcEndpointCopy` were
   updated to clone the two slices instead of a map. `AvailabilityZones`/`VPCId` on the
   response (`types.VPCDerivedInfo`'s other two members) are left unmodeled, matching
   the identical, already-accepted domain-level VPCOptions simplification (see gaps
   below) -- not a new gap, the same one extended to a second operation pair that
   shares the type. Existing unit tests asserted the broken flat-key shape (`VpcId`,
   `SubnetId` as top-level string values) and were corrected to the real
   `SecurityGroupIds`/`SubnetIds` array shape. A new
   `Test_SDKRoundTrip_CreateVpcEndpoint_VpcOptions` (`handler_sdk_roundtrip_test.go`)
   drives the real SDK client with a real `types.VPCOptions` request and asserts the
   response round-trips both fields; verified to fail against the unfixed code
   (`ValidationException: invalid JSON body`) by hand-revert.

**gopherstack-4gzs: CORRECTED** — this section previously argued, "not a bug,
documented for the next auditor", that `ListVpcEndpoints`/
`ListVpcEndpointsForDomain` returning the full `vpcEndpointJSON` shape
(including `Endpoint` and `VpcOptions`) for every list entry was inert
because restjson1 clients ignore unknown response keys, proven by the
existing SDK round-trip test continuing to pass unmodified. The premise is
true but the conclusion was wrong: `types.VpcEndpointSummary`
(elasticsearchservice@v1.45.4 types/types.go:1911, deserializer at
deserializers.go:15436) is a real, narrower type — only
`DomainArn`/`Status`/`VpcEndpointId`/`VpcEndpointOwner`, no `Endpoint` or
`VpcOptions` — so the superset response was a genuine wire-shape lie
regardless of SDK-client tolerance: a raw-body or non-SDK caller sees a
VPC endpoint's connection address and subnet/security-group IDs leaked
through a list call. The existing SDK round-trip test passing was never
proof of correctness here -- see parity-principles.md's no-stub rule on
why a typed-client pass is not sufficient for a leaked-field bug; only a
raw-body assertion catches it (see `TestElasticsearchHandler_VpcEndpointSummary_NoEndpointOrVpcOptionsLeak`,
handler_vpc_endpoints_test.go). Fixed by emitting a dedicated
`vpcEndpointSummaryJSON` via `toVpcEndpointSummaryJSON`/
`toVpcEndpointSummariesJSON` (handler_vpc_endpoints.go) from
`ListVpcEndpoints`, `ListVpcEndpointsForDomain`, and `DeleteVpcEndpoint`'s
`VpcEndpointSummary` response (`DeleteVpcEndpointOutput.VpcEndpointSummary`
is the same narrower `*types.VpcEndpointSummary`, api_op_DeleteVpcEndpoint.go:41-53
-- this call site was not called out by name in the original "not a bug" note's
heading but was mentioned in its last sentence and had the identical bug).
`vpcEndpointJSON` (full shape, with `Endpoint`/`VpcOptions`) stays reserved for
`CreateVpcEndpoint`/`UpdateVpcEndpoint`/`DescribeVpcEndpoints`, which really do
return the full `types.VpcEndpoint`.

**Route audit reconfirmed, not repeated from scratch**: the bd issue this pass closes
(gopherstack-p2mx) cited a prior route audit (gopherstack-4nek) that traced all 51 ops
in `buildOps()` plus all three prefix-router chains in `handler.go` against the SDK's
`serializers.go` method/path pairs, 51/51 match, zero routing bugs -- see "Route audit
method" below, which predates this pass and was spot-checked (not re-run end-to-end)
against the two ops touched here; both were already correctly routed.

**Bug-class coverage for this pass**: class 3 (borrowed shapes/behaviour) accounted for
both bugs found -- `CancelDomainConfigChange` borrowed a different operation's entire
response shape, and `CreateVpcEndpoint`/`UpdateVpcEndpoint` borrowed the wrong Go type
for a field two operations happen to share with domain-level `VPCOptions`. Spot-checked
for classes 1/2/4 (required-input-never-read, required-output-never-populated,
empty-struct inputs) across `PurchaseReservedElasticsearchInstanceOffering`,
`CreateOutboundCrossClusterSearchConnection`, `AuthorizeVpcEndpointAccess`,
`RevokeVpcEndpointAccess`, the four inbound/outbound connection lifecycle ops,
`UpgradeElasticsearchDomain`, `StartElasticsearchServiceSoftwareUpdate`, and all
no-required-input read-only ops (`GetCompatibleElasticsearchVersions`,
`ListElasticsearchVersions`, `ListElasticsearchInstanceTypes`,
`DescribeElasticsearchInstanceTypeLimits`, `GetPackageVersionHistory`,
`ListDomainsForPackage`, `ListPackagesForDomain`, `DescribeDomainAutoTunes`,
`DescribeDomainChangeProgress`, `GetUpgradeHistory`, `GetUpgradeStatus`,
`CancelElasticsearchServiceSoftwareUpdate`, `DeleteElasticsearchServiceRole`) --
none had unread required inputs, unpopulated required outputs, or `struct{}`-typed
inputs hiding real required members. `CreateElasticsearchDomain`/
`UpdateElasticsearchDomainConfig`/`CreatePackage` were not re-audited field-by-field
this pass (already exhaustively covered by the 2026-07-24/08-10 passes above, files
unchanged since `59ab8f6a`) per the manifest's own re-audit protocol.

### 2026-08-10 pass (gopherstack-toz8 follow-up): SAMLOptions, MaintenanceSchedules, DeploymentStrategyOptions, Package timestamps

Five items were bundled in this follow-up issue; ranked by real-client likelihood and
implemented to full depth where feasible, per parity-principles.md rule 1 (no
half-modeled fields) and this campaign's standing "model faithfully or leave and say
why" rule:

1. **`AdvancedSecurityOptions.SAMLOptions` + `AutoTuneOptions.MaintenanceSchedules`**
   (highest priority -- this is the exact "accepted but silently dropped" bug class
   this campaign targets). Both were previously parsed as `json.RawMessage` purely to
   avoid rejecting the request, then discarded. Now fully modeled:
   - `models.go`: added `SAMLIdp`, `SAMLOptions`, `Duration`, `AutoTuneMaintenanceSchedule`
     types, and `SAMLOptions`/`MaintenanceSchedules` fields on `AdvancedSecurityOptions`/
     `AutoTuneOptions`. Field names/requiredness verified against
     `aws-sdk-go-v2/service/elasticsearchservice@v1.45.4`: `validateSAMLIdp`
     (validators.go:1091-1107, both `EntityId`/`MetadataContent` required whenever `Idp`
     is present) and the `SAMLOptionsInput`/`SAMLOptionsOutput`/`AutoTuneMaintenanceSchedule`/
     `Duration` struct declarations in types.go (all plain structs, none are smithy
     unions). `SAMLOptionsOutput` has no `MasterUserName`/`MasterBackendRole` members, so
     (like the pre-existing `MasterUserOptions` treatment) those two are stored but never
     echoed back, matching real AWS.
   - **Found and fixed a second, deeper bug while wiring this in**: the
     `DescribeElasticsearchDomainConfig`/`UpdateElasticsearchDomainConfig` response's
     `DomainConfig.AutoTuneOptions` field was using the *DomainStatus* response's shape
     (`types.AutoTuneOptionsOutput` -- `State`/`ErrorMessage` only) for its `Options`
     member, and the generic `elasticsearchConfigStatus`/`OptionStatus` shape for its
     `Status` member. Neither is correct: per the pinned SDK,
     `AutoTuneOptionsStatus.Options` is `*types.AutoTuneOptions`
     (`DesiredState`/`MaintenanceSchedules`/`RollbackOnDisable`, types.go:283-300) and
     `AutoTuneOptionsStatus.Status` is `*types.AutoTuneStatus` (types.go:344-371) --
     the *only* DomainConfig field with a non-generic Status shape, confirmed against
     `awsRestjson1_deserializeDocumentAutoTuneOptionsStatus`/`...AutoTuneStatus`
     (deserializers.go:9590-9700). `MaintenanceSchedules` could not be bolted onto the
     old (wrong) shape without perpetuating that bug, so `handler_domain_config.go` now
     has dedicated `domainConfigAutoTuneOptionsJSON`/`autoTuneStatusJSON`/
     `autoTuneConfigValue` types and `toDomainConfigAutoTuneOptionsJSON`/
     `autoTuneConfigStatus` builders. `AutoTuneStatus.State` (`ENABLED`/`DISABLED`,
     `AutoTuneState` enum) maps directly from `DesiredState` -- no
     `ENABLE_IN_PROGRESS`/`DISABLE_IN_PROGRESS` transition window, the same synchronous
     simplification already applied elsewhere in this service. The DomainStatus
     response's own `AutoTuneOptions` (`toAutoTuneOptionsJSON`) was already correct and
     is untouched. A pre-existing unit test
     (`TestElasticsearchHandler_UpdateDomainConfig_SecurityFields`) asserted the old
     (wrong) shape's `State` field inside `Options` and was corrected to assert
     `DesiredState` in `Options` and `State` in `Status` separately -- textbook case of
     parity-principles.md rule 3 ("unit tests are not parity proof").
   - `AutoTuneOptions.RollbackOnDisable` (Update-only, no Create equivalent) is
     deliberately NOT modeled -- see gaps.
2. **`DeploymentStrategyOptions`**: real, simple field (`types.DeploymentStrategyOptions`
   has one required member, `DeploymentStrategy`, enum `Default`/`CapacityOptimized` --
   types/enums.go:130-136), present on `CreateElasticsearchDomainInput`,
   `UpdateElasticsearchDomainConfigInput`, `ElasticsearchDomainStatus` (flat, not
   Status-wrapped), and `ElasticsearchDomainConfig` (Status-wrapped with the generic
   `OptionStatus`, confirmed types.go:861-862/550-561). Added end-to-end with request
   validation and defaults to `"Default"` on the DomainConfig response when unset,
   matching the enum's default value.
3. **`Package.CreatedAt`/`LastUpdatedAt`/`ErrorDetails`**: `CreatedAt`/`LastUpdatedAt`
   are now set at `CreatePackage` and `LastUpdatedAt` advances on `UpdatePackage`
   (epoch-seconds via `pkgs/awstime.Epoch`, matching restjson1). `ErrorDetails`
   (`types.ErrorDetails{ErrorMessage, ErrorType}`) is modeled as a real type but is
   always `nil` in practice: this backend has no `COPYING`/`COPY_FAILED` state machine
   (packages transition straight to `AVAILABLE`), so there is no natural source for it
   -- documented as structural, not left silently absent.
4. **`VPCOptions.VPCId`/`.AvailabilityZones`**: confirmed still blocked on an EC2
   lookup this service has no access to (no `ec2` import anywhere in
   `services/elasticsearch`) -- see gaps for the specific `cli.go` wiring this would
   need. Not implemented this pass per the exclusion on editing `cli.go`.
5. **Domains never reach `Processing`**: investigated what real AWS does (no
   `elasticsearchservice` waiter exists in the pinned SDK -- confirmed no
   `waiters.go` in the module -- so real clients like Terraform's
   `aws_elasticsearch_domain` hand-roll polling against `DescribeElasticsearchDomain`,
   checking `Processing`/`Endpoint`). Checked whether `Endpoint`,
   `DomainProcessingStatus`, and `DescribeElasticsearchDomainConfig`'s per-field
   `OptionStatus.State` are all self-consistently "instantly ready" together with
   `Processing` (they are: `Endpoint` is set synchronously in `CreateDomain`,
   `OptionStatus.State` is always `"Active"`) -- so there is no field a poll loop would
   see as "done" while `Processing` still lied about it. Verdict: legitimate emulator
   simplification, not a stub; see gaps for the full reasoning and the separate,
   out-of-scope `Created`/`Deleted` field gap this surfaced.

Round-trip persistence coverage added: `persistence_test.go`'s
`domain_advanced_options_preserved` case now also exercises SAMLOptions/
MaintenanceSchedules/DeploymentStrategyOptions, and `package_and_association_preserved`
asserts Package CreatedAt/LastUpdatedAt survive Snapshot/Restore. No snapshot version
bump: every new field is `omitempty`/`omitzero`, additive-only.

### 2026-07-24 pass: CreateElasticsearchDomain field-coverage gaps closed

The 2026-07-12 audit marked `CreateElasticsearchDomain`/`DescribeElasticsearchDomain`/
`UpdateElasticsearchDomainConfig`/`DescribeElasticsearchDomainConfig` all
`wire: ok` on the strength of top-level shape/route verification, but never
field-diffed `CreateElasticsearchDomainInput` member-by-member against
`types.CreateElasticsearchDomainInput`. Doing that this pass found five
request/response members that were **entirely unmodeled** (no struct field,
no request parsing, no response echo) despite the earlier audit's `ok`
rating: `VPCOptions`, `CognitoOptions` (a `CognitoOptions{Enabled: false}`
was hardcoded into every response regardless of input), `LogPublishingOptions`,
`AdvancedSecurityOptions`, and `AutoTuneOptions`. This is the same bug class
parity-principles.md rule 4 warns about ("a 'real-looking' op may be a
disguised stub") — `wire: ok` was recorded from route/top-level-shape
checking, not a real field enumeration. Fixed this pass:

- `models.go`: added `VPCOptions`, `CognitoOptions`, `LogPublishingOption`,
  `AdvancedSecurityOptions`, `AutoTuneOptions` types and wired them into
  `Domain`/`CreateDomainInput`/`UpdateConfig`. Also added
  `CreatedAt`/`ConfigUpdatedAt`/`ConfigVersion` to back the
  `DescribeElasticsearchDomainConfig` `OptionStatus` fix below, and a `Tags`
  map on `CreateDomainInput` so `CreateElasticsearchDomainInput.TagList` can
  apply tags atomically at creation (previously only reachable via a
  separate `AddTags` call after create).
- `handler_domains.go` / `handler_domain_config.go`: request parsing,
  response echo, and real AWS-matching validation for all five —
  `CognitoOptions.Enabled=true` requires `UserPoolId`/`IdentityPoolId`/`RoleArn`;
  `AdvancedSecurityOptions.Enabled && InternalUserDatabaseEnabled` requires
  `MasterUserOptions`; `AutoTuneOptions.DesiredState` is validated against the
  `ENABLED`/`DISABLED` enum. `MasterUserOptions`/`SAMLOptions` are parsed
  (for presence/validation) but never persisted or echoed back, matching
  real AWS's own behavior of never returning credentials on any response.
  `VPCOptions.VPCId`/`AvailabilityZones` are left empty (no EC2 subnet
  lookup modeled), matching services/opensearch's identical simplification.
- `store.go`: `domainCopy` now deep-clones every new option field (plus the
  pre-existing `AdvancedOptions` map, which was previously shallow-copied —
  a real aliasing bug where a caller mutating a `DescribeDomain` result's
  map could mutate backend state; fixed alongside the new fields).
- `packages.go` / `handler_packages.go`: `CreatePackage` now requires
  `PackageSource.S3BucketName`/`S3Key` (`ValidationException` if missing),
  matching `CreatePackageInput.PackageSource` being a required member in
  `types.CreatePackageInput`. Previously flagged as a known gap in the prior
  audit; now closed. The value is stored on `Package` but never echoed back
  (`types.PackageDetails` has no `PackageSource` member — confirmed against
  the SDK).
- **Invented-field deletion**: `validPackageTypes` (models.go) accepted
  `"ZIP-PLUGIN"` in addition to `"TXT-DICTIONARY"`. Checked against
  `aws-sdk-go-v2/service/elasticsearchservice/types.PackageType` — its only
  enum value is `PackageTypeTxtDictionary`. `ZIP-PLUGIN` is valid for the
  *separate* OpenSearch Service API (`opensearch` package's
  `types.PackageType` does have it) but not for this legacy
  `elasticsearchservice` API; gopherstack's value had bled over from the
  sibling service. Deleted per the no-invented-fields rule; a
  `handler_packages_test.go` test case asserting `ZIP-PLUGIN` returned 200
  was corrected to assert 400.
- `handler_domain_config.go`: closed the 2026-07-12 pass's explicitly
  deferred item — `elasticsearchConfigStatus` (backing every
  `DomainConfig.*.Status` field) now carries `CreationDate`/`UpdateDate`
  (epoch-seconds via `pkgs/awstime.Epoch`, matching restjson1's
  `unixTimestamp` wire format) and `UpdateVersion`/`PendingDeletion`,
  matching `types.OptionStatus` exactly. This backend tracks one
  domain-wide `CreatedAt`/`ConfigUpdatedAt`/`ConfigVersion` rather than AWS's
  true per-option granularity (documented as a gap, not a stub — the same
  class of deliberate simplification as the Processing/DomainProcessingStatus
  note below). `ConfigVersion` increments and `ConfigUpdatedAt` advances on
  every `UpdateElasticsearchDomainConfig` call that changes at least one
  field, verified by `TestElasticsearchHandler_DomainConfig_OptionStatus`.

### 2026-07-12 pass: route-matcher bugs found and fixed (both are the
"route-matcher" bug class: unit tests calling `h.Handler()(c)` with a
self-consistent but AWS-wrong path, so green tests hid an unreachable real op)

1. **`ListDomainNames` was served at the wrong path.** AWS routes it at
   `GET /2015-01-01/domain` (no `es/` segment) — confirmed directly from
   `aws-sdk-go-v2/service/elasticsearchservice@v1.39.1`'s
   `awsRestjson1_serializeOpListDomainNames` (`serializers.go`). gopherstack
   had it aliased onto `GET /2015-01-01/es/domain` (the *same* path as
   `CreateElasticsearchDomain`'s POST, just a different verb) — a path that is
   not a real AWS Elasticsearch endpoint at all. A real `aws-sdk-go-v2` client
   calling `ListDomainNames` would 404 against gopherstack (the bare
   `/2015-01-01/domain` path wasn't even matched by the service's
   `RouteMatcher`, so the request wouldn't route to this handler in the first
   place). Fixed by:
   - registering `GET /2015-01-01/domain` in `buildOps()` (reusing the
     existing `elasticsearchDomainPackages` constant, which already covered
     `ListPackagesForDomain`'s `/2015-01-01/domain/{name}/packages` — AWS's ES
     API has two sibling top-level resources, `/es/domain` and `/domain`, and
     this service only modeled the first),
   - broadening `matchElasticsearchExtPaths` to match the bare path (it
     previously only matched `/2015-01-01/domain/...` with a trailing
     segment),
   - moving the `ExtractOperation` mapping from `extractRootDomainOperation`
     (GET case removed — a bare GET on `/es/domain` is not a real op) to
     `extractPackageDomainOp`,
   - removing the dead `handleDomainRoutes` GET-root branch.
   `services/elasticsearch/handler.go:104-112` (buildOps),
   `handler.go:171-183` (matcher), `handler.go:399-421` (extractPackageDomainOp),
   `handler.go:454-472` (extractUpgradeOp — see bug 2), `handler.go:552-560`
   (extractRootDomainOperation), `handler.go:902-917` (handleDomainRoutes).

2. **`StartElasticsearchServiceSoftwareUpdate` was served at the wrong
   path.** AWS routes it at `POST /2015-01-01/es/serviceSoftwareUpdate/start`
   (confirmed from the same serializer file) — gopherstack registered it at
   the *bare* `POST /2015-01-01/es/serviceSoftwareUpdate` (no `/start`
   suffix), which is not a real endpoint (its sibling,
   `CancelElasticsearchServiceSoftwareUpdate`, correctly used `/cancel`). A
   real SDK client's `StartElasticsearchServiceSoftwareUpdate` call would
   404. Fixed by adding the `/start` suffix to the `buildOps()` registration
   and the `ExtractOperation` match in `extractUpgradeOp`.

Both bugs were invisible to the existing unit-test suite because the tests
constructed requests with the same (wrong) path the handler expected —
`handler_test.go`, `handler_refinement1_test.go`, and
`handler_stateful_ops_test.go` all called `GET /2015-01-01/es/domain` for
"ListDomainNames" and `POST /2015-01-01/es/serviceSoftwareUpdate` (no
`/start`) for the software-update start call. Those tests were corrected to
use the real paths (`/2015-01-01/domain` and
`/2015-01-01/es/serviceSoftwareUpdate/start` respectively) as part of this
fix, so they now exercise the actual wire contract instead of the internal
(mis-)implementation.

### Route audit method

Every one of the 51 operations in `GetSupportedOperations()` was
cross-checked method-by-method and path-by-path against
`aws-sdk-go-v2/service/elasticsearchservice@v1.39.1`'s `serializers.go`
(`SplitURI(...)` calls + `request.Method = "..."` assignments are the ground
truth for restjson1 wire routing — more reliable than botocore JSON models
for this exercise since it's the exact code the target SDK client runs). The
two bugs above were the only mismatches; every other op's path prefix, path
parameters, and HTTP verb matched gopherstack's `RouteMatcher` /
`ExtractOperation` / dispatch tables exactly.

### Wire-shape spot checks (all confirmed correct)

- `DescribeVpcEndpoints` / `ListVpcEndpoints` response field names
  (`VpcEndpoints`/`VpcEndpointErrors` vs `VpcEndpointSummaryList`) match
  `types.DescribeVpcEndpointsOutput` / `types.ListVpcEndpointsOutput` exactly
  — these are two different shapes for two different operations and
  gopherstack does not conflate them.
- `DescribeDomainChangeProgress`'s `ChangeProgressStatus.Status` field name
  matches `types.ChangeProgressStatusDetails.Status`.
- `UpgradeElasticsearchDomain`, `GetUpgradeStatus`, `GetUpgradeHistory`,
  `DescribeElasticsearchInstanceTypeLimits` top-level response field names
  all match their respective `api_op_*.go` output structs.
- Domain-status JSON nesting (`DomainStatus` wrapper on
  create/describe/delete, `DomainConfig` wrapper with per-field
  `{Options, Status}` on describe/update-config) matches
  `types.ElasticsearchDomainStatus` / `types.ElasticsearchDomainConfig`.

### Locking / persistence

Coarse `lockmetrics.RWMutex` per backend, consistent with the pkgs-catalog
rule (no per-map locks). `Snapshot`/`Restore` are exposed on `Handler` via
straight delegation to `InMemoryBackend`. Domain `tags.Tags` are explicitly
`.Close()`'d before being discarded on `Reset()` and before being replaced on
`Restore()`, avoiding a Prometheus-metric leak.

### "Looks-wrong-but-correct" traps for the next auditor

- `CreateElasticsearchDomain` / `DescribeElasticsearchDomain` always return
  `Processing: false` and `DomainProcessingStatus: "Active"` immediately —
  this looks like the "domain never finishes creating" stub anti-pattern at
  first glance, but it's actually the *opposite* (and correct) choice for an
  emulator: no artificial async delay, so SDK callers that poll
  `DescribeElasticsearchDomain` waiting for `Processing == false` succeed on
  the first call instead of spinning forever.
- `CancelDomainConfigChange`, `CancelElasticsearchServiceSoftwareUpdate`,
  `DescribeDomainAutoTunes`, `DescribeDomainChangeProgress`,
  `GetUpgradeHistory`, `GetUpgradeStatus` all validate the domain exists and
  then return a fixed/empty payload — these are legitimately void-result ops
  in a backend with no async config-change or auto-tune state machine, not
  disguised stubs (confirmed by reading the corresponding backend.go methods
  before flagging, per parity-principles.md rule 4).

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 1 confirmed bug

`DescribeElasticsearchDomainConfig`/`UpdateElasticsearchDomainConfig`: {wire:
fixed} -- buildDomainConfigOutput emitted a flat "ColdStorageEnabled" boolean;
types.ColdStorageOptions (elasticsearchservice@v1.45.4 deserializers.go) wraps
Enabled in a nested object under "ColdStorageOptions" -- there is no flat
member. Proven via
`TestDescribeElasticsearchDomainConfig_ColdStorageOptions_RealClient`
(wire_field_fixes_test.go), hand-reverted/confirmed-failing/restored/
`md5sum`-verified byte-identical.

`DescribeElasticsearchInstanceTypeLimits`'s `LimitsByRole.data` key: rejected,
not a bug -- LimitsByRole is `map[string]types.Limits` keyed by role name
("data" is a real role value), not a struct field; correctly absent from the
SDK's per-key case-switch table by construction.

## 2026-08-29 pass: campaign class audit (constraining parameter never honoured)

Measured 24 Describe/List/Get operations against the pinned SDK
(elasticsearchservice@v1.45.4; verified from the SDK output shape, not the
op name -- e.g. DescribeElasticsearchDomains returns a collection despite
its singular-sounding name and was counted). Four real findings, all fixed:

- **DescribeInboundCrossClusterSearchConnections** and
  **DescribeOutboundCrossClusterSearchConnections**: both declare
  `Filters`/`MaxResults`/`NextToken` (restjson1 JSON-body fields per their
  own `serializeOpDocument` functions), but neither handler read the request
  body at all -- always returned every connection. Fixed with a shared
  generic `describeCrossClusterConnections` (list_filter_params.go) applying
  all five real Filternames each op documents (`cross-cluster-search-
  connection-id`, `source-domain-info.{domain-name,owner-id,region}`,
  `destination-domain-info.domain-name` for inbound; the destination-scoped
  mirror for outbound) plus `pkgs/page` pagination.
- **DescribePackages**: its request struct read a `PackageIDs` key that does
  not exist on the real `DescribePackagesInput` (only `Filters`,
  `MaxResults`, `NextToken` do) -- no real client could ever have populated
  it, so this op was unconditionally unfiltered. Fixed to read `Filters`
  (`Name` in PackageID/PackageName/PackageStatus, `Value` a list) plus
  pagination.
- **ListDomainNames**: ignored its query-bound `engineType` parameter. This
  backend only ever manages Elasticsearch-engine domains (OpenSearch domains
  are the separate `services/opensearch` API), so `engineType=OpenSearch`
  now correctly returns none instead of every domain.

Pagination: no shared helper existed before this pass; `pkgs/page` is now
used for the three Describe/List ops above. The rest of this service's
List/Describe ops (DescribeElasticsearchDomains, ListVpcEndpoints, etc.) take
an explicit ID list rather than paginating, matching their real Input shapes.

Tests: `list_filter_params_test.go`, driven through the real SDK client
(`newTestElasticsearchClient`) -- one test per finding above. All fail
against pre-fix code (confirmed per-file by reverting the relevant handler
before writing the fix).

## 2026-08-31 Error-envelope sweep (gopherstack-uox6, errtargetaudit, post-reachability-fix)

`errtargetaudit -dir elasticsearch` reported 4 class-A findings (`AddTags`,
`RemoveTags`, `DescribeElasticsearchDomains`, `ListDomainNames`, all
`ResourceNotFoundException` via `ErrDomainNotFound`). SDK shape: this module
still uses the older `awsRestjson1_deserializeOpError<Op>` `EqualFold`
cascade (`aws-sdk-go-v2/service/elasticsearchservice@v1.45.4`
deserializers.go). Verified per-op: none of the 4 declare
`ResourceNotFoundException` (`AddTags`/`RemoveTags`/`ListDomainNames`
declare `BaseException`/`InternalException`(not RemoveTags)/`ValidationException`(+`LimitExceededException`
on AddTags only); `DescribeElasticsearchDomains` declares
`BaseException`/`InternalException`/`ValidationException`).

**All 4 are false positives -- the "consumed downstream" mechanism**
(distinct from unreachable-branch, per gopherstack-uox6's third documented
defect): in every case the sentinel genuinely fires from the backend, but
the HTTP handler's own error handling discards or repurposes it before any
error-mapper ever runs.

- `AddTags`/`RemoveTags` (`handler_tags.go` `handleAddTags`/`handleRemoveTags`):
  the backend call's error is discarded outright --
  `_ = h.Backend.AddTags(ctx, req.ARN, tagMap)` /
  `_ = h.Backend.RemoveTags(...)` -- both handlers always write
  `http.StatusOK` regardless. `ErrDomainNotFound` is never even inspected,
  let alone mapped to a wire code. This is a real, separate bug (tagging a
  nonexistent domain silently "succeeds") but it is outside this sweep's
  class (wrong-code-for-declared-operation) since no code is ever emitted
  at all -- left unfixed, flagged here for a future pass. FIXED by the
  2026-09-04 pass below (`gopherstack-to9j`); see also the 2026-09-07 entry
  (`gopherstack-8h57`), filed against this same deferral note without
  noticing the fix already shipped.
- `ListDomainNames` (`handler_domains.go` `handleListDomainNames`): calls
  `Backend.DescribeDomain` per name and does `if err != nil { continue }` --
  the error is consumed inside the loop and never reaches any response
  path.
- `DescribeElasticsearchDomains` (`handler_domains.go`
  `handleDescribeElasticsearchDomains`): captures `descErr` but never
  forwards it to an error mapper; instead it hand-writes
  `ErrorType: "ResourceNotFoundException"` into the per-item
  `UnprocessedDomains[].ErrorDetails` field of a `200 OK` response --
  which is the documented real-AWS partial-failure shape for this
  operation (per-domain not-found is reported inline, not as a top-level
  exception), so the emitted string is correct AWS behavior even though it
  never touches the sentinel-to-wire-code mapper the tool is checking.

No code changed. Measured false-positive rate for this service: 4/4
(100%), consistent with this campaign's calibration point of a
100%-false-in-one-service pass.

Gates: `go build ./services/elasticsearch/...`, `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/elasticsearch/...`
(pass, unchanged). No files changed in this pass.

## 2026-09-04 pass: dropped-error and never-honoured-parameter sweep, 3 real bugs fixed

Sentinel-reachability sweep (`git grep` every `errors.go` sentinel outside
`errors.go`/handlers): all 9 sentinels are returned by backend logic and
wired into a handler's error mapping -- no orphaned sentinel found.

Three real bugs found and fixed, all previously-undiscovered except the
first (flagged but explicitly left unfixed by the 2026-08-31 pass above):

- **AddTags/RemoveTags** (`handler_tags.go`): both handlers discarded the
  backend's `ErrDomainNotFound` (`_ = h.Backend.AddTags(...)`) and always
  wrote `200 OK` for an ARN that does not resolve to any domain. Neither
  op's deserializer (elasticsearchservice@v1.45.4 deserializers.go,
  `awsRestjson1_deserializeOpErrorAddTags` /
  `awsRestjson1_deserializeOpErrorRemoveTags`) declares
  `ResourceNotFoundException` -- only `BaseException`/`InternalException`/
  `ValidationException` (+`LimitExceededException` on AddTags) -- so the fix
  maps the unknown-ARN case to `ValidationException` (400), matching
  `services/opensearch/handler_tags.go`'s identical fix for the same
  sibling API (its comment cites the same deserializer fact). A second,
  related bug in `handleAddTags`: `existing, _ := h.Backend.ListTags(...)`
  returns a nil map for an unknown ARN, and the old code did
  `maps.Copy(existing, tagMap)` -- copying a non-empty `tagMap` into a nil
  destination map panics. Fixed by copying into a freshly allocated
  `merged` map instead of mutating `existing` in place, which incidentally
  also closes the panic. Test:
  `TestElasticsearchHandler_AddRemoveTags_UnknownARN`
  (handler_tags_test.go). Fail-before (neutered each handler's guard
  independently, one at a time): both show `Not equal: expected: 400,
  actual: 200`.
- **ListPackagesForDomain** (`packages.go`) never validated that the domain
  existed at all -- `ListPackagesForDomainOutput`'s deserializer
  (elasticsearchservice@v1.45.4 deserializers.go,
  `awsRestjson1_deserializeOpErrorListPackagesForDomain`) declares
  `ResourceNotFoundException` among its modelled errors, but the backend
  had no domain lookup and always returned whatever (possibly empty)
  association list it found, with `200 OK`. Compounding this: deleting a
  domain (`DeleteDomain`, `domains.go`) never removed it from
  `packageAssociationsStore` -- an association-map ghost row -- so a
  deleted domain's packages would keep listing it via
  `ListDomainsForPackage` forever, and `ListPackagesForDomain` against the
  now-gone name would keep returning its stale package list instead of
  404ing. Fixed both: `ListPackagesForDomain` now checks domain existence
  first and returns `ErrDomainNotFound` (-> 404); `DeleteDomain` now prunes
  the deleted domain's name out of every package's association slice.
  Tests: `TestElasticsearchHandler_ListPackagesForDomain_UnknownDomain`,
  `TestElasticsearchHandler_DeleteDomain_ClearsPackageAssociations`
  (handler_packages_test.go). Fail-before (neutered each guard
  independently): the domain-existence check shows `expected: 404, actual:
  200`; the association-cleanup loop shows `Should be empty, but was
  [map[DomainName:ghost-assoc-dom ...]]`.
- **PurchaseReservedElasticsearchInstanceOffering** (`reserved_instances.go`)
  never validated `ReservedElasticsearchInstanceOfferingId` against the one
  real offering (`offer-t3-small-1y`) this backend models.
  `PurchaseReservedElasticsearchInstanceOfferingOutput`'s deserializer
  declares `ResourceNotFoundException`, but an unrecognized offering ID
  silently created a reservation with zero-value
  `InstanceType`/`FixedPrice`/`UsagePrice`/`Duration` and `200 OK` instead.
  Fixed: returns the new `ErrOfferingNotFound` sentinel (-> 404) when no
  offering matches. Test:
  `TestElasticsearchHandler_PurchaseReservedInstanceOffering_UnknownOffering`
  (handler_reserved_instances_test.go). Fail-before: `expected: 404,
  actual: 200`.

Not fixed this pass (lower confidence / out of the two cheap-check
mandate): `PurchaseReservedElasticsearchInstanceOfferingInput.ReservationName`
is also `This member is required` per its doc comment, but the real SDK
client only validates it is non-nil (`validators.go`), not non-empty, and a
well-behaved client always supplies a non-nil string -- weaker evidence
than the offering-ID case, not filed as a bug.

Delete/Update precondition pass: read every Delete*/Update*/Cancel* op doc
comment in the pinned SDK for "you must first"/"cannot be"/"must not be"
wording -- none of `DeleteElasticsearchDomain`, `DeletePackage`,
`DeleteInboundCrossClusterSearchConnection`,
`DeleteOutboundCrossClusterSearchConnection`, `DeleteVpcEndpoint`, or
`DissociatePackage` document any such precondition beyond the one already
enforced (`DeleteElasticsearchServiceRole` / `ErrServiceRoleInUse`, see
top of Notes). No new precondition guards added -- absence of doc wording
is evidence against inventing one.

Gates: `GOTOOLCHAIN=go1.26.6 go build ./services/elasticsearch/...`,
`GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/elasticsearch/...`,
`GOTOOLCHAIN=go1.26.6 golangci-lint run ./services/elasticsearch/...` --
all clean. `ListPackagesForDomain`'s signature gained an `error` return;
confirmed via `git grep` it has no callers outside this package, so no
cross-service gate was needed.

Not checked this pass (package too large to read in full -- see the
audit's scope-discipline instruction): performance (O(n) scans under
write lock, per-call allocations), resource leaks/goroutines beyond a
grep-level check of `domain_lifecycle.go` (no goroutines/timers found
there), and a full re-read of `UpdateElasticsearchDomainConfig`'s
merge-vs-replace semantics (prior passes already audited this in depth;
no local drift found via `git diff <last_audit_commit>..HEAD -- \
services/elasticsearch/domain_config.go`).

## 2026-09-07 pass: gopherstack-8h57 re-verified as already fixed

`gopherstack-8h57` re-filed the AddTags/RemoveTags discarded-error bug from
the 2026-08-31 sweep's deferral note above, without noticing the note's own
next section (2026-09-04 pass, `gopherstack-to9j`) had already fixed it.
`handler_tags.go` at HEAD does not discard either error:

```go
if addErr := h.Backend.AddTags(ctx, req.ARN, tagMap); addErr != nil {
	h.writeError(r, w, http.StatusBadRequest, "ValidationException", addErr.Error())
	return
}
```

and the equivalent `removeErr` check in `handleRemoveTags`. `git log` on the
file confirms this: commit `cff501069` (2026-09-04) is already an ancestor
of the current branch tip.

Re-ran the declared-error-set extraction (`awsRestjson1_deserializeOpError<Op>`
EqualFold cascade, per this service's older SDK shape; the pattern returns a
list here, not an empty match, confirming it still applies):

```
AddTags:    "UnknownError" "BaseException" "InternalException" "LimitExceededException" "ValidationException"
RemoveTags: "UnknownError" "BaseException" "InternalException" "ValidationException"
```

Neither declares `ResourceNotFoundException` -- confirms `ValidationException`
(400) remains the right mapping, unchanged from the 2026-09-04 fix.

Re-verified by neutering each guard independently (reverting to
`_ = h.Backend.AddTags(...)` / `_ = h.Backend.RemoveTags(...)`, one at a
time): both compile and both make
`TestElasticsearchHandler_AddRemoveTags_UnknownARN`'s corresponding subtest
fail with `expected: 400, actual: 200`, then restored to HEAD (`diff` against
`git show HEAD:services/elasticsearch/handler_tags.go` confirmed a clean
restore). Existing coverage already meets the full standard: unknown-ARN
gets 400/ValidationException for both ops
(`TestElasticsearchHandler_AddRemoveTags_UnknownARN`), and an existing
domain's AddTags/RemoveTags still succeeds with the tags readable back via a
subsequent ListTags (`TestElasticsearchHandler_Tags`,
`add_and_list_tags`/`remove_tag` subtests). No pre-existing test asserted
200 for the unknown-domain case -- nothing was pinning the old bug.

`handleListDomainNames`'s `if err != nil { continue }` (per-name
`DescribeDomain` inside `ListDomainNames`) is unchanged and is a distinct
case, decided separately: a name can only reach that loop by first coming
back from `Backend.ListDomainNames`, so the only way `DescribeDomain`
subsequently 404s is a delete racing between the two calls in the same
request -- not a caller naming a domain that never existed. Skipping it
(omitting the vanished entry) is defensible AWS-shaped behavior for a
list-then-describe race, not the same bug as AddTags/RemoveTags discarding
an error for an ARN that never resolved. No change made.

No code changed this pass -- `gopherstack-8h57` is a duplicate of the
already-shipped `gopherstack-to9j` fix.

Gates: `GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/elasticsearch/...`,
`GOTOOLCHAIN=go1.26.6 golangci-lint run ./services/elasticsearch/...` --
both clean (see command output in the issue's closing report).
