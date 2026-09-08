---
# PARITY MANIFEST. services/directconnect/ is fully implemented (all 63 ops, overall: A) --
# the "PRE-IMPLEMENTATION AUDIT, NOT YET BUILT" framing below described the 2026-08-01 state, before
# any code existed; kept for its wire-shape research value (every claim was read directly from the
# SDK module cache, this repo's existing services, or the real Terraform AWS provider source, cited
# per-claim), not because the service is still unbuilt.
service: directconnect
sdk_module: aws-sdk-go-v2/service/directconnect@v1.44.1   # bumped since original audit (v1.43.3);
# 2026-08-05: added ListVirtualInterfaceRoutes -- see its `ops:` entry and the matching gaps entry.
# resolved via `go get .../directconnect@latest`
# in a throwaway scratch module (`go mod init probe && go get`), run in this session's scratchpad,
# NEVER touching this repo's go.mod (another agent was concurrently editing go.mod/go.sum/cli.go
# during this pass; this audit did not read or write any of those three files).
last_audit_commit: 3b90d4523   # STALE (found 2026-08-15, gopherstack-6flj wrapper-key sweep):
# this hash resolves to "test: replace the last unbubbleable sleeps with require.Eventually", a
# cross-service sleep-to-Eventually conversion touching services/lambda and test/{integration,e2e,
# terraform}, NOT a directconnect-specific commit -- almost certainly a stale/copy-pasted value
# carried forward across this file's several passes and never corrected. Left as-is (not chased
# further) rather than guessed at; noted here so the next pass doesn't trust it either. The prose
# below this line (added 2026-08-06) describes real work done in that session, just not AT that
# commit hash.
# 2026-08-06: added test/integration/directconnect_test.go
# (real aws-sdk-go-v2 client against a running Docker container -- connections, LAGs, private/
# public/transit VIFs, BGP peers, DirectConnectGateway/associations/proposals, and tagging), and
# re-judged every gaps: entry: the EC2 cross-service GatewayId/VirtualGatewayId validation this
# audit originally flagged as a nice-to-have (store.go's EC2GatewayResolver, cli.go's
# wireDirectConnectEC2) was already implemented and is now proven end-to-end by the new suite
# (TestIntegration_DirectConnect_GatewayAssociationsCrossService creates a REAL EC2 VpnGateway/
# TransitGateway via the EC2 SDK and confirms both acceptance of the real id and rejection of a
# fabricated one). Previous last_audit_commit was b850093a6.
# 2026-08-15 (gopherstack-6flj): full wrapper-key/nesting sweep of all 20 List/Describe/Get ops
# against directconnect@v1.44.1's own awsAwsjson11_deserializeOpDocument<Op>Output switch cases
# (python-extracted, not hand-transcribed) -- all 20 top-level keys and all 23 nested nested-shape
# types (Connection, Lag, Interconnect, VirtualInterface, DirectConnectGatewayAssociation,
# RouterType, CustomerAgreement, ResourceTag, Location, VirtualGateway,
# DirectConnectGatewayAttachment, DirectConnectGateway, DirectConnectGatewayAssociationProposal,
# AssociatedGateway, Loa, MacSecKey, BGPPeer, Tag, RouteFilterPrefix, Route, AsPathSegment,
# RateLimiterStatus, VirtualInterfaceTestHistory) field-diffed key-for-key against their own
# deserializer -- zero wrapper-key or nesting bugs found. Two never-modeled members found (both
# officially Deprecated in the pinned SDK's own doc comments, zero grep hits anywhere in this
# service before this pass): Connection/Interconnect/Lag.AwsDevice and
# DirectConnectGatewayAssociation.VirtualGatewayRegion -- see gaps: below; left disclosed, not
# fabricated, since no primary source here confirms whether real AWS still populates a deprecated
# field with a live value (deprecation notices don't say). Pagination/filters re-verified across
# all 10 paginate()-using ops plus the 9 correctly-non-paginated ones. Router re-confirmed
# structurally immune (single POST / dispatched purely by X-Amz-Target, no path routing at all).
last_audit_date: 2026-08-15   # was 2026-08-06
overall: A   # test/integration/directconnect_test.go passes for real (make build-linux && go test
# -race -run TestIntegration_DirectConnect ./test/integration/...); every gap that could produce
# real data is closed (cross-service EC2 validation, pkgs/arn.BuildGlobal for dx-gateway, pkgs/page
# pagination, placeholder LOA-CFA PDF, non-authoritative static seed data, empty
# DescribeCustomerMetadata default) -- see "Implementation summary" below and the pruned gaps:/new
# structural_gaps: lists. Remaining gaps: either need a genuinely impossible data source
# (structural_gaps:) or fall outside this service directory's ownership (CloudFormation resource
# types live in services/cloudformation, not here).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
# All 63 ops confirmed present in aws-sdk-go-v2/service/directconnect@v1.43.3
# (`ls api_op_*.go | grep -v _test.go | wc -l` => 63, matching this task's ~63 estimate exactly).
# All 63 are implemented (see ops: below, and test/integration/directconnect_test.go). Method/
# target verified by grepping every awsAwsjson11_serializeOp<Op>'s
# X-Amz-Target header literal in serializers.go (all 63, not sampled). Error sets verified by
# extracting every strings.EqualFold(...) case inside each op's own
# awsAwsjson11_deserializeOpError<Op> switch in deserializers.go (all 63, not sampled from the
# shared types/errors.go list, which enumerates all five shapes without saying which ops use which
# -- same trap the outposts/resiliencehub audits flagged).
ops:
  AcceptDirectConnectGatewayAssociationProposal: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST / (X-Amz-Target: OvertureService.AcceptDirectConnectGatewayAssociationProposal); in: AssociatedGatewayOwnerAccount*, DirectConnectGatewayId*, ProposalId*, OverrideAllowedPrefixesToDirectConnectGateway[]RouteFilterPrefix; out: DirectConnectGatewayAssociation; errors: DirectConnectClientException/DirectConnectServerException only. This is the accepting side of the cross-account proposal flow -- caller here is whoever owns the VGW/TGW being associated, confirming via AssociatedGatewayOwnerAccount (their own account id)."}
  AllocateConnectionOnInterconnect: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: Bandwidth*, ConnectionName*, InterconnectId*, OwnerAccount*, Vlan*int32; out: Connection; errors: base two only (DirectConnectClientException/DirectConnectServerException) -- the ONLY Allocate* op with no Tags input field at all and consequently no tag-related exceptions. Partner-flow op: an interconnect owner (partner) allocates a sub-connection on their interconnect to a named end-customer OwnerAccount."}
  AllocateHostedConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: Bandwidth*, ConnectionId* (the HOSTING connection or LAG), ConnectionName*, OwnerAccount*, Vlan*int32, Tags[]; out: Connection; errors: +DuplicateTagKeysException/TooManyTagsException (has Tags) but NOT LimitExceededException (unlike the VIF Allocate* ops below) -- confirmed by direct per-op grep, not an omission in this note. Same partner/reseller shape as AllocateConnectionOnInterconnect but hosted on an existing standard Connection/LAG rather than an Interconnect."}
  AllocatePrivateVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId* (hosting connection), NewPrivateVirtualInterfaceAllocation* (VirtualInterfaceName*, Vlan*int32, plus AddressFamily/AmazonAddress/Asn/AsnLong/AuthKey/CustomerAddress/Mtu/RateLimit/Tags -- notably NO DirectConnectGatewayId/VirtualGatewayId/EnableSiteLink fields on the Allocation shape, unlike NewPrivateVirtualInterface's non-allocation twin), OwnerAccount*; out: FLATTENED VirtualInterface fields directly on the Output struct (see wire-trap #1 below) including RouteFilterPrefixes even though private VIFs don't use route filters (field just stays empty/nil); errors: +DuplicateTagKeysException/LimitExceededException/TooManyTagsException."}
  AllocatePublicVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, NewPublicVirtualInterfaceAllocation* (VirtualInterfaceName*, Vlan*int32, plus AddressFamily/AmazonAddress/Asn/AsnLong/AuthKey/CustomerAddress/RateLimit/RouteFilterPrefixes[]/Tags), OwnerAccount*; out: FLATTENED VirtualInterface fields (same trap as AllocatePrivateVirtualInterface); errors: +DuplicateTagKeysException/LimitExceededException/TooManyTagsException."}
  AllocateTransitVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, NewTransitVirtualInterfaceAllocation* (all fields optional incl. VirtualInterfaceName/Vlan -- the ONLY New*VirtualInterface(Allocation) variant where the name/VLAN pair isn't marked required in the Go struct tags, though real-world usage surely needs them), OwnerAccount*; out: nested `VirtualInterface *types.VirtualInterface` (UNLIKE the private/public Allocate* siblings -- see wire-trap #1); errors: +DuplicateTagKeysException/LimitExceededException/TooManyTagsException."}
  AssociateConnectionWithLag: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, LagId*; out: Connection; errors: DirectConnectClientException/DirectConnectServerException/LimitExceededException -- has LimitExceededException despite taking no Tags (the only op where LimitExceededException appears WITHOUT the tag-exception pair), presumably bandwidth/link-count validation against Lag.MinimumLinks/ConnectionsBandwidth, not a tagging limit. 2026-09-06 (gopherstack-55so): re-association away from a different LAG now also enforced against that LAG's MinimumLinks, with NO last-member exception (unlike DisassociateConnectionFromLag) -- see gaps: for the doc's separate same-endpoint clause, left unenforced."}
  AssociateHostedConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId* (the hosted connection being reassigned), ParentConnectionId* (new hosting connection/LAG); out: Connection; errors: base two only."}
  AssociateMacSecKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, Cak+Ckn (paired, for a raw new key) OR SecretARN (a pre-existing Secrets Manager secret) -- mutually exclusive input modes, neither marked required at the Go-struct level so validation is runtime-only in real AWS; out: ConnectionId, MacSecKeys[]MacSecKey; errors: base two only. See MACsec notes below -- DisassociateMacSecKey needs a SecretARN to identify the key later, so a Cak/Ckn-provided key must still get a synthesized SecretARN in the response."}
  AssociateVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId* (new owning connection or LAG), VirtualInterfaceId*; out: FLATTENED VirtualInterface fields (same shape as CreatePrivateVirtualInterfaceOutput -- confirmed, not the nested-struct shape); errors: base two only. Moves an existing (already hosted/allocated) virtual interface onto a different connection/LAG."}
  ConfirmConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*; out: ConnectionState only (a bare enum, not the full Connection); errors: base two only. Owner-confirms a hosted connection out of 'ordering' into 'pending'/'available'."}
  ConfirmCustomerAgreement: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: AgreementName (optional, not required per Go struct -- real behavior presumably defaults to the one outstanding agreement if omitted); out: Status *string (free-form, not a typed enum -- CustomerAgreement.Status doc comment elsewhere says 'signed'/'unsigned'); errors: base two only."}
  ConfirmPrivateVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: VirtualInterfaceId*, DirectConnectGatewayId (optional) OR VirtualGatewayId (optional) -- exactly one expected, neither required at struct level; out: VirtualInterfaceState only; errors: base two only. This is the OWNER-side confirmation for a cross-account-hosted private VIF (paired with AllocatePrivateVirtualInterface's OwnerAccount recipient)."}
  ConfirmPublicVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: VirtualInterfaceId* only (no gateway choice -- public VIFs never bind to a DirectConnectGateway/VirtualGateway, consistent with DirectConnectGatewayAttachmentType only having TransitVirtualInterface/PrivateVirtualInterface, no Public variant); out: VirtualInterfaceState only; errors: base two only."}
  ConfirmTransitVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: DirectConnectGatewayId* AND VirtualInterfaceId*, BOTH required (unlike ConfirmPrivateVirtualInterface where the gateway field is optional) -- transit VIFs cannot be confirmed without picking a Direct Connect gateway at confirmation time; out: VirtualInterfaceState only; errors: base two only."}
  CreateBGPPeer: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: NewBGPPeer (optional *types.NewBGPPeer -- the whole struct is nilable, not the individual sub-fields), VirtualInterfaceId (optional *string, not required at struct level despite being obviously necessary); out: nested `VirtualInterface *types.VirtualInterface` (the parent VIF with the new peer appended to BgpPeers[]); errors: base two only."}
  CreateConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: Bandwidth*, ConnectionName*, Location*, LagId (optional -- create directly into an existing LAG), ProviderName, RequestMACSec*bool, Tags[]; out: Connection (full shape, ConnectionState starts 'requested' per the enum's own doc comment for a standard, non-hosted connection); errors: +DuplicateTagKeysException/TooManyTagsException, no LimitExceededException."}
  CreateDirectConnectGateway: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: DirectConnectGatewayName*, AmazonSideAsn (optional *int64 -- AWS auto-assigns if omitted, per general product knowledge, not confirmed in this SDK checkout), Tags[]; out: DirectConnectGateway{DirectConnectGatewayId,DirectConnectGatewayName,AmazonSideAsn,DirectConnectGatewayState(pending on create),OwnerAccount,StateChangeError,Tags}; errors: base two only (no tag-exceptions despite taking Tags -- confirmed by direct grep, an inconsistency vs. every OTHER Tags-accepting Create* op in this service)."}
  CreateDirectConnectGatewayAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: DirectConnectGatewayId*, AddAllowedPrefixesToDirectConnectGateway[]RouteFilterPrefix, GatewayId (optional *string -- the newer generic field, accepts either a VGW or TGW id), VirtualGatewayId (optional *string -- the OLDER field, VGW-only) -- BOTH optional, neither required at struct level, meaning the SDK does not encode 'exactly one of GatewayId/VirtualGatewayId' as a modeled constraint (see wire-trap #3); out: DirectConnectGatewayAssociation; errors: base two only."}
  CreateDirectConnectGatewayAssociationProposal: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: DirectConnectGatewayId*, DirectConnectGatewayOwnerAccount* (the account being asked to approve -- i.e. the DCGW owner, since the proposer here owns the VGW/TGW), GatewayId* (the VGW/TGW to propose associating), AddAllowedPrefixesToDirectConnectGateway[]/RemoveAllowedPrefixesToDirectConnectGateway[]; out: DirectConnectGatewayAssociationProposal{ProposalId,ProposalState(requested on create),AssociatedGateway,DirectConnectGatewayId,DirectConnectGatewayOwnerAccount,Existing/RequestedAllowedPrefixesToDirectConnectGateway}; errors: base two only. This is the PROPOSING side (VGW/TGW owner proposing to the DCGW owner) -- the cross-account direction is the reverse of what a naive reading of 'proposal' might suggest; verify against AcceptDirectConnectGatewayAssociationProposal's AssociatedGatewayOwnerAccount field (the accepter identifies themselves as owning the associated gateway, i.e. the VGW/TGW, confirming the proposer really is that same party)."}
  CreateInterconnect: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: Bandwidth*, InterconnectName*, Location*, LagId (optional), ProviderName, RequestMACSec*bool, Tags[]; out: Interconnect; errors: +DuplicateTagKeysException/TooManyTagsException, no LimitExceededException. AWS-internal/partner-only op in real life (interconnects are provisioned by Direct Connect Partners) -- see gaps for what 'honest simulation' means here."}
  CreateLag: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionsBandwidth*, LagName*, Location*, NumberOfConnections*int32 (how many physical connections to provision INSIDE the new LAG, max 4 at 1/10Gbps or 2 at 100/400Gbps per Lag.NumberOfConnections doc comment), ChildConnectionTags[]Tag (tags applied to the auto-created child connections, distinct from Tags[] on the LAG itself), ConnectionId (optional -- convert an EXISTING standalone connection into the LAG's first member instead of creating N fresh connections), ProviderName, RequestMACSec*bool, Tags[]; out: Lag (includes Connections[]Connection -- the LAG's own auto-provisioned or converted member connections); errors: +DuplicateTagKeysException/TooManyTagsException, no LimitExceededException."}
  CreatePrivateVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, NewPrivateVirtualInterface* (VirtualInterfaceName*, Vlan*int32, plus AddressFamily/AmazonAddress/Asn/AsnLong/AuthKey/CustomerAddress/DirectConnectGatewayId/EnableSiteLink/Mtu/RateLimit/Tags/VirtualGatewayId -- exactly one of DirectConnectGatewayId/VirtualGatewayId expected, neither required at struct level); out: FLATTENED VirtualInterface fields directly on Output (wire-trap #1); errors: +DuplicateTagKeysException/LimitExceededException/TooManyTagsException."}
  CreatePublicVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, NewPublicVirtualInterface* (VirtualInterfaceName*, Vlan*int32, plus AddressFamily/AmazonAddress/Asn/AsnLong/AuthKey/CustomerAddress/RateLimit/RouteFilterPrefixes[]/Tags -- no DirectConnectGatewayId/VirtualGatewayId field at all, confirming public VIFs never attach to a gateway); out: FLATTENED VirtualInterface fields (wire-trap #1); errors: +DuplicateTagKeysException/LimitExceededException/TooManyTagsException."}
  CreateTransitVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, NewTransitVirtualInterface* (DirectConnectGatewayId is present but NOT required at struct level here either, unlike ConfirmTransitVirtualInterface where it IS required -- create-time gateway binding is optional, confirm-time is not); out: nested `VirtualInterface *types.VirtualInterface` (wire-trap #1, matches AllocateTransitVirtualInterface's nested shape, NOT the flattened private/public shape); errors: +DuplicateTagKeysException/LimitExceededException/TooManyTagsException."}
  DeleteBGPPeer: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: Asn/AsnLong/BgpPeerId/CustomerAddress/VirtualInterfaceId -- ALL optional, no required fields at all in this Input struct (the real API presumably needs enough of this combination to uniquely identify one BGP peer, but the SDK does not encode which subset suffices); out: nested `VirtualInterface *types.VirtualInterface` (parent VIF with peer removed/marked deleting); errors: base two only."}
  DeleteConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*; out: full Connection (state transitioning through 'deleting'); errors: base two only."}
  DeleteDirectConnectGateway: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: DirectConnectGatewayId*; out: DirectConnectGateway (state 'deleting'); errors: base two only. Real AWS presumably rejects deletion while associations/attachments exist -- not encoded as a distinct exception type in this SDK (would surface as a generic DirectConnectClientException, not a typed Conflict-style shape -- see wire-trap #2)."}
  DeleteDirectConnectGatewayAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: AssociationId (optional) OR DirectConnectGatewayId+VirtualGatewayId (optional pair) -- ALL THREE fields optional at struct level, two alternate addressing modes with no required-field enforcement in the Go types; out: DirectConnectGatewayAssociation (state 'disassociating'); errors: base two only."}
  DeleteDirectConnectGatewayAssociationProposal: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ProposalId*; out: DirectConnectGatewayAssociationProposal (state 'deleted'); errors: base two only. Withdraws a pending proposal before the other party accepts it."}
  DeleteInterconnect: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: InterconnectId*; out: InterconnectState only (bare enum, NOT the full Interconnect -- unlike DeleteConnection/DeleteLag which return the full object); errors: base two only."}
  DeleteLag: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: LagId*; out: full Lag; errors: base two only. Real AWS presumably rejects deletion while hosted connections remain on the LAG -- again no typed Conflict exception exists to express that, only the generic client exception."}
  DeleteVirtualInterface: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: VirtualInterfaceId*; out: VirtualInterfaceState only (bare enum); errors: base two only."}
  DescribeConnectionLoa: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, LoaContentType (defaults to application/pdf, the only value), ProviderName; out: Loa{LoaContent[]byte, LoaContentType} -- structured, nested Loa object; errors: base two only. See DescribeLoa below for the sibling that returns the same fields FLATTENED instead of nested -- CORRECTION (implementation pass): this op's own SDK doc comment reads \"Deprecated. Use DescribeLoa instead\", the OPPOSITE of what this audit line first assumed from shape alone."}
  DescribeConnections: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId (optional -- omit for 'list all'), MaxResults/NextToken present on the Input struct; out: Connections[]Connection, NextToken; errors: base two only. NOTE: despite MaxResults/NextToken existing on the wire, there is NO generated Paginator type anywhere in this SDK module (confirmed: no pagination*.go file exists in the module) -- see wire-trap #4."}
  DescribeConnectionsOnInterconnect: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: InterconnectId* ONLY -- no MaxResults/NextToken on input at all, yet the Output DOES carry a NextToken field (always presumably nil/unused in practice, an asymmetry worth noting rather than assuming pagination exists here); out: Connections[]Connection, NextToken; errors: base two only."}
  DescribeCustomerMetadata: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: (no input fields at all -- account-scoped, implicit from auth context); out: Agreements[]CustomerAgreement, NniPartnerType(v1|v2|nonPartner); errors: base two only. Real-world this reflects a customer's signed Direct Connect partner agreements and NNI (Network-to-Network Interface) partner tier -- see gaps for what honest simulation looks like."}
  DescribeDirectConnectGatewayAssociationProposals: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: AssociatedGatewayId/DirectConnectGatewayId/ProposalId (all optional filters), MaxResults/NextToken; out: DirectConnectGatewayAssociationProposals[], NextToken; errors: base two only."}
  DescribeDirectConnectGatewayAssociations: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: AssociatedGatewayId/AssociationId/DirectConnectGatewayId/VirtualGatewayId (all optional filters -- four independent optional filters, any combination), MaxResults/NextToken; out: DirectConnectGatewayAssociations[], NextToken; errors: base two only."}
  DescribeDirectConnectGatewayAttachments: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: DirectConnectGatewayId/VirtualInterfaceId (optional filters), MaxResults/NextToken; out: DirectConnectGatewayAttachments[]DirectConnectGatewayAttachment{AttachmentState,AttachmentType(TransitVirtualInterface|PrivateVirtualInterface -- NO PublicVirtualInterface value exists in this enum at all, confirming public VIFs structurally cannot attach to a DCGW), DirectConnectGatewayId,StateChangeError,VirtualInterfaceId,VirtualInterfaceOwnerAccount,VirtualInterfaceRegion}, NextToken; errors: base two only."}
  DescribeDirectConnectGateways: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: DirectConnectGatewayId (optional), MaxResults/NextToken; out: DirectConnectGateways[], NextToken; errors: base two only."}
  DescribeHostedConnections: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId* (the HOSTING connection or LAG -- required, unlike DescribeConnections where the analogous field is optional), MaxResults/NextToken; out: Connections[] (the hosted/child connections riding on it), NextToken; errors: base two only."}
  DescribeInterconnectLoa: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: InterconnectId*, LoaContentType, ProviderName; out: Loa (nested, same shape as DescribeConnectionLoa); errors: base two only."}
  DescribeInterconnects: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: InterconnectId (optional), MaxResults/NextToken; out: Interconnects[], NextToken; errors: base two only."}
  DescribeLags: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: LagId (optional), MaxResults/NextToken; out: Lags[], NextToken; errors: base two only."}
  DescribeLoa: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, LoaContentType, ProviderName; out: LoaContent[]byte + LoaContentType FLATTENED directly on the Output (NOT nested in a Loa struct, unlike DescribeConnectionLoa/DescribeInterconnectLoa which both return a nested `Loa *types.Loa`); errors: base two only. CORRECTION (implementation pass): despite its older-looking flattened shape, THIS op is the CURRENT/preferred one -- DescribeConnectionLoa and DescribeInterconnectLoa are each independently marked \"Deprecated. Use DescribeLoa instead\" in their own SDK doc comments (api_op_DescribeConnectionLoa.go/api_op_DescribeInterconnectLoa.go), the reverse of this audit's original guess."}
  DescribeLocations: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: (no input fields); out: Locations[]Location{AvailableMacSecPortSpeeds[]string,AvailablePortSpeeds[]string,AvailableProviders[]string,LocationCode,LocationName,Region} -- no pagination at all; errors: base two only. Static reference-data op (AWS's published Direct Connect physical colocation facility list) -- see gaps for the seed-data implication."}
  DescribeRouterConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: VirtualInterfaceId*, RouterTypeIdentifier (optional, e.g. 'CiscoSystemsInc-2900SeriesRouters-IOS124' per RouterType.RouterTypeIdentifier's doc comment); out: CustomerRouterConfig*string (router-vendor-specific config text/XML), Router*RouterType, VirtualInterfaceId, VirtualInterfaceName; errors: base two only. Another static-reference/generated-artifact op -- see gaps."}
  DescribeTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ResourceArns*[]string (BATCH -- takes a LIST of ARNs in one call, unlike the single-ARN-per-call ListTagsForResource pattern used by outposts/resiliencehub); out: ResourceTags[]ResourceTag{ResourceArn,Tags[]Tag}; errors: base two only. This is Direct Connect's own native tag-read op, separate from (but shape-compatible with) resourcegroupstaggingapi's cross-service GetResources -- see cross-service wiring section."}
  DescribeVirtualGateways: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: (no input fields -- always returns every VGW usable for private VIF attachment, account-wide); out: VirtualGateways[]VirtualGateway{VirtualGatewayId,VirtualGatewayState*string(NOT a typed enum -- unlike every other *State field in this service, VirtualGatewayState is a bare *string; doc comment on the type lists pending/available/deleting/deleted as prose, not as generated enum consts)}; errors: base two only. NOTE: this almost certainly maps 1:1 onto EC2's own VpnGateway records (see cross-service wiring) rather than being a Direct-Connect-native resource -- a real implementation should probably proxy into services/ec2's VpnGateway list rather than maintain a duplicate store."}
  DescribeVirtualInterfaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId (optional filter), MaxResults/NextToken, VirtualInterfaceId (optional filter) -- both filters independently optional, any combination; out: VirtualInterfaces[]VirtualInterface (full nested shape, unlike the flattened Create/Allocate outputs -- see wire-trap #1), NextToken; errors: base two only."}
  DisassociateConnectionFromLag: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, LagId*; out: Connection; errors: base two only. Real AWS presumably enforces Lag.MinimumLinks (won't let the last required connection leave a LAG that still needs it) -- no typed exception encodes this, would surface as a generic client exception if enforced at all."}
  DisassociateMacSecKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, SecretARN* (REQUIRED here, unlike AssociateMacSecKey where SecretARN is one of two optional alternatives) -- confirms every associated key, even a raw Cak/Ckn-provided one, must have a resolvable SecretARN for later removal; out: ConnectionId, MacSecKeys[]; errors: base two only."}
  ListVirtualInterfaceRoutes: {wire: ok, errors: ok, state: partial, persist: n/a, note: "2026-08-05 (SDK v1.44.1, new op): in: VirtualInterfaceId (validated required at the backend, though the SDK's own client has no validation middleware for this op -- confirmed absent from validators.go), Filters*RouteFilters/MaxResults/NextToken (accepted on the wire, not used to filter anything -- see gaps); out: VirtualInterfaceId, Routes[]Route, NextToken; errors: base two only (DirectConnectClientException for an unknown VirtualInterfaceId). state=partial: existence of the virtual interface is real backend state; Routes is always an honest empty list -- see gaps, no BGP route exchange is modeled anywhere in this backend."}
  ListVirtualInterfaceTestHistory: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: BgpPeers[]string/MaxResults/NextToken/Status*string(free-form, not a typed enum)/TestId/VirtualInterfaceId (all optional filters); out: VirtualInterfaceTestHistory[]VirtualInterfaceTestHistory{BgpPeers[]string,EndTime,OwnerAccount,StartTime,Status,TestDurationInMinutes,TestId,VirtualInterfaceId}, NextToken; errors: base two only. This is the audit trail for StartBgpFailoverTest/StopBgpFailoverTest -- see the BGP-failover-test state machine notes below."}
  StartBgpFailoverTest: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: VirtualInterfaceId*, BgpPeers[]string (optional -- omit to test ALL peers on the VIF), TestDurationInMinutes*int32 (optional, presumably a default applies if omitted -- not specified in the SDK); out: VirtualInterfaceTestHistory (a new open test record); errors: base two only. Real, honestly-simulatable timer-driven state machine: VirtualInterfaceState -> 'testing' for the duration, selected BGP peers forced 'down', auto-reverts on timer expiry or explicit StopBgpFailoverTest -- see State machines section."}
  StopBgpFailoverTest: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: VirtualInterfaceId*; out: VirtualInterfaceTestHistory (the now-closed test record, EndTime populated); errors: base two only."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ResourceArn*, Tags*[]Tag; out: (empty); errors: +DuplicateTagKeysException/TooManyTagsException."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ResourceArn*, TagKeys*[]string; out: (empty); errors: base two only (no DuplicateTagKeysException/TooManyTagsException -- makes sense, nothing to duplicate-check when removing)."}
  UpdateConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: ConnectionId*, ConnectionName (optional rename), EncryptionMode*string (optional -- 'no_encrypt'/'should_encrypt'/'must_encrypt' per the Connection.EncryptionMode doc comment, NOT a typed enum in the Go SDK); out: full Connection; errors: base two only."}
  UpdateDirectConnectGateway: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: DirectConnectGatewayId*, NewDirectConnectGatewayName* (BOTH required -- the only Update* op in this service where the 'what to change' field is itself required, not optional-partial-update like every other Update*); out: DirectConnectGateway; errors: base two only."}
  UpdateDirectConnectGatewayAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: AddAllowedPrefixesToDirectConnectGateway[]/AssociationId/RemoveAllowedPrefixesToDirectConnectGateway[] -- all optional, no required fields at struct level at all; out: DirectConnectGatewayAssociation (state 'updating' while prefix changes propagate); errors: base two only."}
  UpdateLag: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: LagId*, EncryptionMode(optional *string), LagName(optional), MinimumLinks(optional int32 -- lets the caller RAISE the minimum-links requirement post-creation, a real operational lever tied directly to Lag.MinimumLinks/NumberOfConnections semantics); out: full Lag; errors: base two only."}
  UpdateVirtualInterfaceAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "in: VirtualInterfaceId*, EnableSiteLink(optional *bool), Mtu(optional *int32 -- 1500 or 8500 per doc comment, jumbo-frame toggle), RateLimit(optional *string), VirtualInterfaceName(optional); out: full nested VirtualInterface fields FLATTENED directly on Output (same flattened shape as CreatePrivateVirtualInterfaceOutput -- wire-trap #1); errors: base two only."}
# Families audited as a group (when per-op is impractical): none needed -- all 63 ops audited
# individually above; every op in this service is a fixed POST / with no path-parameter routing,
# so there is no natural "route family" grouping the way REST-JSON services have.
gaps:
  - "Connection.AwsDevice, Interconnect.AwsDevice, Lag.AwsDevice, and DirectConnectGatewayAssociation.VirtualGatewayRegion (2026-08-15, gopherstack-6flj): four members confirmed present in directconnect@v1.44.1's own deserializer key switches (awsAwsjson11_deserializeDocumentConnection/Interconnect/Lag/DirectConnectGatewayAssociation, each case \"awsDevice\"/\"virtualGatewayRegion\") but never modeled anywhere in this service (zero grep hits for AwsDevice/awsDevice or VirtualGatewayRegion/virtualGatewayRegion in any non-generated .go file before this pass) -- a real client reading these fields always gets nil/absent, never a wrong value. All four are marked `// Deprecated: This member has been deprecated.` in the pinned SDK's own types.go doc comments (AwsDevice superseded by AwsDeviceV2, which IS correctly populated everywhere; VirtualGatewayRegion has no live successor field documented). Buildable -- AwsDevice could plausibly mirror AwsDeviceV2's value (many AWS services keep a deprecated field populated identically to its replacement for backward compatibility) and VirtualGatewayRegion could plausibly derive from this backend's own region -- but neither is confirmed by any primary source available here (no live AWS response, no SDK comment stating the deprecated field still populates). Left disclosed rather than guessed at, per this file's own disclose-don't-fabricate precedent; a follow-up pass with access to a real AWS account's actual (deprecated-field-inclusive) response could resolve this with certainty."
  - "No AWS::DirectConnect::* CloudFormation resource type exists in this repo (grep -rli directconnect services/cloudformation/ returned zero hits, all 71 resources_*.go files checked). This is genuinely buildable (adding a CFN resource type is ordinary software work, not a physical/legal impossibility) but lives in services/cloudformation's ownership, not services/directconnect's -- out of scope for this pass, left for a CloudFormation-focused audit to pick up."
  - "Real services/secretsmanager integration for AssociateMacSecKey's raw-Cak/Ckn path (connections.go's synthesizeMacSecSecretARN synthesizes a plausible but unbacked ARN instead of creating a real secret): buildable -- this repo has a real services/secretsmanager backend and the EC2 cross-service pattern (store.go's EC2GatewayResolver, cli.go's wireDirectConnectEC2) this would mirror. Not done this pass: cli.go had a concurrent, in-flight edit from another agent working the same branch at the time of this audit, and stacking a second cross-service wiring change onto a shared, actively-changing file risked a lost or garbled merge. The synthesized-ARN simplification is documented, tested (sdk_roundtrip_test.go, test/integration/directconnect_test.go), and wire-correct; left for a follow-up pass once cli.go settles."
  - "AssociateConnectionWithLag's 'the connection must be hosted on the same Direct Connect endpoint as the LAG' clause (2026-09-06, gopherstack-55so): types.Connection and types.Lag both carry a distinct endpoint identity separate from Location -- AwsDeviceV2/AwsLogicalDeviceId (plus deprecated AwsDevice) -- confirmed in directconnect@v1.44.1's types/types.go doc comments ('The Direct Connect endpoint that terminates the physical/logical connection'), so this is NOT a case of the backend having no endpoint concept to guard on: models.go's Connection and Lag structs already carry AwsDeviceV2/AwsLogicalDeviceID fields. The reason it stays unenforced is different and narrower: no code path in this service ever assigns either field (grep confirms writes exist only in wire_convert.go's model-to-wire copy, never in CreateConnection/CreateLag/etc.), so both are always the empty string on every Connection and every Lag. A guard comparing two always-empty fields would pass vacuously on every call -- indistinguishable from no guard except for the false confidence of looking enforced -- which is worse than the honest 'not enforced' status quo. Enforcing this for real requires first deciding what populates AwsDeviceV2/AwsLogicalDeviceID (itself gaps: entry above, deferred pending a real AWS response), so this clause is blocked on that one, not independently invented."
structural_gaps:
  - "Interconnect/hosted-connection/reseller (partner) flow (CreateInterconnect, AllocateConnectionOnInterconnect, AllocateHostedConnection, ConfirmConnection, DescribeConnectionsOnInterconnect, DescribeHostedConnections): real Direct Connect Partners own physical cross-connect infrastructure at colocation facilities. There is no physical link for an emulator to have or lack -- 'is this physically cross-connected' cannot be made real by any amount of implementation effort. Full state bookkeeping (Interconnect/Connection creation, ordering->confirm->available transitions, parent/child relationships) IS implemented and IS the honest ceiling."
  - "LOA-CFA (Letter of Authorization - Connecting Facility Assignment) content (DescribeLoa/DescribeConnectionLoa/DescribeInterconnectLoa): a real LOA-CFA is an authentic AWS-issued document authorizing physical cross-connect work at a named colocation facility. No implementation can produce a genuine one without real physical infrastructure and a real issuing authority. loa.go's placeholderLoaContent (a minimal, well-formed PDF labeled 'PLACEHOLDER - NOT A REAL AUTHORIZATION') is the honest ceiling, never a fabricated real-looking document."
  - "DescribeLocations/DescribeRouterConfiguration (static_data.go's seedLocations/seedRouterTypes): AWS's true, currently-accurate Direct Connect colocation-facility and router-vendor/OS catalogs are proprietary, change over time, and are not distributed anywhere in the SDK -- an emulator cannot maintain a live-accurate copy. The small, explicitly-labeled seed lists already implemented are the honest ceiling, not a claim to be AWS's authoritative catalog."
  - "DescribeCustomerMetadata (CustomerAgreement/NniPartnerType): reflects real signed legal agreements and NNI partner-tier status between a specific customer and AWS/partners. No implementation can honestly derive agreement content that doesn't exist. The empty Agreements list + NniPartnerType 'nonPartner' default already implemented is the honest ceiling."
  - "MACsec traffic encryption (as opposed to key-association STATE, which IS implemented): requires physical port-level encryption hardware in real AWS. Simulating MacSecKeys/associating-associated-disassociating-disassociated state and EncryptionMode enforcement is honest bookkeeping; actually encrypting traffic is meaningless in an emulator with no traffic to encrypt."
  - "BGP peering / router-config / route-exchange realism, including ListVirtualInterfaceRoutes' always-empty Routes list (2026-08-05, SDK v1.44.1): real BGP session establishment and route exchange happen between AWS and the customer's own physical router over the physical link. bgp.go's BGPPeer records track configuration only (ASN, auth key, address family) and STATE transitions (BgpPeerState/BGPStatus, including StartBgpFailoverTest forcing peers down) -- both already implemented and the honest ceiling; no real routing protocol can run here, so ListVirtualInterfaceRoutes correctly validates the VIF exists and returns an honest empty list rather than fabricating CIDRs/AS-paths."
  - "Partner/reseller billing distinction (AllocateConnectionOnInterconnect/AllocateHostedConnection/AssociateHostedConnection/DescribeHostedConnections/DescribeConnectionsOnInterconnect): a hosted connection is billed differently from and owned separately by the interconnect owner in real AWS. No billing/settlement system exists anywhere in this repo to simulate that distinction meaningfully -- these ops are real state bookkeeping (who owns what, which state), already implemented, and should not claim to be more."
  - "AssociatedCoreNetwork (Cloud WAN core-network attachment on DirectConnectGatewayAssociation): no services/cloudwan or equivalent backend exists anywhere in this repo to resolve a core-network id against. The field correctly stays nil/unpopulated rather than fabricating a Cloud WAN integration that has nothing real to attach to."
deferred:
  - "Per-op AWS-published tag-count/rate-limiter quota numbers for TooManyTagsException/LimitExceededException: no such numbers exist in the SDK to derive; this pass uses a defensible, documented 50-tag cap (maxTagsPerResource, errors.go) and a real, derivable LAG-capacity trigger for LimitExceededException (see AssociateConnectionWithLag), but does not fabricate a VIF-rate-limiter quota number for the 6 Allocate*/Create*VirtualInterface ops' own LimitExceededException (wired and error-mapped correctly, just not reachable via a fabricated trigger)."
leaks: {status: clean, note: "Handler.Reset()/InMemoryBackend.Close() wiring confirmed: Close() stops the pkgs/worker.Group backing every scheduleTransition timer (connection/lag/interconnect/VIF/gateway/association/MacSecKey/BGPPeer state chains, plus StartBgpFailoverTest's duration timer); verified clean under go test -race, 3 consecutive runs, 0 races."}
---

## 2026-09-06 pass: AssociateConnectionWithLag re-association minimum-links (gopherstack-55so)

`api_op_AssociateConnectionWithLag.go`'s doc comment carries two rules beyond bandwidth-match and
LAG capacity, verified against both this backend and the SDK types before deciding which to
implement. Clause 1, same-Direct-Connect-endpoint: confirmed AWS models a distinct endpoint field
(AwsDeviceV2/AwsLogicalDeviceId) on both Connection and Lag, so this is not the "no endpoint
concept" situation -- but this backend never populates either field on any Connection or Lag (only
copies them wire-side), so a guard would compare two always-empty strings and pass vacuously on
every call. Left unenforced, now recorded precisely in `gaps:` rather than silently absent. Clause
2, re-association minimum-links: implemented in `connections.go`'s new
`reassociationMinimumLinksErrorLocked`, called from `AssociateConnectionWithLag` when a connection
moves away from a different LAG it already belongs to. Confirmed via the SDK doc text that, unlike
`DisassociateConnectionFromLag`, this rule carries no last-member exception -- re-associating a
LAG's last connection away still fails if that LAG's `MinimumLinks` is nonzero.
`TestAssociateConnectionWithLag_ReassociationMinimumLinks` covers the rejection, the exact-minimum
boundary (must succeed), and the no-last-member-exception asymmetry. No pre-existing test exercised
a re-association that this new guard would reject.

## 2026-08-06 pass: integration suite + gap re-judgment

`test/integration/directconnect_test.go` added (four `TestIntegration_DirectConnect_*` funcs, real
`aws-sdk-go-v2/service/directconnect` client against a running Docker container, per
`.claude/memories/parity-principles.md` rule 3 -- `sdk_roundtrip_test.go`'s in-process client tests
do not satisfy that rule, only a container-driven suite does). Covers connection/LAG lifecycle
including the LAG-capacity `LimitExceededException`; private/public/transit VIF creation, VLAN-
uniqueness rejection, BGP peer create/delete, and the cross-account Allocate*/Confirm* flow;
`DirectConnectGateway` creation and tagging on its GLOBAL ARN; and, the highest-value case, gateway
association/proposal against a REAL `services/ec2` `VpnGateway`/`TransitGateway` created via the EC2
SDK in the same test, proving `store.go`'s `EC2GatewayResolver` cross-service validation actually
runs end-to-end (both accepting the real id and rejecting a fabricated one) rather than only being
exercised by isolated unit tests with no resolver wired. `go test -race -run
TestIntegration_DirectConnect ./test/integration/...` passes.

Every `gaps:` entry was re-read against current code, not re-derived from scratch: the EC2 cross-
service validation this document previously described only as a "clear, concrete, low-risk
improvement... not required for a first-pass implementation" turned out to already be implemented
(`store.go`, `virtualinterfaces.go`'s `resolveGatewayBindingLocked`, `cli.go`'s
`wireDirectConnectEC2`) and untested by any SDK-driven suite -- now it is both. Gaps whose
underlying data source cannot exist in an emulator by any amount of implementation effort (physical
cross-connect state, real LOA-CFA authorization, AWS's proprietary location/router catalogs, real
customer legal agreements, physical MACsec hardware, real BGP sessions, partner billing, Cloud WAN
with no backing service) moved to `structural_gaps:`. The two gaps left in `gaps:` are real but out
of this pass's reach for reasons that are not "too hard": CloudFormation resource types belong to
services/cloudformation's ownership, not this directory, and secretsmanager-backed MACsec keys were
deliberately not attempted because cli.go had a concurrent in-flight edit from another agent on this
branch at audit time.

## Implementation summary (previous pass)

All 63 operations implemented: routed via a flat `X-Amz-Target: OvertureService.<Op>` dispatch
table (handler.go's `opTable()`, merged from six per-family `handler_*.go` files), backed by real
`InMemoryBackend` state (`pkgs/store.Table`/`Index` per resource kind, one coarse
`lockmetrics.RWMutex`), and persisted via `Snapshot`/`Restore` (persistence.go). `sdk_completeness_test.go`
passes with an empty exception list. A real `aws-sdk-go-v2/service/directconnect` client round-trips
against every major flow (`sdk_roundtrip_test.go`) — this caught the wire-shape assumptions below
before they shipped, not after.

**Corrected during implementation** (both documented inline at the relevant `ops:`/wire-trap-7 entries
above): `DescribeConnectionLoa` and `DescribeInterconnectLoa` are each independently marked
`Deprecated: This operation has been deprecated` / "Use DescribeLoa instead" in their own SDK doc
comments — `DescribeLoa` (the flattened, older-looking shape) is actually the CURRENT/preferred op,
the reverse of this audit's first-pass guess from shape alone.

**Judgment calls, each documented at its call site:**
- `AssociatedGatewayOwnerAccount` on `AcceptDirectConnectGatewayAssociationProposal` is treated as the
  accepter's confirmation of who owns the associated VGW/TGW (matching the proposal's own
  `GatewayOwnerAccount`), not the accepter's own identity — a defensible reading of the field name,
  explicitly NOT verified against real AWS behavior (gateway_associations.go).
- `pkgs/arn` gained one additive function, `BuildGlobal(service, region, accountID, resource string) string`,
  for `DirectConnectGateway`'s GLOBAL ARN — a resource-kind-level exception, not a new
  service-level special-case like `Build`'s existing `service=="iam"` branch. `Build`'s existing
  behavior and every other caller are untouched; `BuildGlobal` has its own unit tests
  (pkgs/arn/arn_test.go).
- Real cross-service EC2 binding: `SetEC2GatewayResolver` (store.go) lets
  `CreateDirectConnectGatewayAssociation`/`CreateDirectConnectGatewayAssociationProposal` validate a
  `GatewayId`/`VirtualGatewayId` against real (mock) EC2 `VpnGateway`/`TransitGateway` records, and
  `DescribeVirtualGateways` proxies EC2's own VpnGateway list instead of a duplicate store — wired in
  cli.go's `wireDirectConnectEC2`. Falls back to services/ec2's own `vgw-`/`tgw-` ID-prefix convention
  when no resolver is wired (e.g. isolated unit tests), never fabricating an unconditionally-accepted id.
- Pagination: despite PARITY.md's wire-trap #4 finding no generated `Paginator` type anywhere in this
  SDK module, all ten MaxResults/NextToken-bearing Describe/List ops now genuinely paginate via
  `pkgs/page` (handler.go's `paginate` helper) rather than silently ignoring the caller's MaxResults —
  the option flagged as preferred over "assume it doesn't matter at emulator scale".
- ID formats: `dxcon-`/`dxlag-`/`dxvif-` + 8 hex chars (matching AWS's own published API examples,
  including reusing `dxcon-` for Interconnect, UNCONFIRMED but the more defensible analogy than
  inventing an unevidenced prefix); UUIDs for `DirectConnectGatewayId`/`AssociationId`/`ProposalId`/
  `TestId` (also matching AWS's own published examples); `bgp-` + 8 hex for `BgpPeerId` (UNCONFIRMED,
  no evidence either way).
- LOA-CFA content: a minimal, well-formed placeholder PDF with correct xref offsets computed at
  build time (loa.go's `placeholderLoaContent`), clearly labeled "PLACEHOLDER - NOT A REAL
  AUTHORIZATION" in its own page text — never a fabricated "real-looking" document.
- `DescribeLocations`/`DescribeRouterConfiguration`/`DescribeCustomerMetadata`: small, explicitly
  non-authoritative static seed tables / an always-empty/nonPartner default, per PARITY.md's
  honest-gap guidance — never presented as AWS's real published catalogs or a real agreement
  workflow.

## Purpose of this document

`services/directconnect/` is now fully implemented (overall: A); the section below is kept as-written
from the original 2026-08-01 pre-implementation audit for its wire-shape research value -- a complete
SDK operation inventory plus a behavioral spec, so a re-audit does not have to re-derive wire shapes
from the SDK source itself. All 63 operation names, the wire protocol, every operation's exact per-op
exception set, and
every shared type/enum below were read directly from
`aws-sdk-go-v2/service/directconnect@v1.43.3`'s `serializers.go` / `deserializers.go` /
`types/types.go` / `types/enums.go` / `types/errors.go` in the module cache (resolved via a
throwaway `go mod init probe && go get .../directconnect@latest` in the scratch dir — **not**
added to this repo's `go.mod`, which another agent was concurrently editing during this pass).

## 1. Complete SDK operation inventory

**63 operations**, SDK version **`v1.43.3`** (resolved 2026-08-01, whatever `@latest` currently
resolves to — not a version pinned by this audit). This matches the ~63 estimate exactly:

`ls api_op_*.go | grep -v _test.go | wc -l` against
`/home/agbishop/go/pkg/mod/github.com/aws/aws-sdk-go-v2/service/directconnect@v1.43.3/` returns
**63**.

Alphabetically: AcceptDirectConnectGatewayAssociationProposal, AllocateConnectionOnInterconnect,
AllocateHostedConnection, AllocatePrivateVirtualInterface, AllocatePublicVirtualInterface,
AllocateTransitVirtualInterface, AssociateConnectionWithLag, AssociateHostedConnection,
AssociateMacSecKey, AssociateVirtualInterface, ConfirmConnection, ConfirmCustomerAgreement,
ConfirmPrivateVirtualInterface, ConfirmPublicVirtualInterface, ConfirmTransitVirtualInterface,
CreateBGPPeer, CreateConnection, CreateDirectConnectGateway,
CreateDirectConnectGatewayAssociation, CreateDirectConnectGatewayAssociationProposal,
CreateInterconnect, CreateLag, CreatePrivateVirtualInterface, CreatePublicVirtualInterface,
CreateTransitVirtualInterface, DeleteBGPPeer, DeleteConnection, DeleteDirectConnectGateway,
DeleteDirectConnectGatewayAssociation, DeleteDirectConnectGatewayAssociationProposal,
DeleteInterconnect, DeleteLag, DeleteVirtualInterface, DescribeConnectionLoa,
DescribeConnections, DescribeConnectionsOnInterconnect, DescribeCustomerMetadata,
DescribeDirectConnectGatewayAssociationProposals, DescribeDirectConnectGatewayAssociations,
DescribeDirectConnectGatewayAttachments, DescribeDirectConnectGateways,
DescribeHostedConnections, DescribeInterconnectLoa, DescribeInterconnects, DescribeLags,
DescribeLoa, DescribeLocations, DescribeRouterConfiguration, DescribeTags,
DescribeVirtualGateways, DescribeVirtualInterfaces, DisassociateConnectionFromLag,
DisassociateMacSecKey, ListVirtualInterfaceTestHistory, StartBgpFailoverTest,
StopBgpFailoverTest, TagResource, UntagResource, UpdateConnection, UpdateDirectConnectGateway,
UpdateDirectConnectGatewayAssociation, UpdateLag, UpdateVirtualInterfaceAttributes.

### Protocol and routing shape — NOT REST, confirmed

Protocol is **`awsjson1.1`** (`awsAwsjson11_serializeOp<Op>` struct names throughout
`serializers.go`), confirmed empirically, not assumed from the task brief's guess. Every single
operation serializes to:

- **Method: `POST`**, **Path: `/`** (literally `operationPath := "/"` in every serializer — there
  are zero path parameters and zero `httpbinding.SplitURI` calls anywhere in `serializers.go`,
  confirmed by grep: `grep -n "SplitURI" serializers.go` returns nothing).
- **`Content-Type: application/x-amz-json-1.1`**
- **`X-Amz-Target: OvertureService.<OperationName>`** — every one of the 63 ops carries this exact
  header value with the literal internal service name `OvertureService` (Direct Connect's
  internal AWS codename), confirmed via `grep -c 'X-Amz-Target' serializers.go` => 63, one per op,
  spot-checked several by name (e.g. `OvertureService.CreateConnection`,
  `OvertureService.AcceptDirectConnectGatewayAssociationProposal`).

This means there is **no URL-based routing at all** for this service — a gopherstack handler must
dispatch purely on the `X-Amz-Target` header (or equivalently, on the JSON-RPC-style operation
name most awsjson1.x-protocol services in this repo already parse from that header), the same
shape as any other `awsjson1.0`/`awsjson1.1` service already in this tree (e.g. DynamoDB,
StepFunctions) — not like a REST-JSON service (e.g. outposts, resiliencehub) where the path itself
disambiguates the operation.

### Errors — only 5 shared exception shapes, unusually thin per-op subsets

Exactly 5 modeled exception shapes, all in `types/errors.go`, **every one a bare `Message *string`
+ `ErrorCodeOverride` field — none of the richer `ResourceId`/`ResourceType`-carrying shapes seen
in outposts (`ConflictException`) or resiliencehub (`ConflictException`/`ResourceNotFoundException`)**:

- `DirectConnectClientException` (client fault, HTTP 4xx) — the catch-all for essentially every
  bad-input, not-found, and conflict condition in this API. **There is no dedicated
  `ResourceNotFoundException` or `ValidationException` shape anywhere in this service** — an
  implementer mapping "connection not found" or "malformed VLAN" must both surface as this one
  generic client exception, not a distinct typed 404-style shape. This is a materially thinner
  error model than every other service audited in this campaign so far.
- `DirectConnectServerException` (server fault, HTTP 5xx) — generic internal error.
- `DuplicateTagKeysException` (client fault) — "A tag key was specified more than once."
- `LimitExceededException` (client fault) — doc comment specifically calls out *rate limiters on
  virtual interfaces*, not a generic service-quota message.
- `TooManyTagsException` (client fault) — "You have reached the limit on the number of tags that
  can be assigned."

**Per-op sets, extracted from every op's own `awsAwsjson11_deserializeOpError<Op>` switch in
`deserializers.go` (all 63 read individually, not sampled):**

| Set | Ops |
|---|---|
| Base two only (`DirectConnectClientException`, `DirectConnectServerException`) — 51 ops | AcceptDirectConnectGatewayAssociationProposal, AllocateConnectionOnInterconnect, AssociateHostedConnection, AssociateMacSecKey, AssociateVirtualInterface, ConfirmConnection, ConfirmCustomerAgreement, ConfirmPrivateVirtualInterface, ConfirmPublicVirtualInterface, ConfirmTransitVirtualInterface, CreateBGPPeer, CreateDirectConnectGateway, CreateDirectConnectGatewayAssociation, CreateDirectConnectGatewayAssociationProposal, DeleteBGPPeer, DeleteConnection, DeleteDirectConnectGateway, DeleteDirectConnectGatewayAssociation, DeleteDirectConnectGatewayAssociationProposal, DeleteInterconnect, DeleteLag, DeleteVirtualInterface, DescribeConnectionLoa, DescribeConnections, DescribeConnectionsOnInterconnect, DescribeCustomerMetadata, DescribeDirectConnectGatewayAssociationProposals, DescribeDirectConnectGatewayAssociations, DescribeDirectConnectGatewayAttachments, DescribeDirectConnectGateways, DescribeHostedConnections, DescribeInterconnectLoa, DescribeInterconnects, DescribeLags, DescribeLoa, DescribeLocations, DescribeRouterConfiguration, DescribeTags, DescribeVirtualGateways, DescribeVirtualInterfaces, DisassociateConnectionFromLag, DisassociateMacSecKey, ListVirtualInterfaceTestHistory, StartBgpFailoverTest, StopBgpFailoverTest, UntagResource, UpdateConnection, UpdateDirectConnectGateway, UpdateDirectConnectGatewayAssociation, UpdateLag, UpdateVirtualInterfaceAttributes |
| + `LimitExceededException` only (no tag exceptions) — 1 op | AssociateConnectionWithLag |
| + `DuplicateTagKeysException`/`TooManyTagsException` only (no `LimitExceededException`) — 5 ops | AllocateHostedConnection, CreateConnection, CreateInterconnect, CreateLag, TagResource |
| + all three (`DuplicateTagKeysException`/`LimitExceededException`/`TooManyTagsException`) — 6 ops | AllocatePrivateVirtualInterface, AllocatePublicVirtualInterface, AllocateTransitVirtualInterface, CreatePrivateVirtualInterface, CreatePublicVirtualInterface, CreateTransitVirtualInterface |

That table is 63 ops total (51 + 1 + 5 + 6). This pattern is genuinely useful for an implementer
building one shared "which extra exceptions can this op return" lookup rather than hand-writing
63 switch statements: it tracks exactly with
"does this op accept a `Tags` field in its input" (tag exceptions) and "does this op create/modify
a *virtual interface specifically*, where AWS's real rate-limiter-per-VIF quota applies"
(`LimitExceededException` — note `AssociateConnectionWithLag` is the one exception, gaining
`LimitExceededException` despite operating on a Connection/LAG pair with no VIF involved at all,
likely reflecting a bandwidth/link-count limit rather than the rate-limiter doc comment's literal
text).

### Shared/core types (from `types/types.go` and `types/enums.go`)

- **`Connection`**: `ConnectionId`, `ConnectionName`, `ConnectionState`(`ConnectionState`, 9
  values — see State machines), `Bandwidth`, `Location`, `Region`, `LagId`, `OwnerAccount`,
  `ProviderName`/`PartnerName`, `Vlan int32`, `AwsDeviceV2`/`AwsLogicalDeviceId`
  (`AwsDevice` deprecated), `JumboFrameCapable *bool`, `HasLogicalRedundancy`(`HasLogicalRedundancy`:
  unknown/yes/no), `EncryptionMode *string` (no_encrypt/should_encrypt/must_encrypt, **not a typed
  enum**), `MacSecCapable *bool`, `MacSecKeys []MacSecKey`,
  `PartnerInterconnectMacSecCapable *bool`, `PortEncryptionStatus *string` (Encryption
  Up/Encryption Down, **not a typed enum**), `RateLimiterStatus *RateLimiterStatus`,
  `LoaIssueTime *time.Time`, `Tags []Tag`.
- **`Lag`**: `LagId`, `LagName`, `LagState`(`LagState`, 7 values — no `ordering`, unlike
  `ConnectionState`), `AllowsHostedConnections bool`, `Connections []Connection` (member
  connections nested directly), `ConnectionsBandwidth *string`, `MinimumLinks int32`,
  `NumberOfConnections int32`, `Location`, `OwnerAccount`, `ProviderName`,
  `HasLogicalRedundancy`, `EncryptionMode *string`, `MacSecCapable *bool`/`MacSecKeys`,
  `RateLimiterStatus`, `Tags`.
- **`Interconnect`**: near-identical shape to `Connection` minus `ConnectionId`/`ConnectionName`/
  `LagId`(has its own)/`Vlan` — `InterconnectId`, `InterconnectName`,
  `InterconnectState`(`InterconnectState`, 7 values, same shape as `LagState` — no `ordering`).
- **`VirtualInterface`**: `VirtualInterfaceId`, `VirtualInterfaceName`, `VirtualInterfaceType
  *string` (`"private"`|`"public"`|`"transit"` per doc comment — **not a typed enum**),
  `VirtualInterfaceState`(`VirtualInterfaceState`, 10 values, the richest state enum in the
  service — see State machines), `ConnectionId`, `Vlan int32`, `AddressFamily`(`AddressFamily`:
  ipv4/ipv6), `AmazonAddress`/`CustomerAddress`, `Asn int32`/`AsnLong *int64` (dual legacy/modern
  ASN fields — see wire-trap #5), `AuthKey *string` (BGP MD5 auth key), `AmazonSideAsn *int64`,
  `BgpPeers []BGPPeer`, `CustomerRouterConfig *string`, `DirectConnectGatewayId`/
  `VirtualGatewayId` (mutually-relevant, private/transit only), `Mtu *int32` (1500|8500),
  `JumboFrameCapable *bool`, `RateLimit *string` (a huge closed set of bandwidth-string values from
  `50Mbps` to `1.6Tbps`, enumerated in the doc comment, **not a typed enum**),
  `RouteFilterPrefixes []RouteFilterPrefix` (public VIFs only), `SiteLinkEnabled *bool`, `Tags`.
- **`BGPPeer`**: `BgpPeerId`, `BgpPeerState`(`BGPPeerState`: verifying/pending/available/deleting/
  deleted — 5 values, the peer's *lifecycle*), `BgpStatus`(`BGPStatus`: up/down/unknown — 3
  values, the peer's *operational* up/down state, an entirely separate axis from
  `BgpPeerState`), `Asn`/`AsnLong`, `AuthKey`, `AmazonAddress`/`CustomerAddress`, `AddressFamily`,
  `AwsDeviceV2`/`AwsLogicalDeviceId`.
- **`DirectConnectGateway`**: `DirectConnectGatewayId`, `DirectConnectGatewayName`,
  `DirectConnectGatewayState`(`DirectConnectGatewayState`: pending/available/deleting/deleted — 4
  values), `AmazonSideAsn *int64`, `OwnerAccount`, `StateChangeError *string` (present on
  DirectConnectGateway/DirectConnectGatewayAssociation/DirectConnectGatewayAttachment alike — a
  shared "why did the async transition fail" field), `Tags`.
- **`DirectConnectGatewayAssociation`**: `AssociationId`, `AssociationState`
  (`DirectConnectGatewayAssociationState`: associating/associated/disassociating/disassociated/
  updating — 5 values, the only state enum in this service with an `updating` mid-lifecycle
  value, used when `AllowedPrefixesToDirectConnectGateway` changes post-creation),
  `DirectConnectGatewayId`/`DirectConnectGatewayOwnerAccount`, `AssociatedGateway
  *AssociatedGateway`{`Id`,`OwnerAccount`,`Region`,`Type`(`GatewayType`:
  virtualPrivateGateway|transitGateway)}, `AssociatedCoreNetwork *AssociatedCoreNetwork`{
  `AttachmentId`,`Id`,`OwnerAccount`} (Cloud WAN core-network integration — **no backing service
  exists in this tree for Cloud WAN at all**, an honest unresolvable-reference gap),
  `AllowedPrefixesToDirectConnectGateway []RouteFilterPrefix`, `VirtualGatewayId`/
  `VirtualGatewayOwnerAccount` (the legacy pre-`AssociatedGateway` fields, still present and
  populated in parallel per the doc comments — an implementer must keep both in sync, not just
  pick one).
- **`DirectConnectGatewayAssociationProposal`**: `ProposalId`, `ProposalState`
  (`DirectConnectGatewayAssociationProposalState`: requested/accepted/deleted — only 3 values, no
  "rejected" — declining a proposal is presumably just never accepting it plus the proposer
  deleting it, not a distinct terminal state), `DirectConnectGatewayId`/
  `DirectConnectGatewayOwnerAccount`, `AssociatedGateway`,
  `Existing/RequestedAllowedPrefixesToDirectConnectGateway`.
- **`DirectConnectGatewayAttachment`**: `AttachmentState`(`DirectConnectGatewayAttachmentState`:
  attaching/attached/detaching/detached — 4 values), `AttachmentType`
  (`DirectConnectGatewayAttachmentType`: **only** `TransitVirtualInterface`/
  `PrivateVirtualInterface` — no `PublicVirtualInterface` value exists, structurally confirming
  public VIFs never attach to a Direct Connect gateway), `DirectConnectGatewayId`,
  `VirtualInterfaceId`/`VirtualInterfaceOwnerAccount`/`VirtualInterfaceRegion`,
  `StateChangeError`.
- **`VirtualGateway`**: just `VirtualGatewayId` + `VirtualGatewayState *string` (**bare string,
  not a typed enum** — the doc comment lists pending/available/deleting/deleted as prose only;
  this is the one *State field in the whole service that isn't a generated enum type, an
  inconsistency worth flagging since every sibling *State field elsewhere in this module IS
  typed).
- **`MacSecKey`**: `Ckn`, `SecretARN`, `StartOn *string`, `State *string` (associating/
  associated/disassociating/disassociated per doc comment — **again a bare string, not typed**).
- **`RateLimiterStatus`**: `InUse int32`, `MaxAllowed int32`, `Remaining int32`,
  `TotalBandwidth *string`.
- **`RouteFilterPrefix`**: single field, `Cidr *string`.
- **`RouterType`**: `Platform`, `RouterTypeIdentifier`, `Software`, `Vendor`, `XsltTemplateName`,
  `XsltTemplateNameForMacSec`.
- **`Loa`**: `LoaContent []byte`, `LoaContentType`(`LoaContentType`: only one value,
  `application/pdf`).
- **`Location`**: `LocationCode`, `LocationName`, `Region`, `AvailableMacSecPortSpeeds[]string`,
  `AvailablePortSpeeds[]string`, `AvailableProviders[]string`.
- **`CustomerAgreement`**: `AgreementName`, `Status *string` ("signed"/"unsigned" per doc
  comment, not typed).
- **`VirtualInterfaceTestHistory`**: `TestId`, `VirtualInterfaceId`, `BgpPeers[]string`,
  `Status *string` (not typed), `StartTime`/`EndTime`, `TestDurationInMinutes *int32`,
  `OwnerAccount`.
- **`Tag`**: `Key*` (required), `Value` (optional — unusual, most AWS tag shapes require both).
- **`ResourceTag`**: `ResourceArn`, `Tags []Tag` — the batch-shaped element of `DescribeTags`'
  output.
- **The four `New*VirtualInterface(Allocation)` input-only shapes** (`NewPrivateVirtualInterface`,
  `NewPrivateVirtualInterfaceAllocation`, `NewPublicVirtualInterface`,
  `NewPublicVirtualInterfaceAllocation`, `NewTransitVirtualInterface`,
  `NewTransitVirtualInterfaceAllocation`) all carry near-identical `AddressFamily`/`AmazonAddress`/
  `Asn`/`AsnLong`/`AuthKey`/`CustomerAddress`/`Mtu`/`RateLimit`/`Tags` fields, differing only in
  which of `DirectConnectGatewayId`/`VirtualGatewayId`/`EnableSiteLink`/`RouteFilterPrefixes` each
  variant includes (private/transit get gateway fields, public gets `RouteFilterPrefixes`
  instead, transit's *Allocation* variant is the one place `VirtualInterfaceName`/`Vlan` aren't
  marked required at the Go-struct level).

### Wire-shape traps worth flagging up front (looks-wrong-but-correct, or just easy to miss)

1. **Create/Allocate *VirtualInterface output shapes are inconsistent between private/public vs.
   transit.** `CreatePrivateVirtualInterfaceOutput`, `CreatePublicVirtualInterfaceOutput`,
   `AllocatePrivateVirtualInterfaceOutput`, `AllocatePublicVirtualInterfaceOutput`,
   `UpdateVirtualInterfaceAttributesOutput`, and `AssociateVirtualInterfaceOutput` all **flatten
   every `VirtualInterface` field directly onto the Output struct** (confirmed per-op by reading
   each `api_op_*.go` file directly: every one of these six literally repeats `AddressFamily`,
   `AmazonAddress`, ... as its own top-level fields, not a nested struct). By contrast,
   `CreateTransitVirtualInterfaceOutput`, `AllocateTransitVirtualInterfaceOutput`, and
   `CreateBGPPeerOutput`/`DeleteBGPPeerOutput` instead wrap the whole thing as a single nested
   `VirtualInterface *types.VirtualInterface` field. A generic "serialize a VirtualInterface as
   this op's response" helper needs an explicit flattened-vs-nested branch keyed on the specific
   operation, not a single shared serializer.
2. **The error model has no `ResourceNotFoundException` or `ValidationException` at all** — every
   client-fault condition in this entire 63-op service folds into the one generic
   `DirectConnectClientException`. An implementer must not invent a typed not-found/validation
   exception that doesn't exist in the real wire protocol; gopherstack's internal error-code
   mapping for this service needs its own `errCodeLookup`-style table pointing everything at
   `DirectConnectClientException`/`DirectConnectServerException` (plus the three
   tag/limit-specific ones only where the per-op table above says they're modeled).
3. **`CreateDirectConnectGatewayAssociation`/`DeleteDirectConnectGatewayAssociation` have TWO
   overlapping addressing schemes and neither is marked required.** `CreateDirectConnectGatewayAssociation`
   accepts `GatewayId` (new, generic — either a VGW or TGW id) OR `VirtualGatewayId` (older,
   VGW-only), both `*string` and both optional in the Go struct. `DeleteDirectConnectGatewayAssociation`
   similarly accepts `AssociationId` alone OR the `(DirectConnectGatewayId, VirtualGatewayId)`
   pair, again all three fields optional. A router built by pattern-matching one variant and
   extrapolating to the other will handle real caller traffic incorrectly — check the actual
   fields present on each individual request instead of assuming one canonical addressing mode.
4. **No `Paginator` types are generated anywhere in this SDK module**, despite `MaxResults`/
   `NextToken` fields existing on the wire for `DescribeConnections`, `DescribeHostedConnections`,
   `DescribeInterconnects`, `DescribeLags`, `DescribeVirtualInterfaces`,
   `DescribeDirectConnectGateways`, `DescribeDirectConnectGatewayAssociations`,
   `DescribeDirectConnectGatewayAssociationProposals`, `DescribeDirectConnectGatewayAttachments`,
   `ListVirtualInterfaceTestHistory` (confirmed: no `pagination*.go` file exists in the module
   cache directory at all). `DescribeConnectionsOnInterconnect` is a further asymmetry — its
   Output carries a `NextToken` field but its Input has none at all, so no caller could ever
   supply a continuation token even if the server wanted to paginate it. `DescribeVirtualGateways`,
   `DescribeCustomerMetadata`, `DescribeLocations` have no pagination fields on either side at
   all — these always return everything in one call. Use `pkgs/page` for whichever ops the
   implementer decides to genuinely paginate, but do not assume AWS's real service paginates
   every one of these the same way; the SDK gives no evidence either way beyond field presence.
5. **Dual legacy/modern ASN fields (`Asn int32` + `AsnLong *int64`) appear on `BGPPeer`,
   `VirtualInterface`, `NewBGPPeer`, and all six `New*VirtualInterface(Allocation)` shapes.** Per
   the SDK's own repeated doc comment: use one or the other, never both; if a 4-byte ASN is
   supplied via `AsnLong`, the API response reports `0` for the legacy `Asn` field (since it
   exceeds `int32`'s useful range for this purpose); 2-byte ASNs populate both fields identically
   in responses. An implementer must replicate this exact zero-out/populate-both behavior, not
   just store one value and mirror it blindly into both fields.
6. **Several `*State` fields that intuitively look like they should be typed enums are actually
   bare `*string`**: `VirtualGateway.VirtualGatewayState`, `MacSecKey.State`,
   `CustomerAgreement.Status`, `VirtualInterfaceTestHistory.Status`,
   `Connection.EncryptionMode`/`PortEncryptionStatus`. Every one of these has a prose-only doc
   comment listing the possible values rather than a generated `type XState string` block. Do not
   assume type-safety exists for these fields just because sibling fields elsewhere in the same
   struct are typed enums — verify per-field.
7. **`DescribeLoa` (flattened shape, and — CORRECTED during implementation — the CURRENT/preferred
   op) vs. `DescribeConnectionLoa`/`DescribeInterconnectLoa` (nested shape, and each independently
   marked `Deprecated: This operation has been deprecated` / "Use DescribeLoa instead" in their own
   `api_op_*.go` doc comments)**: `DescribeLoa`'s output flattens `LoaContent []byte` +
   `LoaContentType` directly onto the Output struct, while the other two nest the identical two
   fields inside a `Loa *types.Loa` struct. Same flattened-vs-nested trap class as #1, on a
   completely separate part of the API. This audit's *first* pass guessed the deprecation direction
   from shape alone ("the flattened one looks older, so it must be the deprecated one") and got it
   backwards — always check the actual doc comment, never infer deprecation from shape.
8. **`AssociateConnectionWithLag` is the sole op where `LimitExceededException` appears without
   either tag exception**, despite taking no `Tags` input at all — its limit is presumably
   Lag-capacity/bandwidth-related, not the rate-limiter-on-a-VIF condition the exception's own
   doc comment describes. Do not assume `LimitExceededException`'s doc-commented meaning
   ("rate limiter... on virtual interfaces") applies uniformly everywhere it's modeled.

## 2. Missing simulated functionality (the real emulation work)

Direct Connect is fundamentally a **physical cross-connect / router-configuration** product. The
CRUD shell (Connection/Lag/Interconnect/VirtualInterface/DirectConnectGateway records with
correct states and relationships) is honestly, fully simulatable; a handful of specific fields
depend on hardware or legal/billing relationships this emulator cannot and should not fabricate.

### Connection / LAG / Interconnect lifecycle

Real enum values, all confirmed in `types/enums.go` (not guesses):

- **`ConnectionState`**: `ordering` → `requested` → `pending` → `available` → (`down` |
  `deleting` → `deleted` | `rejected`), plus `unknown`. Per the enum's own doc comments:
  `ordering` is specific to a **hosted** connection provisioned on an interconnect, staying there
  until the hosting owner confirms/declines via `ConfirmConnection`; `requested` is the initial
  state of a **standard** (non-hosted) connection created via `CreateConnection`, staying there
  until an LOA is issued; `rejected` is reached specifically when a hosted connection in
  `ordering` is deleted by the *customer* (as opposed to normal `deleting`/`deleted` when the
  owner tears it down).
- **`LagState`**/**`InterconnectState`**: both 7-value enums, identical shape
  (`requested`→`pending`→`available`→(`down`|`deleting`→`deleted`), `unknown`) — **neither has an
  `ordering` state**, because LAGs and Interconnects are never "hosted" sub-resources the way
  Connections can be; they're always the top-level physical resource.
- **Real invariants to enforce, not decorative**: `Lag.MinimumLinks` (the LAG becomes
  operationally down if fewer than this many member connections are `available`) and
  `Lag.NumberOfConnections`'s doc-commented cap ("maximum of four connections when the port speed
  is 1 Gbps or 10 Gbps, or two when... 100 Gbps or 400 Gbps") are real, checkable business rules —
  `CreateLag`, `AssociateConnectionWithLag`, `DisassociateConnectionFromLag` should all enforce
  them, not silently accept any combination.

### Virtual interfaces — private / public / transit

- **`VirtualInterfaceState`** (10 values, the richest enum in the service): `confirming` →
  `verifying` (public-only, per doc comment) → `pending` → `available` → (`down` | `testing`
  (only via `StartBgpFailoverTest`) | `deleting` → `deleted` | `rejected`), `unknown`.
  `confirming` specifically applies when the VIF owner differs from the connection owner (the
  Allocate*/Confirm* cross-account flow) — a VIF created via `AllocatePrivateVirtualInterface`
  starts in `confirming` until the named `OwnerAccount` calls `ConfirmPrivateVirtualInterface`.
- **VLAN allocation and conflicts**: `Vlan int32` is required on every `New*VirtualInterface`
  variant (except, oddly, `NewTransitVirtualInterface`/`NewTransitVirtualInterfaceAllocation`,
  where it's optional at the struct level — see wire notes). Real AWS enforces VLAN uniqueness
  **per physical connection/LAG** (you cannot double-allocate VLAN 100 twice on the same
  Connection) — this is a genuine, checkable invariant with no dedicated typed exception to
  signal it (would surface as the generic `DirectConnectClientException`).
- **Address family / BGP peering**: `AddressFamily` (ipv4/ipv6) plus `AmazonAddress`/
  `CustomerAddress`/`Asn`/`AsnLong`/`AuthKey` collectively describe one BGP session's addressing
  and MD5 auth. A VIF can carry multiple `BGPPeer`s (`HasLogicalRedundancy` signals whether a
  *secondary* peer in the same address family is supported) — `CreateBGPPeer`/`DeleteBGPPeer`
  manage that list directly on the parent `VirtualInterface.BgpPeers[]`.
- **ASN/auth-key handling**: see wire-trap #5 above — the dual `Asn`/`AsnLong` zero-out behavior
  on 4-byte ASN input is a real, specific rule to replicate, not just "store what's given."
- **Private vs. public vs. transit differences, all structurally confirmed**:
  - *Private*: binds to exactly one of `DirectConnectGatewayId`/`VirtualGatewayId`; has
    `EnableSiteLink`; no `RouteFilterPrefixes`.
  - *Public*: has `RouteFilterPrefixes[]RouteFilterPrefix` (BGP-advertised CIDR routes); **no**
    `DirectConnectGatewayId`/`VirtualGatewayId`/`EnableSiteLink` field exists on either
    `NewPublicVirtualInterface` or `NewPublicVirtualInterfaceAllocation` at all — public VIFs
    structurally cannot attach to a gateway (independently confirmed by
    `DirectConnectGatewayAttachmentType` having no `PublicVirtualInterface` enum value either).
  - *Transit*: binds to `DirectConnectGatewayId` (required at `ConfirmTransitVirtualInterface`
    time, merely optional at create time — a real create-vs-confirm asymmetry); has
    `EnableSiteLink`; no `RouteFilterPrefixes`.

### Direct Connect Gateways, associations, and the cross-account proposal flow

- **`DirectConnectGateway`** is a **global, non-regional** resource conceptually (per its own
  ARN being a `GlobalARN` per Terraform's provider source — see ARN section below) that different
  Direct Connect connections in different regions can all attach virtual interfaces to.
  `DirectConnectGatewayState`: `pending`→`available`, `deleting`→`deleted` (4 values, no `down`/
  `unknown` — a gateway is either up or gone, no degraded state modeled).
- **Association (same-account) vs. Association Proposal (cross-account)**: when the DCGW and the
  VGW/TGW being attached are owned by the **same** account, `CreateDirectConnectGatewayAssociation`
  is used directly. When they're in **different** accounts, the flow is: the VGW/TGW owner calls
  `CreateDirectConnectGatewayAssociationProposal` (supplying `DirectConnectGatewayOwnerAccount` —
  the *other* party) → the DCGW owner calls
  `AcceptDirectConnectGatewayAssociationProposal` (supplying `AssociatedGatewayOwnerAccount` —
  themself, confirming they own the associated gateway... wait, re-read: **the accepter's own
  field name says "AssociatedGatewayOwnerAccount", i.e. the account that owns the associated
  (VGW/TGW) gateway — meaning the ACCEPTER is asserting who owns the OTHER side, not asserting
  their own identity as DCGW owner.** This directionality is subtle and worth an implementer
  re-verifying empirically against real AWS behavior/docs before hardcoding an assumption — this
  audit read the field names directly but doc comments on the two ops do not spell out the full
  cross-account handshake narrative unambiguously.
  `DirectConnectGatewayAssociationProposalState`: `requested`→`accepted`|`deleted` (3 values, no
  `rejected` — declining is presumably "never accept, then the proposer calls
  `DeleteDirectConnectGatewayAssociationProposal`").
- **`AssociationState`** (5 values, the only one with a mid-lifecycle `updating` state):
  `associating`→`associated`, `disassociating`→`disassociated`, plus `updating` specifically when
  `AllowedPrefixesToDirectConnectGateway` changes on an already-`associated` link (via
  `UpdateDirectConnectGatewayAssociation`'s `Add/RemoveAllowedPrefixesToDirectConnectGateway`).
- **Allowed prefixes**: `RouteFilterPrefix{Cidr}` lists gate which VPC CIDR ranges are advertised
  through the gateway association — `CreateDirectConnectGatewayAssociationProposal` supports
  `AddAllowedPrefixesToDirectConnectGateway`/`RemoveAllowedPrefixesToDirectConnectGateway` at
  proposal time, while `AcceptDirectConnectGatewayAssociationProposal`'s
  `OverrideAllowedPrefixesToDirectConnectGateway` lets the *accepter* replace the proposed list
  wholesale rather than accept it as-is — a real, checkable business rule (the final
  `AllowedPrefixesToDirectConnectGateway` on the resulting Association should reflect the
  accepter's override when present, the proposal's requested list otherwise).
- **Attachments** (`DirectConnectGatewayAttachment`): a separate, read-only-via-Describe concept
  from Associations — attachments track individual *virtual interfaces* joining a DCGW
  (`AttachmentType`: `TransitVirtualInterface`|`PrivateVirtualInterface` only, confirming public
  VIFs are structurally excluded), state `attaching`→`attached`, `detaching`→`detached` (4
  values), created automatically whenever a private/transit VIF names a `DirectConnectGatewayId`
  (there is no dedicated `Create*Attachment` op — this is a derived/computed record, not a
  separately-called resource, per this SDK's operation list containing no `CreateDirectConnectGatewayAttachment`).
- **`AssociatedCoreNetwork`** (Cloud WAN core-network attachment) — **no `services/cloudwan` or
  equivalent backend exists in this tree** (not grepped exhaustively this pass, but no such
  directory under `services/`); this field should remain nil/unpopulated, an honest gap, not a
  fabricated Cloud WAN integration.

### Hosted connections, interconnects, and the partner/reseller flow

- **`CreateInterconnect`** models AWS provisioning a *physical cross-connect* for a Direct Connect
  **Partner** (not a typical end customer) at a colocation facility. `AllocateConnectionOnInterconnect`/
  `AllocateHostedConnection` let that partner then allocate sub-connections to *their own*
  end customers (`OwnerAccount`), who confirm via `ConfirmConnection`. `AssociateHostedConnection`
  reassigns an already-hosted connection to a different parent connection/LAG.
- **What is honestly simulatable**: the full state graph (Interconnect/Connection creation,
  `ordering`→confirm→`available` transitions, parent/child `LagId`/`ConnectionId` relationships,
  `DescribeHostedConnections`/`DescribeConnectionsOnInterconnect` correctly listing children) is
  pure bookkeeping — no physical cross-connect is needed to track *who owns what and what state
  it's in*.
- **What cannot be honestly simulated**: there is no real physical link, no real partner
  relationship, and no real billing distinction between a partner's own connections and their
  resold hosted ones. Do not fabricate `Location`/`AvailableProviders` data implying a specific
  real-world colocation facility has capacity, bandwidth availability, or provider presence beyond
  whatever static seed list this implementation chooses to publish via `DescribeLocations`.

### MACsec / encryption-mode

- **`Connection`/`Lag`/`Interconnect` all carry**: `MacSecCapable *bool`, `MacSecKeys []MacSecKey`,
  `EncryptionMode *string` (`no_encrypt`/`should_encrypt`/`must_encrypt`),
  `PortEncryptionStatus *string` (`Encryption Up`/`Encryption Down`). `Connection` additionally has
  `PartnerInterconnectMacSecCapable *bool` (whether the *hosting* interconnect, if any, supports
  MACsec — a derived/read-only fact about the parent, not independently settable).
- **`AssociateMacSecKey`** accepts either a raw `Cak`+`Ckn` pair (Connection Association Key +
  Connection Key Name — the actual MACsec pre-shared-key material) or a pre-existing `SecretARN`
  (a Secrets Manager secret holding the key). **`DisassociateMacSecKey` requires `SecretARN`
  specifically** to identify which key to remove — meaning even a raw `Cak`/`Ckn`-supplied key
  must get a synthesized `SecretARN` in `AssociateMacSecKey`'s response for later removal to be
  possible at all. This repo has `services/secretsmanager` — a real implementation could
  genuinely create a Secrets Manager secret under the hood when given raw `Cak`/`Ckn` (cross-
  service integration, real and valuable) rather than inventing an opaque unbacked ARN string;
  flagged here as the more honest option even though it's more work than a fabricated ARN.
- **`MacSecKey.State`** (`associating`→`associated`, `disassociating`→`disassociated` per doc
  comment, bare string not typed) is a real, simulatable timer/transition state, same shape as
  every other lifecycle enum in this service.
- **Honest limit**: actual MACsec traffic encryption on a physical port cannot exist in an
  emulator. Simulating the *state* (`MacSecKeys` list, `EncryptionMode` acceptance/rejection,
  `PortEncryptionStatus` toggling) is honest bookkeeping; nothing more should be implied.

### BGP failover testing (`StartBgpFailoverTest`/`StopBgpFailoverTest`)

This is a genuinely well-specified, honestly-simulatable timer-driven feature, not a fabrication
risk: `StartBgpFailoverTest` puts the named `BgpPeers` (or all peers on the VIF, if omitted) into
a `down` `BGPStatus` and the parent `VirtualInterface` into `VirtualInterfaceState: testing` for
`TestDurationInMinutes`, creating a `VirtualInterfaceTestHistory` record (`StartTime` set,
`EndTime` nil). `StopBgpFailoverTest` (or timer expiry) reverts state and populates `EndTime`.
`ListVirtualInterfaceTestHistory` is the audit trail. Build this with `pkgs/worker` following the
`services/eks` `scheduleClusterActivation` / `services/grafana` analogous timer pattern already
used elsewhere in this repo (see `services/eks/clusters.go:112-149`,
`services/grafana/workspaces.go:157`).

### Static reference-data operations

`DescribeLocations` (physical colocation facility catalog) and `DescribeRouterConfiguration`
(customer-router vendor/OS template catalog, `RouterType`) both return real-world,
AWS-maintained, slowly-changing operational data not encoded anywhere in the SDK types
themselves — the same class of gap the outposts audit flagged for `ListCatalogItems` and the
resiliencehub audit flagged for `ListSuggestedResiliencyPolicies`. A small defensible static seed
list is reasonable, clearly documented as a stand-in.

## 3. Cross-service wiring needed

### Tagging (`resourcegroupstaggingapi`)

Yes — `TagResource`/`UntagResource`/`DescribeTags` are real, native Direct Connect ops (not named
`ListTagsForResource` like most other services in this campaign — Direct Connect's read op is
`DescribeTags`, and notably **batch-shaped**: it takes `ResourceArns []string`, plural, in one
call, unlike the single-ARN-per-call `ListTagsForResource` pattern seen in outposts/resiliencehub).
This belongs in `wireResourceGroupsTagging` in `cli.go`
(`/home/agbishop/gopherstack/cli.go:5348`, currently wires exactly 30 services enumerated in its
own doc comment at `cli.go:5327-5342`, most recently `wireTaggingGrafana(bk, byName["Grafana"])`
at `cli.go:5399`). Direct Connect would need its own `wireTaggingDirectConnect`-style function
following the `wireTaggingCtxARNResources` generic helper (`cli.go:5510-5537`) or the
`wireTaggingGrafana`/`wireTaggingEFS` model (`cli.go:6675`, `cli.go:6127`) — **five** distinct
taggable resource kinds share the one `directconnect` ARN namespace (`Connection`/`Lag`/
`Interconnect`/`VirtualInterface` all carry `Tags []Tag`; `DirectConnectGateway` also carries
`Tags`), so the tag store backing this wiring needs `resourceTypeFromARN`-style dispatch
(`cli.go:5558-5571`) across (at least) `dxcon`/`dxlag`/`dxvif`/`dx-gateway` resource-type
prefixes, not a single flat map like DynamoDB/SQS's single-resource-kind wiring.

**ARN namespace**: confirmed **`directconnect`** — matching the package name exactly (not a
divergent case like `stepfunctions`→`states` or `efs`→`elasticfilesystem`). Evidence, from the
**real Terraform AWS provider source** (fetched directly via `raw.githubusercontent.com`, not
guessed):

- `internal/service/directconnect/connection.go`, `resourceConnectionRead`:
  `arn.ARN{..., Service: "directconnect", ..., Resource: fmt.Sprintf("dxcon/%s", d.Id())}`
- `internal/service/directconnect/lag.go`, `resourceLagRead`:
  `arn.ARN{..., Service: "directconnect", ..., Resource: fmt.Sprintf("dxlag/%s", d.Id())}`
- `internal/service/directconnect/private_virtual_interface.go`:
  `arn.ARN{..., Service: "directconnect", ..., Resource: fmt.Sprintf("dxvif/%s", d.Id())}`
- `internal/service/directconnect/gateway.go`, function `gatewayARN`:
  `return c.GlobalARN(ctx, "directconnect", "dx-gateway/"+id)` — **notably a `GlobalARN` call,
  not a regional `arn.ARN{Region: ...}` construction like the other three**, meaning
  `DirectConnectGateway` ARNs omit the region segment entirely while `Connection`/`Lag`/
  `VirtualInterface` ARNs include it. `pkgs/arn.Build` (`pkgs/arn/*.go`) only special-cases
  `service == "iam"` for the no-region case today; Direct Connect needs a **resource-kind-level**
  exception (only `dx-gateway`, not the whole `directconnect` service) that `pkgs/arn.Build`'s
  current service-level switch does not support without either a new parameter or a hand-built
  ARN string for this one resource kind specifically.
- **Interconnect's ARN resource-path segment could NOT be confirmed** — no `interconnect.go` (or
  similarly named file) exists anywhere in `hashicorp/terraform-provider-aws`'s
  `internal/service/directconnect/` directory at all (confirmed by listing the full directory via
  `gh api repos/hashicorp/terraform-provider-aws/contents/internal/service/directconnect` — 51
  files, none named for interconnects), consistent with Interconnect being a partner-only resource
  Terraform has no standalone managed-resource type for. Similarly,
  `DirectConnectGatewayAssociation`/`DirectConnectGatewayAssociationProposal` ARN formats (if
  they even have independent ARNs distinct from the DCGW's own — plausible they don't, since
  associations may not be independently taggable resources at all; `DirectConnectGatewayAssociation`/
  `DirectConnectGatewayAssociationProposal` have no `Tags` field in this SDK's `types.go`, so this
  may be moot) were not investigated further, since neither type carries a `Tags` field to begin
  with — only `Connection`/`Lag`/`Interconnect`/`VirtualInterface`/`DirectConnectGateway` do.

### EC2 VPN/Transit Gateway binding (real, existing backends to bind against)

Both real, with file:line:

- **`VpnGateway`** (`services/ec2/advanced_networking.go:100-106`): `VpnGatewayID` (format
  `"vgw-" + 8-char uuid`, per `services/ec2/vpn_gateways.go:22`), `State`, `Type`,
  `AttachedVPCID`, `AttachmentState`. Backend methods:
  `InMemoryBackend.CreateVpnGateway`/`DescribeVpnGateways(ids)`/`DeleteVpnGateway`/
  `AttachVpnGateway`/`DetachVpnGateway` (`services/ec2/vpn_gateways.go:13-131`). **No ARN field at
  all** on this struct today — just a bare ID string.
- **`TransitGateway`** (`services/ec2/vpcs.go:217-225`): `ID`, `Arn` (a real ARN field, unlike
  `VpnGateway`), `Description`, `State`, `OwnerID`, `Options TransitGatewayOptions`. Backend
  methods: `InMemoryBackend.CreateTransitGateway`/`DescribeTransitGateways(ids)`/
  `DeleteTransitGateway`/`ModifyTransitGateway` (`services/ec2/transit_gateways.go:70-225`), plus
  an extensive family of TGW-attachment/route-table/multicast/peering ops across
  `services/ec2/ec2core.go`, `services/ec2/networking1.go`, `services/ec2/tgw_peripherals.go`,
  `services/ec2/tgw_multicast.go`, `services/ec2/transit_gateway_peering.go` (all confirmed
  present, not assumed).
- **This is exactly what `DirectConnectGatewayAssociation`/`AssociatedGateway.Type`
  (`GatewayType`: `virtualPrivateGateway`|`transitGateway`) + `GatewayId`/`VirtualGatewayId` should
  bind against**, and what `DescribeVirtualGateways` should almost certainly proxy from, rather
  than maintaining a separate duplicate VGW store — real cross-service validation (does
  `GatewayId` resolve to an actual `services/ec2` VGW or TGW) is feasible, valuable, non-fabricated
  work, and this repo already has both backends to validate against. Not required for a
  first-pass implementation, but a clear, concrete, low-risk improvement over accepting any
  string unchecked.

### CloudFormation

No `AWS::DirectConnect::*` resource type exists in `services/cloudformation/resources_*.go`
(grepped all 71 `resources_*.go` files case-insensitively for "directconnect" — zero matches).
Confirmed absent, not silently skipped.

### No prior Direct Connect references anywhere in this tree

`grep -rni "directconnect\|dxcon\|dxvif\|dxlag\|dx-gateway\|dxgateway" --include="*.go" services/
cli.go` returns **zero hits** — this is a genuinely from-scratch service with no partial
implementation, no opaque `DirectConnectArn`-style field anywhere else in the tree to
cross-reference (unlike Outposts, which had `OutpostArn` fields scattered across
`services/ec2`, `services/route53resolver`, `services/s3control`, etc.).

## 4. Honest gap list

See the machine-readable `gaps:` list in the frontmatter for the authoritative version. Summary:

1. Zero operations implemented — this document is the spec, not an audit of running code.
2. Interconnect/hosted-connection/partner-reseller flow can only ever be state bookkeeping — no
   physical cross-connect, no real partner relationship, no real billing distinction exists to
   simulate honestly beyond record-keeping.
3. LOA-CFA PDF content (`DescribeLoa`/`DescribeConnectionLoa`/`DescribeInterconnectLoa`) needs a
   defensible placeholder PDF byte stream, clearly flagged as such, never a fabricated
   "real-looking" authorization document.
4. `DescribeLocations`/`DescribeRouterConfiguration` are static AWS-maintained reference catalogs
   with no derivation available from the SDK — a small seed list is defensible if flagged.
5. `DescribeCustomerMetadata` (customer agreements/NNI partner tier) reflects real legal/business
   relationships with no honest way to derive content — default to empty/nonPartner, documented.
6. MACsec and BGP peering can only simulate *state*, never real encryption or real routing.
7. No `AWS::DirectConnect::*` CloudFormation resource type exists in this repo.
8. `DirectConnectGateway`'s ARN is global (no region) while every other Direct Connect ARN kind is
   regional — `pkgs/arn.Build` needs a resource-kind-level exception it doesn't have today.
9. Interconnect's and DirectConnectGatewayAssociation/Proposal's exact ARN resource-path formats
   could not be confirmed from any source reached this pass (the latter two may not even be
   independently taggable/ARN-bearing resources at all, since neither carries a `Tags` field in
   this SDK).
10. `AssociatedCoreNetwork` (Cloud WAN) has no backing service in this tree at all and should
    remain an unresolved, honestly-nil field.

## Top 5 hardest/riskiest things about implementing this service

1. **The thin, generic error model (only `DirectConnectClientException`/
   `DirectConnectServerException` for the vast majority of ops, no typed not-found/validation
   shape at all) means every "resource doesn't exist" and "bad input" condition across 63 ops
   folds into one exception type.** Getting the HTTP-status/error-code mapping right for this one
   generic shape, consistently, across every op that needs to signal "not found" vs. "bad
   request" vs. "conflict" (none of which have their own typed shape here, unlike outposts'
   `ConflictException`/`NotFoundException` split) is a real design decision with no SDK-level
   guidance beyond "it's all `DirectConnectClientException`."
2. **Two structurally different addressing/output-shape conventions coexist without any
   discriminating marker**: flattened vs. nested VIF outputs (wire-trap #1), and
   `AssociationId`-alone vs. `(DirectConnectGatewayId, VirtualGatewayId)`-pair addressing for
   gateway associations (wire-trap #3) — a generic helper written by extrapolating from one
   variant to "the obviously similar" other variant will be wrong in both cases; every op needs
   its own struct checked directly.
3. **The cross-account Direct Connect Gateway association-proposal flow's exact ownership
   semantics on `AcceptDirectConnectGatewayAssociationProposal`'s `AssociatedGatewayOwnerAccount`
   field are subtle and this audit could not fully resolve the directionality from field names and
   doc comments alone** (see the "Direct Connect Gateways, associations, and the cross-account
   proposal flow" section above for the specific ambiguity) — this needs either a real test
   against actual AWS behavior or a very careful re-read of AWS's own (not just this SDK's)
   documentation before an implementer commits to one interpretation.
4. **`DirectConnectGateway`'s global (non-regional) ARN is a structural outlier this repo's
   `pkgs/arn.Build` helper doesn't support today** (its only existing global-service exception is
   keyed on `service == "iam"`, not on a resource-kind within an otherwise-regional service) —
   implementing this correctly means either extending `pkgs/arn` with a resource-kind-aware
   exception or hand-building this one ARN kind outside the shared helper, and either choice has
   ripple effects on consistency with how every other service in this repo builds ARNs.
5. **Five distinct taggable resource kinds (`dxcon`/`dxlag`/`dxvif`/`dx-gateway`, potentially plus
   Interconnect if it turns out to be taggable despite no confirmed ARN format) sharing one
   `directconnect` tagging namespace, with a batch-shaped native tag-read op (`DescribeTags`
   taking `ResourceArns []string`) unlike almost every other service's single-ARN
   `ListTagsForResource`** — wiring this into `wireResourceGroupsTagging` needs the
   `resourceTypeFromARN`-style multi-kind dispatch this repo already has a pattern for
   (`cli.go:5558-5571`), but the batch-input shape of `DescribeTags` itself is a genuine
   Direct-Connect-specific wrinkle relative to every other tagging-wiring precedent audited in
   this campaign so far.
