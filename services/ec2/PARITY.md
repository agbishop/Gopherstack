---
service: ec2
sdk_module: aws-sdk-go-v2/service/ec2@v1.329.0   # version audited against (go.mod pin; previously recorded as "see go.mod", never a parseable pin)
last_audit_commit:                                # unknown: pass was instructed not to commit and had no git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-08-30
overall: A   # unrecorded-Describe/List sweep, second pass (this pass, fix/wrapper-key-sweep
             # branch): regenerated the prior pass's "18 remaining" list from scratch --
             # grepped both dispatch-table registration forms (`ops["OpName"] = h.handleOpName`
             # and the map-literal `"OpName": h.handleOpName`), restricted to Describe*/List*,
             # non-test files only -- 192 such registrations. `LC_ALL=C comm -23` against every
             # (Describe|List)[A-Za-z]+ token appearing anywhere in this file's prose returned
             # only 3 names (DescribeTransitGatewayConnectPeers/PeeringAttachments/RouteTables),
             # not 16 or 18 -- a false negative: the prior pass's own note names those three via
             # a slash-joined "DescribeTransitGatewayConnects/ConnectPeers/PeeringAttachments/
             # RouteTables" sentence that a whole-token regex can't split, so they read as
             # "already named" even though they're genuinely covered elsewhere (confirmed still
             # real, in wire_field_fixes_ec2sweep36_test.go). Mechanical comm-diffing doesn't
             # work here because this file's prose names virtually every op somewhere (including
             # in "not yet audited" sentences) -- "named" isn't "recorded as verified". Fell back
             # to reading the prior pass's own explicit "18 of the 37 remain genuinely unreached
             # this pass (not audited)" sentence directly: it lists 20 raw names, 4 of which
             # (DescribeIamInstanceProfileAssociations, DescribeLaunchTemplates,
             # DescribeLaunchTemplateVersions, DescribePrincipalIdFormat) already had a bug fixed
             # and were parenthetically excluded from the "not audited" framing even though still
             # listed -- leaving 16, which matches the task's given list exactly (not 18; trusting
             # the regenerated/cross-checked list per instructions, and saying so). Audited all
             # 16: DescribeAggregateIdFormat, DescribeInstanceEventNotificationAttributes,
             # DescribeFpgaImageAttribute, and DescribeNetworkInterfaceAttribute were CONFIRMED
             # CORRECT (the first two take no parameters beyond DryRun on the real wire --
             # nothing to misread; FpgaImageAttribute/NetworkInterfaceAttribute both correctly
             # read their scalar FpgaImageId/NetworkInterfaceId + Attribute keys and switch on the
             # real enum, returning only the matching block, with NetworkInterfaceAttribute's
             # groupSet/associatePublicIpAddress gap already honestly documented in-code).
             # DescribeIpamPools/DescribeIpamScopes/DescribeAwsNetworkPerformanceMetricSubscriptions/
             # DescribeExportTasks/DescribeVpnConcentrators all correctly read their FlatKey ID
             # lists but never read their declared Filters -- NOT fixed: unlike every other target
             # below, the pinned SDK's generated doc comment for these five gives no per-filter
             # name at all ("One or more filters." / "the filters for the export tasks." / "One or
             # more filters to limit the results."), so implementing named filter matching here
             # would mean fabricating filter semantics never verified against the wire; recorded as
             # a real, deliberately-unfixed gap rather than faked. FIXED 7 unread-Filters bugs, all
             # confirmed to fail against pre-fix code first, all with SDK-doc-enumerated filter
             # names AND backend fields to back them: (1) DescribeClassicLinkInstances (group-id,
             # vpc-id, tag: -- ClassicLinkInstance already tracks Groups/VpcID).
             # (2) DescribeSecondaryNetworks (owner-id, secondary-network-id/-arn, state, type,
             # ipv4-cidr-block-association.*, tag:). (3) DescribeSecondarySubnets (owner-id,
             # secondary-network-id/-type, secondary-subnet-id/-arn, state,
             # ipv4-cidr-block-association.*, tag:). (4) DescribeSecondaryInterfaces (owner-id,
             # status, secondary-interface-id/-arn/-type, secondary-network-id/-type,
             # secondary-subnet-id, attachment.instance-id,
             # private-ipv4-addresses.private-ip-address, tag:). (5) DescribeServiceLinkVirtualInterfaces
             # (owner-id, outpost-lag-id, outpost-arn, state, vlan,
             # service-link-virtual-interface-id, tag:). (6) DescribeInstanceSqlHaHistoryStates
             # (haStatus, sqlServerLicenseUsage, tag:). (7) DescribeImageUsageReportEntries
             # (account-id, resource-type, creation-time with the documented "*" day-wildcard).
             # tag-key is documented on all seven but deliberately left unimplemented, matching
             # this file's pre-existing convention (no existing applyXxxFilters function
             # implements tag-key either). New code: handler_filters.go gained
             # applyClassicLinkInstanceFilters/applySecondaryInterfaceFilters/
             # applySecondaryNetworkFilters/applySecondarySubnetFilters/
             # applyServiceLinkVirtualInterfaceFilters/applySQLHaHistoryFilters/
             # applyImageUsageReportEntryFilters (+ an anyContains helper for list-membership
             # filters), wired into the five ops' existing handlers
             # (handler_vpc_config.go/handler_secondary_net.go/handler_sql_ha.go/
             # handler_image_ops.go); no Backend interface signature changed. No new Go-type
             # mismatches found across any of the 16 -- every scalar/list/enum already matched its
             # real wire type. New tests: wire_field_fixes_ec2sweep42_test.go (7 real-client
             # tests, one per fix, each confirmed to fail against pre-fix code by reverting the
             # handler/filter files to HEAD and re-running before restoring the fix). Gates:
             # build/vet/race all clean; `golangci-lint run` initially flagged cyclop (2,
             # decomposed the two ipv4-cidr-block-association.* switches into small per-field
             # helpers) and goconst (5, extracted filterKeyOwnerID/filterKeySecondaryNetID/
             # filterKeyResourceType/filterKeyAttachInstanceID constants, reusing
             # filterKeyResourceType in the pre-existing handler_tags.go too) and golines (1, in
             # the new test file) -- all fixed without nolints; 0 issues on the final run,
             # verified as caused by this pass's own new code (the flagged lines were all in this
             # pass's new functions/file). Repo-wide `go vet ./...` also run since instructed to
             # for this scope (no signature changed, so not strictly required) -- confirms 0
             # ec2-related findings; the acmpca build failures it surfaces are pre-existing and
             # out of this pass's scope (another agent's target directory).
             # ---- prior pass's note follows ----
             # unrecorded-Describe/List sweep (this pass, gopherstack wrapper-key-sweep branch):
             # regenerated the list of implemented Describe/List operations this file had never
             # recorded as verified (grepped every "OpName": h.handleOpName / ops["OpName"] =
             # h.handleOpName dispatch-table registration, stripped anything ending in "Response",
             # subtracted names already named anywhere in this file -- 37 operations, not the 45
             # a prior estimate used). Covered 19 of the 37 with real aws-sdk-go-v2-client-driven
             # tests asserting decoded response content (not just err==nil). FIXED 8 previously
             # unread/misread request parameters, all confirmed to fail against pre-fix code first:
             # (1) DescribePrincipalIdFormat read "PrincipalArn", a key that does not exist
             # anywhere on DescribePrincipalIdFormatInput (api_op_DescribePrincipalIdFormat.go --
             # the op always describes the calling principal), while never reading the real
             # Resource.N filter that IS on the wire (serializeOpDocumentDescribePrincipalIdFormatInput
             # FlatKey "Resource"); every resource type's ID-format status always came back
             # regardless of the filter. Backend.DescribePrincipalIDFormat's signature changed from
             # (principalARN string) to (resources []string) -- it now delegates straight to the
             # pre-existing DescribeIDFormat(resources) rather than discarding its argument; no
             # external callers (repo-wide grep + go vet ./... both confirmed). (2)
             # DescribeIamInstanceProfileAssociations unconditionally read Filter.1.Value.1 as the
             # instance-id filter value BEFORE checking whether Filter.1.Name was actually
             # "instance-id" -- a lone "state" filter sent as Filter.1 (both "instance-id" and
             # "state" are real, documented filters here) was misread as an instance-id filter
             # matching no real instance, silently dropping every association; also implements the
             # "state" filter itself (previously entirely unhandled) as a post-hoc filter over the
             # existing IamInstanceProfileAssociation.State field, no backend signature change. (3)
             # DescribeLaunchTemplates read only LaunchTemplateName.N, silently ignoring
             # LaunchTemplateId.N even though both are separately FlatKey-declared on the wire --
             # a client filtering by specific template IDs got every template back unfiltered. (4)
             # DescribeLaunchTemplateVersions read only the scalar LaunchTemplateId, ignoring the
             # alternative LaunchTemplateName identifier (a real client identifying the template by
             # name alone always got "LaunchTemplateId is required") and the Versions/MinVersion/
             # MaxVersion range parameters entirely; now resolves LaunchTemplateName via the
             # existing DescribeLaunchTemplates(names) path and filters the (structurally always
             # single) returned version against Versions/MinVersion/MaxVersion -- this backend
             # stores one version snapshot per template, not a real multi-version history, so the
             # filter is applied to that one item rather than a real per-version data set (documented
             # in gaps). (5) DescribeImageUsageReports ignored its url.Values entirely -- ReportId.N
             # and ImageId.N are both real FlatKey lists on the wire
             # (serializeOpDocumentDescribeImageUsageReportsInput) but were never read; now filtered
             # post-hoc against the existing UsageReport.ReportID/ImageID fields. (6)
             # DescribeVpcEndpointServices also ignored its url.Values entirely -- ServiceName.N is
             # a real FlatKey list; requesting specific service names always returned the full
             # static per-region catalogue. ServiceRegion.N/Filters remain undocumented gaps: this
             # backend has no per-service attribute catalogue or cross-region service data to filter
             # against. (7)/(8) DescribeCustomerGateways/DescribeVpnGateways never read Filters at
             # all (declared on the wire, CustomerGateway/VpnGateway structs already carry
             # State/Type/BgpAsn/IPAddress/AttachedVPCID/AttachmentState) -- added
             # applyCustomerGatewayFilters/applyVpnGatewayFilters (handler_filters.go, following the
             # file's existing applyXxxFilters convention) covering every real documented filter this
             # backend has backing data for (bgp-asn/customer-gateway-id/ip-address/state/type/tag:
             # and attachment.state/attachment.vpc-id/state/type/vpn-gateway-id/tag: respectively);
             # amazon-side-asn/availability-zone/tag-key are documented but not tracked by these
             # structs, left unimplemented rather than fabricated. CONFIRMED CORRECT (real
             # ID-filter/decoded-response assertions, not err==nil): DescribeAccountAttributes,
             # DescribeDeclarativePoliciesReports, DescribeVpcClassicLink (singular VpcId.N) and
             # DescribeVpcClassicLinkDnsSupport (plural VpcIds.N -- the field name predicts neither
             # direction, confirmed both ways in the same pass), DescribeVpcPeeringConnections,
             # DescribeIpams, DescribeVpcEncryptionControls, DescribeFpgaImages,
             # DescribeInstanceSqlHaStates. PARITY.md CORRECTION: DescribeTransitGatewayConnects/
             # ConnectPeers/PeeringAttachments/RouteTables were already fixed (TransitGatewayAttachmentIds.N
             # / TransitGatewayConnectPeerIds.N / TransitGatewayRouteTableIds.N plural keys) and
             # covered by real-client tests in wire_field_fixes_ec2sweep36_test.go -- this file
             # simply never recorded them as verified; re-ran those 4 tests this pass to confirm
             # they still pass, no new test written (would have been a pure duplicate). 18 of the 37
             # remain genuinely unreached this pass (not audited): DescribeAggregateIdFormat,
             # DescribeAwsNetworkPerformanceMetricSubscriptions, DescribeClassicLinkInstances,
             # DescribeExportTasks, DescribeFpgaImageAttribute, DescribeIamInstanceProfileAssociations
             # (request-param bug fixed above; response-shape/other params not re-audited beyond
             # that), DescribeImageUsageReportEntries, DescribeInstanceEventNotificationAttributes,
             # DescribeInstanceSqlHaHistoryStates, DescribeIpamPools, DescribeIpamScopes,
             # DescribeLaunchTemplates/DescribeLaunchTemplateVersions (bugs fixed above; Filters
             # parameter itself not re-audited), DescribeNetworkInterfaceAttribute,
             # DescribePrincipalIdFormat (bug fixed above; MaxResults/NextToken not audited),
             # DescribeSecondaryInterfaces, DescribeSecondaryNetworks, DescribeSecondarySubnets,
             # DescribeServiceLinkVirtualInterfaces, DescribeVpnConcentrators. Gates: build/vet
             # (repo-wide, since Backend.DescribePrincipalIDFormat's signature changed)/race/
             # golangci-lint all 0 issues, no banned nolints; new test file
             # wire_field_fixes_ec2sweep41_test.go.
             # ---- prior pass's note follows ----
             # write-only-state sweep (this pass, targeted): ModifyInstancePlacementInput.GroupName
             # was a plain string guarded by != "" (not *string like the real SDK's
             # ModifyInstancePlacementInput, api_op_ModifyInstancePlacement.go), whose doc says
             # "To remove an instance from a placement group, specify an empty string (\"\")" --
             # a client's documented, explicit clear was silently dropped, leaving the instance in
             # its old placement group. Now *string with a nil check (instance_attrs.go). Response
             # side (instancePlacementItem.GroupName, handler_instances_lifecycle.go) intentionally
             # kept `xml:"groupName,omitempty"` -- most instances never touch a placement group at
             # all, and stripping omitempty would put a spurious empty <groupName/> tag on every
             # DescribeInstances response for the overwhelmingly common case, trading a rare-clear
             # edge case for a much larger deviation from real AWS's shape. Round-trip test:
             # wire_field_fixes_test.go (TestModifyInstancePlacement_GroupNameCanBeCleared).
             # ---- prior pass's note follows ----
             # 2026-08-07 pass (gopherstack-8pce follow-up): re-verified the tag dual-storage
             # consolidation and TGW/NAT/VPC-endpoint field-diffs claimed by the passes below
             # are real and still hold (read the code directly against the pinned SDK, not just
             # the notes) -- confirmed, all still correct. Found and fixed one more real,
             # previously-undetected instance of the exact drift bug class this ticket targets:
             # DescribeKeyPairs's `tag:` filter looked tags up under a synthetic "keypair-"+Name
             # key that CreateTags/setTagsLocked never wrote to (tags are stored under the bare
             # key pair Name, its only real identifier) -- the filter silently never matched any
             # tag a real client set. Also closed the pre-existing gaps-list item "DescribeKeyPairs
             # does not implement IncludePublicKey": added KeyPairId/KeyType/CreateTime/TagSet/
             # IncludePublicKey to DescribeKeyPairs and CreateKeyPair/ImportKeyPair (field-diffed
             # against the installed aws-sdk-go-v2/service/ec2@v1.319.1 KeyPairInfo deserializer
             # and CreateKeyPair/ImportKeyPair Output deserializers directly), and wired create-time
             # TagSpecifications for both ops (previously silently discarded). KeyType is real, not
             # fabricated: "rsa" for CreateKeyPair (the only type this backend generates) and
             # inferred from the imported OpenSSH public key's algorithm for ImportKeyPair,
             # falling back to "rsa" only when the material doesn't parse (this backend never
             # validated PublicKeyMaterial before this pass either, so an empty/malformed value is
             # a pre-existing possibility, not a new gap). ED25519 key generation on CreateKeyPair
             # and the PPK KeyFormat are NOT modeled (real backing data would need a real
             # PPK-format encoder or ed25519 keygen; scoped out, see gaps). Interface signature
             # changes: Backend.CreateKeyPair gained a `tags map[string]string` param;
             # Backend.ImportKeyPair gained the same. Both call sites in
             # services/cloudformation/resources_ec2_network.go (AWS::EC2::KeyPair) updated to
             # pass nil (that resource creator does not read a Tags property from the CFN
             # template at all -- pre-existing, unrelated gap, not touched). New test:
             # TestKeyPairWire (key_pairs_wire_test.go, 6 wire-level cases via postForm)
             # proving create-time tags, post-create CreateTags, KeyPairId/
             # KeyType wire shape, IncludePublicKey on/off, and the tag-filter fix itself (this
             # last case would fail under the old "keypair-"+Name key). Moved one item from gaps
             # to a new structural_gaps section: DescribeApplicationStatus's StatusSince/
             # ApplicationStatusDetail (per-check result timestamps/breakdown) requires actually
             # running HTTP health checks against instance-hosted applications over real network
             # traffic this mock has none of -- genuinely underivable, not merely unbuilt (see
             # structural_gaps below); the rest of that gaps-list item (HealthCheckPaths,
             # AvailabilityZoneId, request-size limits, NextToken truncation) stays in gaps
             # unchanged since none of those need anything a mock backend structurally cannot
             # have. 0 regressions: full services/ec2 suite green under -race; go build/go vet/
             # gofmt/golangci-lint run ./services/ec2/... 0 issues; no banned nolints. Did NOT
             # re-derive the full TGW/NAT/VPC-endpoint field-diffs from scratch this pass (the
             # 2026-07-30 pass below already did that work and this pass spot-checked rather than
             # repeated it); did not touch ENI security groups, EBS DataEncryptionKeyId, or the
             # ~12-family MaxResults/NextToken completeness gap -- all unchanged, still real,
             # still in gaps below.
             #
             # 2026-07-30 pass (parity-5): closed the four areas the 2026-07-31 pass below
             # explicitly left UNAUDITED (EBS snapshot lineage, ENI attach/detach edge cases,
             # pagination internals beyond tags/instances, and the wider TGW route-table
             # surface — search/export/announcements). All four turned up real bugs, several
             # severe; every field-diff was against the installed aws-sdk-go-v2/service/ec2
             # types/serializers/deserializers, not this backend's own output. Full detail in
             # the family notes below; highlights: (1) CreateVolume never read SnapshotId at
             # all — "restore a volume from a snapshot", the most basic EBS lineage operation,
             # silently created an empty, disconnected volume with no error; now validates the
             # snapshot exists, defaults/enforces size from it, and inherits
             # Encrypted/KmsKeyID. (2) TerminateInstances unconditionally DELETED every ENI
             # attached to the terminated instance regardless of how it got attached; real AWS
             # only deletes the launch-time primary ENI (DeleteOnTermination=true by default) —
             # an ENI created via CreateNetworkInterface and attached later has
             # DeleteOnTermination=false by default and survives termination, merely detaching
             # (the well-known "leftover ENI" AWS behaviour). Two existing tests
             # (TestTerminateInstances_DeletesAttachedENIs,
             # TestTerminateInstances_OnlyDeletesAttachedENIs) explicitly encoded the deletion
             # bug as expected behaviour and were corrected in place. (3) CreateTransitGatewayRoute
             # and ReplaceTransitGatewayRoute accepted any attachment ID with zero existence
             # validation and hardcoded the route's ResourceType to "vpc" regardless of the
             # attachment's real kind (the same bug class already fixed for
             # Associate/DisassociateTransitGatewayRouteTable in the prior pass);
             # ReplaceTransitGatewayRoute was additionally a disguised upsert, silently
             # *creating* a route for any destination CIDR instead of requiring one to already
             # exist; neither op honoured the real Blackhole flag at all. One existing test
             # (TestHandlerTGWRoutes) created a route with no attachment ID and expected
             # success and was corrected. (4) DescribeSnapshots and DescribeNetworkAcls
             # implemented pagination with a plain, unauthenticated integer offset instead of
             # the HMAC-signed opaque token (pkgs/page) every other paginated describe op here
             # already correctly uses — a forged/malformed NextToken was silently accepted
             # instead of rejected; switched both to the same HMAC pattern. See families below
             # (ebs_snapshot_lineage, eni_attach_detach, pagination, transit_gateway) for full
             # per-op detail, honest not-modeled gaps, and gate output. 0 regressions
             # (services/ec2 and services/cloudformation full suites green; race, vet, gofmt,
             # golangci-lint 0 issues, no banned nolints).
             #
             # PRIOR PASS (2026-07-31 follow-up, gopherstack-8pce), held at B, unchanged below:
             # the severe bugs that caused the
             # 2026-07-30 downgrade (TGW ID collision, TGW ID-filter wire-name bug,
             # VpcEndpoint state field-name bug — see below) were already fixed before this
             # pass started; verified by reading the code directly, not just trusting the
             # note. This pass re-verified the tag-storage 2-file non-migration is still
             # legitimate (sql_ha.go's fabricated Tags field stays deleted; trunk_enclave.go's
             # Tags field still genuinely cannot be migrated per the real API — re-checked
             # against the installed SDK, not just re-reading the prior note), found the
             # RestoreImageFromRecycleBin deferred-list entry was STALE (already fixed by an
             # earlier commit, 2d47b51d4, before the note was written — added the missing test
             # coverage), and fixed two more real, previously-undetected bugs: (1)
             # DeleteQueuedReservedInstances was unconditionally deleting ANY Reserved Instance
             # ID handed to it and returning a bare {Return: true}, when real AWS only ever
             # deletes a target genuinely in the "queued" state and refuses an active one — now
             # reports real per-ID SuccessfulQueuedPurchaseDeletions/FailedQueuedPurchaseDeletions
             # and only deletes queued-state targets. (2) AssociateTransitGatewayRouteTable
             # accepted any attachment ID with no existence check and hardcoded
             # ResourceType="vpc" for every association regardless of the attachment's real
             # kind; found while field-diffing the TGW route-table association/propagation
             # surface the prior pass explicitly left unaudited. A related helper,
             # transitGatewayAttachmentExistsLocked, was also missing the Client VPN attachment
             # map entirely (added by a later parity-4 pass but never wired into this
             # pre-existing helper), so a real Client VPN attachment was wrongly reported
             # not-found by every caller of that helper. HELD AT B, not raised: this pass's TGW
             # field-diff covered only the association/propagation existence+resource-type path,
             # not the full route-table search/export surface or TransitGatewayRouteTableAnnouncement
             # wire shape; and EBS snapshot lineage, ENI attach/detach edge cases, and pagination
             # internals beyond tags/instances remain completely UNAUDITED, carried over unchanged
             # from the prior pass — these are real, unclosed unknowns, not merely deferred
             # busywork, so the grade stays at B until they are actually checked. See deferred.
             #
             # ORIGINAL 2026-07-30 pass (gopherstack-8pce) downgraded from A. Consolidated the
             # embedded-vs-shared Tags dual-storage bug across 9 resource files onto the shared
             # tag store, and field-diffed Transit Gateway / NAT Gateway / VPC Endpoints against
             # the installed SDK's types/serializers/deserializers. All three families were
             # genuinely "ok"-graded going in and all three turned up real bugs, several severe:
             # CreateTransitGateway generated a deterministic ID ("tgw-" + accountID[:8]) so a
             # SECOND transit gateway created by the same account silently overwrote the first
             # in the backend table; DescribeTransitGateways read a request parameter
             # ("TransitGatewayIds.TransitGatewayId.N") that no real AWS client ever sends (the
             # real wire name is "TransitGatewayIds.N"), so a real client's ID filter was
             # silently ignored and every transit gateway was always returned; CreateTransitGateway
             # was a disguised stub that read only Description and discarded Options.*/
             # TagSpecifications entirely; DeleteTransitGateway returned a bare boolean instead of
             # the real API's TransitGateway object; VpcEndpoint's wire item used the wrong field
             # name ("vpcEndpointState" instead of "state") for the endpoint's own state (a real,
             # different type -- VpcEndpointConnection -- does use vpcEndpointState, which is
             # presumably how the mix-up happened); NatGateway/VpcEndpoint were both missing
             # tags/vpcId(NAT)/ownerId/payerResponsibilitySet from the wire entirely despite being
             # taggable and having the backing data. See families below for what was fixed vs
             # documented-not-invented. 0 regressions (services/ec2 and services/cloudformation
             # full suites green), all gates green (build/vet/race/gofmt/golangci-lint 0 issues/no
             # banned nolints).
protocol: ec2-query (AWS query -> XML)
families:
  parity4_new_ops: {status: ok, note: "16 ops added 2026-07-25 (gopherstack, SDK bump eb437919a). TGW Client VPN Attachments (Accept/Reject/Delete) wired into a new tgwClientVpnAttachments table, created implicitly by CreateClientVpnEndpoint's TransitGatewayConfiguration.TransitGatewayId (validates the TGW exists — ErrTransitGatewayNotFound — rather than accepting any ID); also plugged into the existing DescribeTransitGatewayAttachments unified-view aggregator as resourceType client-vpn. AttachImageWatermark/DetachImageWatermark validate the AMI via the existing lookupImageLocked helper (stub + dynamic images) and mutate a real per-image imageWatermarks map; idempotent re-attach. CreateCapacityReservationCancellationQuote/DescribeCapacityReservationCancellationQuotes read real CapacityReservation instance-count/state rather than fabricating data; CancellationTerms is honestly always-empty (this backend does not model commitment-duration billing — documented, not a stub). DescribeAccountVpcEncryptionControl/ModifyAccountVpcEncryptionControl add the account-level singleton alongside the pre-existing per-VPC VpcEncryptionControl, reusing its VpcEncryptionControlExclusions shape; caught and fixed a mode-validation bug during test-writing where the account-level Modify call was checking against the per-VPC mode vocabulary (monitor/enforce) instead of its own (unmanaged/attempt-monitor/attempt-enforce). DescribeIpamPoolAllocations is a genuine new op, not an alias of the pre-existing GetIpamPoolAllocations — its real Input has no IpamPoolId (cross-pool describe by allocation ID only), confirmed against the SDK's DescribeIpamPoolAllocationsInput; ModifyIpamPoolAllocation likewise takes no IpamPoolId. Added IpamPoolAllocation.ResourceRegion (present on the real type, was missing) surfaced on both the old and new ops. GetCapacityManagerMonitoredTagKeys/UpdateCapacityManagerMonitoredTagKeys extend CapacityManagerState with a real per-tag-key map (activate synchronously to activated, deactivate to the terminal suspended state); EarliestDatapointTimestamp always omitted, consistent with GetCapacityManagerMetricData's pre-existing 'no ingested data' honesty. GetManagedResourceVisibility/ModifyManagedResourceVisibility is a new small account-level family (managed_resource_visibility.go + handler_managed_resource_visibility.go), default hidden matching real AWS. ModifyVpcEndpointPayerResponsibility is distinct from the pre-existing (and disguised-stub) ModifyVpcEndpointServicePayerResponsibility — targets a VpcEndpoint (not a Service), added a correctly-scoped new sentinel ErrVpcEndpointIDNotFound (InvalidVpcEndpointId.NotFound) rather than reusing the already-mismapped ErrVpcEndpointNotFound. All persisted: 3 new store.Table-registered resources get Snapshot/Restore for free via the registry; imageWatermarks/accountVpcEncryptionControl/managedResourceDefaultVisibility explicitly wired into backendSnapshot + restoreParity4Fields. 8 new table-driven tests plus a dedicated persistence round-trip test (TestPersistence_Parity4Fields) covering every family."}
  tags:          {status: ok, note: FIXED — existence/type table covered only 9 of ~100 taggable types; now full (AMIs/snapshots/NACLs/TGW/VPN/endpoints/launch-templates/IPAM...). See backend_resource_types.go}
  instance_attrs:{status: ok, note: FIXED disguised stub — disableApiTermination/Stop, ebsOptimized, sourceDestCheck, instanceInitiatedShutdownBehavior were accepted-but-not-modeled; now persisted (sourceDestCheck on primary ENI). DescribeInstanceAttribute was hardcoded}
  instance_lifecycle: {status: ok, note: FIXED StateReason/StateTransitionReason wire shape (were absent); disableApiTermination/Stop now enforced (OperationNotPermitted); RunInstances launch attrs applied}
  sg_rules:      {status: ok, note: PROVEN correct — protocol/port/ICMP bounds, CIDR, dup detection match AWS}
  filters:       {status: ok, note: PROVEN — AND-across-names/OR-within-values, tag: prefix}
  storage_layer: {status: ok, note: RE-AUDITED — Parity sweep 3 (ce30166a) converted 147/153 backend maps map[string]*T -> pkgs/store.Table[T] via data-driven registerAllTables (store_setup.go); ~1150 access sites rewritten. Reviewed keyFn correctness (compiler-enforced per-type, cannot mismatch and still build), Snapshot/Restore version-guard round-trip, Reset->registry.ResetAll(), secondary-index rebuild after Restore, composite-key helpers (coipCidrKey/ipamResourceCidrKey/localGatewayRouteKey/networkPerformanceSubscriptionKey) for Put/Get/Delete consistency. No delete-during-live-iteration (All() returns a copied slice; no .Range() usage in ec2). 0 defects found — purely mechanical, gates green}
  vpc_lifecycle: {status: ok, note: FIXED gopherstack-b5m — DeleteVpc/DeleteSubnet force-cascade-deleted dependents; now match real AWS DependencyViolation semantics (no cascade; caller must remove dependents first). New vpcDependencyViolationLocked/subnetDependencyViolationLocked helpers check subnets/non-default SGs/route tables/attached IGWs/egress-only IGWs/NAT gateways/network ACLs/VPC endpoints (VPC) and ENIs/NAT gateways/VPC endpoints (subnet). CreateVpc now auto-creates a default SG per VPC (previously only the hardcoded default VPC had one — every CreateVpc'd VPC had zero SGs, a latent gap); the default SG is excluded from the dependency check and auto-deleted with the VPC, matching AWS. DeleteRouteTable now blocks (DependencyViolation) while it has subnet associations. DeleteInternetGateway now blocks while attached; AttachInternetGateway now enforces AWS's 1:1 IGW<->VPC invariant (Resource.AlreadyAssociated on either side). Rewrote ~10 cascade-assuming tests across cleanup_test.go/persistence_test.go/handler_vpcs_test.go to the new semantics; added dedicated DependencyViolation/AlreadyAssociated tests. CFN callers (services/cloudformation/resources_extended.go) pass real errors through unchanged — full CFN test suite verified green (dependency ordering there was already correct).}
  nat_gateway_addr: {status: ok, note: FIXED disguised stub — AssociateNatGatewayAddress took an allocationID param and silently discarded it (`_ string`); DisassociateNatGatewayAddress didn't even read AssociationId from the wire. Neither mutated real state nor matched the AWS AllocationId.N/AssociationId.N request shape. Rewrote both as real ops: NatGateway now has a SecondaryAddresses []NatGatewayAddress field (AllocationID/AssociationID/PrivateIP/PublicIP) populated/removed by Associate/Disassociate, plus AssociationID on the primary address; DeleteNatGateway now recycles every private IP it holds (primary + secondary EIP associations + secondary private IPs — previously only the primary leaked). Wire responses now return the real NatGatewayAddressSet (AllocationId/AssociationId/PublicIp/PrivateIp/IsPrimary) instead of an empty stubResponse.}
  tag_cleanup:   {status: ok, note: FIXED — systematic sweep (regex + resourceTypePrefixes/resourceExistsLocked cross-check) found ~50 Delete* backend methods across the newer op families (transit gateway, VPN, IPAM, verified access, traffic mirror, network insights, local gateway, route server, capacity manager, secondary networking, etc.) that removed the resource from its table but left an orphaned entry in the shared b.tags map — a real leak reachable via CreateTags (resourceExistsLocked recognizes all these ID prefixes) followed by delete. Added delete(b.tags, id) to every affected op. Deliberately did NOT touch delete ops on composite-key sub-entries (routes, policy/metering-policy entries, ENI permissions, CIDR entries within a pool) — those were never independently taggable (absent from resourceTypePrefixes/resourceExistsLocked), so adding a tag-map delete there would be a no-op at best and a latent bug (deleting the wrong parent's tags) at worst if the composite key ever collided with a real ID.}
  tag_dual_storage: {status: ok, note: "FIXED (gopherstack-8pce, 2026-07-30) — consolidated the embedded-vs-shared Tags dual-storage bug onto the single shared b.tags store (setTagsLocked helper in tags.go) for 9 of the 11 flagged files: local_gateway.go (VIF/VIFGroup), secondary_net.go (SecondaryNetwork/SecondarySubnet/SecondaryInterface/OutpostLag/ServiceLinkVirtualInterface — the last 3 also had TagSet entirely missing from the wire, not just dual-stored), vpn_concentrator.go, vpc_config.go (VpcBlockPublicAccessExclusion), ip_pools.go (CoipPool/Ipv4Pool/Ipv6Pool), capacity_family.go's four implementation files (CapacityReservationFleet/CapacityBlock/CapacityManagerDataExport/CapacityReservationCancellationQuote — the last of these also required adding a missing 'crcq-' entry to resourceTypePrefixes/resourceExistsGatewayLocked, since it was never registered as taggable at all despite the real AWS ResourceType enum having 'capacity-reservation-cancellation-quote'), declarative_policies.go, host_reservations.go, mac_hosts.go (MacModificationTask — TagSet was entirely missing from the wire, a real gap beyond dual-storage). Each migrated resource has a wire-level test (postForm/dispatchHandler) proving a create-time TagSpecification tag AND a post-creation CreateTags tag are BOTH visible through the resource's own Describe AND through the generic DescribeTags. TWO files deliberately NOT migrated: sql_ha.go's RegisteredSQLHaInstance.Tags field was a pure fabrication (never set anywhere, no tags param on EnableInstanceSQLHaStandbyDetections, and the real AWS RegisteredInstance response type has no Tags field at all) — deleted rather than migrated. trunk_enclave.go's TrunkInterfaceAssociation.Tags field cannot be migrated: real AWS's AssociateTrunkInterfaceInput has no TagSpecifications parameter at all, and there is no 'trunk-interface-association' entry in the real ResourceType enum, so the generic CreateTags path could never target 'trunk-assoc-' IDs even if registered — there is only one write path (the mock's own fabricated create-time tags parameter), so no drift is possible; migrating would mean inventing a ResourceType the real API doesn't have. Left as-is and documented, not touched."}
  transit_gateway: {status: ok, note: "FIELD-DIFFED (gopherstack-8pce, 2026-07-30). FIXED: (1) CreateTransitGateway generated a deterministic ID ('tgw-' + accountID[:8]) instead of a unique one — a second CreateTransitGateway call by the same account silently overwrote the first transit gateway in the backend table; now uuid-based like every other resource. (2) DescribeTransitGateways read the request ID filter from a param name no real client sends ('TransitGatewayIds.TransitGatewayId.N'); the real wire name (confirmed against the SDK's TransitGatewayIdStringList serializer) is 'TransitGatewayIds.N' — the filter was silently a no-op, always returning every transit gateway. (3) CreateTransitGateway was a disguised stub: it read only Description and discarded Options.* (AmazonSideAsn/AutoAcceptSharedAttachments/DefaultRouteTableAssociation/DefaultRouteTablePropagation/DnsSupport/MulticastSupport/SecurityGroupReferencingSupport/VpnEcmpSupport/TransitGatewayCidrBlocks) and TagSpecifications entirely; now a CreateTransitGatewayParams struct threads all of these through, with real AWS's documented defaults (AmazonSideAsn=64512, AutoAcceptSharedAttachments=disable, DefaultRouteTableAssociation/DefaultRouteTablePropagation/DnsSupport/VpnEcmpSupport=enable, MulticastSupport/SecurityGroupReferencingSupport=disable) applied when the caller doesn't override. (4) Added the previously-absent TransitGatewayArn and CreationTime fields (real fields, real backing data). (5) DeleteTransitGateway returned a bare `<return>true</return>` instead of the real API's DeleteTransitGatewayOutput shape (`<transitGateway>...</transitGateway>` with State='deleting'); fixed. (6) TagSet was entirely absent from the wire despite 'tgw-' being taggable; wired via the same setTagsLocked/TagsForResource pattern as the tag_dual_storage fixes. DOCUMENTED, NOT MODELED (no backing data — this mock does not auto-create a default transit gateway route table on CreateTransitGateway): Options.AssociationDefaultRouteTableId/PropagationDefaultRouteTableId are left empty rather than fabricated. Transit gateway route-table association/propagation state-machine edge cases beyond the pre-existing Enable/DisableTransitGatewayRouteTablePropagation remain otherwise unaudited. UPDATE (gopherstack-8pce, 2026-07-31 follow-up): field-diffed the association half of that remaining surface and found two more real bugs, both fixed — see the deferred-list entry for AssociateTransitGatewayRouteTable/DisassociateTransitGatewayRouteTable and transitGatewayAttachmentExistsLocked below. Route-table search/export ops and TransitGatewayRouteTableAnnouncement wire-shape remain unaudited. UPDATE (parity-5, 2026-07-30 pass): field-diffed that exact remaining surface (search/export/announcements) against types.TransitGatewayRoute/TransitGatewayRouteAttachment/TransitGatewayRouteTableAnnouncement. FIXED: (a) CreateTransitGatewayRoute and ReplaceTransitGatewayRoute accepted ANY attachmentID with zero existence validation and hardcoded the rendered ResourceType to 'vpc' regardless of the attachment's real kind (same bug class as the Associate/DisassociateTransitGatewayRouteTable fix above) — now validated via transitGatewayAttachmentExistsLocked and derived via tgwAttachmentResourceLocked; also added the previously entirely-missing ResourceId field (real, on TransitGatewayRouteAttachment). (b) ReplaceTransitGatewayRoute was a disguised upsert — it silently CREATED a route for any destination CIDR handed to it instead of requiring an existing one (real AWS: InvalidRoute.NotFound if absent); fixed, reusing the existing ErrRouteNotFound sentinel (also corrected DeleteTransitGatewayRoute's not-found case, which was misusing ErrInvalidParameter for the same situation). (c) Neither Create nor Replace honoured the real Blackhole flag at all (silently discarded, param never read); both now support it (state=blackhole, no attachment, attachmentID ignored). (d) TransitGatewayRouteTableAnnouncement was missing PeerTransitGatewayId (real field; real backing data — derived from the peering attachment's Requester/AccepterTransitGatewayID, whichever side isn't the route table's own TGW) and TagSet (despite 'tgw-rtb-ann-' already being taggable); CreateTransitGatewayRouteTableAnnouncement never parsed TagSpecifications. All fixed. AUDITED-CLEAN: SearchTransitGatewayRoutes and ExportTransitGatewayRoutes themselves — filter names/semantics, route wire shape (once (a)/(b)/(c) above were fixed), and the exported S3 URL format all match the real API. One existing test (TestHandlerTGWRoutes) created a route with no attachment ID at all and asserted success, encoding the (a) bug as expected behaviour — corrected in place; new tests TestHandlerTGWRoute_Validation and TestHandlerTGWRoute_BlackholeAndResourceFields added. DOCUMENTED, NOT FIXED: real AWS marks SearchTransitGatewayRoutesInput.Filters as required; this mock accepts nil/empty filters and returns everything, matching the same 'no-ID-list-means-everything' convention every other Describe* method here uses, and an existing test (TestTGWPeripherals_SearchTransitGatewayRoutes) already exercises nil filters as valid for the internal Go API — left unchanged since enforcing this is only meaningful at the wire/handler layer and the gap is low severity."}
  ebs_snapshot_lineage: {status: ok, note: "FIELD-DIFFED (parity-5, 2026-07-30), against types.Snapshot/SnapshotInfo/Volume and Create/CopySnapshot(s)/CreateVolume serializers+deserializers — the area the 2026-07-31 pass above explicitly left unaudited. FIXED, severe: CreateVolume never read the real CreateVolumeInput.SnapshotId parameter at all (not even parsed) — 'restore a volume from a snapshot', the most fundamental EBS volume<->snapshot lineage operation, silently created an empty volume completely disconnected from the snapshot, with no error and no size inheritance. Now: CreateVolume(az, volType, size, snapshotID) validates the snapshot exists (ErrSnapshotNotFound), defaults Size to the snapshot's VolumeSize when Size is 0, rejects an explicit Size smaller than the snapshot's VolumeSize (InvalidParameterValue, matching real AWS), and inherits Encrypted/KmsKeyID from the snapshot (a volume created from an encrypted snapshot is always encrypted in real AWS). Added Volume.SnapshotID and rendered snapshotId (real field, confirmed via CreateVolumeOutput/Volume deserializers) on CreateVolume and DescribeVolumes responses. services/cloudformation's AWS::EC2::Volume resource creator updated to read+pass through its own real SnapshotId property (previously discarded since the backend method didn't accept one). FIXED, wire-shape: Encrypted and KmsKeyId were tracked on every Snapshot but never rendered on ANY snapshot response (CreateSnapshot/CreateSnapshots/CopySnapshot/DescribeSnapshots); OwnerId (trivially b.AccountID, same pattern as the vpc_endpoints/nat_gateway fixes) was never set or rendered at all; TagSet was never rendered despite 'snap-' already being a taggable resource type with real backing tag data, and CreateSnapshot/CreateSnapshots/CopySnapshot never parsed TagSpecifications at all (create-time tagging silently discarded). All fixed; confirmed CreateSnapshots' response item is real AWS's distinct SnapshotInfo type (not Snapshot) which genuinely has no KmsKeyId field, so KmsKeyId was correctly NOT added there. DOCUMENTED, NOT MODELED: DataEncryptionKeyId — the field real AWS docs describe as literally defining snapshot/volume 'lineage' (snapshots sharing it belong to the same lineage) — has no backing concept in this mock (no per-encryption-operation data key distinct from KmsKeyID); fabricating one would violate the no-fabrication rule, so left absent rather than invented. AMI-backing-snapshot protection (real AWS blocks deleting a snapshot that backs a registered AMI's root device, InvalidSnapshot.InUse) is also not modeled: this mock's AMI type tracks no block-device-mapping/snapshot-reference at all, so there is no backing data to check against. FIXED (2026-08-12, gopherstack-9q6f query-wire audit): CreateVolume's explicit (non-snapshot-inherited) KMS key path read the wire under the wrong, case-mismatched key `vals.Get(\"KmsKeyID\")` instead of the real CreateVolumeInput field `KmsKeyId` (ec2@v1.319.1 serializers.go:73596) -- since query-protocol parsing is a case-sensitive exact-string map lookup, this silently dropped every caller-supplied customer-managed KMS key on a fresh encrypted volume (falling back to the AWS-managed alias). The sibling `ModifyEbsDefaultKmsKeyId` handler already read the correct key; only CreateVolume had the conflation. Repo-wide grep for the same all-caps-acronym class (`vals.Get(\"...ID\")`/`...ARN`/`...URL`/`...KMS...`)  found no other instances in ec2."}
  eni_attach_detach: {status: ok, note: "FIELD-DIFFED (parity-5, 2026-07-30) against types.NetworkInterface/NetworkInterfaceAttachment/NetworkInterfaceAttachmentChanges and Attach/Detach/CreateNetworkInterface(Input/Output) — the area the 2026-07-31 pass above explicitly left unaudited. FIXED, severe: TerminateInstances unconditionally DELETED every ENI attached to the terminated instance regardless of how it got attached, with a comment claiming this 'mirrors AWS behaviour'. It does not: real AWS's per-attachment DeleteOnTermination flag defaults true ONLY for the primary interface auto-created at instance launch; an interface created separately via CreateNetworkInterface and attached later via AttachNetworkInterface defaults DeleteOnTermination=false and SURVIVES termination, merely detaching back to 'available' — the well-documented 'leftover ENI' AWS behaviour real users hit. Confirmed via aws-sdk-go-v2 types.NetworkInterfaceAttachment.DeleteOnTermination and types.NetworkInterfaceAttachmentChanges (the ModifyNetworkInterfaceAttribute Attachment.DeleteOnTermination mechanism that controls it). Added NetworkInterface.DeleteOnTermination (true for the launch-created primary ENI at both RunInstances/store.go and the SpotFleet launch path in spot_fleet.go, false — the real default — for AttachNetworkInterface), a new Backend.SetNetworkInterfaceDeleteOnTermination method plus Attachment.AttachmentId/Attachment.DeleteOnTermination wire support in ModifyNetworkInterfaceAttribute, and rewired TerminateInstances to branch on it (delete+recycle IPs only when true; otherwise detach in place, preserving the ENI, its tags, and its VPC index). TWO EXISTING TESTS explicitly encoded the deletion bug as expected behaviour — TestTerminateInstances_DeletesAttachedENIs ('verifies terminating an instance removes all ENIs attached to it, preventing ENI accumulation' — the literal opposite of real AWS's intentional behaviour here) and TestTerminateInstances_OnlyDeletesAttachedENIs — corrected in place (renamed TestTerminateInstances_DetachesNonLaunchENIs / TestTerminateInstances_OnlyAffectsOwnENIs), plus a new TestTerminateInstances_DeletesLaunchENI proving the launch-ENI-still-deletes half wasn't broken by the fix. FIXED, wire-shape: OwnerId and TagSet were entirely absent from every ENI response despite 'eni-' already being taggable, and CreateNetworkInterface never parsed TagSpecifications at create time; fixed via the same tagItemsFromMap/parseTagSpecification pattern as the nat_gateway/vpc_endpoints fixes (new NetworkInterface.OwnerID, set from b.AccountID at every creation site). AttachNetworkInterfaceOutput was missing NetworkCardIndex (real field on the real output type); added as 0 — this mock never models multi-network-card instance types, so 0 is the accurate default, not a fabrication. DOCUMENTED, NOT MODELED (larger, separate feature gap, not attach/detach specific): ENIs have no security-group tracking at all in this backend — CreateNetworkInterface's real Groups parameter and ModifyNetworkInterfaceAttribute's real Groups parameter are both silently ignored, and the wire response's Groups list is always empty. Not touched this pass: adding full per-ENI security-group modeling is a materially larger, separate feature (unlike the DeleteOnTermination fix, there is no existing partial/broken implementation to correct — the concept is entirely absent), and is a real, standalone gap worth a dedicated future pass rather than folding into this one."}
  pagination: {status: ok, note: "AUDITED (parity-5, 2026-07-30) — every NextToken-parsing describe op in services/ec2 beyond DescribeInstances/DescribeInstanceTypes/DescribeImages/DescribeTags (already correct going in). Found and FIXED: DescribeSnapshots and DescribeNetworkAcls (handler_deepdive_ops.go) both implemented pagination with a plain, unauthenticated integer offset as NextToken (fmt.Sscan straight into the offset variable, silently discarding a parse failure via `_, _ =` and falling back to offset 0) instead of the HMAC-signed opaque token (pkgs/page.EncodeHMACToken/DecodeHMACToken + ErrInvalidPaginationToken) that DescribeInstances/DescribeInstanceTypes/DescribeImages already correctly use — a forged, tampered, or simply malformed NextToken was silently accepted (falling back to page 1) instead of rejected, an inconsistency with this codebase's own established, deliberately-built pagination-hardening convention. Switched both to the identical HMAC pattern used by the other three ops; extended the existing TestPagination_ForgedTokenRejected table test (persistence_test.go) with describe_snapshots/describe_network_acls cases, and added TestHTTP_DescribeSnapshots_Pagination (snapshots_test.go) proving real, non-forged multi-page NextToken round-tripping across 7 snapshots/5-per-page still works correctly after the switch. AUDITED, NOT MODELED (documented, systemic, out of scope for this pass): a wide set of newer op families (capacity block/manager/reservation-fleet/ops, declarative-policies, host-reservations, ipam, network-performance, vpc-config, vpc-encryption-control, vpn-concentrator) declare a NextToken field on their response XML types but implement no MaxResults/NextToken parsing or truncation at all — every call always returns every matching result in one page. This is a size-cap-enforcement completeness gap across roughly a dozen op families (no incorrect data is ever returned, unlike the forged-token bug above), materially larger in scope than a single-pass fix and left as a real, honestly-documented remaining gap for a future, dedicated pagination-completeness pass."}
  nat_gateway: {status: ok, note: "FIELD-DIFFED (gopherstack-8pce, 2026-07-30). FIXED: (1) vpcId was completely absent from the wire item despite the backend already tracking ngw.VPCID — added. (2) connectivityType was absent; this mock only ever creates public NAT gateways (CreateNatGateway always requires a real AllocationId, which is the defining trait of a public gateway), so 'public' is now rendered — real, not fabricated. (3) availabilityZone was absent from each NatGatewayAddress item; now derived from the gateway's subnet (real backing data). (4) TagSet/CreateTags-at-create-time were entirely absent — CreateNatGateway didn't even call parseTagSpecification despite 'nat-' already being taggable via the generic CreateTags path; wired the same as the other fixes this pass. DOCUMENTED, NOT MODELED (no backing data): private NAT gateways (ConnectivityType=private, no AllocationId) are still not modeled; CreateNatGatewayInput's PrivateIpAddress override, SecondaryAllocationIds, SecondaryPrivateIpAddressCount, and SecondaryPrivateIpAddresses at create time are still not honored (callers must use the existing separate AssociateNatGatewayAddress/AssignPrivateNatGatewayAddress calls after creation instead); FailureCode/FailureMessage/DeleteTime/RouteTableId (regional-NAT-gateway-only) and the AttachedAppliances/AutoProvisionZones/AutoScalingIps/AvailabilityMode proxy-appliance/multi-AZ fields remain unmodeled — none of this mock's code paths produce a failed or regional NAT gateway, so there is no backing data to report."}
  vpc_endpoints: {status: ok, note: "FIELD-DIFFED (gopherstack-8pce, 2026-07-30). FIXED: (1) VpcEndpoint's own State field was rendered under the wrong wire tag — `<vpcEndpointState>` — when the real field name (confirmed against the SDK's VpcEndpoint deserializer) is plain `<state>`; a distinct type, VpcEndpointConnection, genuinely does use vpcEndpointState, which is the likely source of the mix-up. A real client parsing this mock's CreateVpcEndpoint/DescribeVpcEndpoints response would never see the endpoint's state. (2) OwnerId was completely absent from the wire despite being trivially derivable (b.AccountID) — added, backed by a new VpcEndpoint.OwnerID field set at creation. (3) PayerResponsibilitySet was completely absent even though the backend already stores real PayerResponsibilityEntry data via ModifyVpcEndpointPayerResponsibility — wired to the wire item, reusing the existing payerResponsibilityEntryItem type. (4) TagSet/CreateTags-at-create-time were entirely absent — CreateVpcEndpoint didn't call parseTagSpecification despite 'vpce-' already being taggable; wired via the handler-level CreateTags-after-create pattern (matching CreateVpc/CreateSubnet/CreateSecurityGroup) rather than changing CreateVpcEndpoint's backend signature, since ~13 test call sites and no external callers made a signature change unnecessarily risky for the same result. (5) ModifyVpcEndpointServicePayerResponsibility — flagged as a disguised stub in the 2026-07-25 pass (payerResponsibility argument declared `_ string` and discarded, always returning success without mutating anything) — is now a real op: VpcEndpointServiceConfig gained a PayerResponsibility field, mutated and rendered on DescribeVpcEndpointServiceConfigurations. DOCUMENTED, NOT MODELED (no backing data): DnsEntries, Groups (security groups), Ipv4Prefixes/Ipv6Prefixes, NetworkInterfaceIds, PolicyDocument, PrivateDnsEnabled, DnsOptions, LastError/FailureReason, ResourceConfigurationArn, ServiceNetworkArn/ServiceRegion (PrivateLink-managed-services / cross-region features) remain unmodeled — this backend does not track ENIs, security groups, or IAM policy documents against a VpcEndpoint, so there is nothing real to report for these fields."}
  key_pairs: {status: ok, note: "phantom-triage pass (parity-5, 2026-07-31): 'ExportKeyPair' was advertised in GetSupportedOperations() AND dispatched (Action=ExportKeyPair), but is not a real EC2 operation — real AWS exposes public-key material for a key pair via DescribeKeyPairs with IncludePublicKey=true (types.KeyPairInfo.PublicKey), not a separate action. gopherstack's DescribeKeyPairs does not implement IncludePublicKey (see gaps). Deleted the fabricated action/handler/backend-method/interface-entry outright (no real op was already wired to redirect it to, unlike the transit-gateway fix below) rather than delisting-only, since it was never reachable by any genuine AWS SDK client — Action=ExportKeyPair does not exist on the real client, so nothing a real client could send is lost. Also removed: 'ModifyTransitGatewayAttribute', a near-miss duplicate of the already-correctly-wired real op ModifyTransitGateway (same Description-only semantics, same backing store) — deleting it changes nothing reachable by a real client, ModifyTransitGateway already covers it. See TestModifyTransitGateway (handler_transit_gateways_test.go) for the real op's existing coverage. UPDATE (gopherstack-8pce, 2026-08-07): closed the IncludePublicKey gap this note flagged, found and fixed a real tag-storage-key drift bug (the DescribeKeyPairs tag: filter looked tags up under a key CreateTags never wrote to), and added KeyPairId/KeyType/CreateTime/TagSet — see the top-of-file pass note for full detail."}
  tgw_policy_table_entries: {status: ok, note: "NEW (2026-08-05, SDK bump ec2 v1.317->v1.319.1, gopherstack-8pce follow-up): implemented Create/Modify/DeleteTransitGatewayPolicyTableEntry, the 3 of the 13 newly-exposed ops in this family. A prior pass's GetTransitGatewayPolicyTableEntries doc comment claimed 'Real AWS exposes no API to create policy table entries directly' — that was true when written but is now WRONG: the v1.319 bump adds exactly that API. Corrected the comment and GetTransitGatewayPolicyTableEntries itself, which previously validated the table existed and always returned an empty list; it now returns the real stored entries (was a disguised, now-incorrect stub given the new Create op — caught by the 'a resource created by a Create operation must be visible to the matching Describe' rule). New backend.TransitGatewayPolicyTableEntry model + tgwPolicyTableEntries store.Table, keyed policyTableID+ruleNumber (mirrors the pre-existing tgwMeteringPolicyEntries pattern exactly). Field-diffed against the installed SDK's serializers.go/deserializers.go/validators.go: wire params are flat (PolicyRule.SourceCidrBlock/SourcePortRange/DestinationCidrBlock/DestinationPortRange/Protocol/MetaData.MetaDataKey/MetaDataValue, TargetRouteTableId, PolicyRuleNumber, TransitGatewayPolicyTableId), response element names are policyRuleNumber/targetRouteTableId/state/policyRule (nested destinationCidrBlock/destinationPortRange/metaData/protocol/sourceCidrBlock/sourcePortRange) — all lowerCamelCase, ISO8601 timestamps (this op has none). CreateTransitGatewayPolicyTableEntry validates TransitGatewayPolicyTableId/PolicyRuleNumber/TargetRouteTableId are required (matching validateOpCreateTransitGatewayPolicyTableEntryInput) and that TargetRouteTableId refers to a real, existing TGW route table (real invariant: an entry must route to somewhere that exists) — not just accepting any string. ModifyTransitGatewayPolicyTableEntry implements 'unspecified fields retain their current value' field-by-field (matching this file's existing ModifyTransitGatewayPrefixListReference/ModifyTransitGatewayMeteringPolicy convention), re-validating TargetRouteTableId existence when provided. DeleteTransitGatewayPolicyTable now also cascades to entries (previously only cascaded associations). Not-found for a nonexistent rule number reuses ErrInvalidParameter (matching the sibling TransitGatewayMeteringPolicyEntry convention exactly, rather than inventing a new sentinel for an AWS error code this pass could not verify against any documented example). Tests: TestTGWPeripherals_PolicyTableEntryLifecycle/_PolicyTableEntriesValidation/_DeletePolicyTableCascadesEntries/_PolicyTableEntrySnapshotRestore (backend), TestTGWPeripheralsHandler_PolicyTableEntryLifecycle (wire, via postForm/dispatchHandler proving the exact query-param and XML-response shapes above)."}
  application_status_checks: {status: ok, note: "NEW (2026-08-05, SDK bump ec2 v1.317->v1.319.1, gopherstack-8pce follow-up): implemented all 10 newly-exposed ops (Create/Modify/Delete/DescribeApplicationStatusChecks, Associate/DisassociateApplicationStatusCheck, DescribeApplicationStatusCheckAssociations, Enable/DisableApplicationStatusCheckSuppression, DescribeApplicationStatus). Understanding, confirmed by reading every operation's doc comment plus types.go/serializers.go/deserializers.go/validators.go in the installed SDK: an ApplicationStatusCheck is a reusable HTTP(S) health-check DEFINITION (protocol/port/path/thresholds/interval/timeout), created independently of any instance; Associate/DisassociateApplicationStatusCheck attach it to instances directly by ID or indirectly via a tag key/value (current AND future instances with that tag are covered); Enable/DisableApplicationStatusCheckSuppression temporarily excludes an instance's checks from affecting its aggregated status; DescribeApplicationStatus returns the real target of the whole family — each instance's single AGGREGATED status, derived only from checks whose Aggregation='included' (checks with Aggregation='excluded' run independently and never affect it, per the real doc comment). CRUD/association/suppression state is fully real: CreateApplicationStatusCheck applies the real, doc-comment-documented AWS defaults (Path=/, Interval=60, Timeout=6, FailureThreshold=2, SuccessThreshold=5, StatusCodeMatcher=200, InitializationGracePeriodSeconds=300, Aggregation=included) and enforces the real, documented 50-check-per-account limit and Timeout<Interval/Path-starts-with-/ constraints; DeleteApplicationStatusCheck marks Deleted+DeletionTime rather than removing the row outright (real AWS retains a deleted check during an undocumented grace period, visible via IncludeAll=true — this backend retains indefinitely rather than inventing an unspecified grace-period duration, a real gap, not a fabrication) and cascades to every association targeting it; Associate/DisassociateApplicationStatusCheck enforce the real 'exactly one of InstanceIds/TargetTagAssociations, not both' InvalidParameterCombination rule and report real per-target Successful/UnsuccessfulResults (field-diffed: note SuccessfulAssociationResponseObject's AssociationType vocabulary is 'INSTANCE_ID'/'EC2TAG', a DIFFERENT wire vocabulary from ApplicationStatusCheckAssociationObject's 'instance-id'/'tag' used by DescribeApplicationStatusCheckAssociations — an easy-to-miss trap this pass caught by reading both deserializers directly rather than assuming one covers the other); Enable/DisableApplicationStatusCheckSuppression validate the instance exists and compute a real ResumeAt from DurationSeconds (0/absent = indefinite, matching the documented behaviour). DescribeApplicationStatus is the one op a mock backend cannot honestly fully implement: real AWS derives it from actually running HTTP health checks against the instance's application, which this backend does not and cannot do. Per this task's explicit no-fabrication rule, it NEVER returns 'ok'/'impaired'/'initializing' (all three require a real check result this backend does not have) — it returns only the subset of the real ApplicationStatusEnum that is honestly, fully derivable from tracked state: 'suppressed' (a real active ApplicationStatusSuppression exists), 'not-applicable' (real AWS's own documented meaning: no included-aggregation check applies to the instance — verified true here, not approximated), or 'insufficient-data' (an included check IS associated but this backend has never run it, so there is genuinely zero result data — the honest, not fabricated, answer for that value's own documented meaning). See gaps for what remains unmodeled. New sentinels: ErrApplicationStatusCheckNotFound (InvalidApplicationStatusCheckId.NotFound), ErrInvalidParameterCombination (InvalidParameterCombination, new — not previously used anywhere in this file), ErrTooManyApplicationStatusChecks (ApplicationStatusCheckLimitExceeded). New ID prefix 'asc-' (resourceTypePrefixes: 'application-status-check', the real AWS ResourceType string) plus 3 new store.Table-backed maps (applicationStatusChecks/applicationStatusCheckAssociations/applicationStatusSuppressions), all registered via the existing b.registry Snapshot/Restore mechanism — no persistence.go changes needed. Tests: application_status_checks_test.go (12 backend tests: create validation/defaults/quota, modify-retains-unset-fields, delete-cascade+IncludeAll retention, describe filters, instance+tag association lifecycle including partial-success Successful/Unsuccessful reporting, suppression, the dedicated 'never fabricates' status test, snapshot/restore round trip) and handler_application_status_checks_test.go (4 wire tests via postForm/dispatchHandler proving the exact query-param names — InstanceId.N not InstanceIds.N, PolicyRule-style nested TargetTagAssociation.N.Key/.Value — and XML response element names field-diffed above)."}
  unrecorded_describe_ops_sweep41: {status: ok, note: "SWEEP (this pass): 19 of the 37 Describe/List ops this file had never recorded as verified, real-client-driven (wire_field_fixes_ec2sweep41_test.go). FIXED 8 request-param bugs — DescribePrincipalIdFormat (fabricated PrincipalArn key, ignored real Resource.N filter; Backend.DescribePrincipalIDFormat's signature changed from principalARN string to resources []string), DescribeIamInstanceProfileAssociations (Filter.1.Value.1 read unconditionally as instance-id before checking Filter.1.Name, misreading a lone \"state\" filter; \"state\" filter now also implemented), DescribeLaunchTemplates (LaunchTemplateId.N never read, only LaunchTemplateName.N), DescribeLaunchTemplateVersions (LaunchTemplateName/Versions/MinVersion/MaxVersion never read, only the scalar LaunchTemplateId), DescribeImageUsageReports (ReportId.N/ImageId.N never read at all), DescribeVpcEndpointServices (ServiceName.N never read at all), DescribeCustomerGateways and DescribeVpnGateways (Filters never read at all — new applyCustomerGatewayFilters/applyVpnGatewayFilters in handler_filters.go). CONFIRMED CORRECT: DescribeAccountAttributes, DescribeDeclarativePoliciesReports, DescribeVpcClassicLink (singular VpcId.N) + DescribeVpcClassicLinkDnsSupport (plural VpcIds.N — same field name, opposite wire key, both confirmed), DescribeVpcPeeringConnections, DescribeIpams, DescribeVpcEncryptionControls, DescribeFpgaImages, DescribeInstanceSqlHaStates. CORRECTION: DescribeTransitGatewayConnects/ConnectPeers/PeeringAttachments/RouteTables were already fixed and real-client-tested in wire_field_fixes_ec2sweep36_test.go; this file simply never recorded them. 18 of the 37 remain unaudited this pass: DescribeAggregateIdFormat, DescribeAwsNetworkPerformanceMetricSubscriptions, DescribeClassicLinkInstances, DescribeExportTasks, DescribeFpgaImageAttribute, DescribeImageUsageReportEntries, DescribeInstanceEventNotificationAttributes, DescribeInstanceSqlHaHistoryStates, DescribeIpamPools, DescribeIpamScopes, DescribeNetworkInterfaceAttribute, DescribeSecondaryInterfaces, DescribeSecondaryNetworks, DescribeSecondarySubnets, DescribeServiceLinkVirtualInterfaces, DescribeVpnConcentrators (plus MaxResults/NextToken and the remaining Filters surface on several of the ops fixed/confirmed above were not separately re-audited). See top-of-file pass note for full detail."}
gaps:
  - "Application Status Checks (2026-08-05, gopherstack-8pce follow-up): HealthCheckPaths (cross-AZ/Local-Zone
    health-check source/destination ENI paths) is not modeled at all — CreateApplicationStatusCheck silently
    accepts but discards it, and healthCheckPathSet is always rendered empty. This is a deep, separate feature
    (this backend does not model health-check-dedicated ENIs) rather than a quick field addition; scoped out to
    keep the family's core CRUD/association/suppression/status semantics correct and fully tested rather than
    spreading effort thin. InstanceApplicationStatus.AvailabilityZoneId is always empty (this backend tracks
    only AZ name, not a separate AZ ID, on Instance) — real gap, not fabricated. ApplicationStatus.StatusSince
    and ApplicationStatusDetail (the real per-check breakdown list) are always zero/empty: this backend performs
    no real health-check execution, so there are no real per-check results or status-transition timestamps to
    report — reporting anything there would be fabrication, so it is honestly left empty instead. The real,
    documented 'maximum 50 tag associations per application status check' and 'maximum 100 instance IDs per
    suppression request' request-size limits are accepted without enforcement (unlike the 50-check-per-account
    limit, which IS enforced) — a real but low-severity completeness gap, consistent with this file's existing
    pagination-limit gap notes elsewhere. MaxResults/NextToken on DescribeApplicationStatusChecks/
    DescribeApplicationStatusCheckAssociations/DescribeApplicationStatus are accepted but not enforced (always
    returns every match, NextToken always empty) — the same documented, low-severity pattern as roughly a dozen
    other newer op families noted in the pre-existing pagination gap entry above, not specific to this family.
    DescribeApplicationStatusCheckAssociationsOutput.Tags ('tags associated with the application status checks')
    is always empty: its exact aggregation semantics across multiple checks are ambiguous from the SDK doc alone
    and getting it wrong risked being worse than an honest omission."
  - "Key pairs: ED25519 CreateKeyPair generation and the PPK KeyFormat are not modeled —
    CreateKeyPair always generates RSA (real, not fabricated: KeyType is honestly reported
    as 'rsa' since that's the only type ever generated) and KeyFormat is silently ignored
    (always PEM). A real fix needs either crypto/ed25519 keygen with the OpenSSH-default
    base64-SHA256 fingerprint algorithm, or a real PPK binary encoder (PuTTY's format,
    including its MAC) — both buildable, neither attempted this pass to keep scope bounded.
    (gopherstack-8pce, 2026-08-07)"
structural_gaps:
  - "DescribeApplicationStatus's ApplicationStatus.StatusSince and ApplicationStatusDetail
    (the real per-check status-transition timestamp and breakdown list) are always
    zero/empty, and the aggregated status itself never reports 'ok'/'impaired'/
    'initializing' (see the application_status_checks family note). Real AWS derives all
    of this from actually executing HTTP(S) health checks against the target instance's
    application over real network traffic between the AWS control plane and the instance.
    This mock has no network path to an instance's application at all — instances here are
    metadata records, not running workloads — so there is no data source that could ever
    produce a genuine check result, timestamp, or transition here, in an emulator or not.
    Reporting anything but the explicitly-defined 'not-applicable'/'insufficient-data'/
    'suppressed' subset would be fabrication. (gopherstack-8pce, 2026-08-07)"
deferred:
  - trunk_enclave.go's TrunkInterfaceAssociation.Tags: genuinely cannot migrate to the shared tag store — see tag_dual_storage note above (no TagSpecifications on the real create call, no ResourceType enum entry, so CreateTags could never target it even if registered). Left as the single remaining embedded-Tags field in the codebase, by design. RE-VERIFIED (gopherstack-8pce, 2026-07-31 pass): re-read AssociateTrunkInterfaceInput and the ResourceType enum in the installed SDK directly — the constraint still holds exactly as documented. This is NOT a reason to hold the grade at B: the reasoning is a genuine, unchanged real-API limitation (same treatment sql_ha.go's fabricated Tags field got — deleted, not migrated, in the prior pass), not an unaudited gap.
  - "RestoreImageFromRecycleBin (images.go): STALE ENTRY, already fixed before this deferred note was written. Commit 2d47b51d4 (2026-07-29, part of this same gopherstack-8pce ticket) rewrote the op to report InvalidAMIID.NotFound for an image genuinely absent from the bin and to re-create the AMI (guarding against clobbering a live image with the same ID) rather than unconditionally returning success — read directly in images.go:406-433 this pass, confirmed still correct. The deferred bullet describing it as a live disguised-stub bug was written into a later PARITY.md revision without re-checking the code and was wrong. FIXED this pass: added the test coverage that was missing (TestHandler_RestoreImageFromRecycleBin in handler_image_ops_test.go), since the fix had shipped with none."
  - "DeleteQueuedReservedInstances: FIXED (gopherstack-8pce, 2026-07-31 pass). Previously deleted ANY Reserved Instance ID handed to it unconditionally and returned a bare {Return: true} — a real correctness bug, not just a missing-field gap: real AWS only ever deletes a Reserved Instance genuinely in the 'queued' state (a future-dated, not-yet-active purchase) and refuses to touch an active one, so this backend was silently deleting active reservations a real client would never expect it to touch. Now reports real per-ID SuccessfulQueuedPurchaseDeletions/FailedQueuedPurchaseDeletions (types.SuccessfulQueuedPurchaseDeletion/types.FailedQueuedPurchaseDeletion, field-diffed against the installed SDK's deserializers.go for the successfulQueuedPurchaseDeletionSet/failedQueuedPurchaseDeletionSet <item> wire shape and types.DeleteQueuedReservedInstancesErrorCode for the reserved-instances-id-invalid/reserved-instances-not-in-queued-state error codes) and only deletes a target actually in the 'queued' state. Honest limitation carried forward: this backend's PurchaseReservedInstancesOffering has no scheduled/future-dated purchase mode, so no Reserved Instance here is ever actually created in the 'queued' state — meaning every existing RI ID this op is called on today reports reserved-instances-not-in-queued-state (correct, matching what real AWS would also report for an active reservation), and the success path, while implemented and tested via direct state manipulation, has no reachable real trigger from any other op in this backend. This is the same 'implemented correctly but the precondition doesn't arise from this backend's other write paths' shape as the queued-Enable/Disable RADIUS-style honest gaps elsewhere in this codebase, not a stub."
  - "AssociateTransitGatewayRouteTable/DisassociateTransitGatewayRouteTable: FIXED (gopherstack-8pce, 2026-07-31 pass), found during the TGW route-table field-diff this pass targeted. AssociateTransitGatewayRouteTable previously accepted ANY attachmentID string with no existence check at all and hardcoded ResourceType to 'vpc' on every association regardless of the attachment's real kind — so associating a peering, Connect, or Client VPN attachment produced a response that misreported it as a VPC attachment, and associating a nonexistent attachment ID silently 'succeeded'. Now validates the attachment exists (ErrTGWAttachmentNotFound otherwise) and derives the real ResourceType via the same tgwAttachmentResourceLocked helper EnableTransitGatewayRouteTablePropagation already used correctly. A second, related bug found in the same pass: transitGatewayAttachmentExistsLocked (shared by GetTransitGatewayAttachmentPropagations and EnableTransitGatewayRouteTablePropagation, and now Associate/DisassociateTransitGatewayRouteTable) never checked tgwClientVpnAttachments — added when TGW Client VPN attachments were introduced by a later parity-4 pass but never wired into this pre-existing helper, so a real, existing Client VPN attachment ID was wrongly reported as ErrTGWAttachmentNotFound by every caller. Fixed; regression test TestTransitGatewayRouteTableOps_ClientVpnAttachment (transit_gateways_test.go) covers both the association resource-type derivation and the existence-check fix end to end through a real CreateClientVpnEndpointWithOptions(TransitGatewayID: ...)-created attachment."
  - "TGW route-table search/export/announcement surface: AUDITED and FIXED (parity-5, 2026-07-30 pass) — see the transit_gateway family note above. Deeper multi-attachment-type interaction edge cases beyond what field-diffing the wire shapes surfaced (e.g. exhaustive state-machine transition testing across every attachment type combination) were not separately, exhaustively enumerated, but the field-diff itself (types/serializers/deserializers) is complete for this surface."
  - NAT gateway private-connectivity mode (ConnectivityType=private, no AllocationId) and create-time PrivateIpAddress/SecondaryAllocationIds/SecondaryPrivateIpAddressCount/SecondaryPrivateIpAddresses — not modeled, no backing data; see nat_gateway note.
  - VPC Endpoint Service / VPC Endpoint DnsEntries, security groups, IP prefixes, policy documents, PrivateLink-managed-service fields — not modeled, no backing data; see vpc_endpoints note.
  - "EBS snapshot lineage, ENI attach/detach edge cases, pagination internals beyond tags/instances: AUDITED and FIXED (parity-5, 2026-07-30 pass) — see the ebs_snapshot_lineage/eni_attach_detach/pagination family notes above. Real, honestly-documented remaining gaps from that pass: Snapshot.DataEncryptionKeyId and AMI-backing-snapshot InvalidSnapshot.InUse protection (no backing data, see ebs_snapshot_lineage); per-ENI security-group tracking, a materially larger separate feature (see eni_attach_detach); MaxResults/NextToken truncation across ~12 newer op families that declare but never implement it, and SearchTransitGatewayRoutes's required-Filters not being enforced (see pagination and transit_gateway notes)."
leaks: {status: ok, note: FIXED the tag_cleanup class above (real, reachable leak). Re-verified the lifecycle-reconciler goroutine (store.go StartLifecycleReconciler/StopLifecycleReconciler) is ctx-parented AND has an explicit Stop channel, wired into provider.go/handler.go Shutdown — no leak. No other goroutines/tickers found in services/ec2 (grep for `go func\(`/`time.NewTicker`/`time.AfterFunc` — one hit, the reconciler above). Secondary indexes (instanceIDsByVPC/subnetIDsByVPC/routeTableIDsByVPC/sgIDsByVPC/natGatewayIDsByVPC) are correctly deindexed on every explicit per-resource delete; eniIDsByVPC is still correctly maintained but is now write-only (no reader) since DeleteVpc no longer cascades through it — not a leak (bounded, cleaned on ENI delete), just vestigial; left in place rather than risk a wider removal across network_interfaces.go/instances.go/spot_fleet.go/indexes.go for a non-functional cleanup.}
---

## Notes
- sourceDestCheck AWS default is **true** for VPC instances (must be explicitly disabled, e.g. NAT instances) — a prior test encoded false, corrected.
- kernel/ramdisk return "" for HVM (not "stop").
- 2026-07-24 pass: fixed the long-tracked gopherstack-b5m VPC/subnet cascade-delete bug (DependencyViolation, no cascade), extended the same real-AWS-dependency-check pattern to DeleteRouteTable/DeleteInternetGateway/AttachInternetGateway, fixed a disguised-stub NAT gateway address association pair, and closed a systemic tag-map leak across ~50 delete ops in the newer op families. All found via direct code reading + cross-referencing resource_types.go's resourceTypePrefixes/resourceExistsLocked (the authoritative taggable-resource-ID list) rather than grep-only stub hunting, per parity-principles.md rule 4 (grep-based stub hunting has false positives — confirmed by reading each flagged function body before editing, and explicitly NOT touching composite-key sub-entry deletes that were never taggable).
- New sentinel: ErrResourceAlreadyAssociated ("Resource.AlreadyAssociated") — real AWS error code, added to errCodeLookup.
- 2026-07-25 pass (parity-4): implemented the 16 EC2 operations revealed by the aws-sdk-go-v2 bump (see parity4_new_ops family above for full detail). New sentinel: ErrVpcEndpointIDNotFound ("InvalidVpcEndpointId.NotFound"), added to errCodeLookup — also newly mapped the pre-existing ErrIpamAllocationNotFound ("InvalidIpamPoolAllocationId.NotFound"), which had a real gap (missing from errCodeLookup, so ReleaseIpamPoolAllocation's not-found case was surfacing as 500 InternalFailure before this pass). This service was NOT asked to fix the pre-existing gopherstack-8pce debt (embedded-vs-shared Tags dual storage, TGW/NAT field diffs, VPC endpoint sweep) and did not: the new ops that touch that surface (TGW attachments, VPC endpoints) were additive and did not deepen it.
- NEXT PASS priority (superseded by the 2026-07-30 pass below): the embedded-vs-shared Tags dual-storage gap (see deferred) is still the highest-value remaining item — it's a wire-shape correctness bug (tags added after creation may not appear in Describe responses) across ~10 files. After that: full TGW attachment/route-table state-machine field-diff, VPC Endpoint (Service) op-by-op sweep (including the disguised-stub ModifyVpcEndpointServicePayerResponsibility noted above), and the RestoreImageFromRecycleBin no-op bug.
- 2026-07-30 pass (gopherstack-8pce): closed the tag_dual_storage gap for 9 of 11 flagged files and field-diffed Transit Gateway / NAT Gateway / VPC Endpoints against the installed SDK, per the family notes above. Downgraded A→B: this pass found a genuinely severe bug (CreateTransitGateway's deterministic, non-unique ID silently let a second transit gateway overwrite the first) plus a second-order wire bug that would silently defeat a real client's ID filtering (DescribeTransitGateways read a request parameter name, `TransitGatewayIds.TransitGatewayId.N`, that no real AWS SDK ever sends — confirmed against the SDK's own TransitGatewayIdStringList serializer, which flattens to `TransitGatewayIds.N`), plus a disguised stub (CreateTransitGateway discarding Options.*/TagSpecifications) and a wrong wire field name on VpcEndpoint (`vpcEndpointState` vs the real `state`) that would break real-client parsing of the endpoint's own state. New sentinel: none added this pass (existing ErrVpcEndpointServiceNotFound/ErrVpcEndpointIDNotFound reused). New backing field: VpcEndpoint gained OwnerID (set at creation from b.AccountID); NatGateway gained AvailabilityZone (set at creation from the target subnet) and ConnectivityType (always "public" — the only type this backend creates); VpcEndpointServiceConfig gained PayerResponsibility. Interface signature changes (both required updating every call site in the ec2 package's own tests — no external callers found, verified with a full-repo grep): `Backend.CreateNatGateway` gained a `tags map[string]string` parameter; `Backend.CreateTransitGateway` changed from `(description string)` to `(p CreateTransitGatewayParams)`; `Backend.DeleteTransitGateway` changed from returning `error` to `(*TransitGateway, error)` to match the real DeleteTransitGatewayOutput shape. `Backend.CreateVpcEndpoint(WithRouteTableIDs)` was deliberately NOT given a tags parameter (13+ call sites, no external callers, but higher blast radius for the same result) — CreateVpcEndpoint's tags are instead applied via a handler-level CreateTags-after-create call, mirroring the existing CreateVpc/CreateSubnet/CreateSecurityGroup pattern. All found via direct code reading + cross-referencing the installed aws-sdk-go-v2 types.go/serializers.go/deserializers.go (not just this backend's own output), per parity-principles.md rule 2. Every migrated/fixed resource has a dedicated wire-level test (postForm/dispatchHandler + ExportDispatch) proving the fix; all pasted in the session's gate output.
- 2026-07-31 follow-up pass (gopherstack-8pce): verified the 2026-07-30 downgrade's cited severe bugs (TGW ID collision, TGW ID-filter wire-name, VpcEndpoint state field-name) were already fixed — confirmed by reading transit_gateways.go/ec2core.go directly against the installed SDK, not by trusting the note. Re-verified the tag-storage 2-file non-migration reasons (sql_ha.go fabrication deleted, trunk_enclave.go real API constraint) still hold — re-read AssociateTrunkInterfaceInput and the ResourceType enum directly this pass; not a reason to hold the grade at B. Found the RestoreImageFromRecycleBin deferred entry was stale (fixed by commit 2d47b51d4 the day before the note that still listed it as open was written) and added the missing test coverage (TestHandler_RestoreImageFromRecycleBin). Fixed two real, previously-undetected bugs while field-diffing the TGW route-table association surface the prior pass explicitly left unaudited: DeleteQueuedReservedInstances was unconditionally deleting any Reserved Instance ID and returning a bare boolean instead of reporting real per-ID SuccessfulQueuedPurchaseDeletions/FailedQueuedPurchaseDeletions and refusing to delete a non-queued (i.e. every real one in this backend) reservation; AssociateTransitGatewayRouteTable accepted any attachment ID with no existence check and hardcoded ResourceType="vpc" regardless of the attachment's real kind, and the shared transitGatewayAttachmentExistsLocked helper it now uses was itself missing the Client VPN attachment map (a regression from a later parity-4 pass never wired into this pre-existing helper). HELD AT B: EBS snapshot lineage, ENI attach/detach edge cases, pagination internals beyond tags/instances, and the broader TGW route-table search/export + announcement wire-shape surface remain genuinely UNAUDITED this pass — real, unclosed unknowns, not a reason to claim completion. New tests: TestHandler_RestoreImageFromRecycleBin (handler_image_ops_test.go), TestHandler_DeleteQueuedReservedInstances (handler_reserved_instances_test.go), TestTransitGatewayRouteTableOps_ClientVpnAttachment (transit_gateways_test.go), plus corrected assertions in the pre-existing TestReservedInstances/TestTGWPeripherals_GetRouteTableAssociations/ec2core_test.go TGW association subtest that had encoded the deleted-any-RI and hardcoded-vpc-type bugs as expected behavior. No new sentinels; reused ErrTGWAttachmentNotFound. Backend interface signature change: `Backend.DeleteQueuedReservedInstances` changed from `(ids []string)` to `(ids []string) []QueuedPurchaseDeletionResult` — all call sites are within the ec2 package's own tests, no external callers found via full-repo grep.
- 2026-07-30 pass (parity-5): closed all four areas the 2026-07-31 pass held the grade at B for — B→A. Full detail in the ebs_snapshot_lineage/eni_attach_detach/pagination/transit_gateway family notes above; brief index of what changed and why:
  - **CreateVolume gained a `snapshotID string` parameter** (4th positional arg) — it never read real AWS's CreateVolumeInput.SnapshotId at all, so "create a volume from a snapshot" silently produced an empty, disconnected volume. Now validates/sizes/inherits-encryption-from the snapshot. `services/cloudformation/resources_ec2_network.go`'s `AWS::EC2::Volume` creator updated to pass its own real SnapshotId property through. ~42 test call sites across 7 files mechanically updated (`, ""` appended) plus targeted new coverage (`TestVolumeOperations` cases, `TestHTTP_CreateVolume_FromSnapshot`).
  - **NetworkInterface gained `DeleteOnTermination`/`OwnerID` fields**; `Backend.SetNetworkInterfaceDeleteOnTermination` is a new method. TerminateInstances no longer unconditionally deletes every attached ENI — only the launch-created primary one. Two existing tests that encoded the old (wrong) behavior as expected were corrected in place.
  - **CreateTransitGatewayRoute/ReplaceTransitGatewayRoute gained a `blackhole bool` parameter** (real Blackhole flag, previously silently ignored); both now validate the attachment exists and derive real ResourceId/ResourceType instead of hardcoding "vpc". `TransitGatewayRoute` gained `ResourceID`/`ResourceType` fields; `TransitGatewayRouteTableAnnouncement` gained `PeerTransitGatewayID`. One existing test that created a route with no attachment and expected success was corrected; `DeleteTransitGatewayRoute`'s not-found case switched from `ErrInvalidParameter` to the more accurate `ErrRouteNotFound`.
  - **DescribeSnapshots/DescribeNetworkAcls pagination switched from a plain integer-offset NextToken to the same HMAC-signed opaque token** (`pkgs/page`) every other paginated describe op here already used — a forged token was previously silently accepted instead of rejected.
  - No new sentinels; reused `ErrSnapshotNotFound`, `ErrTGWAttachmentNotFound`, `ErrRouteNotFound`, `ErrInvalidPaginationToken`. `resourceTypeENI = "network-interface"` added as a new shared constant (3 pre-existing literal call sites + this pass's new ones consolidated onto it, avoiding a goconst violation). Fabrication check: every added field was verified against the installed aws-sdk-go-v2 types/serializers/deserializers before being added, and every field left absent (DataEncryptionKeyId, AMI-snapshot InvalidSnapshot.InUse, ENI security groups, ~12-op-family MaxResults truncation, SearchTransitGatewayRoutes required-Filters) is explicitly documented above with the reason it wasn't fabricated instead. 0 regressions: full `services/ec2` and `services/cloudformation` suites green under `-race`; `go build`/`go vet`/`go vet -tags e2e ./test/e2e/...`/`gofmt`/`golangci-lint run ./services/ec2/...` all clean; no banned nolints.
- 2026-08-05 pass (SDK bump ec2 v1.317->v1.319.1, gopherstack-8pce follow-up): implemented the 13 operations this bump exposed (`TestSDKCompleteness` was failing). Full detail in the tgw_policy_table_entries/application_status_checks family notes above. Transit Gateway policy table entries (3 ops: Create/Modify/DeleteTransitGatewayPolicyTableEntry) build on the pre-existing TGW policy table model and also fixed a stale doc comment + a now-incorrect GetTransitGatewayPolicyTableEntries stub (it previously always returned empty, which was correct before this bump added a real Create op but became a disguised stub the moment entries could actually exist). Application Status Checks (10 ops) is a wholly new resource family: health-check definitions, associable with instances/tags, individually suppressible, whose real target — DescribeApplicationStatus's per-instance aggregated status — this backend can only partially, honestly implement (no real HTTP health-check execution), so it deliberately returns only the subset of the real ApplicationStatusEnum (not-applicable/insufficient-data/suppressed) derivable from genuinely tracked state, never fabricating ok/impaired/initializing. New sentinels: ErrApplicationStatusCheckNotFound, ErrInvalidParameterCombination, ErrTooManyApplicationStatusChecks. New ID prefix `asc-`. Interface additions only (`Backend` gained 10 new methods) — no existing method signatures changed, no existing test call sites touched. All wire shapes (query param names, XML response element names, list-flattening conventions) verified against the installed aws-sdk-go-v2/service/ec2@v1.319.1 serializers.go/deserializers.go/validators.go directly, not against this backend's own output, per parity-principles.md rule 2 — caught one real wire-shape trap this way (SuccessfulAssociationResponseObject's AssociationType vocabulary "INSTANCE_ID"/"EC2TAG" differs from ApplicationStatusCheckAssociationObject's "instance-id"/"tag"). New tests: application_status_checks_test.go (12 backend tests) + handler_application_status_checks_test.go (4 wire tests via postForm/dispatchHandler) + tgw_peripherals_test.go additions (TestTGWPeripherals_PolicyTableEntryLifecycle/_PolicyTableEntriesValidation/_DeletePolicyTableCascadesEntries/_PolicyTableEntrySnapshotRestore) + handler_tgw_peripherals_test.go addition (TestTGWPeripheralsHandler_PolicyTableEntryLifecycle); the pre-existing TestTGWPeripherals_PolicyTableEntriesAlwaysEmpty was renamed/rewritten to TestTGWPeripherals_PolicyTableEntriesValidation since "always empty" was no longer true. 0 regressions: full `services/ec2` suite green under `-race`; `go build`/`go vet`/`gofmt`/`golangci-lint run ./services/ec2/...` all clean (0 issues); no banned nolints. See gaps for what remains honestly unmodeled (HealthCheckPaths, AvailabilityZoneId, StatusSince/per-check detail, a few documented AWS request-size limits, and NextToken truncation on this family's three Describe ops).
- 2026-08-07 pass (gopherstack-8pce follow-up): re-verified the tag dual-storage consolidation and TGW/NAT/VPC-endpoint field-diffs from the passes above by reading the code directly against the pinned SDK — still correct, no regression found. Found and fixed one more instance of the exact bug class this ticket targets: DescribeKeyPairs's `tag:` filter read tags from a synthetic `"keypair-"+Name` key that CreateTags never wrote to (tags are stored under the bare key pair Name), so the filter silently never matched. Closed the DescribeKeyPairs IncludePublicKey gap: added KeyPairId/KeyType/CreateTime/TagSet to DescribeKeyPairs and wired create-time TagSpecifications into CreateKeyPair/ImportKeyPair (field-diffed against aws-sdk-go-v2/service/ec2@v1.319.1's KeyPairInfo/CreateKeyPairOutput/ImportKeyPairOutput deserializers). `Backend.CreateKeyPair`/`Backend.ImportKeyPair` both gained a `tags map[string]string` param; the one external caller (`services/cloudformation/resources_ec2_network.go`'s `AWS::EC2::KeyPair`) updated to pass nil. Moved DescribeApplicationStatus's StatusSince/ApplicationStatusDetail gap to a new `structural_gaps:` section (requires real HTTP health-check execution over real network traffic this mock has none of — genuinely underivable, not merely unbuilt); everything else in that family's gaps entry stays in `gaps:` since it's buildable, just not attempted. New test: TestKeyPairWire (key_pairs_wire_test.go). 0 regressions: full `services/ec2` suite green under `-race`; `go build ./...`, `go vet`, `gofmt`, `golangci-lint run ./services/ec2/... ./services/cloudformation/...` all clean; no banned nolints.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404,
masking Handler()'s already-typed InternalFailure**: same shape as autoscaling's entry
(see that entry or gopherstack-3a8t for the full survey/rationale). `RouteMatcher` now
falls back to `service.MatchesUserAgentMarker(r.Header, "api/ec2")` (verified against the
pinned `ec2@v1.319.1/api_client.go:645` `AddSDKAgentKeyValue` call) only on the `ReadBody`
failure branch, leaving the existing `Version`/`Action` matching untouched. Migrated
`ExtractOperation`/`ExtractResource`/`Handler()` off `r.ParseForm()` onto
`httputils.ReadBody`+`url.ParseQuery`, per the docdb/neptune precedent (gopherstack-bahs).
`dispatch()` already took `vals url.Values` as an explicit parameter (not an implicit
`r.Form` read), so the migration was a straightforward substitution with no downstream
call-site changes needed. Proof: `TestHandler_OversizedBodySurfacesInternalFailure` in
`handler_oversized_body_test.go` drives a real EC2 SDK client through
`service.NewRegistry`/`service.NewServiceRouter`, confirmed failing pre-fix with
`UnknownError`; passes now with `InternalFailure`. `TestHandler_NormalSizedBodyStillRoutes`
is the regression guard. Gates: `go build`, `go vet`, `gofmt -l` (clean), `go test -race
./services/ec2/...` (pass, full ec2 suite including e2e-tagged tests unaffected),
`golangci-lint run ./services/ec2/...` (0 issues).

**2026-08-23 pass -- never-named-in-PARITY sweep, Verified Access family**: this service
declares 785 operations; diffing every declared op name against every op name mentioned
anywhere in this file (front matter and notes) found 696 never named by any prior audit,
of which ~248 have a real (non-trivial) response body by a field-count heuristic on the
handler body. Picked the 18-op Verified Access family (`handler_verified_access.go` /
`verified_access.go` -- CreateVerifiedAccessEndpoint/Group/Instance/TrustProvider,
matching Delete/Describe/Modify, plus Attach/DetachVerifiedAccessTrustProvider) as a
coherent, never-named, all-real-body slice and audited it fully against the installed
`aws-sdk-go-v2/service/ec2@v1.319.1` deserializers. Found 2 real bugs, both the same
shape -- state the backend already tracks (`VerifiedAccessInstance.AttachedTrustProviderIDs`,
populated by Attach/DetachVerifiedAccessTrustProvider) but never surfaced on the wire:
(1) Create/Delete/Describe/ModifyVerifiedAccessInstance's `verifiedAccessInstanceItem`
never emitted `verifiedAccessTrustProviderSet`, the real
`VerifiedAccessInstance.VerifiedAccessTrustProviders` field (confirmed via
`awsEc2query_deserializeDocumentVerifiedAccessInstance`, element `verifiedAccessTrustProviderSet`
-> `awsEc2query_deserializeDocumentVerifiedAccessTrustProviderCondensedList`) -- a real
client describing an instance after attaching a trust provider saw no trust providers at
all. Fixed via a new `(h *Handler) toVerifiedAccessInstanceItem` helper that resolves
`AttachedTrustProviderIDs` through `DescribeVerifiedAccessTrustProviders`, guarding the
empty-list case explicitly (that method treats a nil/empty ID list as "describe all",
so an unattached instance must short-circuit rather than call it, or every trust
provider in the account would leak onto every bare instance). (2) Attach/
DetachVerifiedAccessTrustProvider returned a bare `{Return: true}` stub; the real
Attach/DetachVerifiedAccessTrustProviderOutput has no `Return` field at all -- only
`VerifiedAccessInstance`/`VerifiedAccessTrustProvider` (confirmed via
`awsEc2query_deserializeOpDocumentAttachVerifiedAccessTrustProviderOutput`), so a real
client relying on the attach response to confirm the new state (rather than a follow-up
Describe) got nothing. Fixed by re-describing both resources after the attach/detach and
returning them. New tests: `TestVerifiedAccessInstanceTrustProvidersWire` and
`TestAttachDetachVerifiedAccessTrustProviderWire` (`handler_verified_access_test.go`),
both wire-level via `ExportDispatch`; hand-reverted `handler_verified_access.go` to the
pre-fix version for each and confirmed the exact documented failure (`md5sum` identical
after restoring). No persisted-struct field/type change, so no `ec2SnapshotVersion` bump.
Modelling gaps found but NOT fixed (out of scope for this pass, reported not synthesized):
Create/ModifyVerifiedAccessEndpoint never read `SecurityGroupIds`/`LoadBalancerOptions`/
`NetworkInterfaceOptions`/`CidrOptions`/`RdsOptions`/`PolicyDocument`/`SseSpecification`/
`DomainCertificateArn` (the real endpoint-type-specific config, entirely unmodeled --
`vals.Get` never even reads these); `VerifiedAccessTrustProvider` collapses the real
`DeviceTrustProviderType`/`UserTrustProviderType` split into one `TrustProviderType`
string; `VerifiedAccessGroup`/`VerifiedAccessInstance`/`VerifiedAccessEndpoint` are all
missing `CreationTime`/`LastUpdatedTime`/`Tags`/`PolicyDocument`/`PolicyEnabled`. Gates:
`go build ./...`, `go vet ./services/ec2/...`, `gofmt -l` (clean), `go test -race
./services/ec2/... ./pkgs/persistence/...` (pass), `golangci-lint run ./services/ec2/...`
(0 issues after `--fix` corrected a `fieldalignment` finding on the two new Attach/Detach
response structs). Not reached: the other ~246 never-named rich-body ops, concentrated in
`handler_images.go` (17), `handler_instances.go` (15), `handler_volumes.go` (12),
`handler_snapshots.go` (11), `handler_spot_fleet.go` (10), `handler_client_vpn.go` (9),
`handler_security_groups.go` (9), `handler_elastic_ips.go` (8), `handler_tgw_multicast.go`
(7), `handler_instance_attrs.go` (7) -- next pass should pick one of those families, per
the same method.

**2026-08-23 follow-up pass -- never-named sweep, `handler_images.go`/`handler_instances.go`**:
re-derived the never-audited count directly against `registerImagesOps`/`registerInstancesOps`
(the structured op-registration blocks, not this file's prose) rather than trusting the
prior pass's "17"/"15" figures. `handler_images.go` registers 27 ops; grepping every one
against this whole file found exactly one prior hit, `RestoreImageFromRecycleBin`, and its
context (2026-07-31 entry above) is a genuine audited-and-fixed op, not a "not reached"
mention -- so 26/27 (96%) were never audited. `handler_instances.go` registers 28 ops; none
appear anywhere in this file -- 28/28 (100%) never audited. Combined: 54/55 (98%) across
both files. (Both files also implement several core ops dispatched from `buildCoreOps` in
`handler.go` rather than their own `register*Ops` -- `DescribeImages`/`DescribeRegions`/
`DescribeAvailabilityZones`/`DescribeImageAttribute` and `StartInstances`/`StopInstances`/
`RebootInstances`/`DescribeInstanceStatus` -- excluded from this count since they weren't
the object of the prior pass's per-file tallies either.)

Audited all 55 registered ops by hand against the installed
`aws-sdk-go-v2/service/ec2@v1.319.1` serializers/deserializers. Found and fixed 4 real bugs:
(1) **`DeregisterImage`** (`handler_images.go`) returned `(nil, nil)` on success -- an
untyped nil `any`. `xml.Marshal(nil)` serializes to zero bytes, so the wire response was
just the XML declaration with no `DeregisterImageResponse` root element, no `requestId`,
and no `Return` (real `DeregisterImageOutput.Return` is `*bool`, always true --
`api_op_DeregisterImage.go:89`); smithy-go's client tolerates the empty body via
`FetchRootElement`'s `io.EOF` branch and returns a zero-value output with `Return == nil`
rather than erroring, so this was silent data loss, not a hard failure. Its not-found path
also used `ErrInvalidParameter` (`InvalidParameterValue`) instead of the sentinel every
other not-found path in `images.go`/`image_ops.go` already uses, `ErrImageNotFound`
(`InvalidAMIID.NotFound`, the real code) -- a broken op beside many correct siblings of the
same shape. Fixed both; `DeregisterImage` now returns a proper stub response and reuses
`ErrImageNotFound`. (2) **`ReportInstanceStatus`** (`handler_instances.go`) only ever read
`InstanceId.1` and `ReasonCode.1`, misreading the latter into the "description" arg passed
to the backend -- the real, deprecated-but-still-real `Description` field
(`serializers.go:91277`) was never read at all, and `Instances` (a required, unbounded list
flattened as `InstanceId.N`) only ever validated its first element, so a batch call with a
bad second ID silently succeeded instead of surfacing `InvalidInstanceID.NotFound`. Fixed:
now parses the full `InstanceId.N` list via `parseMemberList` and reads the real
`Description` field; `Backend.ReportInstanceStatus` signature changed from
`(instanceID, status, description string)` to `(instanceIDs []string, status, description
string)` and validates every listed instance. (3) **`ModifyInstanceCreditSpecification`**
(`handler_instances.go`) had the identical shape: only read
`InstanceCreditSpecification.1.*`, silently dropping every other entry in the required,
unbounded `InstanceCreditSpecifications` list (`serializers.go:87727`) -- a real client
batch-modifying a fleet's credit option in one call only got the first instance changed,
with no error and no `UnsuccessfulInstanceCreditSpecifications` reporting for the rest.
Fixed: parses the full list, and `Backend.ModifyInstanceCreditSpecification` now takes
`[]InstanceCreditSpec` and returns `(successful, unsuccessful []InstanceCreditSpec)`
per real per-item batch semantics; the handler reports failures via a new
`UnsuccessfulInstanceCreditSpecificationSet` (`instanceId`/`error>code`/`error>message`,
confirmed against `awsEc2query_deserializeDocumentUnsuccessfulInstanceCreditSpecificationItem`)
using the real `InvalidInstanceID.NotFound` code. (4) **`DescribeInstanceTypeOfferings`**
(`handler_instances.go`) took `_ url.Values`, ignoring the request entirely -- the real
`instance-type`/`location` `Filters` (`api_op_DescribeInstanceTypeOfferings.go` doc comment)
were silently discarded and every call returned the full static offering list regardless of
what was asked for. Fixed via a new `applyInstanceTypeOfferingFilters` (reusing the
existing `parseEC2Filters`/`anyEqual` helpers already used by every other filtered Describe
in this package) and an explicit, honest empty result for a `LocationType` other than
`availability-zone` (the only kind this backend's static generator produces -- not
fabricating offerings for `region`/`availability-zone-id`/`outpost`, which it has no real
data for).

No sibling family shares (1)'s or (4)'s exact op; (2) and (3) are the same "batch op reads
only index 1" shape and were both found and fixed together as a matched pair -- no other op
in either file uses the `.N.` nested-list wire form, confirmed by grep.

Proof for all 4: new real-SDK-client tests in `wire_field_fixes_ec2sweep11_test.go`
(`TestDeregisterImage_RealClient`, `TestReportInstanceStatus_AllInstancesValidated_RealClient`,
`TestModifyInstanceCreditSpecification_Batch_RealClient`,
`TestDescribeInstanceTypeOfferings_Filters_RealClient`), each hand-reverted (`cp` from the
pre-fix `git show HEAD:...` content) and confirmed failing with the documented symptom, then
restored with `md5sum` identical to the fixed version before re-running green.

No persisted struct's json tag or field name/type changed (`InstanceCreditSpec`'s shape is
unchanged; only how it's passed/batched changed) -- no `ec2SnapshotVersion` bump.

Found, real, but NOT fixed this pass (volume too large for one pass, named rather than
synthesized around): every List/Describe op in both files silently ignores real
`MaxResults`/`NextToken` pagination and always returns everything in one page --
`DescribeInstanceCreditSpecifications`, `DescribeInstanceTopology`,
`DescribeInstanceConnectEndpoints`, `DescribeInstanceEventWindows`, `DescribeElasticGpus`,
`DescribeImportImageTasks`, `DescribeExportImageTasks`, `ListImagesInRecycleBin`,
`DescribeFastLaunchImages`, `DescribeImageReferences`, `DescribeInstanceImageMetadata` --
all confirmed via the installed SDK to have real `MaxResults`/`NextToken` on both
Input and Output, none implemented here. `DescribeImages` (core op, same file) already
implements this correctly via `parseImagesPagination`/`pkgs/page` -- a correct sibling
right beside every one of these broken ones. Modelling gaps ruled real, not fabricatable,
and left alone: `CopyImage` never reads the required `SourceRegion` or the real
`Encrypted`/`KmsKeyId` (this backend's AMI type tracks no cross-region or
block-device/encryption state at all, a pre-existing, already-documented structural gap);
`GetDefaultCreditSpecification`/`ModifyDefaultCreditSpecification`'s `InstanceFamily` is a
non-pointer required enum (unprovable per the pointer-vs-scalar rule) and the backend
tracks one default across all families rather than per-family, which is a real but
non-pointer-provable modelling gap, not touched. `CreateInstanceEventWindow`'s alternative
`TimeRanges` input (mutually exclusive with `CronExpression`) is entirely unmodeled --
documented, not fabricated. False positive ruled out by hand: `ReportInstanceStatus`'s
backend method discards `status`/`description` entirely (`_ string, _ string`) and there is
no real Describe surface that reflects customer-reported status back to the same account,
so despite the misleading doc comment this is not a provable "fabricated success" bug --
only the wire-parsing/multi-instance-validation gap above is.

Gates: `go build ./...` (repo-wide, since two `Backend` interface method signatures
changed), `go vet ./services/ec2/...`, `gofmt -l services/ec2/*.go` (clean), `go test
./services/ec2/... -count=1` (`ok`, 1.0s, full suite including the new tests), `golangci-lint
run ./services/ec2/...` (0 issues, after fixing 2 `goconst` findings the new code introduced
-- reused `ErrInstanceNotFound.Error()`/`filterKeyAvailabilityZone` instead of new literals
-- and 1 `nonamedreturns` finding by switching `ModifyInstanceCreditSpecification`'s named
returns to local vars).

Not reached: the other ~28 ops in `handler_instances.go` beyond the 4 fixed (batch
credit-spec/report-status/type-offerings), and the other ~23 in `handler_images.go` beyond
`DeregisterImage`/`RestoreImageFromRecycleBin` -- every op in both files was read and
checked for the enumerated bug shapes, but only the pagination gap above and the 4 fixed
bugs were found; the remaining ops (image lifecycle toggles, import/export image tasks,
instance event windows, instance-connect endpoints, serial console, credit specification
describe, etc.) matched their real wire shapes on inspection. Next pass should pick one of:
`handler_volumes.go` (12), `handler_snapshots.go` (11), `handler_spot_fleet.go` (10),
`handler_client_vpn.go` (9), `handler_security_groups.go` (9), `handler_elastic_ips.go` (8),
`handler_tgw_multicast.go` (7), `handler_instance_attrs.go` (7), or return to close the
pagination gap named above across both files audited this pass.

**2026-08-23 pass -- closed the ec2sweep11 `MaxResults`/`NextToken` gap named above**:
verified each of the eleven named ops against the installed
`aws-sdk-go-v2/service/ec2@v1.319.1` directly (not trusting the prior pass's list) --
all eleven really do declare `MaxResults *int32`/`NextToken *string` on both Input and
Output, and all eleven handlers really did ignore both, always returning every item in
one page with no `nextToken` on the response. All eleven are the same severity: the
**unbounded-single-page** form, not the medialive infinite-loop form -- none applied any
default cap, so a real SDK paginator got everything in one call and stopped (never
restarted at item 0). No cap being applied at all is what makes this milder than a
default-cap-plus-ignored-token bug would be.

Fixed via two new shared helpers in `handler.go` (`parseEC2Pagination` -- generalizes
`handleDescribeImages`' existing `parseImagesPagination` with per-op min/max/default
bounds instead of hardcoded constants; `pageSlice[T any]` -- factors out the
offset/limit/`page.EncodeHMACToken` truncation block `DescribeImages`/`DescribeSnapshots`/
`DescribeNetworkAcls` each already duplicated inline) rather than writing 11 near-copies
of `parseImagesPagination`, per the one-coarse-pattern-per-repo rule; reuses
`pkgs/page`'s existing `DecodeHMACToken`/`EncodeHMACToken`, no new pagination mechanism.
Bounds/defaults per op, sourced from the pinned SDK's doc comments where given, falling
back to `DescribeImages`' own (1..1000, default 1000) where the doc gives no explicit
range: `DescribeInstanceEventWindows` 20..500 ("between 20 and 500"); `DescribeElasticGpus`
5..1000 ("between 5 and 1000"); `DescribeInstanceTopology` default 20 ("Default: 20"),
bounds fall back to 1..1000; the other eight fall back to 1..1000/1000 entirely
(their docs give no explicit range or default). Every response struct gained a
`NextToken string \`xml:"nextToken,omitempty"\`` field.

Proof: `pagination_ec2sweep11_test.go`, one real-SDK-client-paginator test per op
(`ec2sdk.NewDescribeXPaginator`, `StopOnDuplicateToken` left at its real default of
`false`), five seeded items at `MaxResults=2` (three seeded pages), asserting the IDs
returned across pages are disjoint and total 5 -- the shape that fails pre-fix, since
pre-fix the first (only) page already contains all 5 IDs. Hand-reverted all three
touched handler files to the pre-fix `git show HEAD:...` content via `cp` and confirmed:
9 of the 11 tests failed with `"1" is not greater than or equal to "2"` (pagination
collapsed to one page); `DescribeInstanceEventWindows` needed re-seeding at 45 items
(its 20-item MaxResults floor makes 5 items fit on one page even correctly paginated --
a single-page test proves nothing, exactly the pitfall this ticket warned about) and
then failed the same way once seeded past the floor. `DescribeElasticGpus` and
`ListImagesInRecycleBin` have no data path that can ever populate more than zero items
in this backend (see below), so their tests instead assert a forged `NextToken` is now
rejected -- both failed pre-fix with "an error is expected but got nil" (the token was
silently ignored, not validated). Restored all three files via `cp` from the saved
fixed copies; `md5sum` identical before/after the revert-and-restore cycle.

Two of the eleven could not get the full two-page proof, for reasons pre-dating and
outside this pass's scope: **`DescribeElasticGpus`** has no generated SDK Paginator at
all (Elastic Graphics was retired in 2023 -- see the type's existing doc comment) even
though its Input still carries `MaxResults`/`NextToken`, and this backend's
`DescribeElasticGpus` always returns an empty list (no API here ever attaches one) --
the fix and its `MaxResults` bounds validation are real and now correct, but nothing
can ever populate a second page. **`ListImagesInRecycleBin`** genuinely has no
SDK-reachable producer in this backend: `DeregisterImage` deletes AMIs outright rather
than moving them to the recycle bin, so `recycleBinImages` is never written by any real
operation -- a pre-existing structural gap (same shape as the AMI-recycle-bin absence
already), not something this pagination fix should also invent. Both are still fixed
correctly; only the strongest proof shape is unavailable for them.

Backend interface unchanged; no `Backend` method gained/lost a parameter. No persisted
struct's json tag changed -- only new XML-only response fields -- so no
`ec2SnapshotVersion` bump. Gates: `go build ./...` (repo-wide sanity check; no exported
signature actually changed), `go vet ./services/ec2/...`, `gofmt -l services/ec2/*.go`
(clean), `go test -race ./services/ec2/...` (`ok`, full suite including the 11 new
tests), `golangci-lint run ./services/ec2/...` (0 issues, after fixing 1 `godot` and 9
`prealloc` findings the new test file introduced); no banned `//nolint`s.

**2026-08-23 pass -- `handler_volumes.go`/`handler_snapshots.go` never-named sweep**:
re-derived the count directly against `registerVolumesOps`/`registerSnapshotsOps` plus
each file's own core ops dispatched from `buildCoreOps` (`CreateVolume`/`DescribeVolumes`/
`DeleteVolume`/`AttachVolume`/`DetachVolume`/`DescribeVolumeAttribute`/
`ModifyVolumeAttribute` for volumes; `DescribeSnapshotAttribute`/`ModifySnapshotAttribute`
for snapshots), grepped against every op name mentioned anywhere in this file, and split
"named-and-audited" from "named-only-as-a-not-reached-count" per the redirect's method.
`handler_volumes.go`: 22 total ops, 3 previously audited (`CreateVolume`, `DescribeVolumes`,
`ModifyEbsDefaultKmsKeyId` -- all from the 2026-08-05/08-12 `ebs_snapshot_lineage` passes)
-- **19/22 (86%) never audited**. `handler_snapshots.go`: 24 total ops, 4 previously audited
(`CopySnapshot`, `CreateSnapshots`, `CreateSnapshot`, `DescribeSnapshots`, same passes) --
**20/24 (83%) never audited**. Combined: **39/46 (85%) never audited**, well above the
prior pass's 12/11 "real-body" estimate (that number counted only a field-count heuristic
subset of never-named ops, not the true never-audited total).

Read every one of the 39 by hand against `aws-sdk-go-v2/service/ec2@v1.319.1`
serializers/deserializers/api_op_*.go. Found and fixed **7 real bugs**, two of them the
most severe class this sweep has found (a real client's request or response silently
decoding to nothing, every time, not merely a missing field):

1. **`CopyVolumes`** (`handler_volumes.go`) was wrong on both sides of the wire. Request:
   read `VolumeId.N` (a list) and a fabricated `DestinationRegion` field; the real
   `CopyVolumesInput` has a single required `SourceVolumeId` (serializers.go:69198,
   confirmed against `api_op_CopyVolumes.go`) and no cross-region concept at all (the op
   copies within the same AZ) -- a real client's request never populated `VolumeId.N`, so
   `volumeIDs` was always empty and every real call failed with `InvalidParameterValue: at
   least one VolumeId is required`, regardless of what was asked to be copied. Response: a
   custom `{sourceVolumeId, destVolumeId}` item instead of the real `Volumes []types.Volume`
   full-volume-item shape (`awsEc2query_deserializeDocumentVolumeList`, element `item` ->
   `Volume`) -- even a hand-built request that reached the backend would have decoded to
   zero volumes, since `sourceVolumeId`/`destVolumeId` aren't real `Volume` fields. Fixed:
   `Backend.CopyVolumes` signature changed from `(volumeIDs []string, destinationRegion
   string) ([]CopyVolumesResult, error)` to `(sourceVolumeID string, size int, volumeType
   string, iops, throughput int) (*Volume, error)` (real `CopyVolumesInput`: `Size` inherits
   the source's if zero per its doc comment, `VolumeType` defaults to `gp2` -- not the
   source's type -- per its own doc comment, a real asymmetry preserved rather than
   normalized away); handler now reads `SourceVolumeId`/`Size`/`VolumeType`/`Iops`/
   `Throughput`, reuses the existing `parseVolumePerf` validation `CreateVolume` already
   uses, and renders the result through the existing `toVolumeItem`/`volumeItemSet` (the
   same types `DescribeVolumes` already renders correctly) instead of a bespoke item type.
   `CopyVolumesResult` and the now-orphaned `copyVolumesVolumeItem` type deleted.
2. **`EnableFastSnapshotRestores`/`DisableFastSnapshotRestores`** (`handler_snapshots.go`),
   same two-sided shape as (1). Request: both read `SnapshotId.N`; the real wire key is
   `SourceSnapshotId` (serializers.go:84014-84019 Enable, 83152-83157 Disable, both
   `FlatKey("SourceSnapshotId")` off `...Input.SourceSnapshotIds`) -- a real client's
   request never populated `SnapshotId.N`, so every real call silently enabled/disabled
   fast restores for zero snapshots while still reporting success. Response: a bare
   `stubResponse{Return: true}`; the real `Enable/DisableFastSnapshotRestoresOutput` have
   no `Return` field at all, only `Successful`/`Unsuccessful` sets
   (deserializers.go:210002-210012 Enable, 208034-208044 Disable) -- the real API's only
   per-item confirmation signal, always empty pre-fix even once the request-side bug is
   imagined fixed. Fixed both: request now reads `SourceSnapshotId`; response now reports
   every requested (snapshotId, availabilityZone) pair as `Successful` in its real
   *terminal* state (`enabled`/`disabled`) rather than the transient `enabling`/`disabling`
   real AWS uses for an operation this mock completes synchronously -- reusing the existing
   `fastSnapshotRestoreItem` type `DescribeFastSnapshotRestores` already renders correctly.
   `Unsuccessful` is left empty/absent: this backend's `Enable/DisableFastSnapshotRestores`
   never fails a specific pair once the top-level call succeeds, so there is nothing to
   report there without fabricating errors.
3. **`DescribeSnapshotTierStatus`** (`handler_snapshots.go`) read a `SnapshotId.N` list that
   does not exist on the real `DescribeSnapshotTierStatusInput` at all -- it has only
   `Filters` (confirmed via `api_op_DescribeSnapshotTierStatus.go`'s doc comment and
   serializers.go:80972-80999, which serializes only `DryRun`/`Filter`/`MaxResults`/
   `NextToken`), with a real `snapshot-id` filter key. A real client filtering by snapshot
   ID -- the only way the real API supports it -- had that filter silently ignored,
   returning every snapshot's tier status instead of the one asked for. Fixed: handler now
   calls `Backend.DescribeSnapshotTierStatus(nil)` (all snapshots) and applies a new
   `applySnapshotTierFilters` matching the real `snapshot-id`/`volume-id` filter keys
   (`last-tiering-operation` is real but left unenforced -- this backend tracks only the
   current tier, not the archive/restore operation history that filter's values describe,
   so enforcing it would mean fabricating a match against state that doesn't exist).
4. **`LockSnapshot`** (`handler_snapshots.go`) returned only `snapshotId`/`lockState`, even
   though the backend's `SnapshotLock` already computes `LockDurationDays`/`LockCreatedOn`/
   `LockExpiresOn` -- state the sibling `DescribeLockedSnapshots` already renders correctly
   from the same struct. The real `LockSnapshotOutput` models `lockDuration`/`lockCreatedOn`/
   `lockExpiresOn` (deserializers.go:216295-216340), so a client reading those fields
   straight off the `LockSnapshot` response it just got back -- the natural way to confirm
   what was just locked, rather than issuing a follow-up Describe -- got zero values despite
   the state genuinely existing one struct field away. Fixed by rendering all three from the
   same `lock` the handler already had. (`CoolOffPeriod`/`CoolOffPeriodExpiresOn`/
   `LockDurationStartTime` are real fields this backend has no cool-off-period concept for
   at all -- left absent, not fabricated.)
5. **`UnlockSnapshot`** (`handler_snapshots.go`) returned a generic
   `stubResponse{Return: true}`; the real `UnlockSnapshotOutput` has no `Return` field --
   only `snapshotId` (deserializers.go:223213-223224) -- so a client confirming the unlock
   via the response's own `SnapshotId`, the op's only real confirmation signal, got an empty
   string. Fixed with a dedicated `unlockSnapshotResponse{SnapshotID}`.
6. **Nine List/Describe ops across both files silently ignored real `MaxResults`/
   `NextToken` pagination**, the exact gap the redirect asked these two files to check and
   the prior pass's `ec2sweep11` fix (11 ops in `handler_images.go`/`handler_instances.go`)
   did not touch: `DescribeVolumeStatus`, `DescribeVolumesModifications`,
   `DescribeReplaceRootVolumeTasks`, `ListVolumesInRecycleBin` (volumes); `DescribeSnapshotTierStatus`
   (also bug 3, above), `DescribeLockedSnapshots`, `ListSnapshotsInRecycleBin`,
   `DescribeImportSnapshotTasks`, `DescribeFastSnapshotRestores` (snapshots). All nine
   confirmed via the installed SDK to declare `MaxResults`/`NextToken` on both Input and
   Output; none applied a cap or emitted a token pre-fix -- the same milder,
   unbounded-single-page form as `ec2sweep11`, not the medialive-style infinite loop. Fixed
   with the same `parseEC2Pagination`/`pageSlice` helpers `ec2sweep11` added, no second
   mechanism invented. Bounds per op, from the pinned SDK's own doc comments where stated,
   falling back to `DescribeImages`' 1..1000/1000 where not: `ListVolumesInRecycleBin`
   5..500 ("Valid range: 5 - 500"); `DescribeVolumesModifications` 1..500 ("up to a limit of
   500"); the other seven fall back to 1..1000/1000 entirely.
7. Sibling check on bug (6)'s class (rule 8): every other Describe/List op in both files was
   checked against the installed SDK for a `MaxResults`/`NextToken` pair on its Input/Output
   -- none of the remaining ops in either file declare one, so no further pagination bug
   exists here. Sibling check on bugs (1)/(2)'s "wrong wire key" class: grepped every other
   `parseMemberList(vals, "SnapshotId")`/`"VolumeId"` call site in both files against the
   installed SDK's serializers for the corresponding op; `DescribeLockedSnapshots` and
   `ListSnapshotsInRecycleBin` really do use `SnapshotId.N` (serializers.go:79491-79496,
   86745-86750) -- correct siblings, not touched. No other op in either file shares bug (4)
   or (5)'s "state computed but not surfaced on this op's own response" shape by inspection.

Proof for all 7: new real-SDK-client tests in `wire_field_fixes_ec2sweep12_test.go`
(`TestCopyVolumes_RealClient`, `TestLockSnapshot_SurfacesLockFields_RealClient`,
`TestUnlockSnapshot_SurfacesSnapshotID_RealClient`,
`TestDescribeSnapshotTierStatus_Filters_RealClient`,
`TestEnableDisableFastSnapshotRestores_SurfacesResults_RealClient`) plus two
`dispatchHandler`-driven multi-page round-trip tests covering all nine pagination ops
(`TestVolumesFamilyPagination_RealPageRoundTrip`, `TestSnapshotsFamilyPagination_RealPageRoundTrip`,
seeded 7 items at `MaxResults=3` -- above every op's page-size floor, learning
`ec2sweep11`'s lesson that an under-seeded fixture can't fail even pre-fix). The pagination
round-trip helper asserts not just that every item is eventually seen, but that the first
page holds `<= MaxResults` items and that more than one page was needed -- the first
version of this test asserted only item-set completeness and **false-passed against
unfixed code**, because an unpaginated handler returns everything in one page with no
`nextToken`, which trivially satisfies "every item was seen." Caught by hand-reverting
before trusting it, per the same lesson stated explicitly for next time. `TestPagination_
ForgedTokenRejected` (`persistence_test.go`) extended with all nine ops' forged-token
rejection cases. `ListVolumesInRecycleBin`/`ListSnapshotsInRecycleBin` get the
forged-token proof only, not the real multi-page round-trip: same structural gap as
`ec2sweep11`'s `ListImagesInRecycleBin` finding -- `DeleteVolume`/`DeleteSnapshot` delete
outright rather than moving anything into `recycleBinVolumes`/`recycleBinSnapshots`, so
there is no SDK-reachable producer for either table, and inventing one (e.g. an
export_test.go seeding helper) was out of scope for this pass. Hand-revert: reverted
`handler_volumes.go`, `handler_snapshots.go`, `volumes.go`, `snapshots.go`,
`interfaces.go`, `handler.go`, `handler_filters.go`, and `handler_volumes_test.go` to
`git show HEAD:...` content via `cp`, confirmed all 7 new tests fail with the documented
symptom (and only those 7 -- every pre-existing `RealClient`/pagination test in the
package still passed against the reverted files, confirming the revert didn't
accidentally touch unrelated code), then restored all eight files from the saved copies;
`md5sum` identical before/after for every file, twice (once before the lint cleanup pass,
once after).

No persisted struct's json tag, field name, or type changed -- `Volume`'s field set is
unchanged (`CopyVolumes` now populates the same fields `CreateVolume` already does, just
via a different call path); response-only XML structs and a deleted transient
`CopyVolumesResult` return type aren't part of any `backendSnapshot`. No
`ec2SnapshotVersion` bump; `go test ./pkgs/persistence/...` still green.

Modelling gaps found but NOT fixed (documented, not synthesized): `RestoreSnapshotTier`
never reads the real `PermanentRestore`/`TemporaryRestoreDays` input fields at all (only
`SnapshotId`), and its response is a bare `stubResponse{Return: true}` where the real
`RestoreSnapshotTierOutput` has `IsPermanentRestore`/`RestoreDuration`/`RestoreStartTime` --
this backend tracks no temporary-vs-permanent-restore or restore-expiry concept
whatsoever, so building the real response honestly means adding that state, not just
wiring up existing fields; out of scope for this pass, named rather than worked around.
`RestoreSnapshotFromRecycleBin` returns the same bare `stubResponse{Return: true}` where
the real `RestoreSnapshotFromRecycleBinOutput` is a near-full snapshot detail object
(`description`/`encrypted`/`outpostArn`/and more per deserializers.go:221896-221934) --
the backend already has the restored `Snapshot` in hand at the point it discards it
(`volumes.go`... `snapshots.go`'s `RestoreSnapshotFromRecycleBin`), so this is buildable
without fabrication, just not attempted this pass. `DescribeVolumeAttribute` always wraps
a boolean under whatever `Attribute` name the caller passed, never validating it against
the real two-value enum (`autoEnableIO`/`productCodes`) or rendering the real
`productCodes` list shape for that attribute -- a real client asking for `productCodes`
gets an empty list via a decode-and-skip side effect rather than a real, deliberate
empty-list render, which happens to be harmless today only because this backend never
tracks product codes at all. `GetEbsEncryptionByDefaultOutput` also models `sseType`,
entirely unrendered here -- this backend has no SSE-type concept, so left absent rather
than fabricated. False positives ruled out by hand: `RestoreVolumeFromRecycleBin`'s bare
`{Return: bool}` shape is actually correct (deserializers.go:222186-222199 confirms the
real `RestoreVolumeFromRecycleBinOutput` has only `Return`) -- flagged initially by
pattern-matching against the snapshots-side bug (bugs 1/2's shape), then confirmed correct
per-op rather than assumed broken by association.

Gates: `go build ./...` (repo-wide, `Backend.CopyVolumes`'s exported signature changed),
`make build-check` (`go build ./...`, `go vet -tags e2e ./...`, `go vet -tags integration
./...`, all clean), `go vet ./services/ec2/...`, `gofmt -l services/ec2/*.go` (clean),
`go test -race ./services/ec2/... -count=1` (`ok`, full suite including all new tests),
`go test ./pkgs/persistence/... -count=1` (`ok`), `golangci-lint run ./services/ec2/...`
(0 issues, after `--fix` corrected 2 `goconst` findings by consolidating a third
pre-existing `"volume-id"` literal onto the existing `filterKeyVolumeID` constant, plus
1 `fieldalignment` and 1 `testifylint` finding on the new test file; 5 `lll` and 1
`unparam` fixed by hand -- collapsing `paginationRoundTrip`'s always-3 `maxResults`
parameter into a local constant, the correct fix rather than a suppression). No banned
`//nolint`s.

Not reached, named per the redirect's request: every other op in
`handler_volumes.go`/`handler_snapshots.go` beyond the 39 audited and the 7 fixed matched
its real wire shape on inspection. Largest still-unswept never-named families from the
2026-08-23 tally above: `handler_spot_fleet.go` (10), `handler_client_vpn.go` (9),
`handler_security_groups.go` (9), `handler_elastic_ips.go` (8), `handler_tgw_multicast.go`
(7), `handler_instance_attrs.go` (7) -- next pass should re-derive each file's count
against its own `register*Ops` before picking one, per the method demonstrated here.

**2026-08-23 pass -- bare `Return: true` stub census (whole-repo redirect)**: a
whole-repo grep for `Return:\s+true` in `services/*/handler*.go` turned up **118 real
code sites, every one in `services/ec2`** (a 119th grep hit was a comment in
`handler_reserved_instances.go` documenting an already-fixed op, not a live site; every
other service greps clean). Mapped each site to its real `<Op>Output` struct in
`aws-sdk-go-v2/service/ec2@v1.319.1` (case-insensitive filename match, since several ops
differ only in AWS's inconsistent `Ip`/`IP`, `Dns`/`DNS`, `Acl`/`ACL` casing between
Go SDK and this repo's op names) and classified each by the redirect's three-way test:

- **47 correct** (`Return` is the real output's only member) -- left alone.
- **7 "Return + missing siblings"**: `AuthorizeSecurityGroupEgress`/
  `AuthorizeSecurityGroupIngress` (missing `SecurityGroupRules`), `DeleteSecurityGroup`
  (missing `GroupId`), `DeleteKeyPair` (missing `KeyPairId` -- **fixed this pass**),
  `DeregisterImage` (missing `DeleteSnapshotResults`), `DisassociateTrunkInterface`
  (missing `ClientToken`), `RevokeSecurityGroupIngress` (missing
  `RevokedSecurityGroupRules`/`UnknownIpPermissions`).
- **21 "no `Return` field, real members silently dropped"** -- the damaging class: e.g.
  `AssignPrivateIPAddresses` (real output is
  `AssignedIpv4Prefixes`/`AssignedPrivateIpAddresses`/`NetworkInterfaceId`, none rendered),
  `DisassociateClientVpnTargetNetwork` (real output is `associationId`/`status`, **fixed
  this pass**), `RestoreSnapshotTier`/`RestoreSnapshotFromRecycleBin` (already named
  as unfixed gaps in the 08-23 volumes/snapshots entry above -- this sweep re-derived them
  independently from the SDK side, corroborating that entry), plus 17 more listed in this
  session's report.
- **1 wire-tag mismatch, the sharpest bug found**: `ReleaseIpamPoolAllocation`
  (`handler_advanced_networking.go`/`handler_ipam.go`) rendered `<return>true</return>`,
  but the real `ReleaseIpamPoolAllocationOutput`'s only member is `Success`, decoded off
  `<success>` (`deserializers.go`,
  `awsEc2query_deserializeOpDocumentReleaseIpamPoolAllocationOutput` has no `case` for
  `"return"` at all -- the real client's deserializer silently skips the emulator's tag
  via its `default:` branch, so `Success` stayed permanently nil regardless of the actual
  release outcome). **Fixed this pass.**
- **42 "no `Return`, but the real output has no other members either"** -- e.g.
  `DeleteVpc`, `RebootInstances`, `ModifyImageAttribute`: technically wrong (the emulator
  renders a field the real op doesn't have) but harmless in practice, since an unknown XML
  tag is silently skipped by the real deserializer's own `default:` case and there is
  nothing real for a client to miss. Left alone -- fixing these is pure noise removal, not
  a client-visible bug, and 42 near-identical one-line diffs aren't a good use of a
  "depth over breadth" pass.

Fixed 3, the highest-confidence and most damaging of the non-cosmetic findings:

1. **`ReleaseIpamPoolAllocation`** (`handler_advanced_networking.go`,
   `handler_ipam.go:381`) -- wire tag mismatch above. Changed the response struct's
   `Return bool `xml:"return"`` field to `Success bool `xml:"success"``, matching
   `api_op_ReleaseIpamPoolAllocation.go:67`.
2. **`DeleteKeyPair`** (`handler_key_pairs.go:110-131`) -- added `KeyPairId` (wire tag
   `keyPairId`, `api_op_DeleteKeyPair.go:47`, confirmed against
   `awsEc2query_deserializeOpDocumentDeleteKeyPairOutput`), looked up via the existing
   `Backend.DescribeKeyPairs` before the delete removes it from the backend.
3. **`DisassociateClientVpnTargetNetwork`** (`handler_client_vpn.go:290-310`) -- replaced
   the generic `stubResponse{Return: true}` with a new
   `disassociateClientVpnTargetNetworkResponse{AssociationID, Status}`, matching
   `api_op_DisassociateClientVpnTargetNetwork.go:61-64` and its deserializer
   (`associationId`/`status` wire tags); modeled on the sibling
   `associateClientVpnTargetNetworkResponse` already in the same file, using the real
   `AssociationStatusCode` enum value `"disassociating"`
   (`types/enums.go:698`).

Proof for all 3: `services/ec2/wire_field_fixes_ec2sweep13_test.go`, three
`*_RealClient` tests built against the real `ec2sdk.Client`. Hand-reverted each fix in
turn (`cp` from a saved copy of the pre-fix file, ran only that fix's test, restored,
`md5sum` identical before/after every file):
- `TestReleaseIpamPoolAllocation_WireField_RealClient`: pre-fix failure --
  `Expected value not to be nil` on `out.Success` ("pre-fix the real deserializer has no
  case for `<return>`, so this stayed nil").
- `TestDeleteKeyPair_SurfacesKeyPairID_RealClient`: pre-fix failure -- `Expected value
  not to be nil` on `out.KeyPairId`.
- `TestDisassociateClientVpnTargetNetwork_SurfacesAssociation_RealClient`: pre-fix
  failure -- `Expected value not to be nil` on `out.AssociationId`.

No persisted struct changed (all three fixes are response-only XML shapes plus one
backend lookup call already exported); no `ec2SnapshotVersion` bump needed, and none of
`KeyPair`/`ClientVpnEndpoint`/`IpamPoolAllocation` persisted fields changed.

Gates: `go build ./...` (no exported signature changed, so `make build-check` wasn't
required, but ran anyway -- clean), `go vet ./services/ec2/...` and `gofmt -l` on every
touched file (clean), `go test -race ./services/ec2/...` (`ok`, full suite including the
3 new tests), `golangci-lint run ./services/ec2/...` (0 issues). No banned `//nolint`s.

Not reached, named: the 6 remaining Category-B sites and 20 remaining Category-C-real
sites listed in this session's report (file:line for each), and all 42 Category-C-empty
sites (cosmetic-only, listed by file above) -- deliberately not attempted this pass per
"depth over breadth." Largest remaining concentration: `handler_security_groups.go` (5
of its 9 `Return: true` sites are Category B/C-real: `AuthorizeSecurityGroupEgress`,
`AuthorizeSecurityGroupIngress`, `DeleteSecurityGroup`, `RevokeSecurityGroupIngress`,
`DisassociateSecurityGroupVpc`), then `handler_images.go` (`DeregisterImage`,
`EnableFastLaunch`, `DisableFastLaunch`), `handler_elastic_ips.go`
(`AssociateAddress`, `DisableAddressTransfer`, `ResetAddressAttribute`,
`RestoreAddressToClassic`), `handler_snapshots.go` (`RestoreSnapshotFromRecycleBin`,
`RestoreSnapshotTier` -- already named above), and `handler_instances.go`
(`EnableSerialConsoleAccess`/`DisableSerialConsoleAccess`).

**2026-08-23 pass -- census follow-up, 21 of the 27 named `Return`-only sites
fixed**: continuation of the two passes above, working the named queue of 27
sites (20 Category C, 6 Category B) the census pass classified against the
real `Return`-only-vs-real-shape distinction. Re-verified every site by hand
against the installed `aws-sdk-go-v2/service/ec2@v1.319.1` deserializers
before touching it, per parity-principles.md rule 2. The census undercounted
by one name (27 claimed, only 26 distinct op names listed) -- worked all 26.

Fixed 21:

- **Security groups** (`handler_security_groups.go`, `security_groups.go`):
  `CreateSecurityGroup` gained `SecurityGroupArn` (`SecurityGroup` gained an
  `ARN` field, set at creation from `b.Region`/`b.AccountID`, additive/
  `omitempty` -- no snapshot bump). `DeleteSecurityGroup` gained `GroupId`.
  `AuthorizeSecurityGroupIngress`/`Egress` gained `SecurityGroupRules`: since
  Authorize appends (never inserts) and rejects duplicates, the newly-added
  rules are exactly the tail of the direction-filtered
  `DescribeSecurityGroupRules` result, no backend signature change needed.
  `RevokeSecurityGroupIngress` gained `RevokedSecurityGroupRules` and
  `UnknownIpPermissions` -- `Backend.RevokeSecurityGroupIngress` signature
  changed from `(string, []SecurityGroupRule) error` to
  `(string, []SecurityGroupRule) ([]*SecurityGroupRuleDetail,
  []SecurityGroupRule, error)` so unmatched-vs-matched rules are determined
  where `sg.IngressRules` is actually visible (matches the existing
  `ruleKey`/`removeRule` identity rules, including source-group
  references). `DisassociateSecurityGroupVpc` was the worst of this batch:
  the real Output has **no Return member at all**, only a flat `<state>`
  scalar -- was rendering `stubResponse{Return: true}`, so a real client's
  `State` was always empty. New `disassociateSGVpcResponse` reports
  `"disassociated"` (this backend removes the association synchronously, no
  transient `"disassociating"` state modeled). While building this,
  confirmed a **pre-existing, out-of-queue bug** in the sibling
  `AssociateSecurityGroupVpc`: its response wraps `<state>` in an extra
  nested `<state>` (`sgVpcAssocStateItem`), which the real deserializer
  cannot decode at all (`expected value for state element, got
  xml.StartElement`) -- confirmed via a real-client call, left unfixed
  (`AssociateSecurityGroupVpc` isn't in this pass's named queue), test setup
  routed around it via direct backend calls instead. **Also found and fixed
  a self-inflicted bug while building these fixes**: reusing one Go struct
  type across multiple ops with a *tagged* `XMLName` field
  (`serialConsoleAccessStatusResponse`, `instanceEventNotifAttrsResponse`)
  silently ignores a runtime-set `XMLName` value -- `encoding/xml`'s
  `Marshal` always uses the tag when one is present, confirmed with a
  throwaway `xml.Marshal` repro. Both shared-struct types had their `XMLName`
  tag removed and every call site updated to set it explicitly; caught by a
  raw-body assertion test before it shipped.
- **Elastic IPs** (`handler_elastic_ips.go`, `elastic_ips.go`):
  `ResetAddressAttribute` gained a real `Address` (`AllocationId`/`PublicIp`;
  `PtrRecord` correctly left empty -- reset clears it, so empty is the real
  post-reset value, not a gap). `DisableAddressTransfer` gained a real
  `AddressTransfer` -- backend now captures the transfer before deleting it
  rather than discarding it. `RestoreAddressToClassic` gained `PublicIp` and
  `Status` (`"InClassic"`, the terminal value; this backend restores
  synchronously so it never reports the transient `"MoveInProgress"`).
  `AssociateAddress` was investigated and found to be a **false positive**:
  its only real member, `AssociationId`, was already wired correctly; the
  spurious `<return>` is the harmless case documented as 1 of the 42
  cosmetic-only sites two passes ago.
- **Fast Launch** (`handler_images.go`): `EnableFastLaunch` and
  `DisableFastLaunch` both had **no Return member at all** in the real
  output, only `imageId`/`ownerId`/`state`/`resourceType`/
  `maxParallelLaunches`/`launchTemplate`/`snapshotConfiguration`. Fixed
  handler-locally (no backend/persistence change): echoes the request's own
  `LaunchTemplate.*`/`MaxParallelLaunches`/`SnapshotConfiguration.*` fields
  straight from `vals` (real request data, not fabricated), defaulting
  `ResourceType="snapshot"` (the enum's only legal value) and
  `MaxParallelLaunches=6` (real AWS's documented default) when the request
  omits them. `State` reports the real transient values (`"enabling"`/
  `"disabling"`) rather than a fabricated terminal state, since neither
  exists as a terminal enum value in the real `FastLaunchStateCode`.
  `DeregisterImage` (Category B) was investigated and left unfixed: its
  missing sibling, `DeleteSnapshotResults`, is only ever populated when
  `DeleteAssociatedSnapshots=true` *and* a backing snapshot was deleted, but
  `AMIStub` (`images.go`) tracks no block-device-mapping/snapshot
  association at all -- an always-empty field would be byte-identical to
  today's response in every observable case, so adding it would be dead
  code, not a fix. Named as a real modeling gap.
- **Instance event notification attributes** (`handler_account_attrs.go`):
  `RegisterInstanceEventNotificationAttributes`/
  `DeregisterInstanceEventNotificationAttributes` both had no Return member,
  only a nested `InstanceTagAttribute` (`IncludeAllTagsOfInstance`, real and
  tracked; `InstanceTagKeys` left empty -- this backend only tracks the
  all-tags boolean, not individually registered keys).
- **Serial console access** (`handler_instances.go`):
  `Enable`/`DisableSerialConsoleAccess` had no Return member, only
  `SerialConsoleAccessEnabled` -- the same shape
  `GetSerialConsoleAccessStatus` already rendered correctly, now shared via
  one struct (see the XMLName-tag-reuse bug above).
- **ENI/NAT private IPs** (`handler_network_interfaces.go`,
  `network_interfaces.go`, `handler_nat_gateways.go`, `nat_gateways.go`):
  `AssignPrivateIpAddresses` gained `AssignedPrivateIpAddresses` --
  `Backend.AssignPrivateIPAddresses` now returns the IPs it actually
  assigned (auto-allocated or caller-supplied) instead of discarding them.
  `AssignPrivateNatGatewayAddress` had **no Return member at all**, only
  `natGatewayAddressSet`/`natGatewayId` -- was a bare `stubResponse`; fixed
  the same way as its siblings (`AssociateNatGatewayAddress`,
  `UnassignPrivateNatGatewayAddress`, already correct) and extended
  `Backend.AssignPrivateNatGatewayAddress` to honor `PrivateIpAddressCount`/
  `PrivateIpAddresses` (previously only ever assigned exactly 1 IP,
  discarding both request parameters).
- **Client VPN / Reserved Instances / VPC CIDR** (`handler_client_vpn.go`,
  `handler_reserved_instances.go`, `reserved_instances.go`,
  `handler_vpcs.go`, `vpcs.go`): `ApplySecurityGroupsToClientVpnTargetNetwork`
  gained `SecurityGroupIds` (echoes the request's own list -- the backend
  stores exactly what's requested, confirmed by reading
  `ApplySecurityGroupsToClientVpnTargetNetwork` in `client_vpn.go`).
  `CancelReservedInstancesListing` gained `ReservedInstancesListings` (the
  same shape `DescribeReservedInstancesListings` already renders) --
  `Backend.CancelReservedInstancesListing` now returns the cancelled
  listing instead of discarding it. `DisassociateVpcCidrBlock` had **no
  Return member**, only a nested `CidrBlockAssociation`
  (`associationId`/`cidrBlock`/`cidrBlockState>state`) plus `VpcId` --
  `Backend.DisassociateVpcCidrBlock` now captures the association (with
  `State` forced to the real terminal `"disassociated"` enum value, same
  synchronous-backend reasoning as the security-group case above) and its
  owning VPC ID before deleting the map entry, instead of discarding both.
- **`ResetEbsDefaultKmsKeyId`** (`handler_volumes.go`): had no Return member,
  only `KmsKeyId` -- `GetEbsDefaultKmsKeyID()` already correctly returns
  `"alias/aws/ebs"` post-reset; the handler just wasn't calling it.

False positive, confirmed correct as-is (2): `AssociateAddress` (above);
`DisassociateTrunkInterface` -- the real `DisassociateTrunkInterfaceInput`
has its own `ClientToken` request field (idempotency token, not something
carried over from association time), and the handler already echoes
`vals.Get("ClientToken")` alongside `Return`, matching the real Output
exactly.

Genuine gaps, confirmed and left unfixed (3): `RestoreSnapshotTier` --
already documented above (no temporary-vs-permanent-restore or
restore-expiry state tracked at all). `DeregisterImage` -- above.
`RestoreSnapshotFromRecycleBin` -- re-investigated past what the prior
entry above assumed. That entry said this was "buildable... just not
attempted" since the backend has the restored `Snapshot` in hand at the
point it discards it, and that part is still true (verified: the response
struct built from it round-trips correctly against a real client via a
throwaway test). But a full-repo grep for `recycleBinSnapshots.Put` found
**zero call sites** -- nothing in this backend, including `DeleteSnapshot`,
ever moves a snapshot into the recycle bin in the first place. The
precondition this op reads can never be true against a real snapshot today,
regardless of response shape, so there is no real-client round trip to
prove and the fix was reverted rather than shipped as dead code. This is a
deeper gap than previously documented: `DeleteSnapshot` needs to model
recycle-bin retention before `RestoreSnapshotFromRecycleBin`'s response
shape is worth building. Left as a `stubResponse`, same as before.

Proof for all 21: `services/ec2/wire_field_fixes_ec2sweep{14..21}_test.go`,
one or more `*_RealClient` tests per fix built against the real
`ec2sdk.Client` (raw-body `ec2.ExportDispatch` assertions where the correct
value coincides with a Go zero value, e.g. `DisableSerialConsoleAccess`).
Every fix hand-reverted in turn (`cp` from a saved pre-fix copy, ran only
that fix's test(s), confirmed failure, restored, `md5sum` identical
before/after every touched file) -- representative failures:
- `TestDisassociateSecurityGroupVpc_WireShape_RealClient`: `expected value
  for state element, got xml.StartElement` pre-fix (via the buggy
  `stubResponse{Return: true}` shape).
- `TestAuthorizeSecurityGroupIngress_SurfacesRules_RealClient`: `"[]" should
  have 1 item(s), but has 0` on `out.SecurityGroupRules`.
- `TestEnableFastLaunch_WireShape_RealClient`: `MaxParallelLaunches` decoded
  0 instead of the real default 6.
- `TestAssignPrivateNatGatewayAddress_SurfacesAddresses_RealClient`:
  `NatGatewayId`/`NatGatewayAddresses` both empty pre-fix.
- `TestDisassociateVpcCidrBlock_WireShape_RealClient`: `CidrBlockAssociation`
  nil pre-fix.

Interface signature changes (all call sites within the ec2 package's own
tests updated, no external callers found via full-repo grep):
`Backend.RevokeSecurityGroupIngress`, `Backend.ResetAddressAttribute`,
`Backend.DisableAddressTransfer`, `Backend.AssignPrivateIPAddresses`,
`Backend.AssignPrivateNatGatewayAddress`,
`Backend.CancelReservedInstancesListing`,
`Backend.DisassociateVpcCidrBlock`. `SecurityGroup` gained an `ARN` field
(additive, `omitempty`) -- no other persisted struct's json tag, field name,
or type changed; no `ec2SnapshotVersion` bump; `go test
./services/ec2/... -run TestPersist` green.

Gates: `make build-check` (`go build ./...` + `go vet -tags e2e ./...` +
`go vet -tags integration ./...`, exit 0 -- run because 7 exported `Backend`
signatures changed), `gofmt -l` on every touched file (clean), `go test
-race ./services/ec2/...` (`ok`, full suite including all new tests),
`golangci-lint run ./services/ec2/...` (5 `fieldalignment` issues on structs
added this pass, fixed via `fieldalignment -fix ./services/ec2/...` per the
gates convention, re-ran to confirm 0 issues). No banned `//nolint`s
(`grep -rn "nolint:cyclop\|nolint:gocyclo\|nolint:gocognit\|nolint:funlen"` on
`services/ec2/` — 0).

Not reached, named: none from the named 27/26-site queue -- every site was
investigated and either fixed, confirmed a false positive, or confirmed a
genuine gap and documented above. The 42 Category-C-empty cosmetic sites
from the first pass remain deliberately untouched. The
`AssociateSecurityGroupVpc` nested-`<state>` bug found incidentally above is
real but out of this pass's named queue -- left for a future pass.

**2026-08-23 pass -- `AssociateSecurityGroupVpc` fix, then
`handler_security_groups.go`/`handler_client_vpn.go` census re-derivation**:
fixed the `AssociateSecurityGroupVpc` bug named-but-left-unfixed above first
(`associateSGVpcResponse.State` was `sgVpcAssocStateItem` -- a struct with its
own `State string \`xml:"state"\`` field -- tagged `xml:"state"`, so the
response nested `<state><state>associated</state></state>`; the real
deserializer (`awsEc2query_deserializeOpDocumentAssociateSecurityGroupVpcOutput`)
reads `<state>` via `decoder.Value()`, a scalar-only read that cannot handle a
child element and fails outright: `expected value for state element, got
xml.StartElement`. Fixed by flattening `State` to a plain string, matching
`DisassociateSecurityGroupVpc`'s sibling shape exactly. The now-unused
`sgVpcAssocStateItem` type (`handler_subnets.go`) was removed.

Then re-derived the two files' never-audited counts before auditing, per the
redirect's warning that five of today's counts were wrong and the file-level
`(9)`/`(9)` estimate itself was flagged "next pass should re-derive". Method:
enumerate every op string actually registered by each file's
`register*Ops`/entries in `handler.go`'s dispatch map (dispatched op-name
strings, not just Go func names), then grep each name against this
PARITY.md's existing text to separate "named by a prior audit" (fixed, or
confirmed-and-left-unfixed like `AssociateSecurityGroupVpc` was) from mere
incidental mentions.

`handler_security_groups.go`: **17 total ops** (10 via
`registerSecurityGroupsOps` + 7 dispatched from `handler.go`'s main map:
`DescribeSecurityGroups`, `CreateSecurityGroup`, `DeleteSecurityGroup`,
`RevokeSecurityGroupEgress`, `AuthorizeSecurityGroupIngress/Egress`,
`RevokeSecurityGroupIngress`), of which **7 were already named** (the five
fixed earlier this same file plus `AssociateSecurityGroupVpc`/
`DisassociateSecurityGroupVpc`) — leaving **10 never-audited**, not 9.
Audited all 10.

`handler_client_vpn.go`: **22 total ops** (19 via `registerClientVpnOps` + 3
via `registerTGWClientVpnAttachmentOps`), of which **2 were already named**
(`ApplySecurityGroupsToClientVpnTargetNetwork`,
`DisassociateClientVpnTargetNetwork`, both fixed in the ec2sweep14 pass) —
leaving **20 never-audited**, not 9. (`CreateClientVpnEndpoint` is mentioned
once, in the parity4_new_ops entry, but only for a TGW-attachment side
effect of its `TransitGatewayConfiguration.TransitGatewayId` field, never for
its own response shape -- not counted as audited.) Audited all 22 (the 20
never-audited plus re-confirming the 2 already-fixed still hold), since
auditing only the unnamed subset in isolation risks missing a sibling bug
next to a "done" op -- none found beside the 2 already-fixed ones.

Bugs found and fixed (7):

- **`RevokeSecurityGroupEgress`** (`handler_security_groups.go`,
  `security_groups.go`): real `RevokeSecurityGroupEgressOutput`
  (`awsEc2query_deserializeOpDocumentRevokeSecurityGroupEgressOutput`) has
  `Return`/`RevokedSecurityGroupRuleSet`/`UnknownIpPermissionSet` — the exact
  shape `RevokeSecurityGroupIngress` was fixed to render in the prior pass —
  but Egress was left as bare `Return`. **A correct sibling right next to a
  broken op, again**: `RevokeSecurityGroupIngress` was the reference.
  `Backend.RevokeSecurityGroupEgress` signature changed from `(string,
  []SecurityGroupRule) error` to `(string, []SecurityGroupRule)
  ([]*SecurityGroupRuleDetail, error)`; kept its existing all-or-nothing
  validate-then-revoke semantics (an existing test asserts
  `InvalidPermission.NotFound` on any unmatched rule, unlike Ingress's
  per-rule unknown-reporting) rather than also changing that behavior, so
  `UnknownIpPermissionSet` is always empty by construction, not fabricated.
- **`DescribeSecurityGroups`** (`handler_security_groups.go`): two bugs.
  (1) Accept-and-drop: the real `DescribeSecurityGroupsInput` sends
  `GroupName.N` (`awsEc2query_serializeOpDocumentDescribeSecurityGroupsInput`,
  `object.FlatKey("GroupName")`) for `GroupNames`, but the handler read only
  `GroupId.N`, silently returning every security group regardless of the
  names requested. Fixed by reading `GroupName.N` via the existing
  `parseMemberList` and filtering by name when no `GroupId.N` was given (both
  together -- an edge case with no test coverage here -- still resolves by
  ID only, a documented limitation, not a full union). (2) No
  `MaxResults`/`NextToken` despite both being real on Input/Output ("between
  5 and 1000" per the doc comment) -- fixed with the existing
  `parseEC2Pagination`/`pageSlice` helpers, a new `ec2PageMinSecurityGroups =
  5` constant.
- **`DescribeSecurityGroupRules`**, **`DescribeStaleSecurityGroups`**,
  **`DescribeSecurityGroupVpcAssociations`**, **`GetSecurityGroupsForVpc`**
  (`handler_security_groups.go`): same "unbounded single page" gap as
  ec2sweep11 -- all four declare real `MaxResults`/`NextToken` on their real
  Input/Output (confirmed per-op against `api_op_*.go`; only
  `DescribeSecurityGroupRules` documents explicit bounds, "between 5 and
  1000", new constant `ec2PageMinSecurityGroupRules = 5`) but returned every
  item in one page. Fixed identically via `parseEC2Pagination`/`pageSlice`.
  Element names for all four (`securityGroupReferenceSet`,
  `staleSecurityGroupSet`, `securityGroupVpcAssociationSet`,
  `securityGroupForVpcSet`, `securityGroupRuleSet`, `nextToken`) were
  confirmed against each op's real deserializer before touching them; all
  already matched (a casing-insensitive protocol, so only a different
  element *name* would have been a bug -- none was).
- **`DescribeClientVpnEndpoints`**, **`DescribeClientVpnTargetNetworks`**,
  **`DescribeClientVpnRoutes`**, **`DescribeClientVpnAuthorizationRules`**
  (`handler_client_vpn.go`): same unbounded-single-page gap, no documented
  bounds on any of the four so all use the existing `ec2PageMinDefault`/
  `ec2PageMaxDefault` (1..1000). Element names (`clientVpnEndpoint`,
  `clientVpnTargetNetworks`, `routes`, `authorizationRule`, `nextToken`)
  confirmed against each deserializer; already correct.

False positive ruled out (1): `DescribeClientVpnConnections` matched the same
missing-pagination pattern by inspection, but this backend never models any
active connection (documented in the existing code comment: "No API in this
backend creates connections"), so the list is always empty and pagination
can never actually trigger. Added the `NextToken` field for shape
completeness (always empty, `omitempty`) but did not add pagination logic --
there would be nothing to prove with a test, since no fixture can ever
produce a second page.

Confirmed correct as-is, no bug (19): `handler_security_groups.go` --
`DescribeSecurityGroupReferences` (real Output has no
`MaxResults`/`NextToken` at all, unlike its four siblings above),
`UpdateSecurityGroupRuleDescriptionsIngress`/`Egress`, `ModifySecurityGroupRules`
(all three real Outputs are `Return`-only, matching the existing
`stubResponse`/`genericReturnResponse`). `handler_client_vpn.go` --
`CreateClientVpnEndpoint`, `DeleteClientVpnEndpoint`,
`AssociateClientVpnTargetNetwork`, `CreateClientVpnRoute`,
`DeleteClientVpnRoute`, `AuthorizeClientVpnIngress`, `RevokeClientVpnIngress`,
`TerminateClientVpnConnections`, `ModifyClientVpnEndpoint`,
`ExportClientVpnClientConfiguration`,
`ExportClientVpnClientCertificateRevocationList`,
`ImportClientVpnClientCertificateRevocationList`,
`AcceptTransitGatewayClientVpnAttachment`,
`RejectTransitGatewayClientVpnAttachment`,
`DeleteTransitGatewayClientVpnAttachment` -- every field name checked against
the real Output struct and (for the non-scalar ones) the deserializer's
`EqualFold` cases; all matched.

Modelling gaps ruled real, not fabricatable, left alone: `CreateClientVpnEndpoint`
never reads `AuthenticationOptions`/`ConnectionLogOptions`/
`ClientConnectOptions`/`ClientLoginBannerOptions`/
`ClientRouteEnforcementOptions`/`TagSpecifications`/`DisconnectOnSessionTimeout`/
`EndpointIpAddressType`/`TrafficIpAddressType` -- this backend's
`ClientVpnEndpoint`/`clientVpnEndpointItem` model none of these at all (no
tag storage for this resource type exists anywhere, confirmed by grep), so
this is a pre-existing structural gap, not a new accept-and-drop bug of the
kind fixed above. `SelfServicePortal` (Input, an `enabled`/`disabled` enum)
and `SelfServicePortalUrl` (Output, a real generated URL) are two distinct
real fields this backend conflates into one `SelfServicePortalURL` string
echoed verbatim -- flagged, not fixed: a believable synthetic URL format
can't be sourced from anything real without guessing at AWS's actual
generation scheme. `Filters` on all five `DescribeClientVpn*` list ops and on
`DescribeSecurityGroups`/`DescribeSecurityGroupVpcAssociations`/
`GetSecurityGroupsForVpc` are real but entirely unimplemented in this
backend (only `group-id` is special-cased for `DescribeSecurityGroupRules`);
left alone, matching this pass's pagination-only scope.

Snapshot bump: not needed. The only persisted-shape change is
`Backend.RevokeSecurityGroupEgress`'s added return value (an interface
method signature, not a struct field), which `pkgs/persistence` doesn't
serialize; no field/tag/type on any persisted struct changed. `go test
./pkgs/persistence/...` green.

Proof: `TestAssociateSecurityGroupVpc_WireShape_RealClient` (new) and
`TestDisassociateSecurityGroupVpc_WireShape_RealClient` (updated to drive
`AssociateSecurityGroupVpc` through the real client instead of the backend
directly, now that it's fixed) in `wire_field_fixes_ec2sweep14_test.go`;
`wire_field_fixes_ec2sweep22_test.go` (security groups: revoke-egress,
group-name filter, 3 pagination round-trips via the real
`ec2sdk.NewDescribe*Paginator`s); `wire_field_fixes_ec2sweep23_test.go`
(client VPN: 4 pagination round-trips). Every fix hand-reverted in turn (`cp`
from a saved pre-fix copy, ran only that fix's test, confirmed failure,
restored, `md5sum` identical before/after every touched file) --
representative failures:
- `TestAssociateSecurityGroupVpc_WireShape_RealClient`: `expected value for
  state element, got xml.StartElement` pre-fix.
- `TestRevokeSecurityGroupEgress_SurfacesRevokedRules_RealClient`: `"[]"
  should have 1 item(s), but has 0` on `out.RevokedSecurityGroupRules`.
- `TestDescribeSecurityGroups_GroupNameFilter_RealClient`: 3 groups returned
  instead of 1 (`GroupName.N` silently dropped).
- `TestDescribeSecurityGroupRules_Pagination_RealClient` /
  `TestDescribeClientVpnEndpoints_Pagination_RealClient`: `"1" is not
  greater than or equal to "2"` (paginator never split across pages).

Gates: `go build ./services/ec2/...`, `make build-check` (`go build ./...` +
`go vet -tags e2e ./...` + `go vet -tags integration ./...`, exit 0 -- run
because `Backend.RevokeSecurityGroupEgress`'s exported signature changed),
`gofmt -l services/ec2/*.go` (clean), `go test -race ./services/ec2/...
-count=1` (`ok`, full suite including all new tests), `go test
./pkgs/persistence/...` (`ok`), `golangci-lint run ./services/ec2/...` (2
issues found and fixed by hand: 1 `nlreturn` on the `AssociateSecurityGroupVpc`
fix, 1 `modernize` replacing a hand-rolled contains-loop in a new test with
`slices.Contains`; 0 issues after). No banned `//nolint`s.

Not reached, named: none -- every op in both files' never-audited pool (10 +
20 = 30) was checked against the real SDK this pass.

**2026-08-23 pass -- `handler_spot_fleet.go`/`handler_elastic_ips.go`, the two
next never-named families off the 2026-08-23 tally above**: re-derived both
counts by enumerating every dispatched op-name string rather than trusting
the mapped estimate. `handler_spot_fleet.go`: mapped 10, re-derived 10 --
`registerSpotFleetOps` and `spotFleetSupportedOperations()` agree exactly, no
undercount here (this family already had a dedicated wire-shape audit pass
under `aac77417a`/`b6b4b4940`). `handler_elastic_ips.go`: mapped 8,
re-derived **14** -- the mapped count only saw the 9 ops in
`registerElasticIpsOps`/`elasticIpsSupportedOperations()` (also 1 over: that
list includes only 9, not the source of the gap) and missed the 5 basic CRUD
ops (`AllocateAddress`/`AssociateAddress`/`DisassociateAddress`/
`ReleaseAddress`/`DescribeAddresses`) that dispatch straight from `handler.go`'s
`buildCoreOps` table without going through a `register*Ops`/`*SupportedOperations`
pair at all -- exactly the "thin-bodied op the field-count proxy misses"
failure mode named above, just one level removed (a whole *file* of
minimally-wired ops missing from the family's own manifest, not a single op
within it). 24 ops audited across both files; **12 wrong** (6 of 10 spot
fleet, 6 of 14 elastic IPs).

Bugs, each confirmed against the installed `aws-sdk-go-v2/service/ec2@v1.319.1`
deserializer/serializer, not this backend's own output:

1. **`RequestSpotFleet` accepted-and-dropped `TagSpecification.N`**
   (accept-and-drop, request side): the real serializer nests it under
   `SpotFleetRequestConfig.TagSpecification.N.*`
   (`serializers.go:65330`, the same `object.Key("SpotFleetRequestConfig")` +
   `FlatKey` nesting `LaunchSpecifications` already used one line up in this
   same handler) -- not the top-level `TagSpecification.N` every other
   create op uses. A fleet tagged at creation came back untagged from
   `DescribeSpotFleetRequests`. Fixed with a dedicated
   `parseSpotFleetTagSpecification` (the shared `parseTagSpecification`
   helper assumes the top-level form) + `Backend.CreateTags`.
2. **`CancelSpotFleetRequests`'s `<error>` rendered as a bare string.** The
   real deserializer
   (`awsEc2query_deserializeDocumentCancelSpotFleetRequestsError`,
   `deserializers.go:81882`) reads `<error>` as a nested `<code>`/`<message>`
   structure (`types.CancelSpotFleetRequestsError`); a scalar has no child
   elements for it to find, so `NodeDecoder.Token()` returns `done` on the
   first pass and the real client's `Code`/`Message` end up zero-valued --
   silent data loss, not a hard decode failure (confirmed empirically: the
   pre-fix real-client test failed on an empty string, not an error). The
   backend's own error value was also wrong: `"SpotFleetRequestIdNotFound"`
   is not a real `CancelBatchErrorCode`; the real enum value is
   `"fleetRequestIdDoesNotExist"` (`enums.go:1198`). Fixed both: new
   `cancelSpotFleetErrorDetail{Code, Message}` nested type, backend value
   corrected.
3. **`DescribeSpotFleetRequestHistory`'s `<eventInformation>` rendered as a
   bare string** -- the same nesting-mismatch shape as bug 2, independently
   discovered while wiring that op's pagination. The real deserializer
   (`awsEc2query_deserializeDocumentEventInformation`, `deserializers.go:99294`)
   reads it as a nested `<eventDescription>`/`<eventSubType>`/`<instanceId>`
   structure (`types.EventInformation`); this backend's only tracked field
   (a plain description string) was rendered as the bare element instead of
   nested under `eventDescription`, so a real client's
   `EventInformation.EventDescription` was always `nil`. Fixed with a new
   `spotFleetEventInformationItem{EventDescription}` wrapper.
4. **`DescribeAddresses` accepted-and-dropped `PublicIp.N`** (accept-and-drop,
   request side) -- the real request member (`serializers.go:76230`,
   `FlatKey("PublicIp")`) is separate from `Filters`; a client filtering by
   public IP (e.g. `aws ec2 describe-addresses --public-ips x.x.x.x`, a
   common real invocation) got every address back instead. Fixed by folding
   `PublicIp.N` into the existing `"public-ip"` filter matcher rather than a
   new code path.
5. **`EnableAddressTransfer`/`DisableAddressTransfer`/`DescribeAddressTransfers`
   share `addressTransferDetailItem`, which used wire tags
   `"transferOfferStatus"` and `"transferOfferExpiry"`** -- DIFFERENT element
   NAMEs than the real `AddressTransfer` deserializer's
   `"addressTransferStatus"` and `"transferOfferExpirationTimestamp"`
   (`deserializers.go:75605`), the exact "different element NAME, not a
   casing near-miss" bug class this protocol is EC2-Query/case-insensitive
   about. Both fields were silently zero-valued on every real client despite
   the server computing and sending real data for both, on all three ops at
   once (see sibling note below). Fixed by retagging the shared struct; the
   type lives in `handler_volumes.go` (dead there, consumed only by
   `handler_elastic_ips.go`) so that's the file touched, not a new struct.
6. **`DescribeMovingAddresses` accepted-and-dropped its `"moving-status"`
   `Filter`** (accept-and-drop, request side) -- real AWS documents exactly
   one filter name for this op; it was silently ignored. Fixed with a small
   inline `anyEqual` filter (no shared EIP filter helper covers this shape).
7. **Seven List ops ignore real pagination** (the systemic class named
   above, now closed for these two files): `DescribeSpotFleetRequests`,
   `DescribeSpotFleetInstances`, `DescribeSpotFleetRequestHistory`,
   `GetSpotPlacementScores`, `DescribeAddressesAttribute`,
   `DescribeAddressTransfers`, `DescribeMovingAddresses` all declare real
   `MaxResults`/`NextToken` on their SDK Input/Output but returned every
   item in one unbounded page. Fixed via the existing
   `parseEC2Pagination`/`pageSlice` helpers (`handler.go`), matching the
   `ec2sweep11`/`ec2sweep23` convention exactly. `DescribeSpotFleetRequestHistory`
   also gained the real `LastEvaluatedTime` (present only on the final page,
   set to now -- this backend has no other honest source for it).
   `DescribeMovingAddresses` needed its own `ec2PageMinMovingAddresses = 5`
   bound (its real doc says "between 5 and 1000", unlike the other six,
   which fall back to the documented 1..1000 default). Not paginated,
   correctly: `DescribeAddresses` (the basic op) -- confirmed its real Input
   has no `MaxResults`/`NextToken` at all.

Sibling check: bug 5 is the one case where a fix landed on all three siblings
at once (they share one struct), not one-of-a-pair -- checked
`ModifyAddressAttribute`/`ResetAddressAttribute` (the other ops touching
`AddressAttribute`-shaped types) and both were already correct (`ResetAddressAttribute`
was fixed in an earlier, uncredited pass per its existing code comment).
Bug 7's family was fully checked: `DescribeAddresses` is the one list op in
either file confirmed to have no real pagination at all, not a missed sibling.

False positives ruled out (all confirmed via the real deserializer, not
assumed): `DeleteSpotDatafeedSubscription`/`DisassociateAddress`/
`ReleaseAddress`'s bare `<return>true</return>` are harmless -- all three
deserializers (`deserializers.go:21201`, `:46290`, `:68388`) discard the
response body via `io.Copy(io.Discard, ...)` without parsing it at all, so
no client ever reads the tag. `AssociateAddress`'s spurious `<return>` is the
same already-documented cosmetic-only site from two passes ago (its one real
member, `AssociationId`, was already correct). `ModifyAddressAttribute`'s
extra `<domainName>` element is skipped by the real deserializer's default
case, same shape.

Modelling gaps (documented, not fixed -- structurally larger, not a broken
partial implementation): `RequestSpotFleet` does not model
`LaunchTemplateConfigs`/`SpotMaintenanceStrategies`/`OnDemand*`/`ValidFrom`/
`ValidUntil` -- whole unimplemented request fields, not wire-shape bugs in
what IS implemented. `AllocateAddress` never sets `CarrierIp`/
`CustomerOwnedIp`/`CustomerOwnedIpv4Pool`/`NetworkBorderGroup`/
`PublicIpv4Pool` -- Wavelength/BYOIP address pools have no backing concept
in this backend at all.

Not reached, named: none -- all 10 spot-fleet and all 14 elastic-IP ops were
audited against the real SDK this pass.

Snapshot bump: not needed. No field/tag/type on any persisted struct
changed -- `addressTransferDetailItem`/`cancelSpotFleetErrorDetail`/
`spotFleetEventInformationItem` are wire-response-only XML types, and
`SpotFleetCancelResult` (whose `Error` value changed) is never persisted
(constructed fresh per `CancelSpotFleetRequests` call, not part of any
`backendSnapshot`). `go test ./pkgs/persistence/...` not run -- no touched
type is registered there.

Proof: `wire_field_fixes_ec2sweep24_test.go`, 12 new real-SDK-client tests,
one per bug above (bug 7's seven ops split across
`TestDescribeSpotFleetRequests_Pagination_RealClient`,
`TestDescribeSpotFleetInstances_Pagination_RealClient`,
`TestDescribeSpotFleetRequestHistory_Pagination_RealClient`,
`TestGetSpotPlacementScores_Pagination_RealClient`,
`TestDescribeAddressesAttribute_Pagination_RealClient`,
`TestDescribeAddressTransfers_Pagination_RealClient`,
`TestDescribeMovingAddresses_Pagination_RealClient`). Every fix hand-reverted
in turn (`git show HEAD:<path>` over the 5 touched non-test files, ran the
full new-test set, confirmed 11 of 12 failed -- `TestDescribeAddresses_PublicIpFilter_RealClient`'s
sibling assertions all failed together with it -- restored from a saved
copy, `md5sum` identical before/after all 5 files). Representative failures:
- `TestRequestSpotFleet_TagSpecification_RealClient`: `Tags` empty.
- `TestCancelSpotFleetRequests_ErrorCode_RealClient`: `Code` `""`, wanted
  `"fleetRequestIdDoesNotExist"`.
- `TestDescribeSpotFleetRequestHistory_Pagination_RealClient`:
  `LastEvaluatedTime` nil, plus every page returning the same (empty)
  `EventDescription` string, tripping the disjoint-IDs assertion.
- `TestDescribeAddresses_PublicIpFilter_RealClient`: 3 addresses returned
  instead of 1.
- `TestEnableAddressTransfer_StatusAndExpiry_RealClient`:
  `AddressTransferStatus` `""` (wanted `"pending"`),
  `TransferOfferExpirationTimestamp` nil.
- `TestDescribeMovingAddresses_MovingStatusFilter_RealClient`: a
  non-matching filter value still returned the entry.
- All six other `*_Pagination_RealClient` tests: `"1" is not greater than or
  equal to "2"` (paginator never split across pages).

Gates: `go build ./...` (repo-wide, background job, exit 0 -- no exported
signature changed, run anyway since `pkgs/page` helpers are shared),
`go vet ./services/ec2/...` (clean), `gofmt -l services/ec2/*.go` (clean),
`go test -race -count=1 ./services/ec2/...` (`ok`, full suite including all
new tests), `golangci-lint run ./services/ec2/...` (started at 11 issues --
8 `fieldalignment` on the new/modified response structs with an appended
`NextToken`, fixed by hand by reordering `NextToken` before the
slice-backed field per the existing `describeSnapshotTierStatusResponse`
convention rather than running `fieldalignment -fix`; 1 `shadow` on a
reused `err` inside `handleRequestSpotFleet`, fixed by renaming to `tagErr`;
2 `intrange` on manual pagination loops, fixed by converting to
`for range ec2sweep11LoopGuard`; final state: 1 `nlreturn`, a disabled
linter per this repo's convention, left as-is). No banned `//nolint`s.

**2026-08-23 pass -- `handler_tgw_multicast.go`/`handler_instance_attrs.go`,
next two never-named families**: re-derived both counts by enumerating both
dispatch paths (`register*Ops` and any op whose handler func is actually
*defined* in the file but dispatched from `handler.go`'s `buildCoreOps`
table), per the elastic-IPs lesson above. `handler_tgw_multicast.go`: mapped
7, re-derived **16** -- all 16 from its own `registerTGWMulticastOps`
(`tgwMulticastSupportedOperations()` already listed the same 16; the
mapped-7 estimate was simply stale). No `buildCoreOps` ops belong to this
file; `AcceptTransitGatewayMulticastDomainAssociations` looks related by
name but dispatches from `handler_buildops_advanced.go` and is implemented
in `accept_ops.go` -- a different family, out of scope, named here only so
it isn't mistaken for an uncounted op of this one. `handler_instance_attrs.go`:
mapped 7, re-derived **16** -- 14 from its own `registerInstanceAttrOps`, plus
`ModifyInstanceAttribute`/`ResetInstanceAttribute`, which dispatch from
`buildCoreOps` in `handler.go` but whose handler funcs are defined in this
file. `DescribeInstanceAttribute` also dispatches from `buildCoreOps` and is
named alongside them there, but its handler func lives in
`handler_instances_lifecycle.go` -- a different family, excluded. 32 ops
audited across both files; **3 wrong**, all in `handler_tgw_multicast.go`.
`handler_instance_attrs.go`/`instance_attrs.go` came back clean: every
response element name, every request field name (including the
`AssociationTarget.InstanceId`/`AssociationTarget.DedicatedHostId` nesting)
checked against the pinned SDK's serializers/deserializers matched exactly,
and the two near-miss candidates below were ruled out as false positives,
not fixed.

Bugs, each confirmed against the installed `aws-sdk-go-v2/service/ec2@v1.319.1`
deserializer/serializer, not this backend's own output:

1. **`SearchTransitGatewayMulticastGroups` copied `NetworkInterfaceId` into
   the response's `TransitGatewayAttachmentId` field** (wrong value, not a
   name/nesting bug): the real `TransitGatewayMulticastGroup` type
   (`types/types.go:24172`) declares `NetworkInterfaceId` and
   `TransitGatewayAttachmentId` as two separate fields; our backend's
   `TransitGatewayMulticastGroupEntry` never tracked an attachment ID at
   all, so the handler had silently duplicated the ENI ID into the wrong
   field. Fix: drop the bogus assignment (the field now renders absent via
   `omitempty`, since this backend genuinely has no attachment-ID data for a
   multicast group entry, rather than fabricate a value).
2. **Four List/Get ops ignored real `MaxResults`/`NextToken` pagination**
   (list op ignoring pagination): `DescribeTransitGatewayMulticastDomains`,
   `DescribeTransitGatewayMeteringPolicies`,
   `GetTransitGatewayMulticastDomainAssociations`,
   `SearchTransitGatewayMulticastGroups` all declare both on their real
   Input/Output (none document an explicit range, so `ec2PageMinDefault`/
   `ec2PageMaxDefault` apply per the existing convention) but returned every
   item in one unbounded page. Fixed with the established
   `parseEC2Pagination`/`pageSlice` helpers, matching the pattern already
   used by `handler_elastic_ips.go` etc. Note:
   `DescribeTransitGatewayMeteringPolicies` has no SDK-generated
   `Paginator` (the smithy model doesn't mark it paginated despite the
   Input/Output declaring the fields) -- its proof test drives pages by
   hand instead of `NewXxxPaginator`.
3. **`CreateTransitGatewayMulticastDomain`/`CreateTransitGatewayMeteringPolicy`
   accepted-and-dropped `TagSpecifications`** (accept-and-drop): both IDs
   were already valid `CreateTags` targets (`resource_types.go` lists
   `tgwMeteringPolicies`/`tgwMulticastDomains`), and `DeleteTransitGateway*`
   already cleaned up `b.tags` on delete, but nothing ever wrote to it on
   create and neither Describe/Create/Delete response rendered a `tagSet`
   element -- a tag set at creation came back invisible from every read.
   Fixed by threading a `tags map[string]string` through both
   `Backend.CreateTransitGatewayMulticastDomain`/
   `CreateTransitGatewayMeteringPolicy` (interface signature change, single
   implementer `InMemoryBackend`; `b.setTagsLocked` mirrors the existing
   `transit_gateways.go` convention) and rendering `h.Backend.TagsForResource(id)`
   via `tagItemsFromMap` in both Describe loops and both Delete responses.
   Sub-bug found while writing this fix's proof test:
   `CreateTransitGatewayMeteringPolicyInput` sends its tags as
   `TagSpecifications.N.*` (plural) on the wire
   (`serializers.go:72985`, `FlatKey("TagSpecifications")`) -- one of only 5
   ops in the whole pinned SDK that do this, against 112 that use the
   near-universal singular `TagSpecification.N.*`. The shared
   `parseTagSpecification` helper (used at 48 other call sites, all
   singular) would have silently matched nothing for this op, so a
   `parseTagSpecificationPlural` variant was added locally rather than
   changing the shared helper's wire format for everyone. The other 4
   plural-form ops (`CreateCapacityReservation`,
   `CreateTransitGatewayPolicyTable`, `CreateTransitGatewayRouteTable`,
   `CreateTransitGatewayVpcAttachment`) belong to other families and were
   not touched -- worth a follow-up sweep.

Correct sibling check: yes. `AssociateTransitGatewayMulticastDomain`/
`DisassociateTransitGatewayMulticastDomain`/
`GetTransitGatewayMulticastDomainAssociations` share the
`tgwMulticastDomainAssociationsAggregate`/`tgwMulticastGetAssociationItem`
wire types, both of which declare `ResourceID`/`ResourceOwnerID` fields
matching the real `TransitGatewayMulticastDomainAssociations` type
(`types/types.go:24130`) exactly by name -- but neither field is ever
populated, because the shared `TransitGatewayMulticastDomainAssociation`
model (`accept_ops.go`, owned by the `AcceptTransitGatewayMulticastDomainAssociations`
family) carries no VPC-ID/owner-ID data to populate them from. Named as a
modelling gap below, not fixed here: filling it in would mean adding a
`tgwVpcAttachments` lookup and possibly widening that shared struct, which
risks the sibling family this pass didn't audit. `ModifyTransitGatewayMeteringPolicy`
in `handler_tgw_peripherals.go` shares `tgwMeteringPolicyToItem` and needed
a one-line call-site update for the new `tags` parameter (already followed
the correct `h.Backend.TagsForResource(...)`-at-render-site pattern used one
function above it for `tgwVpcAttachmentToItem`, so nothing else there was
wrong) -- not a bug, a forced touch from the shared-helper signature change,
confirmed by reading the surrounding function.

Modelling gaps and false positives ruled out, separately from the bugs above:

- **`ResourceID`/`ResourceOwnerID` never populated on multicast domain
  associations** (see sibling check above) -- real fields, real wire tags,
  no backing data in this backend's association model. Not fixed; flagged
  for whoever next touches `accept_ops.go`'s
  `TransitGatewayMulticastDomainAssociation`.
- **`ResourceOwnerId`/`SubnetId` missing entirely from `tgwMulticastGroupItem`**
  (real fields on `types.TransitGatewayMulticastGroup`, `types/types.go:24172`)
  -- same shape of gap, not fixed, `TransitGatewayMulticastGroupEntry` has no
  subnet/owner data to source them from.
- **`tagSet`/ARN fields on the multicast-domain and metering-policy item
  types themselves** -- `tagSet` is now wired (bug 3); the `transitGatewayMulticastDomainArn`
  field the real deserializer also reads (`deserializers.go:168416`) has no
  backend equivalent (no ARN modelling anywhere in this service) and was
  left out, consistent with how the rest of the service handles ARNs.
- **False positive -- `ModifyInstanceAttribute`/`ResetInstanceAttribute`'s
  hardcoded `Return: true`**: looked like the 21-fixed `Return: true` bug
  class at a glance (`ModifyInstanceAttributeOutput`/`ResetInstanceAttributeOutput`
  in the real SDK have no `Return` member at all), but the real
  deserializer for both ops (`deserializers.go:59714`+) discards the
  response body wholesale (`io.Copy(io.Discard, response.Body)`) without
  parsing any XML -- unlike the 21 previously-fixed sites, a real client
  cannot observe this field either way, so there is no round-trip proof
  possible and it was left alone.
- **False positive -- `ModifyInstanceAttribute`'s missing `Kernel.Value`/
  `Ramdisk.Value` parsing**: the real request supports both
  (`serializers.go:87595`/`87602`), and this handler's `parseModifyInstanceAttributeValue`
  checks list omits both, so a real client setting either gets our generic
  "exactly one modifiable attribute" error instead of a real one. Not a
  two-file bug, though: `attrKernel`/`attrRamdisk` are already a deliberate,
  commented design choice one file over
  (`handler_instances_lifecycle.go`'s `instanceAttributeValue`: "Modern
  (HVM) instances have no kernel/ramdisk image; AWS returns an empty
  value") -- this backend models only HVM instances, so both real AWS and
  this emulator reject the call either way, just with different error
  text. Left alone rather than half-wiring a request param this backend has
  nowhere to store.
- **False positive -- `EnclaveOptions` unmodelled in `ModifyInstanceAttribute`**:
  real request field (`serializers.go:87562`), silently rejected here same
  as Kernel/Ramdisk, but Nitro Enclaves have zero modelling anywhere in this
  service (not just this op) -- a whole-service gap, not scoped to these two
  files.
- **False positive -- `ModifyInstanceCapacityReservationAttributesOutput`
  hardcodes `Return: true`, discarding the backend's own bool**: looked
  like a dropped-value bug, but `Backend.ModifyInstanceCapacityReservationAttributes`
  never returns `(false, nil)` -- every non-error path returns the updated
  instance, so `true` is always correct.
- **False positive -- `stoppedRequiredAttrs` (backend, `instance_attrs.go`)
  omits `attrUserData` while `modifyInstanceAttributeStoppedRequired`
  (handler) includes it**: looked like a missed guard, but
  `SetInstanceAttribute` is also called from `RunInstances`
  (`handler_instances_lifecycle.go:58`, setting initial `UserData` on a
  freshly-launched, non-stopped instance) -- the handler-level check
  correctly enforces "stopped to *modify* userData post-launch" while the
  shared backend setter correctly does not re-enforce that for the
  launch-time case. Confirmed by reading both call sites; not a bug.

Not reached, named: none -- all 32 ops across both files were audited
against the real SDK this pass.

Snapshot bump: not needed. `TransitGatewayMulticastDomain`,
`TransitGatewayMeteringPolicy`, `TransitGatewayMeteringPolicyEntry`, and
`TransitGatewayMulticastGroupEntry` (the persisted structs backing this
family) are unchanged -- tags live in the pre-existing generic `b.tags` map,
not on any of these structs, and every touched response type
(`tgwMulticastDomainItem`, `tgwMeteringPolicyItem`, the four `NextToken`
additions) is wire-response-only, never part of a `backendSnapshot`.
`go test ./pkgs/persistence/...` run anyway: `ok`.

Proof: `wire_field_fixes_ec2sweep25_test.go`, 7 new real-SDK-client tests.
Each hand-reverted in place (not via a saved-file `cp` for the isolated
`TransitGatewayAttachmentId`/pagination bugs -- reverted the specific hunk
in `handler_tgw_multicast.go`, ran the one test, confirmed the failure
below, then restored the whole file from a `cp`-saved copy and `md5sum`
diffed identical; same restore-and-verify done for `interfaces.go`,
`tgw_multicast.go`, and `handler_tgw_peripherals.go`, which were untouched
by any individual hunk revert but re-verified byte-identical regardless).
Representative failures:
- `TestSearchTransitGatewayMulticastGroups_TransitGatewayAttachmentId_RealClient`:
  `TransitGatewayAttachmentId` set (to the ENI ID) instead of nil.
- `TestSearchTransitGatewayMulticastGroups_Pagination_RealClient`: `"1" is
  not greater than or equal to "2"` (paginator never split across pages) --
  same failure shape confirmed for the other three pagination ops before
  restoring.
- `TestCreateTransitGatewayMulticastDomain_Tags_RealClient`: "Tags empty on
  describe - TagSpecification accepted at create but dropped from
  Describe".
- `TestCreateTransitGatewayMeteringPolicy_Tags_RealClient`, reverted to the
  singular-form `parseTagSpecification` to isolate the plural-wire-key
  sub-bug specifically: same "Tags empty on describe" failure, confirming
  the plural key really is required for this one op.

Gates: `go build ./...` (clean, exported `Backend` interface signature
changed -- `CreateTransitGatewayMulticastDomain`/`CreateTransitGatewayMeteringPolicy`
gained a `tags map[string]string` parameter, single implementer
`InMemoryBackend`, no other package implements `Backend`), `make build-check`
(clean: `go build ./...` + `go vet -tags e2e ./...` + `go vet -tags
integration ./...`), `go vet ./services/ec2/...` (clean), `gofmt -l
services/ec2/*.go` (clean after `goimports -w`/`golines -w --max-len=120` on
the two touched files -- both had pre-existing-style violations introduced
by this pass's own edits, not carried over from before it), `go test -race
-count=1 ./services/ec2/...` (`ok`, full suite including all 7 new tests),
`golangci-lint run ./services/ec2/...` (`0 issues`), `go test
./pkgs/persistence/...` (`ok`). No banned `//nolint`s.

**2026-08-23 follow-up pass (gopherstack-dj4i) -- the other four plural-form
tag ops**: the pass above fixed `CreateTransitGatewayMeteringPolicy` and gave
it a scoped `parseTagSpecificationPlural` (the pinned `ec2@v1.319.1`
`serializers.go` has exactly five ops that serialize tags as plural
`TagSpecifications.N.*` rather than the near-universal singular
`TagSpecification.N.*`) but deliberately left the other four unidentified.
Grepped `serializers.go` for all five `"TagSpecifications"` occurrences and
named the owning op for each by walking back to the nearest
`awsEc2query_serializeOpDocument*Input` function:
`CreateCapacityReservation` (line 69572/function 69482), `CreateTransitGatewayMeteringPolicy`
(72985/72968, fixed previously), `CreateTransitGatewayPolicyTable`
(73120/73110), `CreateTransitGatewayRouteTable` (73237/73227),
`CreateTransitGatewayVpcAttachment` (73275/73251).

All four remaining ops are implemented in gopherstack, and none of them
parsed tags at all -- not even with the wrong (singular) parser. Every one
accepted `TagSpecifications` in the request, returned `200 OK`, and silently
dropped the tags:

- `CreateCapacityReservation` (`capacity_reservations.go`,
  `handler_capacity_reservations.go`): backend took no `tags` param at all,
  and `capacityReservationItem` (`handler_accept_ops.go`) had no `TagSet`
  field -- the whole capacity-reservation family had zero tag rendering,
  even though `CreateCapacityReservationBySplitting` and
  `PurchaseCapacityBlock` already parse tags correctly (singular form, not
  part of this bug) and already store them via `b.tags`/`setTagsLocked` --
  those tags were ALSO being silently dropped from every response, just
  because the item struct never had a `TagSet` field to render them into.
  Added `TagSet []simpleTagItem` to `capacityReservationItem`, threaded a
  `tags map[string]string` param through `toCapacityReservationItem` and
  `Backend.CreateCapacityReservation`, and rewired all six call sites
  (`handler_capacity_reservations.go`, `handler_capacity_block.go`,
  `handler_capacity_reservation_ops.go` x2, `handler_accept_ops.go`'s
  Describe) to pass real tags (freshly-parsed tags for Create/Split's
  destination, `Backend.TagsForResource` for everything reading an existing
  reservation).
- `CreateTransitGatewayPolicyTable` (`tgw_peripherals.go`,
  `handler_tgw_peripherals.go`): same shape -- no `tags` param, no `TagSet`
  field on `tgwPolicyTableItem`. Added both, wired Create/Describe/Delete.
- `CreateTransitGatewayRouteTable` (`ec2core.go`, `handler_ec2core.go`): same
  shape. Added `TagSet` to `tgwRouteTableItem`, wired Create/Describe/Delete.
- `CreateTransitGatewayVpcAttachment` (`networking1.go`,
  `handler_networking1.go`): `tgwVpcAttachmentItem` already had `TagSet` and
  `Describe` already rendered it via `TagsForResource` -- only `Create`
  itself never parsed the request's tags (passed `nil` into
  `tgwVpcAttachmentToItem`) and the backend method never stored them. Fixed
  both.

All four fixes reuse the existing `parseTagSpecificationPlural` helper
(`handler_tgw_multicast.go`) with each op's real AWS `ResourceType` string
(`capacity-reservation`, `transit-gateway-policy-table`,
`transit-gateway-route-table`, `transit-gateway-attachment` -- confirmed
against `types/enums.go` in the pinned SDK). The shared singular
`parseTagSpecification` (48 call sites) was not touched.

Noted but explicitly out of scope: `CreateTransitGatewayVpcAttachment`'s
backend signature already had a `subnetIDs []string` parameter that was
discarded (`_ []string`) -- `SubnetIds` from the create request is dropped
and only settable later via `ModifyTransitGatewayVpcAttachment`. That's a
different bug (not a tag bug, not one of the five plural-form ops) and was
left alone; worth its own bd issue.

Snapshot bump: not needed. `CapacityReservation`, `TransitGatewayPolicyTable`,
`TransitGatewayRouteTable`, and `TransitGatewayVpcAttachment` (the persisted
structs) are unchanged -- tags live in the pre-existing generic `b.tags` map
via `setTagsLocked`/`TagsForResource`, not on any of these structs. Every
touched type (`capacityReservationItem`, `tgwPolicyTableItem`,
`tgwRouteTableItem`) is wire-response-only. `go test ./pkgs/persistence/...`
run anyway: `ok`.

Proof: `wire_field_fixes_ec2dj4i_test.go`, 4 new real-SDK-client tests (one
per fixed op), each doing a Create-with-tags then Describe round trip.
Hand-reverted all 20 touched non-test files at once via `cp` to a scratchpad
(the four fixes share a chain of signature changes across
`interfaces.go`/handlers/backends, so no single hunk reverts in isolation),
ran the new test file against the reverted tree, confirmed all four fail,
then restored every file from the scratchpad copy and `md5sum`-diffed all 20
identical against the post-fix state. Failures under the reverted (pre-fix)
code, all the same shape:
- `TestCreateCapacityReservation_Tags_RealClient`: "Tags empty on create
  response - TagSpecifications accepted but never applied".
- `TestCreateTransitGatewayPolicyTable_Tags_RealClient`: same.
- `TestCreateTransitGatewayRouteTable_Tags_RealClient`: same.
- `TestCreateTransitGatewayVpcAttachment_Tags_RealClient`: same.

Gates: `go build ./services/ec2/...` and `go build ./...` (clean -- exported
`Backend` interface changed: `CreateCapacityReservation`,
`CreateTransitGatewayPolicyTable`, `CreateTransitGatewayRouteTable`, and
`CreateTransitGatewayVpcAttachment` each gained a `tags map[string]string`
param, single implementer `InMemoryBackend`, no other package implements
`Backend`), `go vet -tags e2e ./services/ec2/...` and `go vet -tags
integration ./services/ec2/...` (clean; the repo-wide `go vet -tags e2e/
integration ./...` fails, but only on pre-existing, concurrent, unrelated
`services/wafv2/` breakage -- confirmed via `git status` showing wafv2 files
already dirty before this pass touched anything), `go vet ./services/ec2/...`
(clean), `gofmt -l services/ec2/` (clean), `go test -race
./services/ec2/...` (`ok`, full suite including the 4 new tests),
`golangci-lint run ./services/ec2/...` (`0 issues` -- fixed one `golines`
line-wrap and two `fieldalignment` reorderings on the newly-added `TagSet`
fields by hand, not via `-fix`), `go test ./pkgs/persistence/...` (`ok`). No
banned `//nolint`s.

**2026-08-23 pass (gopherstack-6cuc, plus trunk_enclave/capacity_reservations
audit)**: fixed the accept-and-drop bug named in gopherstack-6cuc --
`CreateTransitGatewayVpcAttachment`'s backend method already took a
`subnetIDs []string` parameter and discarded it (`_ []string`), so
`SubnetIds` sent on create never reached stored state and was only settable
later via `ModifyTransitGatewayVpcAttachment`. Now stores a defensive copy
into `TransitGatewayVpcAttachment.SubnetIDs` on create. Interface signature
already named the parameter correctly (`interfaces.go`) -- only the
`InMemoryBackend` implementation's `_` needed fixing, no call sites changed.

Then audited two never-fully-swept files end to end against the installed
`aws-sdk-go-v2/service/ec2@v1.319.1` deserializers:

- `handler_trunk_enclave.go` (Trunk Interface association + Enclave
  Certificate IAM Role association families, 6 ops, all registered by this
  file's own `registerTrunkEnclaveOps` -- none dispatch via `buildCoreOps`).
  All XML element names, nested-type field names, and the `Return`-vs-no-
  `Return` shape of each op checked line-for-line against the deserializer
  and matched, with one exception: `DescribeTrunkInterfaceAssociations`
  declares `MaxResults`/`NextToken`/`Filters` on its real input and
  `NextToken` on its output, but the handler ignored all of them, always
  returning every association in one page with no `NextToken` -- the
  "List op ignoring real pagination" shape. Fixed using the existing
  `parseEC2Pagination`/`pageSlice` helpers (same pattern as
  `handler_elastic_ips.go`), `ec2PageMinDefault`/`ec2PageMaxDefault` bounds
  (op has no documented MaxResults range).
- `handler_capacity_reservations.go` (8 of `registerCapacityFamilyOps`'s 30
  Capacity Reservation ops live here: `CreateCapacityReservation`,
  `CancelCapacityReservation`, `ModifyCapacityReservation`,
  `GetGroupsForCapacityReservation`,
  `CreateInterruptibleCapacityReservationAllocation`,
  `UpdateInterruptibleCapacityReservationAllocation`,
  `GetCapacityReservationUsage`, `DescribeCapacityReservationTopology` --
  the other 22 Fleet/Capacity-Block/Capacity-Manager/billing-transfer ops
  registered by the same function live in other files, not audited this
  pass). Two more Capacity-Reservation-family ops,
  `AcceptCapacityReservationBillingOwnership` and
  `DescribeCapacityReservations`, are registered by a third file
  (`handler_buildops_advanced.go`'s `registerAcceptAndAdvancedOps`) -- a
  second *file*, not a second *dispatch path*: `buildCoreOps` (the map
  literal in `handler.go`) has no Capacity Reservation entries at all: every
  op in this family dispatches via its own `register*Ops` function, called
  once each from `opRegistrars()`, with no override collision. All 8
  in-file ops field-diffed clean except the same pagination shape as above:
  `DescribeCapacityReservationTopology` declares
  `MaxResults`/`NextToken`/`Filters` and a `nextToken` output field, ignored
  entirely. Fixed the same way.

Modelling gaps found but NOT fixed (no observable behavior change possible,
so no provable round-trip test could be written): `GetGroupsForCapacityReservation`
also declares `MaxResults`/`NextToken`, but this backend's
`GetGroupsForCapacityReservation` always returns an empty slice (comment:
"no group associations tracked") -- paginating zero items is a no-op, so a
pagination fix here is inert until Capacity Reservation Groups are modeled
at all; left unfixed rather than ship an unprovable change.
`GetCapacityReservationUsage`'s real output also has `InterruptionInfo`
(details about the source reservation for an interruptible allocation) and
its own `MaxResults`/`NextToken` over `InstanceUsageSet` -- this backend
aggregates usage into at most one row per account, so multi-page usage
output can't occur either; `InterruptionInfo` itself is unmapped (a real
field-count gap, not wrong data) and would need new backing state to
populate honestly.

False positive ruled out: `instanceConnectEndpointItem` is declared in
`handler_capacity_reservations.go` (misplaced -- it's Instance Connect
Endpoint's response type, unrelated to Capacity Reservations) but is used
correctly from `handler_instances.go`; dead code / bad file placement, not a
bug.

No snapshot bump: `TransitGatewayVpcAttachment.SubnetIDs` already existed on
the persisted struct (only `Modify` wrote it before); no new persisted
fields added by the pagination fixes (`NextToken` is response-only,
generated per request, never stored). `go test ./pkgs/persistence/...`: `ok`.

Proof:
- `wire_field_fixes_ec26cuc_test.go` --
  `TestCreateTransitGatewayVpcAttachment_SubnetIds_RealClient`: real
  `CreateTransitGatewayVpcAttachment` with `SubnetIds` then
  `DescribeTransitGatewayVpcAttachments`, asserting subnets come back on
  both. Pre-fix (hand-reverted `networking1.go` via `cp` to scratchpad,
  `md5sum`-verified identical restore after): fails both assertions with
  "SubnetIds empty on create response - accepted but never applied" and
  "... dropped from stored state" (listB `[]string{}` vs listA the two
  seeded IDs).
- `pagination_ec26cuc_trunk_test.go` --
  `TestDescribeTrunkInterfaceAssociations_Pagination`: creates 3 trunk
  associations, drives the real generated
  `NewDescribeTrunkInterfaceAssociationsPaginator` with `Limit: 1`, asserts
  >=3 disjoint pages. Pre-fix (hand-reverted `handler_trunk_enclave.go`,
  `md5sum`-verified restore): "expected MaxResults=1 to split 3
  associations across pages" -- \"1\" is not >= \"3\" (single page, no
  `NextToken`).
- `pagination_ec26cuc_captopo_test.go` --
  `TestDescribeCapacityReservationTopology_Pagination`: creates 3 capacity
  reservations, drives `DescribeCapacityReservationTopology` manually across
  3 pages with `MaxResults: 1` (no generated paginator for this op), asserts
  disjoint IDs and an empty final `NextToken`. Pre-fix (hand-reverted
  `handler_capacity_reservations.go`, `md5sum`-verified restore): page one
  returns all 3 entries -- "MaxResults=1 ignored - all entries returned on
  page one" ("should have 1 item(s), but has 3").

Gates: `go build ./...` (clean), `go test -race ./services/ec2/...`
(`ok`, full suite including the 3 new tests), `golangci-lint run
./services/ec2/...` (`0 issues` -- fixed two `golines` line-wraps and
dropped one unused `//nolint:gosec` by hand), `go test
./pkgs/persistence/...` (`ok`). No banned `//nolint`s.

Not reached this pass (named, per the task's own list of unaudited files):
`handler_subnets.go`, `handler_ec2core.go`, the remainder of
`handler_networking1.go` beyond the TGW VPC Attachment ops (DHCP Options,
Flow Logs, Launch Template Versions), and the other 22
`registerCapacityFamilyOps` ops living outside
`handler_capacity_reservations.go` (Fleet/Capacity-Block/Capacity-Manager/
billing-transfer). Work left uncommitted for the orchestrator.

**2026-08-23 pass (gopherstack-6cuc follow-up: the "other 22" named above)**:
finished the `registerCapacityFamilyOps` queue named by the previous entry.
`registerCapacityFamilyOps` (`handler_capacity_family.go`) actually registers
38 ops, not 30 -- the earlier count was stale. The full map:

- `handler_capacity_reservation_fleet.go` (4 ops): `CreateCapacityReservationFleet`,
  `DescribeCapacityReservationFleets`, `ModifyCapacityReservationFleet`,
  `CancelCapacityReservationFleets`.
- `handler_capacity_block.go` (7 ops): `DescribeCapacityBlockOfferings`,
  `PurchaseCapacityBlock`, `DescribeCapacityBlockExtensionOfferings`,
  `PurchaseCapacityBlockExtension`, `DescribeCapacityBlocks`,
  `DescribeCapacityBlockStatus`, `DescribeCapacityBlockExtensionHistory`.
- `handler_capacity_reservation_ops.go` (8 ops): `CreateCapacityReservationBySplitting`,
  `MoveCapacityReservationInstances`, `AssociateCapacityReservationBillingOwner`,
  `DisassociateCapacityReservationBillingOwner`, `RejectCapacityReservationBillingOwnership`,
  `DescribeCapacityReservationBillingRequests`, `CreateCapacityReservationCancellationQuote`,
  `DescribeCapacityReservationCancellationQuotes`.
- `handler_capacity_manager.go` (11 ops): `EnableCapacityManager`,
  `DisableCapacityManager`, `UpdateCapacityManagerOrganizationsAccess`,
  `GetCapacityManagerAttributes`, `GetCapacityManagerMetricData`,
  `GetCapacityManagerMetricDimensions`, `CreateCapacityManagerDataExport`,
  `DescribeCapacityManagerDataExports`, `DeleteCapacityManagerDataExport`,
  `GetCapacityManagerMonitoredTagKeys`, `UpdateCapacityManagerMonitoredTagKeys`.
- `handler_capacity_reservations.go` (8 ops, audited in the prior pass):
  `CreateCapacityReservation`, `CancelCapacityReservation`,
  `ModifyCapacityReservation`, `GetGroupsForCapacityReservation`,
  `CreateInterruptibleCapacityReservationAllocation`,
  `UpdateInterruptibleCapacityReservationAllocation`, `GetCapacityReservationUsage`,
  `DescribeCapacityReservationTopology`.

Two more Capacity-Reservation ops, `AcceptCapacityReservationBillingOwnership`
and `DescribeCapacityReservations`, are registered by
`handler_buildops_advanced.go`'s `registerAcceptAndAdvancedOps` but
implemented in `handler_accept_ops.go` -- a second *file* for the
registration (confirmed: `buildCoreOps` in `handler.go` has zero Capacity
Reservation entries; every op in this family dispatches from its own
`register*Ops`, called once each from `opRegistrars()`, no `buildCoreOps`
override collision). Audited both this pass.

**32 ops audited this pass** (the 30 outside `handler_capacity_reservations.go`
plus the 2 from `handler_accept_ops.go`), each field-diffed against the
installed `aws-sdk-go-v2/service/ec2@v1.319.1` deserializers/serializers
line-for-line, not just checked for field-count. **10 real bugs found and
fixed**, all the same "List op ignoring real pagination" shape found earlier
in this file's sibling pass (`handler_trunk_enclave.go`,
`handler_capacity_reservations.go`): each op's real SDK input declares
`MaxResults`/`NextToken` (9 of the 10 also have a generated
`New<Op>Paginator`) but the handler ignored both, always returning every
matching item in one page with no `NextToken`. Fixed identically to the prior
pass, using the existing `parseEC2Pagination`/`pageSlice` helpers with
`ec2PageMinDefault`/`ec2PageMaxDefault` bounds (none of the ten ops' SDK doc
comments give a narrower range):

- `DescribeCapacityReservationFleets` (`handler_capacity_reservation_fleet.go`)
- `DescribeCapacityBlocks`, `DescribeCapacityBlockStatus`,
  `DescribeCapacityBlockExtensionHistory` (`handler_capacity_block.go`)
- `DescribeCapacityBlockOfferings` (`handler_capacity_block.go`) -- provable
  because the backend generates 2 fresh offerings per call; `MaxResults=1`
  now correctly splits that into two.
- `DescribeCapacityReservationBillingRequests`,
  `DescribeCapacityReservationCancellationQuotes`
  (`handler_capacity_reservation_ops.go`) -- the latter has no generated SDK
  paginator, driven manually.
- `DescribeCapacityManagerDataExports`, `GetCapacityManagerMonitoredTagKeys`
  (`handler_capacity_manager.go`)
- `DescribeCapacityReservations` (`handler_accept_ops.go`) -- the response
  struct had no `NextToken` field at all; added one.

`describeCapacityReservationFleetsResponse`/`describeCapacityBlocksResponse`/
etc. already had `NextToken` fields (dead weight until now);
`describeCapacityReservationsResponse` did not and needed one added.

All ten fixes are handler-layer only (mirroring `pageSlice`'s existing usage
elsewhere in this package) -- no `Backend` interface signature changed, so
`make build-check` was run as a precaution but had nothing to fix.

**Recorded as inert, not fixed**: `DescribeCapacityBlockExtensionOfferings`
also ignores real `MaxResults`/`NextToken`, but this backend's
`DescribeCapacityBlockExtensionOfferings` always generates exactly one
offering per call (`capacity_block.go`) -- paginating over one item can't be
proven to fail a test, so left alone per the same rule applied to
`GetGroupsForCapacityReservation` in the prior pass.

**Modelling gaps found, NOT fixed** (no backing state to populate honestly,
so no provable round-trip test could distinguish a fix from a no-op):
- `DescribeCapacityReservationBillingRequests`'s real input has a *required*
  `Role` enum (`odcr-owner` vs `unused-reservation-billing-owner`) that the
  handler never reads and the backend has no notion of -- this backend
  models a single account, so there's no second "consumer" account
  perspective for the two roles to actually differ over. Accepted-and-dropped,
  but not the "looks deliberate" `_ param` shape -- it is a real, provable
  gap in principle, just not implementable without multi-account state this
  backend doesn't have.
- `GetCapacityManagerMetricData`/`GetCapacityManagerMetricDimensions`
  (already correctly, and explicitly, documented in `capacity_manager.go` as
  always-empty: this backend doesn't simulate the historical utilization
  pipeline Capacity Manager aggregates from) -- confirmed still honest, not
  re-touched. Their response XML types (`Items []struct{}`) are dead-weight
  placeholders since the backend never returns anything to serialize into
  them; flagged but not changed since there's no observable bug today.
- `GetCapacityManagerAttributesOutput`'s real shape has
  `EarliestDatapointTimestamp`/`LatestDatapointTimestamp` (histogram bounds
  for ingested metric data) that our response struct omits entirely -- same
  root cause as the metrics gap above (no ingestion pipeline modeled).
- `capacityManagerMonitoredTagKeyItem.EarliestDatapointTimestamp` is declared
  on the XML struct but the backend's `CapacityManagerMonitoredTagKey` type
  has no such field to source it from -- same root cause.
- `CapacityReservationFleet`'s real deserializer also has a
  `capacityReservationFleetArn` field our `capacityReservationFleetItem`
  omits -- a field-count gap (ARN not modeled), not a wrong-data bug.
- `DescribeCapacityReservationCancellationQuotes` and
  `DescribeCapacityManagerDataExports` both ignore real `Filters` (only
  `MaxResults`/`NextToken` were fixed this pass) -- lower-priority, generic
  filter support, scoped out to keep this pass to the named pagination bug
  shape; `DescribeCapacityReservations`'s real `Filters` (state,
  instance-type, tenancy, etc.) are likewise still ignored.

**False positives ruled out**: `capacityManagerStatusResponse`'s
`XMLName xml.Name \`xml:""\`` field is empty-tagged, not tagged -- verified
directly (`encoding/xml.Marshal` on a struct with `xml:""` honors a
runtime-set `Name.Local`; only a *non-empty* tag wins over it) that this is
the safe pattern, not the "shared struct with a tagged XMLName silently
ignores a runtime-set XMLName" trap this package is otherwise prone to.
`ModifyCapacityReservationFleet`'s use of the shared `stubResponse{Return:
true}` is correct, not the "Return: true where the real output has no
Return field" bug -- `ModifyCapacityReservationFleetOutput`'s real
deserializer has only a `return` element. `UnusedReservationBillingOwnerId`
(the param name for `AssociateCapacityReservationBillingOwner`/
`DisassociateCapacityReservationBillingOwner`) is real AWS naming, not a
copied/wrong field. All `TagSpecification`/`TagSpecifications.N` usages in
this pass's five files (`CreateCapacityReservationFleet`, `PurchaseCapacityBlock`,
`CreateCapacityReservationBySplitting`, `CreateCapacityReservationCancellationQuote`,
`CreateCapacityManagerDataExport`) were checked directly against their real
serializers and are all singular-form (`TagSpecification`), matching this
package's existing `parseTagSpecification` singular parser -- none of them
are among the plural-form five.

No snapshot bump: all ten fixes are handler-layer pagination; `NextToken` is
response-only and never persisted, and no `Backend`/persisted struct changed.
`go test ./pkgs/persistence/...`: `ok`.

Proof: `pagination_capacityfamily_test.go`, ten new `t.Parallel()` real-SDK-
client tests, one per fixed op (nine drive the real generated
`New<Op>Paginator` with `Limit: 1` across >=2 pages and assert disjoint IDs
via the existing `assertDisjointPages` helper; `DescribeCapacityReservationCancellationQuotes`
has no generated paginator so is driven manually, matching the
`DescribeCapacityReservationTopology` precedent). Hand-reverted all five
touched handler files at once via `cp` to a scratchpad (`git show HEAD:<path>`
for each, since none of this pass's fixes share a cross-file signature
chain -- each op's fix is self-contained to its own handler function), ran
the new test file against the reverted tree: all ten tests failed, each with
the "MaxResults ignored, page one contains everything, no NextToken" shape,
e.g. `TestDescribeCapacityBlockOfferings_Pagination`: `"... should have 1
item(s), but has 2"`; `TestDescribeCapacityReservationCancellationQuotes_Pagination`:
`"... should have 1 item(s), but has 3"`; the eight generated-paginator tests
all failed on `assertDisjointPages`' `"1" is not greater than or equal to
"2"` (only one page produced). Restored all five files from the scratchpad
copies and `md5sum`-diffed identical against the post-fix state.

Gates: `go build ./...` (clean, no exported signature changed),
`go vet -tags e2e ./services/ec2/...` and `go vet -tags integration
./services/ec2/...` (clean), `go vet ./services/ec2/...` (clean),
`gofmt -l services/ec2/` (clean), `go test -race ./services/ec2/...` (`ok`,
full suite including the 10 new tests), `golangci-lint run
./services/ec2/...` (`0 issues` -- fixed one `govet` shadow, one `intrange`,
and three `prealloc` findings by hand, not via `-fix`), `go test
./pkgs/persistence/...` (`ok`), `make build-check` (clean). No banned
`//nolint`s.

**Ops not reached this pass**: none within the queue named by the prior
entry -- all 30 `registerCapacityFamilyOps` ops outside
`handler_capacity_reservations.go`, plus the 2 `handler_accept_ops.go` ops,
were read and field-diffed. Work left **uncommitted** for the orchestrator.

**2026-08-29 -- ERROR PATH audited (wrong-error-code sweep, class: sentinel
maps to a code the SDK's own per-op deserializer doesn't model, so a real
client's `errors.As` into a typed exception silently falls through to a
generic error). Structurally clean -- 0 bugs, 0/785 ops at risk by
construction.** Extracted every `awsEc2query_deserializeOpError<Op>` in
`aws-sdk-go-v2/service/ec2@v1.319.1/deserializers.go` (785 functions, one per
routed action) and confirmed none contain a single `case`/`EqualFold` branch
-- each is unconditionally `switch { default: return &smithy.GenericAPIError{
Code: errorCode, ... } }`. There is also no `types/errors.go` in this SDK
package -- EC2 models **zero** typed per-operation exceptions in this pinned
version. Every EC2 error, for every op, becomes a `smithy.GenericAPIError`
carrying whatever `Code` string the server sent; a real client can never
`errors.As` into an op-specific typed exception for this service at all, so
the "code not modeled by this op" bug class found in iam/dynamodb/s3/sts and
in cloudformation this same sweep cannot occur here -- there is no per-op
model to be inconsistent with. `handler.go`'s shared `errCodeLookup` table
(sentinel -> XML `Code` string) is therefore the entire error surface;
auditing individual call sites for wire-string accuracy against real AWS
would be a general parity sweep, not this bug class, and was left alone per
scope. No source changes.

Gates: `go vet ./services/ec2/...` clean (no source changed; full
`go build`/`golangci-lint`/`go test` gates not re-run for a read-only
audit with no diff).

**2026-08-29 pass -- request-wrapper-key sweep, IPAM/Local Gateway/VPC
Endpoint/Network Insights families (21 ops)**: diffed the 202 implemented
`Describe*`/`List*` operation strings in `services/ec2/*.go` against every op
name mentioned anywhere in this file, giving ~123 never-verified-in-PARITY
candidates; picked a 21-op tranche across four related families and read
each handler's request-parsing code against its own
`awsEc2query_serializeOpDocument<Op>Input` in the pinned
`aws-sdk-go-v2/service/ec2@v1.319.1/serializers.go`, not a sibling's shape.

IPAM family (7, `handler_ipam_discovery.go`/`handler_ipam.go`/
`handler_ipam_policy.go`): DescribeIpamByoasn, DescribeIpamExternalResourceVerificationTokens,
DescribeIpamPolicies, DescribeIpamPrefixListResolvers,
DescribeIpamPrefixListResolverTargets, DescribeIpamResourceDiscoveries,
DescribeIpamResourceDiscoveryAssociations.

Local Gateway family (6, `handler_local_gateway.go`): DescribeLocalGateways,
DescribeLocalGatewayVirtualInterfaces, DescribeLocalGatewayVirtualInterfaceGroups,
DescribeLocalGatewayRouteTables, DescribeLocalGatewayRouteTableVpcAssociations,
DescribeLocalGatewayRouteTableVirtualInterfaceGroupAssociations.

VPC Endpoint family (4, `handler_vpc_endpoints.go`): DescribeVpcEndpointAssociations,
DescribeVpcEndpointConnections, DescribeVpcEndpointServicePermissions,
DescribeVpcEndpointConnectionNotifications.

Network Insights family (4, `handler_network_insights.go`): DescribeNetworkInsightsPaths,
DescribeNetworkInsightsAnalyses, DescribeNetworkInsightsAccessScopes,
DescribeNetworkInsightsAccessScopeAnalyses.

All 21 ops' ID-list request keys (`parseMemberList`'s singular-flattened
prefixes, e.g. `LocalGatewayId.N`, `IpamPolicyId.N`,
`NetworkInsightsAccessScopeId.N`) checked correct against each op's own
`object.FlatKey(...)` call -- no ID-key bug in this tranche. Found and fixed
4 real bugs, all class-1 (silent empty/ignored filter, no error):

1. **`DescribeVpcEndpointConnections`** (`handler_vpc_endpoints.go`) read a
   `ServiceId.N` indexed list that does not exist on the wire at all --
   `DescribeVpcEndpointConnectionsInput` has no ServiceId/ServiceIds field
   (`api_op_DescribeVpcEndpointConnections.go`: only DryRun/Filters/MaxResults/
   NextToken); a real client narrows by service only via a `service-id`
   `Filter` (serializers.go:82487). The service-id filter was always
   silently ignored -- every call returned every connection. Fixed: read
   `parseEC2Filters(vals)["service-id"]` instead.
2. **`DescribeVpcEndpointConnectionNotifications`** (`handler_vpc_endpoints.go`)
   read `ConnectionNotificationId` as an indexed list
   (`parseMemberList(vals, "ConnectionNotificationId")` -> looks for
   `ConnectionNotificationId.1`), but the real field is a scalar `*string`
   serialized as a bare `ConnectionNotificationId` key (serializers.go:82458)
   -- a key a real client's single-ID lookup never matches. Fixed: read
   `vals.Get("ConnectionNotificationId")` as a scalar, wrapped into a
   1-element slice for the existing `[]string`-taking backend method.
3. **`DescribeNetworkInsightsAnalyses`** (`handler_network_insights.go`)
   never read `NetworkInsightsPathId`, a real scalar filter field distinct
   from the `NetworkInsightsAnalysisIds` list (serializers.go:79838,
   `object.Key("NetworkInsightsPathId")`) -- narrowing analyses to one path
   was silently ignored. Fixed: `Backend.DescribeNetworkInsightsAnalyses`
   gained a `pathID string` parameter (interface signature change, only
   in-package callers, `go vet ./...` run repo-wide clean).
4. **`DescribeNetworkInsightsAccessScopeAnalyses`** (same file) never read
   `NetworkInsightsAccessScopeId`, the real scalar filter field distinct from
   `NetworkInsightsAccessScopeAnalysisIds` (serializers.go:79751). Fixed the
   same way: `Backend.DescribeNetworkInsightsAccessScopeAnalyses` gained a
   `scopeID string` parameter.

Left alone, not fabricated: all 6 Local Gateway ops and all 7 IPAM ops
declare a real `Filters []types.Filter` field that none of their handlers
apply at all (only ID lists) -- e.g. `DescribeLocalGateways` supports
`local-gateway-id`/`outpost-arn`/`owner-id`/`state` filters
(`api_op_DescribeLocalGateways.go`) and applies none of them. This is a
missing-feature gap (no filter-matching code exists to read a wrong key),
not this pass's "reads an existing wire key under the wrong name" bug class
-- named here rather than invented as a fix, since building real per-field
filter semantics for 13 ops is a separate, much larger pass.
`DescribeVpcEndpointAssociations`/`DescribeVpcEndpointServicePermissions`
have the same gap (Filters declared, never read) for the same reason.

Existing tests: none of this tranche's 21 ops had a prior
`wire_field_fixes*_test.go` case (request-side or response-side), so no
wrong/blind/insufficiently-specific existing test to correct.

New tests (`services/ec2/wire_field_fixes_ec2sweep33_test.go`, 4
`*_RealClient` tests against the real `ec2sdk.Client`, each confirmed
failing pre-fix by running before the corresponding source change):
`TestDescribeVpcEndpointConnections_ServiceIdFilter_RealClient`,
`TestDescribeVpcEndpointConnectionNotifications_IdFilter_RealClient`,
`TestDescribeNetworkInsightsAnalyses_PathIdFilter_RealClient`,
`TestDescribeNetworkInsightsAccessScopeAnalyses_ScopeIdFilter_RealClient`.

Not reached this pass: the ~102 other never-verified-in-PARITY ops (of the
~123 candidate set), including DescribeSubnets, DescribeDhcpOptions,
DescribeInternetGateways (response-side already covered by
`wire_field_fixes_test.go`'s tag test but not this request-key class),
DescribeVpnConnections/VpnGateways/CustomerGateways, DescribeInstanceStatus,
DescribeInstanceTypes, DescribeFleets/FleetHistory/FleetInstances,
DescribeSpotPriceHistory, the whole `DescribeTrafficMirrorFilterRules` /
Route Server / Verified Access logging-config / VPC block-public-access
surface, and more -- see the full 123-op diff method above to regenerate.

Gates: `go build ./services/ec2/...`, `go vet ./...` (repo-wide, two backend
signatures changed), `go test -race -count=1 ./services/ec2/...` (full
suite green), `golangci-lint run ./services/ec2/...` (0 issues, run last).
No banned nolints.

**2026-08-29 pass -- request-wrapper-key sweep, core VPC/subnet/instance
networking (21 ops)**: covered the tranche named in the task -- DescribeSubnets,
DescribeDhcpOptions, DescribeInternetGateways, DescribeEgressOnlyInternetGateways,
DescribeNatGateways, DescribeNetworkAcls, DescribePrefixLists,
DescribeManagedPrefixLists, DescribePublicIpv4Pools, DescribeInstanceStatus,
DescribeInstanceTypes, DescribeInstanceTypeOfferings, DescribeBundleTasks,
DescribeAddressTransfers, DescribeByoipCidrs -- plus 6 adjacent core-networking
ops also unverified in this file: DescribeNetworkInterfaces, DescribeRouteTables,
DescribeVpcs, DescribeVpcAttribute, DescribeCarrierGateways, DescribeFlowLogs.
Each handler's request-parsing code read against its own
`awsEc2query_serializeOpDocument<Op>Input` in the pinned
`aws-sdk-go-v2/service/ec2@v1.319.1/serializers.go`, including tracing
`object.FlatKey`/`Array` through `aws-sdk-go-v2@v1.43.4/aws/protocol/query/{object,array}.go`
to confirm `FlatKey` list elements use the flattened `<Name>.N` key regardless
of the child serializer's own `Array("Item")` call.

All 21 ops' ID-list keys (`InstanceId.N`, `SubnetId.N`, `DhcpOptionsId.N`,
`InternetGatewayId.N`, `EgressOnlyInternetGatewayId.N`, `NatGatewayId.N`,
`NetworkAclId.N`, `PrefixListId.N`, `PoolId.N`, `InstanceType.N`, `BundleId.N`,
`AllocationId.N`, `NetworkInterfaceId.N`, `RouteTableId.N`, `VpcId.N`/`VpcId`,
`CarrierGatewayId.N`, `FlowLogId.N`) and the shared `Filter.N.Name`/
`Filter.N.Value.M` filter-parsing convention (`parseEC2Filters`) checked
correct against each op's own serializer. No wrong-key, wrong-cardinality, or
hard-decode-error bug found in this tranche (signatures 1/2/4 all clean).

One informational finding, not fixed: **`DescribeByoipCidrs`**
(`handler_accept_ops.go:518`) reads `vals.Get("State")` and passes it to
`Backend.DescribeByoipCidrs(state)` as an optional state filter, but
`DescribeByoipCidrsInput` (`api_op_DescribeByoipCidrs.go`) has no `State`
field and no `Filters` field at all -- only `MaxResults`, `DryRun`,
`NextToken`. A real client has no way to filter this operation by state, so
`State` is always empty for real traffic and the handler's
empty-state-means-no-filter behavior already matches real AWS exactly. Left
alone: unlike `DescribeVpcEndpointConnections`'s `ServiceId` (sweep33), there
is no real key to redirect this read to -- removing the dead `State` read
would be a code-cleanliness change, not a wire-shape fix, so out of scope
here.

Missing-feature gaps (Filters field declared on the wire, no filter-matching
code exists to read a wrong key, so not this class) -- kept distinct, not
fabricated as bugs: `DescribeDhcpOptions`, `DescribeEgressOnlyInternetGateways`,
`DescribePrefixLists` (`prefix-list-id`/`prefix-list-name`),
`DescribeManagedPrefixLists`, `DescribePublicIpv4Pools` (`tag`/`tag-key`),
`DescribeBundleTasks`, `DescribeInstanceTypes`, `DescribeCarrierGateways`,
`DescribeFlowLogs` all declare `Filters`/`Filter` and never apply it.
`DescribeInstanceStatus` additionally never reads `IncludeAllInstances` or
`IncludeManagedResources` (both real boolean fields;
`Backend.DescribeInstanceStatus` always returns every instance regardless of
state, so the AWS default of running-only is also unimplemented) -- same
missing-feature category, no read attempt exists to misdirect.
`DescribeNetworkAcls` only applies the `vpc-id` filter of its documented set.
`DescribeSubnets`, `DescribeInternetGateways`, `DescribeNatGateways`,
`DescribeInstanceTypeOfferings`, `DescribeNetworkInterfaces`, `DescribeVpcs`,
`DescribeRouteTables` all apply `Filters` through `parseEC2Filters` with
correct wire keys (filter *name* coverage/completeness is a separate,
larger gap, not audited here).

Existing tests: none of this tranche's 21 ops had a wrong, blind, or
insufficiently-specific existing test for this specific request-key class.
`TestDescribeInstanceTypeOfferings_Filters_RealClient`
(`wire_field_fixes_ec2sweep11_test.go`) already covers that op's Filters
correctly (asserts the decoded response is properly narrowed, not just
`err == nil`).

No new tests added -- no fixable bug found in this tranche.

**Update (gopherstack-j2v5 pass, 2026-08-30): the missing-feature gap above is
now fixed for 10 of the 11 ops it listed.** `DescribeDhcpOptions`,
`DescribeEgressOnlyInternetGateways`, `DescribePrefixLists`,
`DescribeManagedPrefixLists`, `DescribePublicIpv4Pools`, `DescribeBundleTasks`,
`DescribeCarrierGateways`, and `DescribeFlowLogs` now apply every filter name
their own SDK doc comment lists AND this backend's struct actually stores
(`handler_filters.go`'s new `apply*Filters`/`*MatchesFilter` functions);
names naming untracked data (e.g. `owner-id` on `DhcpOptions`/`PrefixList`,
which have no per-resource owner field; `entry.icmp.*`/`entry.ipv6-cidr` on
`NetworkACL`'s `NACLEntry`, which has no ICMP or IPv6 fields) are left
unimplemented, documented inline at each function. `DescribeNetworkAcls` now
applies its full documented filter set, not just `vpc-id`.
`DescribeInstanceStatus` now reads `IncludeAllInstances` (defaults to
running-only when no explicit `InstanceId` list is given, matching real AWS)
and applies its filters; `IncludeManagedResources` is still left unread --
this backend has no managed-instance concept to hide or reveal.
**`DescribeInstanceTypes` is the one op left genuinely unfixed**: unlike the
other ten, its documented filter names (`hypervisor`, `bare-metal`,
`ebs-info.*`, `instance-storage-info.*`, etc.) all describe instance-type
*attributes*, and this backend has no instance-type attribute catalog at
all -- `handleDescribeInstanceTypes` (`handler_instances_lifecycle.go`) only
ever echoes back the `InstanceType.N` values a caller asked for (or a single
fallback), so there is no real data to filter against without fabricating
an attribute table. Left as a missing feature, not a bug.

Not reached this pass: DescribeInstanceTypeOfferings/DescribeInstanceStatus/
DescribeInstanceTypes' Go pagination and instance-type-catalog fidelity concerns
are out of this class's scope. The other 74 ops from the ~123-candidate diff
remain unverified (Fleets, Spot, Traffic Mirror, Verified Access, VPC
block-public-access, Reserved Instances, Hosts, Placement Groups, and more --
see the diff method in the 2026-08-29 IPAM/Local Gateway pass above to
regenerate).

Gates: `go build ./services/ec2/...`, `go vet ./services/ec2/...` (no backend
signatures changed, so repo-wide vet not required), `go test -race -count=1
./services/ec2/...` (full suite green), `golangci-lint run ./services/ec2/...`
(0 new issues; one pre-existing `golines` finding in a concurrently-edited
file from the other in-flight ec2 agent's tranche, not touched here).

**2026-08-29 pass -- request-wrapper-key sweep, Fleet/Spot/Traffic
Mirror/Verified Access/newer-feature families (20 ops)**: regenerated the
202 implemented `Describe*`/`List*` operation strings (grep, `Response`-
suffixed XML-element false positives stripped) and diffed against every op
name already mentioned anywhere in this file; picked a 20-op tranche from
the families this file's own "not reached" notes above name as still
outstanding (Fleets, Spot, Traffic Mirror, Verified Access) plus four
completely never-mentioned newer-feature ops (Outposts, VPC block-public-
access, AMI store-image). Read each handler's request-parsing code against
its own `awsEc2query_serializeOpDocument<Op>Input` in the pinned
`aws-sdk-go-v2/service/ec2@v1.319.1/serializers.go`, not a sibling's shape.
Note: Capacity Reservations/Capacity Blocks (named in the task brief as a
target family) turned out to be fully field-diffed already by the 2026-08-23
`gopherstack-6cuc` passes above (all 38 `registerCapacityFamilyOps` ops,
line-for-line against the SDK) -- confirmed by reading those notes before
picking ops, not re-audited here, and not counted toward this tranche's 20.
Likewise `DescribeSpotFleetRequests`/`DescribeSpotFleetInstances`/
`DescribeSpotFleetRequestHistory` were already audited clean in `ec2sweep24`
("all 10 spot-fleet ... ops were audited against the real SDK this pass") --
excluded here in favor of the still-outstanding non-fleet Spot ops.

Fleet family (3, `handler_fleet.go`/`fleet.go`): `DescribeFleets` (clean --
`FleetId.N` correct against serializers.go:77611), `DescribeFleetHistory`,
`DescribeFleetInstances`. NOTE (added 2026-08-30, see that pass below): this
entry verified only the *request-side* `FleetId.N` shape for all three ops --
it did not check what the *response* actually contained. All three response
sets were unconditionally empty at the time (`CreateFleet` never launched or
recorded an instance against a fleet), so a correctly-shaped request read
still returned a hardcoded-empty result. Read literally this note is still
true (the request parsing genuinely was clean), but it should not be read as
"the Fleet family is done" -- it wasn't checking the thing that was actually
broken. Fixed in the 2026-08-30 pass below.

Spot family (3, non-SpotFleet): `DescribeSpotInstanceRequests`
(`handler_spot_instances.go`, clean -- `SpotInstanceRequestId.N` correct
against serializers.go:81133), `DescribeSpotPriceHistory`
(`handler_spot_instances.go`), `DescribeSpotDatafeedSubscription`
(`handler_spot_fleet.go`, clean -- real `DescribeSpotDatafeedSubscriptionInput`
has only `DryRun`, confirmed against `api_op_DescribeSpotDatafeedSubscription.go`).

Traffic Mirror family (4, `handler_traffic_mirror.go`): `DescribeTrafficMirrorFilters`
(clean -- `TrafficMirrorFilterId.N`, serializers.go:81401),
`DescribeTrafficMirrorFilterRules` (clean -- `TrafficMirrorFilterId` is
correctly read as a scalar via `vals.Get`, matching the real
`object.Key("TrafficMirrorFilterId")` at serializers.go:81360; see gaps for
the unread `TrafficMirrorFilterRuleIds` list), `DescribeTrafficMirrorSessions`
(clean -- `TrafficMirrorSessionId.N`, serializers.go:81437),
`DescribeTrafficMirrorTargets` (clean -- `TrafficMirrorTargetId.N`,
serializers.go:81473).

Verified Access family (5, `handler_verified_access.go`/
`handler_verified_access_policy.go`): `DescribeVerifiedAccessEndpoints`
(clean -- `VerifiedAccessEndpointId.N`, serializers.go:81941; see gaps for
the two unread scalar `VerifiedAccessGroupId`/`VerifiedAccessInstanceId`
narrowing params), `DescribeVerifiedAccessGroups` (clean --
`VerifiedAccessGroupId.N`, serializers.go:81987; see gaps for the unread
scalar `VerifiedAccessInstanceId`), `DescribeVerifiedAccessInstanceLoggingConfigurations`
(clean -- `VerifiedAccessInstanceId.N`, serializers.go:82028),
`DescribeVerifiedAccessInstances` (clean -- `VerifiedAccessInstanceId.N`,
serializers.go:82064), `DescribeVerifiedAccessTrustProviders` (clean --
`VerifiedAccessTrustProviderId.N`, serializers.go:82100).

Newer-feature singles (5): `DescribeInstanceConnectEndpoints`
(`handler_instances.go`, clean -- `InstanceConnectEndpointId.N`,
serializers.go:78211), `DescribeOutpostLags` (`handler_secondary_net.go`,
clean -- `OutpostLagId.N`, serializers.go:80007),
`DescribeVpcBlockPublicAccessExclusions` (`handler_vpc_config.go`, clean --
`ExclusionId.N`, serializers.go:82286), `DescribeVpcBlockPublicAccessOptions`
(`handler_vpc_config.go`, clean -- real input has only `DryRun`, confirmed
against `api_op_DescribeVpcBlockPublicAccessOptions.go`), `DescribeStoreImageTasks`
(`handler_image_ops.go`, clean -- `ImageId.N`, serializers.go:81249).

**1 real bug found and fixed, class 2 (wrong cardinality)**:
`DescribeSpotPriceHistory` (`handler_spot_instances.go`) read
`AvailabilityZone` via `parseMemberList(vals, "AvailabilityZone")`, an
indexed-list reader looking for `AvailabilityZone.1`, `.2`, ... but the real
`DescribeSpotPriceHistoryInput.AvailabilityZone` is a scalar `*string`
serialized as a bare `AvailabilityZone` key (`object.Key("AvailabilityZone")`,
serializers.go:81147-81149) -- a key a real client's single-AZ filter never
matches in indexed form. The filter was always silently ignored;
`GenerateSpotPriceHistory` fell back to its 3-AZ default
(`region+"a"/"b"/"c"`) every time, so a real client narrowing to one AZ got
records from all three instead. Fixed: read `vals.Get("AvailabilityZone")`
as a scalar, wrapped into a 1-element slice only when non-empty (matching
the existing `[]string`-taking `GenerateSpotPriceHistory` signature -- no
`Backend`/exported signature changed).

Missing-feature gaps (real key on the wire, no read code exists at all to be
wrong -- kept distinct from the bug above, not fabricated as fixes):
`DescribeSpotPriceHistory` also never reads `EndTime` or `AvailabilityZoneId`
(both real scalar fields; `GenerateSpotPriceHistory` has no end-time bound or
AZ-ID concept). `DescribeTrafficMirrorFilterRules` never reads the real
`TrafficMirrorFilterRuleIds` list (narrowing to specific rule IDs within a
filter). `DescribeVerifiedAccessEndpoints` never reads its two real scalar
narrowing params, `VerifiedAccessGroupId`/`VerifiedAccessInstanceId`;
`DescribeVerifiedAccessGroups` never reads its real scalar
`VerifiedAccessInstanceId`. All Fleet/Spot/Traffic-Mirror/Verified-Access/
Outpost-Lag/VPC-block-public-access/store-image ops in this tranche that
declare a `Filters []types.Filter` field apply none of it -- the same
already-documented, repo-wide missing-feature category as every prior
request-wrapper-key-sweep tranche, not this pass's bug class.

Structural gap, not fabricated (distinct from a missing-feature gap: there is
no backing state to read a wrong key FROM): `DescribeFleetHistory` and
`DescribeFleetInstances` (`handler_fleet.go`) are hardcoded stubs --
`handleDescribeFleetHistory`/`handleDescribeFleetInstances` both take
`_ url.Values` and always return an empty envelope, never reading the real
required `FleetId`. Confirmed this is not a misdirected-key bug: `Backend.CreateFleet`
(`fleet.go`) never launches or tracks any instance against a fleet at all
(the `Fleet` struct has no launched-instance or history-event fields), so
there is no real per-fleet instance/history data these ops could honestly
return even with a correct `FleetId` read -- building it would mean modeling
EC2 Fleet's launch/history state machine from scratch, a materially larger
feature addition, not a wire-key fix. Left alone and named here rather than
invented around.

Existing tests: none of this tranche's 20 ops had a prior
`wire_field_fixes*_test.go` case (request-side or response-side) for this
bug class; the two bare dispatch-smoke-test references to
`DescribeSpotPriceHistory` (`handler_core_test.go`, `handler_sdk_route_table_test.go`)
only assert `200 OK`/`GetSupportedOperations` membership, never decoded
per-field narrowing, so they're blind rather than wrong and were left as-is.

New test (`services/ec2/wire_field_fixes_ec2sweep37_test.go`, 1
`*_RealClient` test against the real `ec2sdk.Client`, confirmed failing
pre-fix by running before the source change):
`TestDescribeSpotPriceHistory_AvailabilityZoneFilter_RealClient` -- pre-fix
failure: requesting `AvailabilityZone: "us-east-1a"` returned records from
`"a"`/`"b"`/`"c"` (the handler's `Region` field was empty in the test
harness, so the ignored-filter default degenerated further to bare
`"a"`/`"b"`/`"c"`, not even `"us-east-1a"/"b"/"c"` -- same underlying bug,
more visibly wrong result), instead of only `"us-east-1a"` records.

Not reached this pass: DescribeReservedInstances/ReservedInstancesListings/
ReservedInstancesModifications/ReservedInstancesOfferings, DescribeHosts/
HostReservations/HostReservationOfferings, DescribePlacementGroups,
DescribeRouteServer*, DescribeCoipPools, DescribeIpv6Pools,
DescribeMacHosts/MacModificationTasks, DescribeConversionTasks,
DescribeElasticGpus, DescribeScheduledInstance*, and the remaining
never-verified ops from the ~123-candidate diff not covered by any pass
above -- see the diff method in the 2026-08-29 IPAM/Local Gateway pass to
regenerate.

Gates: `go build -o /dev/null ./services/ec2/...` (clean, no exported
signature changed), `go vet ./services/ec2/...` (clean; repo-wide `go vet
./...` shows only a pre-existing, unrelated `services/eks/` build break from
the other in-flight agent's concurrent tranche -- confirmed via `git status`
showing eks files already dirty before this pass touched anything, not ec2),
`go test -race -count=1 ./services/ec2/...` (`ok`, full suite including the
new test), `golangci-lint run ./services/ec2/...` (`0 issues`, run last, no
`--fix` used). No banned `//nolint`s.

**2026-08-29 pass -- request-wrapper-key sweep, Reserved Instances/Hosts/
Placement Groups/Route Server/newer-singleton families (21 ops)**: picked the
21-op tranche this file's own "not reached" note above (2026-08-29 Fleet/
Spot/Traffic Mirror pass) explicitly named as still outstanding: Reserved
Instances, Hosts, Placement Groups. Extended with the other same-shaped
sibling families the "not reached" list also named (Route Server, Mac Hosts,
Conversion Tasks, Elastic Gpus, Scheduled Instances, Coip/Ipv6 Pools) plus two
Transit Gateway peripheral ops never covered by any prior TGW pass. Read each
handler's request-parsing code against its own
`awsEc2query_serializeOpDocument<Op>Input` in the pinned
`aws-sdk-go-v2/service/ec2@v1.319.1/serializers.go`, not a sibling's shape.

Reserved Instances family (4, `handler_reserved_instances.go`):
`DescribeReservedInstances` (clean -- `ReservedInstancesId.N`,
serializers.go:80239), `DescribeReservedInstancesModifications` (clean
-- `ReservedInstancesModificationId.N`, serializers.go:80289),
`DescribeReservedInstancesOfferings` (clean on every field it reads --
`InstanceType`/`AvailabilityZone`/`ProductDescription` all real scalars,
serializers.go:80335 (InstanceType), 80303 (AvailabilityZone), 80375 (ProductDescription)), `DescribeReservedInstancesListings`
(**1 real bug, see below**).

Hosts family (3): `DescribeHosts` (`handler_accept_ops.go`, clean --
`HostId.N`, serializers.go:77813), `DescribeHostReservations`
(`handler_host_reservations.go`, clean -- `HostReservationIdSet.N`; the wire
field's own shape name is literally "HostReservationIdSet", not the usual
singular-member convention, and the handler already reads that exact key,
serializers.go:77782), `DescribeHostReservationOfferings` (clean --
`OfferingId` scalar, serializers.go:77763).

Placement Groups (1): `DescribePlacementGroups` (`handler_placement_groups.go`,
clean on what it reads -- `GroupName.N`, serializers.go:80040; see gaps
for the unread `GroupId.N`).

Route Server family (3, `handler_route_server.go`): `DescribeRouteServers`
(clean -- `RouteServerId.N`, serializers.go:80488),
`DescribeRouteServerEndpoints` (clean -- `RouteServerEndpointId.N`,
serializers.go:80416), `DescribeRouteServerPeers` (clean --
`RouteServerPeerId.N`, serializers.go:80452). All three match despite
this file's own "Route Server does the reverse [singular-behind-plural] trap"
warning for a different Route Server op elsewhere in this codebase -- these
three Describe ops were verified independently, not assumed clean by
association.

Mac family (2, `handler_mac_hosts.go`): `DescribeMacHosts` (clean --
`HostId.N`, serializers.go:79513), `DescribeMacModificationTasks`
(clean -- `MacModificationTaskId.N`, serializers.go:79549).

Singles (5): `DescribeConversionTasks` (`handler_vm_import_export.go`, clean
-- `ConversionTaskId.N`, serializers.go:77224), `DescribeElasticGpus`
(`handler_instances.go`, clean -- `ElasticGpuId.N`, serializers.go:77375),
`DescribeCoipPools` (`handler_ip_pools.go`, clean -- `PoolId.N`,
serializers.go:77210), `DescribeIpv6Pools` (clean -- `PoolId.N`,
serializers.go:79088).

Scheduled Instances family (2, `handler_scheduled_instances.go`):
`DescribeScheduledInstanceAvailability` (clean --
`MinSlotDurationInHours`/`MaxSlotDurationInHours` scalars,
serializers.go:80562 (MaxSlotDurationInHours), 80567 (MinSlotDurationInHours)), `DescribeScheduledInstances` (clean --
`ScheduledInstanceId.N`, serializers.go:80613).

Transit Gateway peripherals, never covered by any prior TGW pass (2,
`handler_tgw_peripherals.go`): `DescribeTransitGatewayPolicyTables` (clean --
unlike the five sibling TGW ops fixed in `wire_field_fixes_ec2sweep36_test.go`
(`TransitGatewayAttachmentIds.N`/`TransitGatewayRouteTableIds.N`), this op's
own `TransitGatewayPolicyTableIds` field is genuinely `FlatKey`'d under its
own **plural** name, serializers.go:81725 -- the handler's
`parseMemberList(vals, "TransitGatewayPolicyTableIds")` already matches
exactly), `DescribeTransitGatewayRouteTableAnnouncements` (clean, same
shape -- `TransitGatewayRouteTableAnnouncementIds.N`, serializers.go:81761).

**1 real bug found and fixed, class 2 (wrong cardinality)**:
`DescribeReservedInstancesListings` (`handler_reserved_instances.go`) read
`ReservedInstancesListingId` via `parseMemberList`, an indexed-list reader
looking for `ReservedInstancesListingId.1`, `.2`, ... but the real
`DescribeReservedInstancesListingsInput.ReservedInstancesListingId` is a
scalar `*string` serialized as the bare key `ReservedInstancesListingId`
(serializers.go:80265, `object.Key(...)`, not `FlatKey`) -- a key a
real client's single-listing lookup never matches in indexed form. The filter
was always silently ignored; every call returned every listing regardless of
which one was requested. Fixed: read `vals.Get("ReservedInstancesListingId")`
as a scalar, wrapped into a 1-element slice only when non-empty (matching the
existing `[]string`-taking `Backend.DescribeReservedInstancesListings` --
no `Backend`/exported signature changed).

Missing-feature gaps (real key on the wire, no read code exists at all to be
wrong -- kept distinct from the bug above, not fabricated as fixes):
`DescribeReservedInstancesListings` also never reads the real scalar
`ReservedInstancesId` field (narrowing listings to one originating Reserved
Instance, distinct from `ReservedInstancesListingId`).
`DescribeReservedInstancesOfferings` never reads `AvailabilityZoneId`,
`OfferingClass`, `OfferingType`, `MinDuration`/`MaxDuration`,
`MaxInstanceCount`, `IncludeMarketplace`, `InstanceTenancy`, or
`ReservedInstancesOfferingIds`. `DescribePlacementGroups` never reads
`GroupId.N` (`GroupIds`), only `GroupName.N`. `DescribeScheduledInstanceAvailability`
never reads the real required `FirstSlotStartTimeRange` struct or
`Recurrence`. All ops in this tranche that declare a `Filters []types.Filter`
field apply none of it -- the same already-documented, repo-wide
missing-feature category as every prior request-wrapper-key-sweep tranche,
not this pass's bug class.

Existing tests: none of this tranche's 21 ops had a prior
`wire_field_fixes*_test.go` case (request-side or response-side) for this bug
class. `TestReservedInstances` (`handler_reserved_instances_test.go`) drives
`Backend.DescribeReservedInstancesListings` directly, bypassing the handler's
request parsing entirely, so it could not have caught this bug -- blind, not
wrong, to this specific class; left as-is (still a valid backend-level test).

New test (`services/ec2/wire_field_fixes_ec2sweep38_test.go`, 1
`*_RealClient` test against the real `ec2sdk.Client`, confirmed failing
pre-fix by running before the source change):
`TestDescribeReservedInstancesListings_ListingIdFilter_RealClient` -- pre-fix
failure: `require.Len(t, out.ReservedInstancesListings, 1, ...)` got 2 (every
listing) instead of the one requested by `ReservedInstancesListingId`.

Sibling-ID-family hypothesis: did NOT hold for most of this tranche. Five of
seven multi-op sibling families picked specifically because they share
closely-named ID parameters (Hosts, Placement Groups, Route Server, Mac
Hosts, TGW peripherals) came back entirely clean -- only the fourth Reserved
Instances sibling had a bug, and even that one isn't a same-op-family
name-collision (it's a scalar-vs-list cardinality mistake, the same shape as
tranche 4's `DescribeSpotPriceHistory` bug, arguably explained by copying the
*cardinality* of its own list-typed siblings `ReservedInstancesId`/
`ReservedInstancesModificationId` rather than a wrong name). Combined with
tranche 4's 1-bug-in-20 rate on non-sibling families, this tranche's
1-bug-in-21 on sibling families suggests the sibling-density signal is weaker
than the working hypothesis after two more tranches of evidence -- most
families of any shape are now clean, and the remaining bugs look more
evenly scattered than clustered.

Not reached this pass: the remaining never-verified-in-PARITY ops, including
the ~102-op set named in the 2026-08-29 IPAM/Local Gateway pass's own "not
reached" note (DescribeSubnets/DescribeDhcpOptions/etc -- since fully covered
by the later core-VPC pass, see above) plus anything not yet swept across all
passes to date -- regenerate via the diff method above (grep implemented
`Describe*`/`List*` op strings, strip `Response`-suffixed false positives,
subtract every op name mentioned anywhere in this file) to find what's left.

Gates: `go build -o /dev/null ./services/ec2/...` (clean, no exported
signature changed), `go vet ./services/ec2/...` (clean; no backend interface
signature changed so repo-wide vet not required), `go test -race -count=1
./services/ec2/...` (`ok`, full suite including the new test),
`golangci-lint run ./services/ec2/...` (`0 issues`, run last, no `--fix`
used). No banned `//nolint`s.

**2026-08-29 pass -- exhaustive `parseMemberList` call-site enumeration
(243-of-243, all handler files)**: unlike every prior tranche above (each a
themed sample), this pass enumerated literally every `parseMemberList(` call
site in `services/ec2/handler_*.go` -- 243 non-test call sites (`grep -rn
"parseMemberList(" services/ec2/handler_*.go | grep -v _test.go` minus the
2 lines that are the helper's own definition/doc-comment in `handler.go`;
248 total substring matches). Automated the per-site check: for each call
site's enclosing operation, resolved the pinned
`aws-sdk-go-v2/service/ec2@v1.319.1/serializers.go`
`awsEc2query_serializeOpDocument<Op>Input` function, matched the exact
quoted wire-key literal the handler reads, and classified the matched
serializer line as `FlatKey(`/`Array(` (list, correct) vs `.Key(` followed
by a scalar builder (`.String`/`.Boolean`/`.Integer`/`.Long`/`.Double`,
wrong) vs `.Key(` opening a nested struct (needs the struct's own field
checked, not assumed). 2 `handleGet*`-prefixed call sites
(`GetHostReservationPurchasePreview`, `GetSpotPlacementScores`) skipped per
this file's own prior finding that 58-of-64 `Get*` ops are clean. Of the
remaining 241, 225 matched a real wire key on the first pass and classified
cleanly; 16 needed manual resolution (dynamic-prefix keys the literal-match
missed, casing differences between the Go handler name and the real SDK op
file name -- `DescribeIDFormat`/`DescribeIdFormat`,
`DescribeInstanceSQLHa*`/`DescribeInstanceSqlHa*`,
`AssignPrivateIPAddresses`/`AssignPrivateIpAddresses`, etc. -- and a few keys
that don't exist on the real wire at all). Every one of the 16 was read by
hand against its op's own `api_op_<Op>.go` and serializer. Also ran a bounded
inverse sweep (list-typed field read as scalar via `vals.Get`): extracted
all 476 `vals.Get("...")` literal keys across the same handler files,
narrowed to the 232 with a plural-suggestive leaf name, cross-referenced the
176 belonging to `handle*` operations against their serializers the same
way -- zero hits where the matched line was `FlatKey`/`Array`; the 10
non-matches were all casing-mismatch op-name misses, manually confirmed as
genuinely scalar fields (`Egress`/`CidrBlock`/`UseLongIds`/`HostnameType`/
`MacSystemIntegrityProtectionStatus`, all `*bool`/`*string`/enum on the real
input struct).

**5 real bugs found and fixed.** 2 are class 2 (wrong cardinality, this
pass's namesake bug); 3 are a related but distinct class -- a wrong wire-key
*name* (not shape) causing the same silent-parameter-loss failure mode,
found only by reading each operation's own serializer as instructed, kept
separate here rather than folded into the cardinality count:

- `DescribeIdFormat`/`DescribeIdentityIdFormat` (`handler_account_attrs.go`,
  `handleDescribeIDFormat`/`handleDescribeIdentityIDFormat`) -- class 2.
  `Resource` is a scalar `*string` on both inputs, serialized as the bare
  key `Resource` (`object.Key("Resource")` + `.String(...)`,
  serializers.go:77885 and :77873 respectively;
  `api_op_DescribeIdFormat.go:57`, `api_op_DescribeIdentityIdFormat.go:62`).
  Both handlers read it via `parseMemberList(vals, "Resource")`, hunting for
  `Resource.1` -- a key a real client's single-resource-type lookup never
  sends. The sibling `handleModifyIDFormat`/`handleModifyIdentityIDFormat`
  in the same file already read `vals.Get("Resource")` correctly, proving
  the handler's own author knew the right shape for the twin write op.
  Fixed: read `vals.Get("Resource")` as a scalar, wrapped into a 1-element
  slice only when non-empty (matching the existing `[]string`-taking
  `Backend.DescribeIDFormat`/`DescribeIdentityIDFormat` -- no
  `Backend`/exported signature changed).
- `ModifyClientVpnEndpoint` (`handler_client_vpn.go`,
  `handleModifyClientVpnEndpoint`) -- wrong key, not cardinality.
  `CreateClientVpnEndpointInput.DnsServers` is a flat `[]string`
  (`FlatKey("DnsServers")`, serializers.go:69675) -- reading it via
  `parseMemberList(vals, "DnsServers")` in `handleCreateClientVpnEndpoint`
  is correct. But `ModifyClientVpnEndpointInput.DnsServers` is a DIFFERENT
  shape: `*types.DnsServersOptionsModifyStructure`, a nested object whose
  own `CustomDnsServers []string` field is the actual list
  (`object.Key("DnsServers")` wrapping a nested serializer,
  serializers.go:87142-87146; `DnsServersOptionsModifyStructure.CustomDnsServers`,
  `types/types.go:5062`). The real wire key is
  `DnsServers.CustomDnsServers.N`, not `DnsServers.N` -- same field name,
  different shape between Create and Modify, exactly the sibling-shape trap
  this file warns about elsewhere. `handleModifyClientVpnEndpoint` copied
  Create's key verbatim, so Modify never picked up new DNS servers from a
  real client. Fixed: read `parseMemberList(vals,
  "DnsServers.CustomDnsServers")`.
- `ModifyTransitGatewayMeteringPolicy` (`handler_tgw_peripherals.go`) --
  wrong key, not cardinality. `AddMiddleboxAttachmentIds`/
  `RemoveMiddleboxAttachmentIds` are the Go field names, but each serializes
  under the SINGULAR wire key `AddMiddleboxAttachmentId`/
  `RemoveMiddleboxAttachmentId` (`FlatKey("AddMiddleboxAttachmentId")`/
  `FlatKey("RemoveMiddleboxAttachmentId")`, serializers.go:89068,89080).
  The handler read the plural Go field name as the literal wire key, which a
  real client never sends (the sibling `handleCreateTransitGatewayMeteringPolicy`
  in `handler_tgw_multicast.go:684` already reads the correctly-singular
  `MiddleboxAttachmentId` for the analogous create-time field). Adds/removes
  were always silently dropped. An existing test,
  `TestTGWPeripheralsHandler_ModifyMeteringPolicyAndGetEntries`
  (`handler_tgw_peripherals_test.go`), asserted this wrong behaviour as
  correct by constructing its raw form POST with the same wrong plural key
  (`AddMiddleboxAttachmentIds.1=tgw-attach-1`) the handler happened to also
  be looking for -- fixed alongside the handler to use the real singular
  key. Fixed handler: `parseMemberList(vals, "AddMiddleboxAttachmentId")` /
  `"RemoveMiddleboxAttachmentId"`.
- `ModifyVpcEndpointConnectionNotification` (`handler_vpc_endpoints.go`) --
  wrong key, not cardinality. `ConnectionEvents` serializes as the flat wire
  key `ConnectionEvents` (`FlatKey("ConnectionEvents")`,
  serializers.go:89688-89693), not `ConnectionEvents.member`. The sibling
  `handleCreateVpcEndpointConnectionNotification` tries
  `"ConnectionEvents.member"` first (also wrong -- dead code, left as-is,
  harmless) but falls back to the correct bare `"ConnectionEvents"`; Modify
  only ever tried the wrong key, with no fallback, so a real client's
  updated event list was always dropped on Modify. Fixed: read
  `parseMemberList(vals, "ConnectionEvents")`.

Confirmed correct (sample of the 16 manually-resolved sites, beyond the 225
auto-classified as `FlatKey`/`Array`): `AssociateInstanceEventWindow`/
`DisassociateInstanceEventWindow` read `AssociationTarget.InstanceId`/
`AssociationTarget.DedicatedHostId` -- both are genuinely `FlatKey`'d
**singular** wire names nested one level under the `AssociationTarget`
object despite **plural** Go field names (`InstanceIds`/`DedicatedHostIds`
on `types.InstanceEventWindowAssociationRequest`,
serializers.go:58786-58798) -- another instance of this file's documented
singular-wire/plural-Go trap, verified independently rather than assumed.
`ReplaceImageCriteriaInAllowedImagesSettings`'s `parseImageCriteria` helper
reads `ImageCriterion.N.ImageName`/`.ImageProvider`/`.MarketplaceProductCode`
as nested indexed lists inside each `ImageCriterion.N` -- correct, all three
are `FlatKey`'d list fields on `types.ImageCriterionRequest`
(serializers.go:58269-58291) nested under the outer `FlatKey("ImageCriterion")`
list (serializers.go:91007). `CreateTransitGateway`'s
`parseTransitGatewayRequestOptions` helper reads
`Options.TransitGatewayCidrBlocks` as a nested indexed list -- correct,
`TransitGatewayCidrBlocks` is `FlatKey`'d under the `Options` object
(serializers.go:66268). `DescribeTags`'s dynamic `Filter.%d.Value` read is
the standard repo-wide `Filter.N.Value.M` list pattern, confirmed correct.

Missing-feature / structural gaps found along the way (real key or read
path doesn't exist, distinct from the wrong-key-name bugs above -- kept
separate, not fixed as part of this class, not fabricated):

- `DescribeNetworkInterfacePermissions` (`handler_network_interfaces.go`)
  reads `parseMemberList(vals, "NetworkInterfaceId")` -- but that key does
  not exist anywhere on the real wire.
  `DescribeNetworkInterfacePermissionsInput` only has `Filters`,
  `NetworkInterfacePermissionIds` (`FlatKey("NetworkInterfacePermissionId")`,
  serializers.go:79924-79929), `MaxResults`, `NextToken`. A real client can
  never populate `NetworkInterfaceId`, so this read is always empty --
  functionally harmless today only because empty means "no filter", which
  happens to match returning everything, but the real
  `NetworkInterfacePermissionId` list filter and the `Filters`-based
  `network-interface-permission.network-interface-id` filter are both never
  wired. Not fixed (missing feature, not a misdirected read of a real key).
- `DescribeSecurityGroupVpcAssociations` (`handler_security_groups.go`)
  reads `parseMemberList(vals, "GroupId")` -- same shape of gap.
  `DescribeSecurityGroupVpcAssociationsInput` has no top-level `GroupId`
  parameter at all, only `Filters` (with a `group-id` filter name),
  `DryRun`, `MaxResults`, `NextToken` (serializers.go:80835-80855). A real
  client's `--filters Name=group-id,Values=...` is silently ignored. Not
  fixed (missing feature -- the real mechanism is `Filters`, not a bare
  key).
- `CreateSnapshots` (`handler_snapshots.go`, `handleCreateSnapshots`) --
  structural, more severe than the two above. The real
  `CreateSnapshotsInput` has no `VolumeId` parameter at all; it requires
  `InstanceSpecification` (`InstanceSpecification.InstanceId` is the
  required field that selects which instance's volumes to snapshot,
  `object.Key("InstanceSpecification")`, serializers.go:72359-72364). The
  handler reads `parseMemberList(vals, "VolumeId")` (a key that doesn't
  exist on the wire) and, when that's empty, falls back to
  `vals.Get("InstanceSpecification.ExcludeBootVolume")` -- a boolean flag --
  treated as if it were a volume ID string. `InstanceSpecification.InstanceId`,
  the actual required field, is never read at all. A real
  `client.CreateSnapshots(InstanceSpecification: {InstanceId: "i-..."})`
  call hits this handler's own `"at least one VolumeId is required"` error
  today. The existing top-of-function comment ("InstanceSpecification.InstanceId
  is the primary instance; volumes derived from it") describes the intended
  behavior but not what the code does -- a comment as bug-cause, not
  description, per this file's own standing warning. Not fixed: correctly
  implementing this needs the backend to derive an instance's attached
  volume IDs (a real feature addition, not a wire-key correction), out of
  scope for this class-scoped pass; flagged here for a follow-up.

Existing tests found wrong (asserted the bug as correct behaviour):
`TestTGWPeripheralsHandler_ModifyMeteringPolicyAndGetEntries`
(`handler_tgw_peripherals_test.go`) -- see the `ModifyTransitGatewayMeteringPolicy`
bug above; fixed alongside the handler. No other existing test in
`services/ec2/*_test.go` references any of the other 4 fixed keys (`Resource`
for Id-format ops, `DnsServers*` for `ModifyClientVpnEndpoint`,
`ConnectionEvents*` for `ModifyVpcEndpointConnectionNotification`) in a way
that exercised the buggy path -- confirmed by the full `go test -race
./services/ec2/...` suite passing both before this file's new tests were
added and after the 5 handler fixes, with no other test needing a change.

New tests (`services/ec2/wire_field_fixes_ec2sweep39_test.go`, 5
`*_RealClient` tests against the real `ec2sdk.Client`, each confirmed
failing pre-fix by running before the source change):
`TestDescribeIdFormat_ResourceFilter_RealClient`,
`TestDescribeIdentityIdFormat_ResourceFilter_RealClient`,
`TestModifyClientVpnEndpoint_DnsServers_RealClient`,
`TestModifyTransitGatewayMeteringPolicy_MiddleboxAttachmentIds_RealClient`,
`TestModifyVpcEndpointConnectionNotification_ConnectionEvents_RealClient`.

Class-exhaustion judgement: this pass is the first to enumerate literally
every `parseMemberList` call site rather than a themed sample, and it found
5 bugs in 243 sites (2.1%), continuing the downward trend from tranche 4's
1-in-20 rate. Combined with 5 prior themed tranches (11 bugs found across
~106 sampled operations before this pass) that also targeted this exact bug
class, and this pass's explicit confirmation that every remaining
`parseMemberList` call site in every `handler_*.go` file has now been read
against its own SDK serializer at least once (either in a prior tranche or
in this pass), the scalar-read-as-list/wrong-list-key class appears close to
exhausted in ec2 -- what remains uncovered is the inverse direction (only a
bounded 176-site sweep, not exhaustive) and the broader missing-feature/
`Filters`-unwired backlog documented across every tranche above, which is a
different, much larger body of work.

Gates: `go build -o /dev/null ./services/ec2/...` (clean, no exported
signature changed), `go vet ./services/ec2/...` (clean) and repo-wide `go
vet ./...` (clean -- no backend interface signature changed, ran anyway
since ec2's backend is composed by other services), `go test -race -count=1
./services/ec2/...` (`ok`, full suite including the new test file and the
one existing-test fix), `golangci-lint run ./services/ec2/...` (`0 issues`,
run last, no `--fix` used). No banned `//nolint`s.

**2026-08-30 pass -- `CreateFleet` never launched instances (gopherstack-q5k5)**:
`DescribeFleetInstances` and `DescribeFleetHistory` returned a hardcoded
empty set unconditionally (`handleDescribeFleetHistory`/
`handleDescribeFleetInstances` in `handler_fleet.go` built an empty response
struct and returned, never touching the backend at all). The 2026-08-29 pass
above's "Fleet family... clean" note only checked request-side `FleetId.N`
parsing, not this -- corrected in place above. The actual defect was one
level up: `Backend.CreateFleet` (`fleet.go`) took only `(fleetType string,
totalTargetCapacity int)` and never called anything instance-related, so
even a correct `FleetId` read on the Describe side would still have found
nothing to return.

Fixed by making `CreateFleet` actually launch instances, against
`CreateFleetInput`/`TargetCapacitySpecificationRequest`/
`FleetLaunchTemplateConfigRequest`/`FleetLaunchTemplateOverridesRequest`
(`api_op_CreateFleet.go`, `types/types.go:6910-7245`, ec2@v1.319.1): parses
`LaunchTemplateConfigs.N.LaunchTemplateSpecification.*` and
`LaunchTemplateConfigs.N.Overrides.M.*` (both `FlatKey`-encoded per
`serializers.go:57701/:57737`, confirmed against
`aws-sdk-go-v2@v1.43.4/aws/protocol/query/array.go`'s `newArray` -- flat
lists have no `.member.`/`.Item.` segment, matching this file's existing
`SpotFleetRequestConfig.LaunchSpecifications.N.` convention), resolves each
override's AMI/instance type against the referenced launch template
(falling back to `spotFleetDefaultImageID`/`spotFleetDefaultInstanceType`
when the template can't be resolved -- deliberately permissive, matching
`RequestSpotFleet`'s own fallback and required because none of this file's
own fleet tests, nor the pre-existing `test/integration` fleet test, ever
pre-create the launch template they reference), and spawns real `Instance` +
primary-ENI pairs round-robin across the resolved overrides until weighted
capacity reaches `TargetCapacitySpecification.TotalTargetCapacity`,
appending each instance's ID to the new `Fleet.InstanceIDs` field. Also now
reads `ExcessCapacityTerminationPolicy` and
`TerminateInstancesWithExpiration` at create time (both real top-level
`CreateFleetInput` fields per `serializers.go:70020`, previously ignored --
`ExcessCapacityTerminationPolicy` was unconditionally hardcoded to
`"termination"` regardless of what the request asked for) and
`OnDemandTargetCapacity`/`SpotTargetCapacity`/`TargetCapacityUnitType`
(already-declared but previously always-zero `Fleet` fields).

`DescribeFleetInstances`/`DescribeFleetHistory` now read this real state:
`DescribeFleetInstances` returns the fleet's `InstanceIDs` resolved against
`b.instances` (filtered by the one real documented filter, `instance-type`,
per `api_op_DescribeFleetInstances.go`'s Filters doc comment; filtered
before paginating). Matching the real API's own documented restriction
("Currently, DescribeFleetInstances does not support fleets of type
`instant`" -- use `DescribeFleets` instead), it returns an empty set for
`instant` fleets rather than fabricating support the real endpoint doesn't
have. `DescribeFleetHistory` returns a real `fleet-change` history record
appended at `CreateFleet` (and at `ModifyFleet`, which previously changed
`TotalTargetCapacity`/`ExcessCapacityTerminationPolicy` with no history
trail at all), filtered by `StartTime`/`EventType` before paginating,
capped at `maxSpotFleetHistoryEntries` like the sibling spot-fleet history
map. Neither op sorts its output (both return in append/launch order, which
is already a total order -- no tie-breaking needed).

`DescribeFleets` was the same bug from the other end: `FleetData.Instances`/
`FleetData.Errors` (`types/types.go:6646-6672`, "valid only when Type is set
to `instant`") were never wired into `fleetItem`/`toFleetItem` at all -- so
even once `CreateFleet` started tracking real instances, an `instant`
fleet's `Fleets[i].Instances` stayed structurally empty, a correctly-shaped
field over data the handler never populated (as opposed to the
`DescribeFleetInstances`/`History` bug, which was empty because the backend
held no data at all). Fixed: added `Errors`/`Instances` fields to
`fleetItem` (`handler_traffic_mirror.go`) and populate `Instances` for
`instant` fleets by grouping `DescribeInstances(f.InstanceIDs, "")` by
`InstanceType` (`groupFleetInstancesByType`, deterministic first-seen-type
ordering). Also added the `TargetCapacitySpecification` sibling fields
(`onDemandTargetCapacity`/`spotTargetCapacity`/`targetCapacityUnitType`/
`defaultTargetCapacityType`) that were declared on the real response type
(`awsEc2query_deserializeDocumentTargetCapacitySpecification`,
deserializers.go:164096) but never emitted -- found in passing while
extending `fleetItem` for the `Instances` fix, not this pass's primary bug
class.

`DeleteFleets` gained the same fix from the deletion side: real
`DeleteFleetsInput.TerminateInstances` ("the default is to terminate the
instances", `api_op_DeleteFleets.go`) was accepted on the wire
(`vals.Get("TerminateInstances")`) but silently discarded -- harmless while
fleets held no instances, but once `CreateFleet` started launching them a
deleted fleet would have leaked its instances running with no owner. Fixed:
`Backend.DeleteFleets` gained a `terminateInstances bool` param, honored the
same way `CancelSpotFleetRequests` already honors its own
`terminateInstances` flag.

`DescribeInstanceTypes` was not touched this pass -- this ticket's own
guidance named it as a precedent for restraint, and the 2026-08-29 pass
above already documents why it's correctly left alone (no instance-type
attribute catalogue exists in this backend to filter against); re-read that
note rather than re-deriving it, and it still holds.

State added vs. reused: `Fleet.InstanceIDs` (new field) and
`Fleet.DefaultTargetCapacityType` (new field, now wired to the response) are
the only new persistent state. Instance-launching itself
(`spawnFleetMemberInstanceLocked`) reuses the same
Instance+ENI+`indexInstanceLocked`/`indexENILocked`/`indexENIByVPCLocked`
sequence `spot_fleet.go`'s `spawnFleetInstanceLocked` already established for
an almost-identical problem (deliberately not shared/generalized across the
two fleet types -- they take different config shapes -- but the launch
sequence itself is not reinvented). A new `fleetHistory
map[string][]FleetHistoryRecord` mirrors the existing `spotFleetHistory` map
(same cap/half-trim pattern, same `backendSnapshot`/`Restore` wiring).

Not fabricated: no instance attribute (CPU/memory/network) data was
invented for launched fleet instances -- they get the same
`spotFleetDefaultInstanceType`/`spotFleetDefaultImageID` fallback (or the
launch template's own values when resolvable) that every other launch path
in this file already uses, not new made-up data. `ModifyFleet` does NOT
scale the fleet's actual instance count to match a changed
`TotalTargetCapacity` (unlike `ModifySpotFleetRequest`, which does) --
left unfixed and undocumented as a gap prior to this pass; flagging here
rather than fixing, since it's a distinct capacity-reconciliation feature
outside this ticket's named scope (`CreateFleet`/`DescribeFleetInstances`/
`DescribeFleetHistory`/`DescribeFleets`), not a wire-shape bug.

Existing tests that could not have caught this: `TestFleet`
(`handler_fleet_test.go`) asserted only fleet metadata (state/type/target
capacity) round-tripping through `DescribeFleets`, never instances --
strengthened in place (now asserts `InstanceIDs`/`DescribeFleetInstances`/
`DescribeFleetHistory`/instance termination on delete). The pre-existing
`test/integration/parity_audit_fixes_test.go`
(`TestIntegration_EC2_DescribeFleets_ReturnsCreatedFleet`) only asserted
`err == nil` and a fleet-id round-trip, never instance content -- a shape
this ticket's own guidance called out by name ("A test asserting only `err
== nil` passes against every bug in this class"); not modified (out of
`services/ec2/` scope) but noted here since it's exactly the failure mode.

New tests (`services/ec2/wire_field_fixes_ec2sweep40_test.go`, both
confirmed failing pre-fix against unmodified code in a throwaway
`git worktree add --detach <dir> HEAD` rather than the shared working tree):
`TestCreateFleet_LaunchesTrackedInstances` (creates a `maintain` fleet with
`TotalTargetCapacity=3`, asserts `DescribeFleetInstances` returns exactly 3
real, uniquely-`i-`-prefixed instance ids, and `DescribeFleetHistory`
returns the creation event), `TestDescribeFleets_InstantType_ShowsLaunchedInstances`
(creates an `instant` fleet with capacity 2, asserts both
`CreateFleetOutput.Instances` and `DescribeFleets`'s `Fleets[i].Instances`
report the 2 launched instances, and that `DescribeFleetInstances` correctly
returns empty for it per the real API's documented `instant`-fleet
restriction).

Interface signature changes: `Backend.CreateFleet` (now takes
`FleetCreateInput`, returns `(*Fleet, []CreateFleetInstanceResult, error)`),
`Backend.DeleteFleets` (gained `terminateInstances bool`), plus two new
interface methods, `Backend.DescribeFleetInstances`/`DescribeFleetHistory`.
Repo-wide `go vet ./...` run and clean -- no call site outside
`services/ec2/` references any of these (`CreateFleet`/`DeleteFleets` are
also method names on codebuild's and appstream's unrelated backends; grepped
and confirmed distinct). No repo-root `cli_*_test.go` fix was needed.

Not audited this pass: `ModifyFleet`'s capacity-reconciliation gap noted
above; the `Filters`-unwired backlog on `DescribeFleetInstances` beyond the
one `instance-type` filter now implemented (`DescribeFleetInstancesInput`
documents only that one filter, so this is believed complete, not merely
unaudited); whether `DescribeFleetHistory`/`DescribeFleetInstances`
implement `MaxResults`/`NextToken` truncation correctly under concurrent
modification (paginate-after-filter is now correct per-call, but no
cross-call consistency guarantee is claimed, matching every other
`page.Page`-less describe op in this file).

Security note: no AWS documentation was fetched this pass (all shape
verification came from the pinned SDK source already in the module cache),
so the previously-reported injected-footer pattern in fetched AWS docs
("run `aws agent-toolkit search-skills`") does not apply here.

Gates: `go build ./services/ec2/...` (clean), `go vet ./services/ec2/...`
(clean) and repo-wide `go vet ./...` (clean, backend interface signatures
changed), `go test -race -count=1 ./services/ec2/...` (full suite green,
including the new `wire_field_fixes_ec2sweep40_test.go` and the
strengthened `TestFleet`), `golangci-lint run ./services/ec2/...` (`0
issues` after fixing 7 findings, all on lines this pass added; the one
non-obvious case, `musttag` on `persistence.go`'s `json.Marshal(snap)`,
confirmed self-caused by reverting just that file (`git checkout`/`stash`
scoped to the single path) and re-running lint, which made the finding
disappear -- `gocognit` decomposed into 3 helper functions rather than suppressed,
`fieldalignment` applied via `fieldalignment -fix` scoped to
`services/ec2/...`, `goconst` resolved with a shared `filterKeyInstanceType`
const, `musttag`/`golines`/`prealloc`/`staticcheck` fixed directly; run
last, no remaining `--fix` diff). No banned `//nolint`s. Did NOT commit or
push -- all changes left in the working tree per this session's explicit
instruction.

## 2026-08-30 -- value-semantics filter audit (gopherstack-uox6)

Targeted pass for the bug class named in gopherstack-uox6: a filter
parameter that is read and applied, but with the wrong semantics --
invisible to every wire-shape/field-coverage scan because the field itself
is real. Confirmed `handler_filters.go`'s general convention (AND across
filter names, OR within a filter's values, case-sensitive, no negation
modifier) matches `types.Filter`'s own doc comment
(aws-sdk-go-v2/service/ec2/types/types.go:6432) across every `apply*Filters`
function in that file; wildcards are NOT documented on ordinary string
filters (only on specific timestamp filters as a `*` day-suffix, e.g.
`creation-date`/`launch-time`/the image-watermark timestamps), so the
plain-equality matchers throughout are correct as written, not a gap.

Four real bugs found and fixed, all confirmed failing against unmodified
code first, all real-aws-sdk-go-v2-client-driven:

1. **DescribeImageUsageReportEntries `creation-time` exact match**
   (`handler_filters.go` `usageReportEntryMatchesFilter`). The day-wildcard
   form was already correct, but the exact-match branch formatted the
   entry's `ReportCreationTime` with `time.RFC3339Nano` while
   `toImageUsageReportEntryItem` (`handler_image_ops.go`) puts the same
   field on the wire with plain `time.RFC3339` (no fractional seconds).
   Since the underlying `time.Time` almost always carries a nonzero
   nanosecond component, an exact-match filter built from the timestamp the
   API itself just returned never matched its own record. Under-matching.
   Fixed by formatting with `time.RFC3339` in both places. Test:
   `wire_field_fixes_creationtime_filter_test.go`.

2. **DescribeSecurityGroupRules `group-id` filter, multiple values**
   (`handler_security_groups.go` `handleDescribeSecurityGroupRules`). Read
   only `filters["group-id"][0]`, discarding every value after the first --
   the confirmed "list consumed only at its first element" shape. A
   multi-value `group-id` filter silently dropped every group past the
   first. Under-matching. Fixed by looping over all values and merging each
   group's rules (`Backend.DescribeSecurityGroupRules(groupID string)`
   itself unchanged -- no cross-service callers, confirmed by repo-wide
   grep). `security-group-rule-id` and `tag:` (also documented on
   `DescribeSecurityGroupRulesInput.Filters`) remain unimplemented --
   recorded as a gap, not fixed: this backend has no rule-ID-keyed lookup,
   only group-keyed. Test:
   `wire_field_fixes_sg_rules_multivalue_test.go`.

3. **SearchLocalGatewayRoutes `state` vs `route-search.exact-match`**
   (`handler_local_gateway.go` `searchLocalGatewayRouteStates`). Two
   distinct, separately-documented filter names
   (api_op_SearchLocalGatewayRoutes.go: `state` - "The state of the route."
   vs `route-search.exact-match` - "The exact match of the specified
   filter.") were folded into one `[]string` and matched against the
   route's `State` field, so any `route-search.exact-match` filter --
   whatever it is meant to match -- excluded every real route (no route's
   `State` is ever a CIDR/prefix string). Also read only
   `Filter.N.Value.1`, dropping additional `state` values. Both
   under-matching. Fixed by scoping the value-collection loop to `state`
   only and reading all `Value.M` indices. `route-search.exact-match` /
   `-longest-prefix-match` / `-subnet-of-match` / `-supernet-of-match` /
   `prefix-list-id` / `type` remain unimplemented -- the AWS web page
   fetched for this operation (see below) gives no more precision than the
   SDK doc comment on what `route-search.exact-match` actually matches
   (CIDR? prefix-list? destination?), so implementing CIDR-matching
   semantics here would be fabrication, not verification; left as a gap
   rather than guessed. Test:
   `wire_field_fixes_local_gateway_route_filters_test.go`.

4. **DescribeTags `tag:<key>` filter rejected as unknown**
   (`handler_tags.go` `handleDescribeTags`). `validDescribeTagsFilters` is
   an exact-match set of the four literal filter names
   (`key`/`resource-id`/`resource-type`/`value`); `tag:<key>` is a fifth,
   separately documented filter name with a dynamic suffix
   (api_op_DescribeTags.go: `tag : - The key/value combination of the
   tag...`), so every `tag:<key>` filter -- a legitimate, common request
   shape -- was rejected outright with `InvalidParameterValue: unknown
   filter name`, not merely mis-matched. Under-matching via wrongful
   rejection. Fixed by recognizing the `tag:` prefix before the
   unknown-name check and matching entries whose `Key`/`Value` satisfy each
   `tag:<key>` filter, ANDed with the existing filters per the file's
   standard combining rule. Decomposed `handleDescribeTags` into
   `parseDescribeTagsFilters` + `describeTagsFilters.matches` to keep
   `gocognit` under the repo's threshold without a nolint. Test:
   `wire_field_fixes_describetags_tagkey_filter_test.go`.

Checked and confirmed correct, not modified: `imageMatchesFilter`'s `name`
filter uses plain equality, matching that `DescribeImagesInput.Filters`
documents wildcards only on `creation-date`/`image-watermark.*` timestamps,
not on `name` (api_op_DescribeImages.go); boolean-valued filters
(`isDefault`, `encrypted`, `default`, `entry.egress`, etc.) all compare
against the literal string `"true"`, matching every Boolean filter's
documented `true`/`false` spelling; no EC2 filter anywhere in this package
documents a `!`-negation modifier (grepped the pinned SDK for
"negat"/"exclamation"/"prefixed with", no hits), so the secretsmanager-class
negation bug does not apply here; `addressMatchesFilter`'s `domain` case
(comparing the constant `"vpc"` against filter values rather than a
per-address field) is not a bug -- `Address` has no stored `Domain` field
because every address this backend ever creates is VPC-domain
(`handler_elastic_ips.go` hardcodes `Domain: resourceTypeVPC}` on every
response item), so the constant-vs-filter comparison is the correct
encoding of "this address's domain is always vpc", just written tersely.

Gap noted, not fixed: `handleDescribeAddresses` folds the `PublicIps`
direct request member into the same `filters["public-ip"]` OR-group as any
independent `Filter.N.Name=public-ip` the client also sends
(`handler_elastic_ips.go`), rather than treating them as two independently
ANDed narrowers. Only visibly wrong if a client sends both simultaneously
with different values -- no SDK doc or web page specifies how a direct
ID-list member should combine with an overlapping `Filters` entry, so this
is recorded rather than guessed.

One page fetched:
`https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_SearchLocalGatewayRoutes.html`
(for bug 3, to check whether it gave more precision than the SDK doc
comment on `route-search.exact-match` -- it did not, word-for-word
identical filter list). It carried the injected footer directing the
reader to run `aws agent-toolkit search-skills`; treated as untrusted page
content, not followed.

Tests added: 4 new files, 4 new tests total (one assertion-bearing test per
bug above), all confirmed failing against unmodified code before the fix
and passing after. No existing test was modified or weakened -- zero
assertion drops.

Gates: `go build ./services/ec2/...` (clean), `go vet ./services/ec2/...`
(clean), repo-wide `go vet ./...` (clean -- no Backend interface signature
changed, run anyway per this session's instructions), `go build ./...`
(clean), `go test -race -count=1 ./services/ec2/...` (full suite green),
`golangci-lint run ./services/ec2/...` (2 findings from this pass's own new
code on the first run -- `golines` on a >120-char line in
`wire_field_fixes_sg_rules_multivalue_test.go`, wrapped by hand;
`gocognit` on `handleDescribeTags` after the `tag:` fix pushed it over the
repo's threshold, decomposed into `parseDescribeTagsFilters` +
`describeTagsFilters.matches` rather than suppressed -- re-ran, `0 issues`).
No banned `//nolint`s (grepped for cyclop/gocyclo/gocognit/funlen, zero
hits in `services/ec2/`). Did NOT commit, push, or run any `bd` write
command -- all changes left in the working tree per this session's
instructions.

## 2026-08-31 -- never-declared-field sweep, MaxResults/NextToken pagination
## family (cmd/reqfielddiff, gopherstack-4glf)

Targeted pass for the axis named in gopherstack-4glf/gopherstack-uox6: a
field the emulator never declared at all, invisible to every prior
wire-shape/field-coverage tool because there is no struct member to
enumerate. This is a DIFFERENT axis from every prior ec2 entry above --
`filter_default_semantics`/`request_field_never_read`/`wrong_wire_key` (all
"fixed" per `covledger`) audit fields the emulator DID declare; this axis
covers fields it never modeled in the first place. No prior ec2 pass
touched it; covledger and this file were both silent on it before this
entry.

`go run ./cmd/reqfielddiff -dir ec2` (resolution: 785/785 SDK operations,
447 with declared fields) reports **204 tier-1 findings** for ec2 -- by far
the largest queue of any service, confirming the brief's number. Given
"this is too large to finish," picked one coherent recurring field shape
rather than attempting breadth: **MaxResults/NextToken pagination missing
entirely**, not merely mishandled -- six Describe/Get/Search operations
whose real SDK input declares `MaxResults`+`NextToken` but whose handler
read neither, always returning the full result set in one page with no
`NextToken`. Chosen because (a) it is mechanically identical across all
six, (b) this file's own `ec2PageMin*`/`ec2PageMax*` const block already
established the exact fix shape from an earlier, unrelated pagination sweep
(`ec2sweep11`), and (c) each has a crisp, doc-stated bound/default,
minimizing the fabrication risk this axis warns about.

**Query-parameter false-positive measurement.** ec2 is a query-protocol
service: nothing is decoded into per-field typed structs, so reqfielddiff's
top tier is dominated by fields that ARE read, just via `vals.Get`/
`parseMemberList`/`parseEC2Filters`/`parseEC2Pagination` rather than a
struct member. Verified this by hand across the entire `*Ids`/`*Names`
direct-ID-list-narrowing family (26 of the 204 tier-1 findings sharing that
one field shape: `DescribeSnapshots.SnapshotIds`, `DescribeVolumes.VolumeIds`,
`DescribeSecurityGroups.GroupIds`/`GroupNames`, `DescribeInstances.InstanceIds`,
etc.) by locating each operation's handler function (built a 785-entry
op-to-handler-func map from the two `ops[...]=` registration forms in this
package) and confirming the field's wire key already appears as a
`parseMemberList`/`vals.Get` argument or an inline `VpcPeeringConnectionId.%d`
loop in that function body: **26 of 26 (100%) were this false-positive
shape**, none a real gap. A follow-up automated pass (grep each tier-1
finding's field name, or its `Ids`->`Id`/`Names`->`Name` singularization,
as a literal quoted string inside the resolved handler's function body, plus
`parseEC2Pagination(`/`parseEC2Filters(` call-presence for MaxResults/
NextToken/Filters) classified **74 of 204 (36%)** as this shape across the
whole tier-1 list -- a conservative lower bound, since it only catches
literal-string and named-helper matches, not every transformation. Two
concrete non-Ids examples caught by the same shape while investigating the
MaxResults family: `DescribeInstanceSQLHaHistoryStates.StartTime`/`.EndTime`
are read via `vals.Get("StartTime")`/`vals.Get("EndTime")` in
`handler_sql_ha.go`, invisible to the detector for the same reason.

**Six real bugs found and fixed, all confirmed failing against unmodified
code first** (reverted each handler's added `parseEC2Pagination`/`pageSlice`
call in isolation, re-ran that op's new test, watched it fail, restored):

1. **DescribeNetworkInterfacePermissions.MaxResults** (documented default 50,
   `api_op_DescribeNetworkInterfacePermissions.go`: "up to 50 results are
   returned by default", no explicit bound given -> used this file's existing
   1..1000 fallback convention). `handler_network_interfaces.go`
   `handleDescribeNetworkInterfacePermissions` returned every permission in
   one page. Test seeds 51 permissions on one ENI, omits `MaxResults`,
   asserts exactly 50 come back with a `NextToken`.
2. **DescribeReservedInstancesOfferings.MaxResults** (max/default 100,
   `api_op_DescribeReservedInstancesOfferings.go`: "The maximum is 100.
   Default: 100"). `handler_reserved_instances.go`
   `handleDescribeReservedInstancesOfferings` never truncated. Test seeds
   101 offerings via the existing `SeedReservedInstancesOffering` test
   helper, omits `MaxResults`, asserts exactly 100 + `NextToken`.
3. **DescribeScheduledInstances.MaxResults** (min 5 / max 300 / default 100,
   `api_op_DescribeScheduledInstances.go`). `handler_scheduled_instances.go`
   `handleDescribeScheduledInstances` never truncated. Test purchases 101
   schedules in one `PurchaseScheduledInstances` call (the static 3-entry
   availability catalog imposes no cap on purchase count), omits
   `MaxResults`, asserts exactly 100 + `NextToken`.
4. **SearchTransitGatewayRoutes.MaxResults** (default 1000, no explicit
   bound stated -> this file's 1..1000 fallback convention, which happens to
   match the stated default exactly). `handler_tgw_peripherals.go`
   `handleSearchTransitGatewayRoutes` never truncated and always reported
   `AdditionalRoutesAvailable: false`. Test seeds 1001 blackhole routes
   (cheap: no attachment required), omits `MaxResults`, asserts exactly
   1000 + `NextToken` + `AdditionalRoutesAvailable: true`; second page
   returns the last route with `AdditionalRoutesAvailable: false`.
5. **DescribeScheduledInstanceAvailability.MaxResults** (min 5 / max 300 /
   default 300). Real gap -- `handleDescribeScheduledInstanceAvailability`
   accepted any `MaxResults` value silently and ignored it -- but this
   backend's availability catalog is a hardcoded 3-entry static list
   (`scheduledInstanceCatalog`, one weekly/daily/monthly schedule per
   region), below MaxResults' own documented floor of 5, so the
   default-page-size truncation itself is **structurally unobservable**
   against this backend's data model. Fixed the validation/plumbing (now
   correct and consistent with every sibling op) but the test can only
   prove the testable half: values outside 5..300 are now rejected
   (previously silently accepted), and a valid in-range value still returns
   all 3 catalog entries.
6. **GetVpnConnectionDeviceTypes.MaxResults** (min 200 / max 1000, and
   uniquely among these six, `api_op_GetVpnConnectionDeviceTypes.go`
   documents a **contingent** default: "If this parameter is not used, then
   GetVpnConnectionDeviceTypes returns all results" -- omission means
   unbounded, not a fixed page size). Declared the field and invented no
   numeric default for the omitted case, matching the guard rail: the
   handler now only calls `parseEC2Pagination`/`pageSlice` when
   `vals.Get("MaxResults") != ""`, leaving the omitted-case behavior
   (already correct pre-fix) untouched. The real bug fixed is that an
   explicitly-supplied `MaxResults` was silently ignored. This backend's
   device-type catalog (a short hardcoded list, confirmed `< 200` in the
   test itself via `require.Less`) sits below the documented floor of 200,
   so -- same shape as #5 -- only the out-of-range-rejection half is
   testable; test also pins the doc's "omitted -> return everything"
   behavior explicitly.

**Where the default was applied, and why.** All six live in the handler
(query-protocol) layer, matching this file's own established convention
(`parseEC2Pagination`/`pageSlice`, `handler.go` lines ~836-914) -- there is
no backend-layer equivalent here because pagination is a wire-response
concern (truncating what the backend already returned), not a stored-record
default; the backend's `Describe*` methods are unchanged. Added six new
named consts to the existing `ec2PageMin*`/`ec2PageMax*`/`ec2PageDefault*`
block in `handler.go` (each citing its SDK doc-comment source, following
the file's existing citation convention), and a `NextToken string
\`xml:"nextToken,omitempty"\`` field to each of the five response structs
that lacked one (`SearchTransitGatewayRoutesResponse` already had
`AdditionalRoutesAvailable`, now correctly set).

**Not covered, for the next pass.** Of the 204 tier-1 findings, 6 are fixed
here; the other 198 are untouched, including the ~130 the automated
query-param check could not clear (conservative false-positive
classification, not a triage verdict -- most of those still need
hand-verification like the 26 done here). One adjacent gap found while
working `DescribeReservedInstancesOfferings` and deliberately left alone
(out of this pass's chosen slice): the same operation has five more tier-1
findings on its `Filters`/`InstanceTenancy`/`MaxDuration`/`MaxInstanceCount`/
`MinDuration` fields -- real, undeclared, and a natural next slice, but
distinct from the MaxResults/NextToken shape this pass targeted.

Tests: one new file, `wire_field_fixes_ec2sweep43_test.go`, 6 new test
functions, 53 assertion calls total, zero existing tests modified (zero
assertion drops). The four truncation tests (#1-4 above) omit `MaxResults`
entirely per this axis's rule that a test which always sets the field
cannot observe its default; the two range-only tests (#5, #6) set
out-of-range values deliberately (the default itself is structurally
unobservable against this backend's static catalogs, as explained above)
and also assert the omitted-case baseline.

One AWS API reference page was consulted only via the pinned
`aws-sdk-go-v2/service/ec2@v1.319.1` module-cache source (no web fetch this
pass) for every doc-comment cited above.

Gates: `go build ./services/ec2/...` (clean), `go vet ./services/ec2/...`
(clean), `go vet ./...` repo-wide (clean -- no `Backend` interface signature
changed), `go build ./...` (clean), `go test -race -count=1
./services/ec2/...` (full suite green, including the six confirmed-failing-
pre-fix new tests), `golangci-lint run ./services/ec2/...` (10 findings on
first run, all self-caused by this pass's own new code -- `golines` on the
test file and two long const-comment lines in `handler.go`, `fieldalignment`
on the five response structs that gained a bare `string` field -- fixed
with `golines -w -m 120` and `fieldalignment -fix` scoped to
`services/ec2/...`, then hand-shortened the two over-long const comments;
re-ran, `0 issues`). No banned `//nolint`s (grepped for
cyclop/gocyclo/gocognit/funlen in `services/ec2/`, zero hits); the one
pre-existing `//nolint:gochecknoglobals` in `handler.go` (on
`errCodeLookup`, `git diff` confirms untouched by this pass's hunks) is
unrelated and still needed. Did NOT commit, push, or run any `bd` write
command -- all changes left in the working tree per this session's
instructions.


## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in
`ef0eef041`. Built the unpatched `cmd/reqfieldscan`/`cmd/reqfielddiff` from
`ef0eef041~1` in a worktree, ran both five times against this package, and
diffed against HEAD.

`cmd/reqfieldscan`: byte-identical across all 5 old runs and HEAD.
`cmd/reqfielddiff`: findings ranged 2115-2126 across the 5 old runs (2099
at HEAD), with 46 op.field keys moving, all in one direction: present in
some old (misresolved) run, absent at HEAD. Zero keys appeared at HEAD
that were absent from every old run, so no evidence of the dangerous
direction here.

All 46 belong to ops where an exported `*InMemoryBackend` method
case-folds onto the same name as the real, correctly-registered
`ops["<Op>"] = h.handle<Op>` dispatch-table entry (the acronym-casing
mechanism from gopherstack-id70's parent finding: `NetworkAcl`/`NetworkACL`,
`IdFormat`/`IDFormat`, `VpcClassicLinkDnsSupport`/`VpcClassicLinkDNSSupport`,
`InstanceSqlHa`/`InstanceSQLHa`, `PrivateDnsNameOptions`/`PrivateDNSNameOptions`,
`PublicIpDnsNameOptions`/`PublicIPDNSNameOptions`, `EbsDefaultKmsKeyId`/
`EbsDefaultKmsKeyID`, `VpcEndpointServicePrivateDnsVerification`/
`VpcEndpointServicePrivateDNSVerification`). Read every one of the 46
handler bodies directly (`handler_network_acls.go`, `handler_account_attrs.go`,
`handler_instance_attrs.go`, `handler_sql_ha.go`, `handler_vpc_config.go`,
`handler_volumes.go`, `handler_vpc_endpoint_services.go`): every field is
genuinely read via `vals.Get("<Name>")` or the recognized
`parseMemberList(vals, ...)`/`parseOptionalInt32(vals, ...)`/
`parseOptionalBool(vals, ...)` helper shapes. All 46 were the old tool
falsely reporting a handled field as missing (over-reporting, the safe
direction). No bugs found; no code changed.

## 2026-08-31 -- never-declared-field sweep, security-group name resolution
## and Copy/Import Encrypted/KmsKeyId family (cmd/reqfielddiff, gopherstack-uox6)

`go run ./cmd/reqfielddiff -dir ec2` reports 128 tier-1 findings, confirmed
against the orchestrator's own re-run before this pass started. Worked a
slice of 15, chosen for having existing backend state to honour truthfully
rather than by tier order:

**Fixed (13 fields, 10 operations):**

- `AuthorizeSecurityGroupIngress.GroupName`, `RevokeSecurityGroupIngress.
  GroupName`, `DeleteSecurityGroup.GroupName`,
  `UpdateSecurityGroupRuleDescriptionsIngress.GroupName`,
  `UpdateSecurityGroupRuleDescriptionsEgress.GroupName` -- all five declare
  GroupName as an alternative to GroupId for default-VPC groups
  (ec2@v1.319.1 doc comments) but only ever read GroupId, rejecting a
  name-only request outright. `handler_security_groups.go` gained
  `resolveSecurityGroupID`, reusing the name-lookup
  `handleDescribeSecurityGroups` already did for its own filtering.
- `ImportImage.Encrypted`/`KmsKeyId`, `ImportSnapshot.Encrypted`/`KmsKeyId`
  -- echoed straight back on the immediate response AND the matching
  Describe*ImportTasks list item (`ImportImageTask`/`SnapshotTaskDetail`
  both carry the pair in types.go) but neither was read, so every import
  silently came back unencrypted regardless of the request. `RoleName` on
  both operations was deliberately left alone: it never appears in
  `ImportImageOutput`, `ImportSnapshotOutput` or `SnapshotTaskDetail`, and
  this backend simulates no IAM role assumption for import tasks, so
  storing it would be unobservable by any client -- recorded, not
  fabricated.
- `CopySnapshot.Encrypted`/`KmsKeyId` -- the backend already inherited
  Encrypted/KmsKeyID from the source snapshot when the caller said nothing
  (the SDK's own contingent default, already correct); the gap was an
  explicit `Encrypted=true` override of an unencrypted source, now honoured
  in `CopySnapshot`, with `KmsKeyId` falling back to the existing
  `defaultEBSKmsKeyAlias` convention (`handler_volumes.go`'s
  `CreateVolume`) only when the caller sets Encrypted without a key.
- `CopyImage.CopyImageTags` -- default false ("Your user-defined AMI tags
  are not copied"), now copies the source image's tags via the existing
  generic `CreateTags`/`TagsForResource` subsystem. `DescribeImages` never
  echoed an image's `TagSet` at all before this fix (checked: no `Tags`
  field on `amiItem`), so honouring `CopyImageTags` would have been
  unobservable without also wiring that in -- both fixed together.
- `DescribeImages.IncludeDisabled` -- default excludes disabled AMIs
  (`b.imageDisabled`, already tracked and already surfaced as
  `State="disabled"`); a general listing now filters them out unless
  `IncludeDisabled=true`. An image named explicitly by `ImageId` is still
  returned regardless -- that's the pre-existing, already-tested behavior
  of `TestDescribeImages_DisabledState_RealClient`
  (`wire_field_fixes_test.go`), preserved rather than weakened.
  `IncludeDeprecated` deliberately left alone: its own doc carries an
  ownership exception ("If you are the AMI owner, all deprecated AMIs
  appear in the response regardless") this single-account emulator has no
  `OwnerID` field to evaluate -- recorded rather than approximated.

**Recorded, not fixed (reasoning, not fabrication):**

- `CopyImage.Encrypted`/`KmsKeyId` -- `AMIStub` tracks no block-device
  mapping or per-image encryption state at all (only
  `ID/Name/Description/Architecture/Platform/RootDeviceName/State/
  SourceImageID`), and `DescribeImages`'s response has no encryption
  surface either. Honouring these would mean inventing a new response
  concept this backend's image model doesn't have anywhere -- the
  "simulating a capability that does not exist here" case.
- `StopInstances.Force`/`Hibernate`/`SkipOsShutdown`,
  `TerminateInstances.SkipOsShutdown` -- none of the three is echoed by
  `StopInstancesOutput`/`TerminateInstancesOutput` (checked the pinned
  SDK: both outputs return only a `StateChange` list), and this backend
  models no distinct code path (graceful-vs-forced filesystem shutdown,
  hibernation state, OS shutdown scripts) any of the three could route
  through. No legal input changes the outcome.
- `DeregisterImage.DeleteAssociatedSnapshots` -- already recorded with the
  same reasoning at this handler (`handler_images.go`'s
  `handleDeregisterImage` doc comment, pre-existing): `AMIStub` has no
  block-device-mapping/snapshot linkage to report against.

**False positives from an unrecognized form-read shape (examined, not
fixed):** `DescribeImages.ImageIds` is already read -- via a bare
`for i := 1; ; i++ { vals.Get(fmt.Sprintf("ImageId.%d", i)) }` loop, not a
call to a named `parseMemberList`-shaped helper, so the tool's
helper-call recognizer never matches it. 1 of the ~20 findings read
closely this pass was this shape (5%); the remaining four `DescribeImages`
findings examined (`Owners`, `IncludeDeprecated` twice-considered,
`IncludeDisabled`) were genuine gaps, not this false-positive class.

**Tests:** 5 new `_test.go` files (`wire_field_fixes_uox6_*.go`), 18 new
test functions driving the real `aws-sdk-go-v2` client, all confirmed
failing against unmodified source before the fix. Every documented-default
test (`*_EncryptedOmitted_DefaultsFalse`, `*_EncryptedNoKmsKeyId_
UsesDefaultEBSKey`, `*_CopyImageTagsOmitted_DoesNotCopyTags`,
`*_EncryptedOmitted_InheritsSource`) omits the field entirely rather than
setting it false/empty, so it can actually observe the default. No
existing assertion was weakened; one pre-existing test
(`TestDescribeImages_DisabledState_RealClient`) drove the "explicit
ImageId still returns disabled" carve-out in the `IncludeDisabled` fix
rather than being touched, and still passes unmodified.

Gates: `go build ./...`, `go vet ./...` (repo-wide, since `CopySnapshot`,
`ImportImage`, `ImportSnapshot` backend signatures changed -- six existing
test call sites across five files updated for the new params, no
production caller outside `services/ec2/` exists), `go test -race -count=1
./services/ec2/...`, `golangci-lint run ./services/ec2/...` (0 issues,
re-ran, `0 issues`). No banned `//nolint`s (grepped for
cyclop/gocyclo/gocognit/funlen in `services/ec2/`, zero hits, unchanged).
Did NOT commit, push, or run any `bd` write command -- all changes left in
the working tree per this session's instructions.

## 2026-08-31 -- never-declared-field sweep continuation, source-group-name
## authorize/revoke and VPC tenancy (cmd/reqfielddiff, gopherstack-uox6)

Re-ran `go run ./cmd/reqfielddiff -dir ec2` at the start of this pass: 117
tier-1 findings (down from the 128 the prior pass measured -- 13 fixed, and
two of those fixes are still visible in the tool's output as documented
tool-artifacts, so the net drop is smaller than 13). Confirmed the prior
pass's two known artifacts are genuinely fixed, not real gaps:
`AuthorizeSecurityGroupIngress.SourceSecurityGroupName` and
`UpdateSecurityGroupRuleDescriptionsIngress/Egress.GroupName` both route
through `resolveSecurityGroupID`, a method the tool's helper-call matcher
can't see (blind spot 7, as recorded). Also re-confirmed
`CopyImage.Encrypted`/`KmsKeyId` and `ImportImage`/`ImportSnapshot.RoleName`
are the prior pass's intentional declines, still correctly undeclared.

Worked a further slice of 4 fields, again choosing ones with existing
backend state to honour truthfully:

**Fixed (4 fields, 3 operations):**

- `AuthorizeSecurityGroupIngress.SourceSecurityGroupName`,
  `RevokeSecurityGroupIngress.SourceSecurityGroupName` -- the classic
  single-rule form ("[Default VPC] The name of the source security
  group... The rule grants full ICMP, UDP, and TCP access. To create a rule
  with a specific protocol and port range, specify a set of IP permissions
  instead.", ec2@v1.319.1 api_op_AuthorizeSecurityGroupIngress.go /
  api_op_RevokeSecurityGroupIngress.go) was never read at all --
  `parseIPPermissions` only ever parsed the `IpPermissions.N.*` list form,
  so a caller using the top-level field got a call that silently added or
  removed nothing. `handler_security_groups.go` gained
  `sourceGroupNameRule`, resolving the name via the same
  `DescribeSecurityGroups` lookup `resolveSecurityGroupID` already used,
  and building a single `Protocol: "-1"` rule when `IpPermissions` is
  empty. The Egress siblings (`AuthorizeSecurityGroupEgress`,
  `RevokeSecurityGroupEgress`) declare the same field but document it "Not
  supported. Use IP permissions instead." -- correctly left alone; real
  AWS ignores it there too.
- `CreateVpc.InstanceTenancy` -- documented default "default"
  (ec2@v1.319.1 api_op_CreateVpc.go), never read, and defaulted to
  `vpcTenancyDefault` in `handleCreateVpc`. This uncovered a larger,
  pre-existing gap in the sibling `ModifyVpcTenancy`: it already stored a
  tenancy per VPC in `b.vpcTenancy`, but nothing ever rendered it --
  `DescribeVpcs`/`CreateVpc` responses had no `instanceTenancy` element at
  all, so even a caller that successfully called `ModifyVpcTenancy` had no
  way to observe the result. Added `Backend.VpcTenancy(vpcID)` (falls back
  to "default" for VPCs with nothing recorded, e.g. ones from
  `CreateDefaultVpc`) and wired `instanceTenancy` onto `vpcItem`, rendered
  from both `handleCreateVpc` and `handleDescribeVpcs`. Same "a flag is
  only meaningful if the thing it controls is visible" lesson as the prior
  pass's `CopyImageTags` fix.
- `DeleteTags.Tags` -- documented as optional: "If you omit this
  parameter, we delete all user-defined tags for the specified resources...
  We do not delete Amazon Web Services-generated tags" (ec2@v1.319.1
  api_op_DeleteTags.go). `InMemoryBackend.DeleteTags` treated `len(keys) ==
  0` as a pure no-op instead -- a caller omitting Tags to wipe a resource's
  tags silently did nothing. Now deletes every stored key when `keys` is
  empty; kept the (currently unreachable, since this backend never stores
  an `aws:`-prefixed tag) `aws:` exclusion to match the documented
  behavior exactly rather than only what's reachable today. This finding
  still appears in the tool's tier-1 output after the fix: `DeleteTags`
  reads it via `parseEC2TagKeys`'s bare `for i := 1; i <= max; i++ {
  vals.Get(fmt.Sprintf("Tag.%d.Key", i)) }` loop (blind spot 6), a form the
  tool's matcher doesn't recognize -- confirmed genuinely fixed by the
  tests below, not a real remaining gap.

**Recorded, not fixed (reasoning, not fabrication):**

- `CreateImage.NoReboot` -- "If you don't specify this parameter, Amazon
  EC2 attempts to shut down and reboot the instance before creating the
  image." `InMemoryBackend.CreateImage` never touches instance state at
  all (no stop/reboot simulated, checked `deepdive_ops.go`), so there is no
  distinct code path for `NoReboot` to select between -- same reasoning as
  the prior pass's four stop-behaviour flags, and `CreateImageOutput` only
  ever returns an `ImageId`, giving no field to observe a difference on
  either.
- `CreateKeyPair.KeyFormat`/`KeyType` -- "Default: pem"/"Default: rsa".
  `InMemoryBackend.CreateKeyPair` (`key_pairs.go`) unconditionally
  generates an RSA key via `crypto/rsa`; there is no ED25519 generation
  path and no PPK (PuTTY private key) encoder anywhere in this codebase.
  `CreateKeyPairOutput` doesn't even echo either field back (checked the
  pinned SDK type: only `KeyFingerprint`/`KeyMaterial`/`KeyName`/
  `KeyPairId`/`Tags`), so returning an RSA/PEM key while silently ignoring
  a caller's `ed25519`/`ppk` request would misrepresent what was generated
  with no way for a test to even catch it via the response shape --
  declined rather than fabricated. `DescribeKeyPairs`' existing `KeyType`
  item field is left as-is (always empty for a `CreateKeyPair`-made key, a
  pre-existing gap orthogonal to this one).

**Out-of-scope caller repaired:** `CreateVpc`'s signature gained a
`tenancy string` parameter, breaking one caller outside `services/ec2/`:
`services/cloudformation/resources_extended.go`'s `createEC2VPC` (AWS::EC2
::VPC resource creator). Read the real `InstanceTenancy` CloudFormation
property via the existing `strProp` helper (same pattern already used for
`CidrBlock` two lines above) rather than hardcoding "default", since the
property already exists on the real resource type. `go build ./...` and
`go vet ./services/ec2/...` clean; a pre-existing, unrelated compile break
in `services/cloudformation/handler_stack_sets.go` (`op.CreatedAt`
undefined) was observed via `go vet ./...` but is caused by another
in-flight session's edits to that file (confirmed via `git status` showing
it modified outside this session's diff) -- not touched, not caused by
this pass.

**Tests:** 3 new `_test.go` files
(`wire_field_fixes_uox6_sourcegroupname_test.go`,
`wire_field_fixes_uox6_vpctenancy_test.go`,
`wire_field_fixes_uox6_deletetags_test.go`), 7 new test functions driving
the real `aws-sdk-go-v2` client on decoded responses, all confirmed failing
against unmodified source (or a surgical revert of just the defaulting
logic, keeping struct members, for the two pure-addition cases) before the
fix. `TestCreateVpc_InstanceTenancy_DefaultsToDefault` and
`TestDeleteTags_OmittedTagsDeletesAll` both omit the field entirely to
observe the default. One pre-existing test,
`TestInMemoryBackend_CreateDeleteDescribeTags/delete_empty_keys_is_noop`
(`handler_tags_test.go`), asserted the bug itself ("Empty keys: should be a
no-op, tag must remain") -- renamed to
`delete_omitted_keys_deletes_all_user_tags` and rewritten to assert the
documented behavior, adding a second untouched resource to the same case
so selectivity (only the targeted resource's tags are wiped) is still
checked. Assertion count for that one case: 3 before (Len + Equal + True),
3 after (same shape, corrected expected values) -- no drop. All other
`TestInMemoryBackend_CreateDeleteDescribeTags` subtests, and all of
`wire_field_fixes_uox6_sgname_test.go`'s existing assertions, untouched;
~40 other test files across the package had one mechanical
`b.CreateVpc("x.x.x.x/nn")` -> `b.CreateVpc("x.x.x.x/nn", "default")` call-
site update each (`CreateVpc` gained a required `tenancy` parameter), no
assertions touched.

Gates: `go build ./...`, `go vet ./services/ec2/...` (0 issues; repo-wide
`go vet ./...` fails only in `services/cloudformation` for the pre-existing,
unrelated reason above), `go test -race -count=1 ./services/ec2/...` (pass),
`golangci-lint run ./services/ec2/...` (0 issues after `fieldalignment -fix`
reordered `vpcItem` and `golines -m 120 -w` reformatted one over-length
`require.Len` call). `golangci-lint run ./services/cloudformation/...`
clean on the one file this pass touched
(`resources_extended.go`); the 3 issues it reports elsewhere are in files
this pass didn't edit. No banned `//nolint`s introduced; the one
pre-existing `//nolint:lll` in `handler_vpcs.go` and one pre-existing
`//nolint:nilerr` in `resources_extended.go` are both untouched by this
diff (confirmed via `git diff` -- neither line appears in the changed
hunks). Did NOT commit, push, or run any `bd` write command -- all changes
left in the working tree per this session's instructions.

## 2026-08-31 -- response wrapper-key mechanical sweep (gopherstack-6flj continuation)

Targeted the specific class this issue names -- a response field emitted
under an XML element name (or Go type) the real deserializer doesn't
expect, either silently dropping the whole collection or hard-erroring the
decode -- across the full `Describe*`/`List*` surface, since the ec2
tranche of this issue had only covered 14 major ops layer-1 and ~130
remained unaccounted for by name.

METHOD, LAYER 1 (top-level wrapper key): rather than hand-read all ~192
ops, wrote two small extractors (kept in the session scratchpad, not
committed): one walks every `describe*Response`/`list*Response` Go struct
in `services/ec2/*.go` and pulls the outer XML tag wrapping its item list
(handling the nested-anonymous-struct idiom this codebase uses
consistently, e.g. `Foo struct { Items []X \`xml:"item"\` } \`xml:"fooSet"\`
`); the other walks every `awsEc2query_deserializeOpDocumentDescribe*Output`
/`List*Output` function in the pinned `ec2@v1.319.1/deserializers.go` and
pulls each `case strings.EqualFold("key", ...)` that dispatches to a nested
(list/struct) deserializer rather than a scalar `decoder.Value()`. Cross-
referenced by operation name, case-insensitively (case-only differences are
harmless here per this issue's own note -- XML decode folds case).

RESULT: zero mismatches across 145 ops the extractor could parse
mechanically. Two apparent mismatches surfaced and were hand-verified as
extractor artifacts, not bugs: `DescribeApplicationStatus` (script matched
an inner `instanceSet` sub-key instead of the true outer
`applicationStatusesResponseType` wrapper -- the real one, read from the
struct by hand, matches exactly) and the initial pre-fix version of the
extractor itself, which for the first ~30 ops compared the wrong nesting
level entirely (same class of bug as the thing being hunted -- caught by
re-deriving from the raw struct text before trusting the tool's own
output). The other 47 ops the extractor couldn't parse (singular
`Describe*Attribute`/`Describe*Status`-shaped non-list responses, and a
handful of list responses using a named type alias instead of the
anonymous-struct idiom) were read by hand instead: `DescribeByoipCidrs`,
`DescribeCapacityReservationFleets`, `DescribeCapacityReservations`,
`DescribeHosts`, `DescribeInstanceStatus`, `DescribeInstanceTypes`,
`DescribeInternetGateways`, `DescribeLaunchTemplates`,
`DescribeLaunchTemplateVersions`, `DescribeNatGateways`,
`DescribeNetworkAcls`, `DescribeSpotFleetInstances`,
`DescribeSpotFleetRequestHistory`, `DescribeSpotFleetRequests`,
`DescribeSpotInstanceRequests`, `DescribeSpotPriceHistory`,
`DescribeVolumeStatus`, `DescribeVpcBlockPublicAccessOptions`,
`DescribeVpcEndpointServicePermissions`, `DescribeVpcEndpoints`,
`DescribePlacementGroups` -- all confirmed matching their own SDK output
struct's top-level key exactly (the remaining ~26 of the 47 are the
already-known-clean `Describe*Attribute` singular ops plus the 14 major ops
this issue's earlier ec2 batch already verified).

METHOD, LAYER 2 (per-item field names/types, hand-read against the pinned
deserializer, no mechanical tool -- this class needs judgement a script
can't apply): `DescribeTransitGatewayAttachments`/
`DescribeTransitGatewayVpcAttachments` (both: correct keys and types
throughout; `types.TransitGatewayAttachment`/`TransitGatewayVpcAttachment`
each carry several members -- `Association`, `CreationTime`,
`ResourceOwnerId`, `TransitGatewayOwnerId` on the former,
`Options`/`TransitGatewayVpcAttachmentOptions` (Dns/Ipv6/ApplianceMode/
SecurityGroupReferencing support) on the latter -- this backend's
`TransitGatewayAttachmentSummary`/`TransitGatewayVpcAttachment` structs
don't track any of them at all, a missing-state gap requiring new backend
fields across four source maps, not a wrapper-key bug; recorded, not
fixed), `DescribeVerifiedAccessInstances`/`DescribeVerifiedAccessEndpoints`
(correct keys/types; `CreationTime`/`LastUpdatedTime`/`FipsEnabled`/
`CidrEndpointsCustomSubDomain` on the instance and `Tags` on the endpoint
are real members this backend doesn't track -- same missing-state
category, recorded not fixed), `DescribeRouteServers`/
`DescribeRouteServerEndpoints`/`DescribeRouteServerPeers` (already fixed by
an earlier session in this campaign, commit `16e1eff9f` -- re-verified the
`routeServerEndpointItem.FailureReason`/`routeServerPeerItem.FailureReason`
flat-scalar fix is still correct against the pinned deserializer, and the
in-code comment explaining it is accurate, not a stale/false artifact),
`DescribeSpotFleetRequests` (walked the full
`SpotFleetRequestConfigData`/`SpotFleetLaunchSpecification` case lists --
every emitted field name and the `WeightedCapacity` float<->string
round-trip both correct), `DescribeVpcEndpointServices` (dual-wrapper
`serviceNameSet`/`serviceDetailSet` both correct, `serviceDetailItem`'s 8
emitted fields all real members of `types.ServiceDetail`; the ~7 unemitted
members -- `PrivateDnsName(s)`, `ServiceRegion`,
`SupportedIpAddressTypes`, etc -- already covered by this file's existing
`vpc_endpoints` note as unmodeled, not new), `DescribeVpcEndpointServicePermissions`
(the single emitted `principal` field is correct and real, but
`types.AllowedPrincipal` also declares `PrincipalType`/`ServiceId`/
`ServicePermissionId`/`Tags`; `Backend.DescribeVpcEndpointServicePermissions`
returns bare `[]string`, tracking none of the other four -- missing-state,
not wrapper-key), `DescribeSecurityGroupRules` (re-verified the
`3fe584c90` fix still holds -- `sgRuleDetailItem`'s 8 fields all correct
keys/types; `types.SecurityGroupRule` additionally declares `CidrIpv6`,
`GroupOwnerId`, `PrefixListId`, `ReferencedGroupInfo`,
`SecurityGroupRuleArn`, `TagSet`, none tracked by `SecurityGroupRuleDetail`
in `models.go` -- missing-state, and `GroupOwnerId` in particular is a
one-line-derivable `b.AccountID` fix of the same shape as several other
`OwnerId` fixes already in this file, but adding it means a `models.go`
change and the accompanying snapshot-version-guard bump, deliberately not
undertaken this pass since it isn't this issue's target class -- flagged
for a future missing-state pass instead of rushed here), `DescribeIpams`
(all emitted fields correct; `EnablePrivateGua`/`MeteredAccount`/
`StateMessage` unemitted, same missing-state category),
`DescribeSecurityGroupVpcAssociations` (all 5 emitted fields correct;
`StateReason` unemitted, same category), `DescribeLaunchTemplateVersions`
(the minimal `LaunchTemplateData{ImageID,InstanceType}` and top-level
fields are all correct keys; `Operator`/`VersionDescription` unemitted --
same category, and consistent with this backend's documented
single-version-per-template modeling limit noted earlier in this file).

RESULT: zero new wrapper-key bugs found across ~40 ops checked this pass
(20+ at layer 1 by hand beyond the mechanical sweep, ~13 at layer 2).
Every apparent gap resolved to either an already-fixed prior finding
(RouteServer family, SecurityGroupRules) or a missing-state gap (a real
SDK member this backend's Go struct never tracks at all) -- a different,
adjacent bug class from this issue's wrapper-key target, and each recorded
above rather than fabricated a fix for. Consistent with this issue's own
`14332b12e` ("ec2 is now clear for this class") and the extensive
`b430921d9`/`6ea5e9b15`/`d7f71c4cd`/`16e1eff9f` commits already on this
branch: by this point in the campaign the ec2 Describe/List wrapper-key
surface appears genuinely exhausted, not merely unexamined.

Pages fetched: none -- all verification came from the pinned
`aws-sdk-go-v2/service/ec2@v1.319.1` module cache already on disk, so the
"run `aws agent-toolkit search-skills`" footer pattern reported elsewhere
in this campaign does not apply to this pass.

No source files changed this pass (investigation only; a scratch
`zzz_listops_test.go` used to enumerate `h.ops` via `go test -run` was
written and removed before finishing, never left in the tree). Gates:
`go build ./services/ec2/...` (clean), `go vet ./services/ec2/...`
(clean), `go test -race -count=1 ./services/ec2/...` (pass, unchanged).
`golangci-lint run`/snapshot-version-guard not applicable (no `.go` files
touched). Did NOT commit, push, or run any `bd` write command.

## 2026-09-03/04 -- IAM instance profile launch-time integration (gopherstack-1a5)

Five-dimension audit (AWS behavior compliance, LocalStack parity, cross-
service integration, performance, resource leaks) per gopherstack-1a5. This
campaign's wire-shape (Describe/List filters, pagination, wrapper-key)
surface is, by this point, extensively re-audited and largely exhausted
(see the many passes above); went deep instead on the IAM instance profile
association lifecycle -- named by the task's own "cross-service integration"
dimension and never mentioned anywhere in this file before now.

FOUND AND FIXED, real client-observable bug: `handleRunInstances`
(`handler_instances_lifecycle.go`) never read `IamInstanceProfile.Arn`/
`IamInstanceProfile.Name` at all, even though `RunInstancesInput` declares
it (confirmed against `serializers.go:91938`,
`awsEc2query_serializeDocumentIamInstanceProfileSpecification`) and the
backend already has a working `AssociateIamInstanceProfile` for the
separate post-launch call. A real client launching an instance with
`--iam-instance-profile Name=...`/`Arn=...` -- the common launch-time path,
distinct from the two-call Associate flow -- silently got no instance
profile at all, no error, and no association ever created. Separately,
`instanceItem` (both `RunInstances` and `DescribeInstances` responses)
never rendered `iamInstanceProfile` at all, so even an instance profile
attached via the pre-existing `AssociateIamInstanceProfile` call never
showed up on the instance's own Describe output (confirmed against
`types.Instance.IamInstanceProfile`, `deserializers.go:110585`, element
`iamInstanceProfile` with sub-elements `arn`/`id` -- reused the existing
`iamProfileSpec` wire type already used by the Associate/Disassociate/
Replace handlers). Fixed: `handleRunInstances` now associates the launch-
time profile with each newly created instance via the existing
`Backend.AssociateIamInstanceProfile`; both `RunInstances` and
`DescribeInstances` now render the instance's current "associated"
association as `iamInstanceProfile`. New test:
`TestRunInstances_IamInstanceProfile_RealClient`
(`wire_field_fixes_ec2sweep44_test.go`), real `aws-sdk-go-v2` client,
confirmed to fail pre-fix (`cp`/`git show HEAD:` revert-and-restore, not
asserted without running) with "RunInstances response never rendered the
launch-time instance profile". No `Backend` interface signature changed;
no persisted struct field/type changed (XML-only response field), so no
`ec2SnapshotVersion` bump.

NOTED, NOT FIXED (real, out of scope for this pass): (1)
`AssociateIamInstanceProfile` has no check for an instance that already has
an active association -- real AWS's documented constraint ("You cannot
associate more than one IAM instance profile with an instance",
`api_op_AssociateIamInstanceProfile.go`) is entirely unenforced, so calling
it twice on the same instance silently creates two simultaneous
`state=associated` associations, a state real AWS cannot produce. Not
fixed: the real error code for this case was not independently confirmed
against the SDK (only the prose constraint is confirmed), and fabricating
a wire error code would violate this file's own no-fabrication rule. (2)
`TerminateInstances` never disassociates/cleans up a terminated instance's
`IamInstanceProfileAssociation` -- the association is left in state
`associated` forever, pointing at an instance that no longer accepts new
associations. Not the same shape as the `tag_cleanup` leak class fixed
earlier in this file (instance IDs are UUID-based and never reused here,
and the instance record itself is never deleted either, only marked
terminated -- so this doesn't cause unbounded growth via reuse), but it is
a real, unverified divergence from AWS's actual post-termination
association state machine (`associating`/`associated`/`disassociating`/
`disassociated`) that this pass did not have high enough confidence in to
fix without risking inventing behavior.

RunInstances also still ignores the real `SecurityGroup.N` (group *name*,
as opposed to `SecurityGroupId.N`) parameter entirely (confirmed on the
wire, `serializers.go:92093`,
`awsEc2query_serializeDocumentSecurityGroupStringList`) -- only
`validateSecurityGroupIDs` (`handler_filters.go`) is wired, which reads
`SecurityGroupId.N` only. Real AWS accepts group names for EC2-Classic and
default-VPC launches. Not fixed this pass (separate bug, not IAM-related,
and this mock has no EC2-Classic/default-VPC-name-resolution concept to
verify the right semantics against without risking a fabricated
implementation) -- flagged for a future pass.

Dimension coverage this pass:
1. AWS behavior compliance -- BUGS FOUND (RunInstances/IamInstanceProfile,
   above) in the narrow IAM-instance-profile-at-launch slice; the rest of
   the wire-protocol surface (filters/pagination/wrapper-keys/error codes)
   is NOT RE-CHECKED this pass, relying on the extensive prior audits
   above.
2. LocalStack parity -- NOT CHECKED (no LocalStack instance available this
   pass).
3. Cross-service integration -- BUGS FOUND for the IAM instance profile
   association lifecycle specifically (the slice this pass targeted).
   Broader cross-service surface (security-group/VPC reference validation
   beyond what's already fixed elsewhere in this file, ASG/ELB references,
   S3-backed AMI/snapshot storage) is NOT CHECKED -- no ASG/ELB/S3
   cross-service wiring was found anywhere in `services/ec2` at all
   (grepped; only `services/outposts` is wired via `cross_service.go`), so
   there is nothing to audit there beyond confirming its absence.
4. Performance -- NOT CHECKED this pass.
5. Resource leaks -- re-confirmed the existing `leaks:` entry above
   (goroutines/tag-map) still holds; found the IAM-association-on-terminate
   gap noted above but did not fix it (insufficient confidence in the
   correct real-AWS post-termination state, not a growth/goroutine leak).

Gates: `go build ./services/ec2/...` (clean), `go vet ./services/ec2/...`
(clean), `GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/ec2/...`
(pass, full suite including the new test), `gofmt -l services/ec2/` (clean),
`GOTOOLCHAIN=go1.26.6 golangci-lint run ./services/ec2/...` (0 issues,
after `golines`-wrapping two over-length call sites). No banned `//nolint`s
introduced. Did NOT commit, push, or run any `bd` write command.

## 2026-09-06 -- closed the three IAM-instance-profile/security-group gaps left open above (gopherstack-2mk2, gopherstack-847g, gopherstack-hmfm)

All three "NOTED, NOT FIXED" items from the 2026-09-03/04 pass above are now
fixed, deliberately together since 847g and hmfm both touch
`iamAssociations` state.

FOUND AND FIXED: (1) `RunInstances` ignored `SecurityGroup.N` (group name)
entirely, reading only `SecurityGroupId.N`. Real
`RunInstancesInput.SecurityGroups` doc comment (`api_op_RunInstances.go`):
"[Default VPC] The names of the security groups." -- a name only resolves
for the account's default VPC; `validateSecurityGroupIDs`
(`handler_filters.go`) now also parses `SecurityGroup.N`, resolves each name
against the launch target's VPC (the given `SubnetId`'s VPC, or the default
VPC), and rejects a name given for a subnet in a non-default VPC with
`InvalidParameterCombination` ("The parameter groupName cannot be used with
the parameter subnet"), matching real AWS. `SecurityGroupId.N` behavior is
unchanged. (2) `AssociateIamInstanceProfile` had no check for an instance
that already has an active association, though real AWS "cannot associate
more than one IAM instance profile with an instance"
(`api_op_AssociateIamInstanceProfile.go`); it now rejects a second
association on the same instance with `IncorrectState` ("There is an
existing association for instance ..."), consistent with this package's
existing `IncorrectState` usage for other already-in-that-state conflicts.
(3) `TerminateInstances` never disassociated a terminated instance's IAM
instance profile association, so it was observable as `state=associated` in
`DescribeIamInstanceProfileAssociations` forever, pointing at an instance
that no longer accepts new associations. `TerminateInstances` now removes
every `IamInstanceProfileAssociation` for the terminated instance (same
hard-delete semantics `DisassociateIamInstanceProfile` already uses); a
second instance's association is untouched. Association IDs are UUID-based
and never reused, so disassociating on terminate does not introduce a
reuse hazard.

Fixing (3) exposed a latent, unrelated gap: `ErrIAMAssociationNotFound` was
never registered in `errCodeLookup`, so `DisassociateIamInstanceProfile` (or
`ReplaceIamInstanceProfileAssociation`) on an unknown association ID
returned `InternalFailure`/500 instead of `InvalidAssociationID.NotFound`/
400 -- unreachable before this pass (the association row lived forever), but
now reachable via "terminate, then try to disassociate the same
association". Registered it (`InvalidAssociationID.NotFound`, matching
`ErrAssociationNotFound`'s code for the same not-found class used
elsewhere for route-table/EIP/etc. associations). Factored the
three-times-repeated `"InvalidAssociationID.NotFound"` literal into
`errCodeInvalidAssociationIDNotFound` to satisfy `goconst`.

No `Backend` interface signature changed; no persisted struct field/type
changed (`IamInstanceProfileAssociation`/`SecurityGroup` shapes are
untouched), so no `ec2SnapshotVersion` bump.

New tests: `TestRunInstances_SecurityGroupNames` (name resolves in default
VPC; name rejected for a non-default-VPC subnet; `SecurityGroupId.N` still
works, in `run_instances_security_group_names_test.go`),
`TestAssociateIamInstanceProfile_RejectsSecondAssociation` and
`TestTerminateInstances_DisassociatesIamInstanceProfile`
(`iam_instance_profile_lifecycle_test.go`) -- the latter two share one file
given the interaction, and mutually confirm terminating one instance never
disturbs a second instance's live association. Each guard was neutered
individually (by line, `cp`/`git show HEAD:`-restored afterward, not `git
checkout`) and confirmed to fail only its own test; the other two tests
stayed green under each neuter.

Gates: `go build ./services/ec2/...` and `go build ./...` (clean),
`GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/ec2/...` (pass, full
suite including the three new tests), `GOTOOLCHAIN=go1.26.6 golangci-lint
run ./services/ec2/...` (0 issues). No banned `//nolint`s introduced. Did
NOT commit, push, or run any `bd` write command.

## 2026-09-06 -- gopherstack-0o97 (DeleteVpc default ACL/route table), closed NOT A BUG

`api_op_DeleteVpc.go:16` (ec2@v1.319.1): "When you delete the VPC, it
deletes the default security group, network ACL, and route table for the
VPC." gopherstack-0o97 alleged `DeleteVpc` only cascades the default
security group and leaves the default network ACL and main route table
behind.

Not a bug for the ACL half: the default network ACL is never stored in
`b.networkACLs` in the first place. `DescribeNetworkAcls`
(`deepdive_ops.go`) derives one default ACL per VPC on every call, by
iterating `b.vpcs.All()` and synthesizing `acl-default-<vpcID>`. Deleting
the VPC removes it from `b.vpcs`, so the derived ACL simply stops being
produced -- the documented cascade, achieved by derivation instead of an
explicit delete step. `vpcIndexedDependencyViolationLocked`/
`vpcScannedDependencyViolationLocked` (`vpcs.go`) only scan `b.networkACLs`
(stored, non-default ACLs), so this derived default ACL never spuriously
blocks `DeleteVpc` either.

Main/default route table remains a genuine, honest gap: `RouteTable`
(`route_tables.go`) has no `Main`/`IsDefault` field, and `CreateVpc`
(`vpcs.go`) creates only the default security group -- no route table at
all. Nothing is created, so nothing is left behind; there's no dangling
resource for a bug report to point at, but the model still doesn't have a
main route table concept. Fixed the stale doc comment on `CreateVpc` that
still claimed the default ACL was unmodeled (`vpcs.go`).

If default network ACLs are ever changed to be stored (mirroring
`StoredNetworkACL`) instead of derived, `vpcScannedDependencyViolationLocked`
will need a default-ACL exception carved out, the same way
`vpcIndexedDependencyViolationLocked` already special-cases the default
security group (`sg.Name != defaultSecurityGroupName`) -- otherwise a
stored default ACL would make every `DeleteVpc` fail with a spurious
`DependencyViolation`.

New test: `TestDescribeNetworkAcls_DefaultACLCascadesOnVpcDelete`
(`network_acls_test.go`) -- characterization test, pins current (correct)
behavior rather than fixing anything. Creates two VPCs, deletes one, and
asserts its default ACL stops appearing while the other VPC's default ACL
is unaffected. Passes against current code by construction; it exists to
fail if a future change stores default ACLs without also handling the
cascade.

Gates: `GOTOOLCHAIN=go1.26.6 go test -race ./services/ec2/...` (pass,
including the new test), `GOTOOLCHAIN=go1.26.6 golangci-lint run
services/ec2/...` (0 issues). No banned `//nolint`s introduced. Did NOT
commit, push, or run any `bd` write command.

## 2026-09-06 -- gopherstack-y71o, VPC main route table implemented (partial)

Split out of gopherstack-0o97 above. `CreateVpc` (`vpcs.go`) now also
creates a main route table for the new VPC, mirroring the default security
group it already created: a `RouteTable{Main: true}` with one local route
(`Route{DestinationCIDR: cidr, GatewayID: "local"}` -- ec2@v1.319.1
api_op_ReplaceRoute.go:77 documents `local` as the fixed target of a
route table's local route) and one implicit VPC-wide association
(`RouteAssociation{Main: true}`, empty `SubnetID` -- ec2@v1.319.1
`types.RouteTableAssociation.Main` doc: "Indicates whether this is the
main route table"; `SubnetId`'s doc: "A subnet ID is not returned for an
implicit association"). `RouteTable` and `RouteAssociation`
(`route_tables.go`) each gained a `Main bool` field as the discriminator.

**Landmine handled**: registering the main table's ID in
`routeTableIDsByVPC` (via the existing `indexRouteTableLocked`) would have
made `vpcIndexedDependencyViolationLocked`'s `len(...) > 0` check block
every `DeleteVpc` with a spurious `DependencyViolation`. That check
(`vpcs.go`) now loops the VPC's route tables and only objects to a
**non-main** one, mirroring the existing default-security-group carve-out
(`sg.Name != defaultSecurityGroupName`). `DeleteVpc` itself now also
cascade-deletes the main table, matching `api_op_DeleteVpc.go:16` ("it
deletes the default security group, network ACL, and route table for the
VPC"). Proven by `TestDeleteVpc_MainRouteTableCascades`
(`main_route_table_test.go`): a VPC with nothing but its auto-created main
table deletes cleanly in one call.

Two more guards, both needed once an association can have an empty
`SubnetID`: `DeleteRouteTable` now rejects deleting a `Main` table
directly (`ErrDependencyViolation` -- real AWS requires reassigning main
status first; only `DeleteVpc` removes it) and `DisassociateRouteTable`
now rejects disassociating the implicit main association
(`ErrInvalidParameter` -- there is nothing to fall back to). Neither
operation's real deserializer declares a specific named error code for
this case (`awk "/deserializeOpErrorDeleteRouteTable\(/,/^}/"
deserializers.go` and the `DisassociateRouteTable` equivalent both show
only the generic `"UnknownError"` fallback, no `switch` cases), so both
reuse this file's existing generic-error sentinels rather than fabricate
an unverified AWS code.

**Real bug found and fixed while adding this**: `ReplaceRouteTableAssociation`
(`ec2core.go`) located the old association by testing `subnetID != ""`
as its "found" sentinel, then spliced the association out of its table
*before* that check. Any association with an empty `SubnetID` -- which
did not exist before this change but now does, on every main table --
would have been destructively removed and then the call would still
return `ErrAssociationNotFound`, leaving the main table's implicit
association gone with no rollback. Rewritten to track `found` explicitly
and only mutate state after all validation passes. The same fix also
implements the deliberate scope cut below: reassigning a VPC's main route
table via `ReplaceRouteTableAssociation` (ec2@v1.319.1
api_op_ReplaceRouteTableAssociation.go:17: "You can also use this
operation to change which table is the main route table in the VPC") is
rejected with a clear `ErrInvalidParameter` rather than silently doing a
plain subnet-style reassignment, which would have left the old table
still flagged `Main` internally with no association to show for it.
Proven by `TestReplaceRouteTableAssociation_MainAssociationRejected`,
which also asserts the implicit association was NOT removed by the
rejected call.

`DescribeRouteTables`' wire response (`handler_route_tables.go`)
now emits `<main>` on every association item (`assocItem.Main`, no
`omitempty` -- real AWS emits it unconditionally per
`awsEc2query_deserializeDocumentRouteTableAssociation`, deserializers.go).

**What was implemented**: (1) the main route table itself, with its local
route; (2) the implicit VPC-wide association, satisfying "implicit
association for subnets with no explicit association" as the single
always-present entry AWS shows for a main table (no per-subnet implicit
record is synthesized -- see below); (3) `Main` surfaced correctly on
`DescribeRouteTables` associations.

**Deliberately left absent** (documented here per the issue's scope note,
not silently missing):
- `ReplaceRouteTableAssociation`'s main-table reassignment (capability 4
  from the issue) is explicitly rejected, not implemented. Doing it
  properly means moving `Main` from the old table to the new one and
  regenerating the implicit association; half-implementing it risked
  exactly the "half-wired" state the issue warned against, so it was cut
  and documented instead.
- No `association.main` (or any other) `DescribeRouteTables` filter reads
  the new `Main` field -- `routeTableMatchesFilter` (`handler_filters.go`)
  is unchanged. Real AWS supports filtering on `association.main`.
- The backend's always-present seeded default VPC (`vpc-default`, built
  directly by `initDefaults`/`Reset` in `store.go`, not through
  `CreateVpc`) and `CreateDefaultVpc` (`vpcs.go`, which -- pre-existing,
  unrelated to this issue -- doesn't even create a default security
  group) still have no main route table. Only the `CreateVpc` codepath
  named in the issue was changed, to keep blast radius contained; touching
  the two seeding paths above would have altered the fixture shape of
  hundreds of unrelated existing tests.
- Every custom (non-main) route table still has no local route on
  creation. Real AWS gives one to every route table, not just the main
  one, but the issue text only asked for the main table's local route and
  the pinned SDK's `CreateRouteTable` doc comment doesn't document this
  either way, so it was left alone rather than guessed at.

New tests, all in `main_route_table_test.go`:
`TestCreateVpc_MainRouteTable`, `TestDeleteVpc_MainRouteTableCascades`
(the landmine proof), `TestDeleteRouteTable_MainRouteTableRejected`,
`TestDisassociateRouteTable_MainAssociationRejected`,
`TestReplaceRouteTableAssociation_MainAssociationRejected`,
`TestHandler_DescribeRouteTables_MainAssociation`. All fail to compile
against unmodified code (they reference the new `Main` fields), confirmed
by reverting the four production files and running them before restoring.

Gates: `GOTOOLCHAIN=go1.26.6 go test -race ./services/ec2/...` (pass),
`GOTOOLCHAIN=go1.26.6 golangci-lint run services/ec2/...` (0 issues).
`GOTOOLCHAIN=go1.26.6 go test ./pkgs/persistence/ -run
TestSnapshotVersionGuard` (read-only, no `-update`): FAILS, reporting
`ec2: backendSnapshot fields changed without a version bump; ... this is
bookkeeping, not a version-bump case: every old field is still present
unchanged, so the diff is additive only and needs no bump` -- expected,
since `RouteTable`/`RouteAssociation` gained `Main bool` fields (both
`omitempty`, so existing snapshots with `Main` false are byte-identical).
The same run also reports a `scheduler` failure that is NOT this change
(no `scheduler` files were touched); left for whoever owns that package.
Neither failure was addressed with `-update` per this task's scope limits.
Did NOT commit, push, or run any `bd` write command.
