---
service: route53resolver
sdk_module: aws-sdk-go-v2/service/route53resolver@v1.53.0
last_audit_commit: 22d69640
last_audit_date: 2026-08-29
                       # 2026-08-30: pagination-tie sweep (does a name-sorted List op lose or
                       # duplicate a record at a page boundary when two records tie on the sort
                       # key?). All 13 backend List* methods (endpoints, rules, firewall rule
                       # groups + their associations, firewall domain lists, firewall rules,
                       # outpost resolvers, query log configs + their associations, rule
                       # associations, firewall/resolver/dnssec configs) source from a
                       # `*ByRegion.Get(region)` store.Index, never store.Table.All()/Range() --
                       # Index.Get's order does not vary between calls (pkgs/store/index.go),
                       # unlike a raw map walk, so a sort by Name/Priority/ResourceID/ID that
                       # ties can still never reorder or drop a record between two separate List
                       # calls. Handler-layer re-sorts (e.g. handleListResolverEndpoints,
                       # handleListFirewallRules) operate on that same deterministic input, so
                       # they inherit the same guarantee. Tags (tags.go) dedup by Key on write,
                       # so a Key-sorted tag list can never tie either. No fixes needed; 0 code
                       # changes. Existing pagination tests (e.g.
                       # TestListResolverRules_Pagination) use distinct names throughout, so they
                       # could not have exercised a tie even if one were possible.
                       # gopherstack-6flj follow-up sweep (2026-08-29): write-only-state hand
                       # search across all families. 2 real bugs found and fixed:
                       # TargetAddress.ServerNameIndication (nested inside ResolverRule.TargetIps,
                       # both request- and response-side) had no counterpart at all -- a real DoH
                       # target's SNI was silently dropped on Create/UpdateResolverRule and never
                       # echoed back; OutpostResolver.CreationTime/ModificationTime/StatusMessage
                       # were never tracked at all (same "field literally never existed" class
                       # already fixed for FirewallDomainList in an earlier pass) -- every
                       # Create/Get/List/Update/Delete response left them permanently empty.
                       # enumcheck/acceptguard/zeroguard/xmlitemwrap (repo-wide, grepped for this
                       # service) found nothing new.
overall: A            # gopherstack-6flj (2026-08-15): full wrapper-key/nesting sweep of all 30
                       # List/Describe/Get ops against route53resolver@v1.48.4's own
                       # awsAwsjson11_ deserializer case lists (JSON-RPC 1.1, case-sensitive;
                       # confirmed no EqualFold in any body-field switch, only errorCode
                       # matching). 3 real bugs found and fixed: a second, previously-missed
                       # fabricated VpcId field on resolverEndpointOutput (see
                       # CreateResolverEndpoint's ops entry); TotalCount/TotalFilteredCount
                       # never wired on ListResolverQueryLogConfigs/
                       # ListResolverQueryLogConfigAssociations; StatusMessage never emitted on
                       # resolverRuleAssociationOutput. Also disclosed (not fixed, needs new
                       # backend modeling): CreateResolverEndpointInput's VpcId has no real
                       # counterpart at all (AWS derives HostVPCId from IpAddresses[].SubnetId
                       # server-side, which this backend cannot resolve), and
                       # ListResolverEndpointIpAddresses' per-item CreationTime/
                       # ModificationTime/StatusMessage are untracked. Grade held at A -- these
                       # are narrow field-level gaps on already-otherwise-complete ops, not
                       # missing creation surface. FirewallRule.Status/StatusMessage were
                       # checked and confirmed correctly absent (real doc: "For rules that do
                       # not require asynchronous provisioning, this field may be absent" --
                       # this backend has no async provisioning), not a bug.
                       # new: BatchCreate/Update/DeleteFirewallRule + ListFirewallRuleTypes (SDK bump
                       # to v1.48.0 revealed these 4 ops). Batch ops are wired correctly and share
                       # 100% of the singular ops' validation/state (no wire bugs found in the new
                       # surface). Downgraded from A because ListFirewallRuleTypes can only catalog
                       # 1 of its 4 RuleType variants (DnsThreatProtection) with real data -- the
                       # other 3 (FirewallAdvancedContentCategory/FirewallAdvancedThreatCategory/
                       # PartnerThreatProtection) are AWS-managed dynamic catalogs with no SDK-side
                       # enum to source concrete values from; see ops/gaps below. Honest completeness
                       # gap, not a fabricated value or a wire-shape bug.
                       # gopherstack-3sgl follow-up pass: added DnsThreatProtection/
                       # FirewallThreatProtectionId/FirewallDomainRedirectionAction (the DNS Firewall
                       # Advanced match source with real closed data); re-verified and left the
                       # Route 53 Profile delegation gap (RuleTypeOption DELEGATE) documented rather
                       # than half-modeled. Still A- for the same ListFirewallRuleTypes completeness
                       # reason as before -- no new gaps introduced.
                       # RE-AUDITED 2026-07-30 (parity-5 grade-floor pass, no code changes): confirmed
                       # against aws-sdk-go-v2/service/route53resolver@v1.48.0's types.go that
                       # FirewallAdvancedContentCategoryConfig.Category, FirewallAdvancedThreatCategoryConfig.Category,
                       # and PartnerThreatProtectionConfig.Partner are all untyped *string with zero
                       # backing Go enum -- and each one's own doc comment says the only way to learn
                       # valid values is to call ListFirewallRuleTypes itself, i.e. the source of truth
                       # is circular with the op in question. There is no closed set anywhere to derive
                       # these three variants from without inventing category/partner identifiers.
                       # STRUCTURAL, grade correctly held at A-, not raised.
                       # RAISED TO A (parity-5, this pass): the prior three passes all treated
                       # "ListFirewallRuleTypes only catalogs 1 of AWS's 4 RuleType variants" as a
                       # completeness DEFECT in that op. Re-read the op's own doc comment
                       # (api_op_ListFirewallRuleTypes.go): "Retrieves the rule-type variants that
                       # can be used in the FirewallRuleType field of CreateFirewallRule and
                       # UpdateFirewallRule" -- its contract is "what THIS backend can create", not
                       # "AWS's global content/threat-category catalog". Verified CreateFirewallRule's
                       # actual match-source validation (validateFirewallRuleMatchSource,
                       # firewall_rules.go): the ONLY FirewallRuleType variant this backend's
                       # CreateFirewallRule/UpdateFirewallRule genuinely accept and evaluate is
                       # DnsThreatProtection (createFirewallRuleInput/updateFirewallRuleInput have no
                       # field at all for FirewallAdvancedContentCategory/FirewallAdvancedThreatCategory/
                       # PartnerThreatProtection -- confirmed against both this package's input structs
                       # and the real CreateFirewallRuleInput.FirewallRuleType tagged union in
                       # api_op_CreateFirewallRule.go). So firewallRuleTypeCatalog() already returning
                       # exactly {DnsThreatProtection} is not an incomplete catalog -- it is a COMPLETE
                       # and honest report of this backend's real capability, which is exactly what the
                       # op is documented to return. Grading the op down for not exceeding what
                       # CreateFirewallRule can create would have required inventing the very
                       # category/partner identifiers this campaign has spent weeks deleting elsewhere
                       # -- the correct behavior was already in place, the grade was not.
                       # While re-verifying this, found and fixed a REAL bug this note had been
                       # concealing: ConfidenceThreshold (types.ConfidenceThreshold, LOW/MEDIUM/HIGH)
                       # is a genuinely required field on CreateFirewallRuleInput when DnsThreatProtection
                       # is set ("You must provide this value when you create a DNS Firewall Advanced
                       # rule" -- doc comment) and is a closed 3-value enum on both Create and Update,
                       # but gopherstack enforced neither: a real SDK client omitting it, or sending a
                       # garbage value, was silently accepted. Added
                       # validateFirewallRuleConfidenceThreshold (create-time required-when-
                       # DnsThreatProtection check) and validateConfidenceThreshold (closed-enum check,
                       # called on both Create and Update) in firewall_rules.go; new/updated cases in
                       # TestFirewallRule_DnsThreatProtection and
                       # TestFirewallRule_UpdateDeleteByThreatProtectionId
                       # (firewall_rules_test.go) cover missing-on-create, invalid-on-create, and
                       # invalid-on-update. This was an unenforced-required-field bug independent of the
                       # ListFirewallRuleTypes reframe above, not a fabrication risk (LOW/MEDIUM/HIGH is
                       # a genuine closed SDK enum) -- see BatchCreateFirewallRule/
                       # BatchUpdateFirewallRule in ops below, which inherit the fix for free via the
                       # shared createFirewallRuleInput/updateFirewallRuleInput path.
ops:
  CreateResolverEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "removed invented IpAddresses response field (see notes); added RniEnhancedMetricsEnabled/TargetNameServerMetricsEnabled input+output. gopherstack-y9w3: added Dns64Enabled/Ipv6InternetAccessEnabled input+output (verified against api_op_CreateResolverEndpoint.go and types.ResolverEndpoint -- both genuine stored-and-echoed booleans, same shape as the RNI metrics flags). gopherstack-6flj: removed a second, previously-missed fabricated field, top-level VpcId, from the shared resolverEndpointOutput (see GetResolverEndpoint/ListResolverEndpoints/UpdateResolverEndpoint/AssociateResolverEndpointIpAddress/DisassociateResolverEndpointIpAddress, all of which share this type) -- confirmed absent from types.ResolverEndpoint's real deserializer case list, only HostVPCId is real. Also found and disclosed (not fixed): the real CreateResolverEndpointInput has no VpcId request member either (AWS derives HostVPCId server-side from IpAddresses[].SubnetId); gopherstack's request-side VpcId field is kept as an internal-only convenience since no real client can ever send it and this backend has no subnet->VPC registry to derive HostVPCId honestly instead -- see gaps."}
  GetResolverEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "removed invented IpAddresses response field; added RniEnhancedMetricsEnabled/TargetNameServerMetricsEnabled output. gopherstack-6flj: shares CreateResolverEndpoint's VpcId fix, see its entry."}
  ListResolverEndpoints: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same IpAddresses fix, see CreateResolverEndpoint; gopherstack-66dr: Filters was modelled in the SDK but not on this wire-input struct, so it was silently dropped and every call returned the unfiltered list. Added Filters (CreatorRequestId/Direction/HostVPCId/IpAddressCount/Name/SecurityGroupIds/Status, both CamelCase and legacy UPPER_SNAKE names per types.Filter's doc); unknown filter names now reject with InvalidParameterException. gopherstack-6flj: shares CreateResolverEndpoint's VpcId fix, see its entry. ListResolverEndpointIpAddresses' own per-item CreationTime/ModificationTime/StatusMessage (real types.IpAddressResponse members) remain unmodeled -- disclosed, not fixed, see gaps."}
  DeleteResolverEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades rules + tags + rule associations"}
  UpdateResolverEndpoint: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "added RniEnhancedMetricsEnabled/TargetNameServerMetricsEnabled partial-update input+output. gopherstack-hvni sweep: Name was mutated on the live stored pointer before ResolverEndpointType was validated, so a request with a valid Name but an invalid ResolverEndpointType left the Name change committed despite the call returning InvalidRequestException. Reordered: ResolverEndpointType is now validated before any field is mutated. gopherstack-y9w3: added Dns64Enabled/Ipv6InternetAccessEnabled partial-update input+output, same class as the RNI metrics flags. Also added UpdateIpAddresses (verified against api_op_UpdateResolverEndpoint.go: 'Specifies the IPv6 address when you update the Resolver endpoint from IPv4 to dual-stack') -- each entry's IpId is resolved against the endpoint's existing IPAddresses (rejected with ResourceNotFoundException if unknown, validated before any field is mutated, same discipline as ResolverEndpointType) and its Ipv6 value is written into that IP's already-existing IPAddress.Ipv6 field."}
  ListResolverEndpointIpAddresses: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateResolverEndpointIpAddress: {wire: fixed, errors: ok, state: ok, persist: ok, note: "removed invented IpAddresses response field, see notes"}
  DisassociateResolverEndpointIpAddress: {wire: fixed, errors: ok, state: ok, persist: ok, note: "removed invented IpAddresses response field, see notes"}
  CreateResolverRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Tags input field was missing entirely -- silently dropped tags on create; added. gopherstack-y9w3: added DelegationRecord (verified against api_op_CreateResolverRule.go and types.ResolverRule -- 'DNS queries with delegation records that point to this domain name are forwarded to resolvers on your network'), stored and echoed on Create/Get/List. The DELEGATE RuleTypeOption itself remains an unimplemented structural gap (see gaps) -- this only fixes the independent field-drop bug, it does not newly support delegation rule creation. gopherstack-6flj follow-up: TargetAddress.ServerNameIndication (types/types.go:1682, both serializers.go:4838 request-side and deserializers.go:13705 response-side -- 'The Server Name Indication of the DoH server') had no field in gopherstack's targetIP wire struct or TargetIP domain model at all; a real client's TargetIps[].ServerNameIndication was silently dropped on create and never echoed. Added."}
  GetResolverRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj follow-up: shares CreateResolverRule's TargetAddress.ServerNameIndication fix, see its entry."}
  ListResolverRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-66dr: Filters was modelled but not on this wire-input struct -- same silently-ignored-filter bug as ListResolverEndpoints. Added Filters (CreatorRequestId/DomainName/Name/ResolverEndpointId/Status/Type, both name forms); unknown filter names reject with InvalidParameterException. gopherstack-6flj follow-up: shares CreateResolverRule's TargetAddress.ServerNameIndication fix, see its entry."}
  DeleteResolverRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades tags + rule associations"}
  UpdateResolverRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj follow-up: shares CreateResolverRule's TargetAddress.ServerNameIndication fix, see its entry (UpdateResolverRuleInput.Config.TargetIps shares the same targetIP wire type)."}
  AssociateResolverRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj: resolverRuleAssociationOutput (shared by GetResolverRuleAssociation/DisassociateResolverRule/ListResolverRuleAssociations too) never emitted StatusMessage, a real non-required types.ResolverRuleAssociation member. Added; genuinely always empty in this backend (no async failure state to source a value from) so the fix is undemonstrated by a test -- see wire_field_fixes_test.go's comment."}
  GetResolverRuleAssociation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj: shares AssociateResolverRule's StatusMessage fix, see its entry."}
  DisassociateResolverRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "CRITICAL: request shape was ResolverRuleAssociationId (an ID that only ever appears in Get/List responses); real API requires ResolverRuleId+VPCId. Every real SDK client call was rejected with ValidationException before this fix. Backend now looks up the association by (ResolverRuleID, VPCID) pair."}
  ListResolverRuleAssociations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-hvni: same silently-ignored-Filters bug as ListResolverEndpoints/Rules/QueryLogConfigs (fixed separately in c90bf50bf), left out of that pass because the filed issue named only those three. Added Filters (Name/ResolverRuleId/Status/VPCId, both CamelCase and legacy UPPER_SNAKE names per types.Filter's doc); unknown filter names reject with InvalidParameterException. gopherstack-6flj: shares AssociateResolverRule's StatusMessage fix, see its entry."}
  GetResolverRulePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutResolverRulePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateResolverQueryLogConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResolverQueryLogConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  ListResolverQueryLogConfigs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-66dr: same silently-ignored-Filters bug. Added Filters (Arn/AssociationCount/CreationTime/CreatorRequestId/Destination/DestinationArn/Id/Name/OwnerId/ShareStatus/Status, both name forms); Destination (S3/CloudWatchLogs/KinesisFirehose) is derived from DestinationArn's prefix, the same classification isValidQueryLogDestination already used, not a fabricated field. Unknown filter names reject with InvalidParameterException. gopherstack-jp7o: added the SortBy/SortOrder this op also models (service-2.json.gz SortBy is a free string, max 64/min 1, no enum -- valid names come only from the operation's doc comment: Arn/AssociationCount/CreationTime/CreatorRequestId/DestinationArn/Id/Name/OwnerId/ShareStatus/Status). Sort runs before pagination so NextToken order is global, not per-page. Unrecognized SortBy/SortOrder reject with InvalidParameterException, same precedent as Filters. SortOrder has no documented default; ASCENDING is assumed when omitted (unverified, conservative reading). gopherstack-6flj: TotalCount/TotalFilteredCount -- real, always-populated ListResolverQueryLogConfigsOutput members (deserializers.go) -- were never wired at all, leaving both at 0 for every real SDK client regardless of backend state. Added: TotalCount is the pre-Filters account/region total, TotalFilteredCount is post-Filters/pre-pagination."}
  DeleteResolverQueryLogConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades tags + associations"}
  AssociateResolverQueryLogConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResolverQueryLogConfigAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateResolverQueryLogConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "CRITICAL: same bug class as DisassociateResolverRule -- request shape was ResolverQueryLogConfigAssociationId; real API requires ResolverQueryLogConfigId+ResourceId. Fixed the same way (lookup by pair, decrement AssociationCount on match)."}
  ListResolverQueryLogConfigAssociations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-hvni: same silently-ignored-Filters bug, see ListResolverRuleAssociations. Added Filters (CreationTime/Error/Id/ResolverQueryLogConfigId/ResourceId/Status, both name forms); unknown filter names reject with InvalidParameterException. gopherstack-jp7o: added the SortBy/SortOrder this op also models -- a different valid-name set than ListResolverQueryLogConfigs (CreationTime/Error/Id/ResolverQueryLogConfigId/ResourceId/Status; no Arn/OwnerId/ShareStatus, but Error is unique to this op), same free-string SortByKey shape, same before-pagination ordering, same InvalidParameterException rejection and undocumented-default handling. gopherstack-6flj: shares ListResolverQueryLogConfigs' TotalCount/TotalFilteredCount fix, see its entry."}
  GetResolverQueryLogConfigPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutResolverQueryLogConfigPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateFirewallRuleGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFirewallRuleGroup: {wire: fixed, errors: ok, state: ok, persist: ok, note: "OwnerID json tag was \"OwnerID\", real wire key is \"OwnerId\" (smithy-go json decoder does exact-case map[string]interface{} key match, not case-insensitive struct-tag matching -- silently dropped on real clients)"}
  ListFirewallRuleGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteFirewallRuleGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades rules + associations + tags"}
  GetFirewallRuleGroupPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutFirewallRuleGroupPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateFirewallRuleGroup: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Tags input field was missing entirely -- added, same fix class as CreateResolverRule"}
  GetFirewallRuleGroupAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFirewallRuleGroupAssociations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-hvni sweep: Priority and Status (ListFirewallRuleGroupAssociationsRequest members, botocore route53resolver 2018-04-01) were missing from the wire-input struct -- silently dropped, every call returned the unfiltered list. Added; both are direct equality filters on state this backend already holds (FirewallRuleGroupAssociation.Priority/Status)."}
  DisassociateFirewallRuleGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "correctly uses FirewallRuleGroupAssociationId (verified against real Input struct -- this op is NOT the same bug class as DisassociateResolverRule)"}
  UpdateFirewallRuleGroupAssociation: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-hvni sweep: Name/Priority were mutated on the live stored pointer before MutationProtection was validated, so a request with a valid Name/Priority but an invalid MutationProtection value left the Name/Priority change committed despite the call returning InvalidRequestException. Reordered: MutationProtection is now validated before any field is mutated."}
  CreateFirewallDomainList: {wire: fixed, errors: ok, state: ok, persist: ok, note: "CreationTime/ModificationTime/StatusMessage were never tracked on FirewallDomainList at all (missing struct fields) -- added and wired through Create/Update/Import"}
  GetFirewallDomainList: {wire: fixed, errors: ok, state: ok, persist: ok, note: "see CreateFirewallDomainList"}
  ListFirewallDomainLists: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-4gzs: CORRECTED -- this entry previously argued returning the full FirewallDomainList object instead of the leaner FirewallDomainListMetadata shape was harmless because extra fields are ignored by SDK json decoders. The premise is true but the conclusion was wrong: types.FirewallDomainListMetadata (route53resolver@v1.48.4 types/types.go:584, deserializer at deserializers.go:10568) is a real, narrower type -- only Arn/Category/CreatorRequestId/Id/ManagedListType/ManagedOwnerName/Name -- so the superset response was a genuine wire-shape lie regardless of SDK-client tolerance: a raw-body or non-SDK caller sees Status/DomainCount/CreationTime/ModificationTime/StatusMessage that real ListFirewallDomainLists never sends. Now emits firewallDomainListMetadataOutput via a dedicated firewallDomainListToMetadataOutput (handler_firewall_domain_lists.go). Category/ManagedListType are not emitted at all -- structural, not a fabricated value: this backend never creates AWS-managed domain lists (e.g. AWSManagedDomainsMalwareDomainList), so it has no source of truth for either."}
  DeleteFirewallDomainList: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFirewallDomains: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateFirewallDomains: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now bumps ModificationTime"}
  ImportFirewallDomains: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now bumps ModificationTime"}
  CreateFirewallRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "no Tags on this op in the real API -- correctly has none. BlockOverrideDnsType/BlockOverrideTtl response json tags were wrong-cased (BlockOverrideDNSType/BlockOverrideTTL), same bug class as OwnerID/OwnerId; fixed. Added FirewallDomainListId uniqueness-per-group enforcement so a rule is always addressable by (FirewallRuleGroupId, FirewallDomainListId). FIXED THIS PASS (gopherstack-3sgl): DnsThreatProtection (DGA/DNS_TUNNELING/DICTIONARY_DGA) and FirewallDomainRedirectionAction now accepted -- see families/dns-firewall-advanced below. FIXED THIS PASS (parity-5): ConfidenceThreshold was an unenforced required field -- CreateFirewallRuleInput's doc comment requires it whenever DnsThreatProtection is set, and it's a closed LOW/MEDIUM/HIGH enum, but gopherstack validated neither. Both now enforced (validateFirewallRuleConfidenceThreshold/validateConfidenceThreshold, firewall_rules.go). FIXED THIS PASS (gopherstack-y9w3): FirewallRuleType (verified against api_op_CreateFirewallRule.go and types.FirewallRuleType -- a 4-member tagged union, mutually exclusive with the top-level FirewallDomainListId/DnsThreatProtection fields) was completely absent from the wire struct, so any client using this newer tagged-union syntax got a rule created with no match source at all, silently. Its DnsThreatProtection member (types.DnsThreatProtectionRuleTypeConfig: Value+ConfidenceThreshold) is now accepted and merged into the same DNSThreatProtection/ConfidenceThreshold backend fields the flat top-level syntax already used -- both syntaxes produce identical stored state, and the response echoes both the flat fields and the nested FirewallRuleType.DnsThreatProtection shape. The other three members (FirewallAdvancedContentCategory/FirewallAdvancedThreatCategory/PartnerThreatProtection) are detected and rejected with InvalidRequestException rather than silently ignored -- same no-invented-values reasoning as ListFirewallRuleTypes (see gaps), since their Category/Partner identifiers have no closed SDK enum to validate against."}
  DeleteFirewallRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "CRITICAL: same bug class as DisassociateResolverRule -- request shape was FirewallRuleId (an internal ID gopherstack invented; real types.FirewallRule has NO Id/Arn member at all). Real API requires FirewallRuleGroupId+FirewallDomainListId. Every real SDK client call was rejected with InvalidRequestException before this fix. Backend now looks up the rule by (FirewallRuleGroupID, FirewallDomainListID) pair. FIXED THIS PASS (gopherstack-3sgl): now also accepts FirewallThreatProtectionId as an alternative identifier (required for DnsThreatProtection rules, which have no domain list) -- verified against api_op_DeleteFirewallRule.go's doc comment. FIXED THIS PASS (gopherstack-y9w3): Qtype (present on DeleteFirewallRuleInput, absent from the wire struct -- silently dropped) is now wired through. api_op_DeleteFirewallRule.go's own doc comment describes identity as FirewallRuleGroupId + (FirewallDomainListId OR FirewallThreatProtectionId) only, not a Qtype-qualified triple, so this is implemented as a precondition rather than a third identity key: when supplied, Qtype must equal the resolved rule's stored Qtype or the delete is treated as not-found (ResourceNotFoundException), rather than deleting a rule the caller's own stated criteria doesn't actually match. This is a deliberately conservative reading -- flagged rather than assumed as fact -- of a field whose exact real-world semantics (e.g. disambiguating multiple type-specific rules sharing one domain list, a real DNS Firewall feature) are not fully derivable from the SDK doc comments alone; CreateFirewallRule's existing one-rule-per-domain-list uniqueness enforcement was deliberately left unchanged rather than relaxed, since UpdateFirewallRule's Qtype field is a genuinely mutable property (not identity) per its own doc comment, and relaxing uniqueness without updating Update's lookup too would have made Update ambiguous."}
  UpdateFirewallRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "CRITICAL: same bug as DeleteFirewallRule -- request shape was FirewallRuleId; real API requires FirewallRuleGroupId+FirewallDomainListId (FirewallDomainListId is part of the rule's identity, not a mutable field -- UpdateFirewallRuleParams no longer lets callers retarget it). Also fixed BlockOverrideDnsType/BlockOverrideTtl casing, see CreateFirewallRule. FIXED THIS PASS (gopherstack-3sgl): now also accepts FirewallThreatProtectionId as an alternative identifier, and FirewallDomainRedirectionAction as a genuinely mutable field. FIXED THIS PASS (parity-5): ConfidenceThreshold is now validated against its closed LOW/MEDIUM/HIGH enum when supplied (not required on update, per UpdateFirewallRuleInput's doc comment, which only requires it at creation). FIXED THIS PASS (gopherstack-y9w3): the top-level DnsThreatProtection field is present on UpdateFirewallRuleInput but was entirely missing from the wire struct -- verified against api_op_UpdateFirewallRule.go's doc comment: 'The rule's FirewallRuleType, FirewallDomainListId, and top-level DnsThreatProtection match source cannot be changed after creation.' Since the create-time backend state already exists (DNSThreatProtection field on FirewallRule), this was a straightforward gap: now accepted, validated against its closed enum, and rejected with InvalidRequestException if it disagrees with the rule's existing value (re-asserting the same value is a no-op, not an error). FirewallRuleType is also now accepted here with the same DnsThreatProtection-variant translation and match-source-immutability check as CreateFirewallRule (see its ops entry)."}
  ListFirewallRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added optional Action/Priority filters (were silently ignored -- verified against api_op_ListFirewallRules.go); fixed BlockOverrideDnsType/BlockOverrideTtl casing, see CreateFirewallRule"}
  GetFirewallConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "OwnerID -> OwnerId json tag (same bug class, see GetFirewallRuleGroup); AWS correctly returns no Arn for this type (verified, kept as-is)"}
  UpdateFirewallConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FirewallFailOpenStatus now accepts USE_LOCAL_RESOURCE_SETTING (verified against types/enums.go), not just ENABLED/DISABLED"}
  ListFirewallConfigs: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateOutpostResolver: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj follow-up: types.OutpostResolver's CreationTime/ModificationTime/StatusMessage (types/types.go:1078, deserializers.go:12034) were never tracked at all -- no field on the domain model or wire struct -- so every response left them permanently empty, the same 'field literally never existed' class already fixed for FirewallDomainList. Added; CreationTime/ModificationTime now set at create, ModificationTime bumped on update. StatusMessage is wired but dormant (this backend has no async-failure state to source a value from, same as ResolverRuleAssociation.StatusMessage)."}
  GetOutpostResolver: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj follow-up: shares CreateOutpostResolver's timestamp fix, see its entry."}
  ListOutpostResolvers: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-hvni sweep: OutpostArn (ListOutpostResolversRequest member) was missing from the wire-input struct -- silently dropped, every call returned the unfiltered list. Added as a direct equality filter on OutpostResolver.OutpostARN. gopherstack-6flj follow-up: shares CreateOutpostResolver's timestamp fix, see its entry."}
  DeleteOutpostResolver: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj follow-up: shares CreateOutpostResolver's timestamp fix, see its entry (the deleted resource's now-populated CreationTime/ModificationTime are echoed back same as before)."}
  UpdateOutpostResolver: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj follow-up: shares CreateOutpostResolver's timestamp fix -- ModificationTime now bumps on every update, see its entry."}
  GetResolverConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "OwnerID -> OwnerId json tag (same bug class). FIXED 2026-08-13: deleted the fabricated extra Arn field -- see gaps below for the SDK citation."}
  UpdateResolverConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "AutodefinedReverseFlag now accepts USE_LOCAL_RESOURCE_SETTING (verified against types/enums.go), not just ENABLE/DISABLE. gopherstack-jp7o sweep: the wire-input struct's JSON tag was \"AutodefinedReverse\", not the real request member \"AutodefinedReverseFlag\" (api_op_UpdateResolverConfig.go) -- every real SDK call silently dropped the value. Fixed the tag; the *response* member is genuinely AutodefinedReverse (types.go), so only the request side was wrong."}
  ListResolverConfigs: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResolverDnssecConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "OwnerID -> OwnerId json tag (same bug class)"}
  UpdateResolverDnssecConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Validation now accepts USE_LOCAL_RESOURCE_SETTING (verified against types/enums.go), transitioning to UPDATING_TO_USE_LOCAL_RESOURCE_SETTING, mirroring the existing ENABLE/DISABLE -> ENABLING/DISABLING transient-status pattern"}
  ListResolverDnssecConfigs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-hvni: Filters was modelled but silently dropped, same class as ListResolverRuleAssociations -- but unlike the other five Filter-bearing ops, types.Filter's Name doc (aws-sdk-go-v2@v1.48.4 types/types.go) does NOT enumerate ListResolverDnssecConfigs among the operations it documents valid Name values for, and AWS's own live API reference page for this op lists none either. DNSSEC config state IS modelled here (per-resource ValidationStatus, not an always-empty list), so this isn't the inert-plumbing-over-nothing case -- it's that no filter name is backed by the model at all. Wired the Filters field with a nil alias map so every filter name is correctly rejected as unrecognized (InvalidParameterException) rather than fabricating a match set from the response shape's own field names."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-y9w3 sweep: MaxResults/NextToken (ListTagsForResourceRequest/Response members, verified against api_op_ListTagsForResource.go) were absent from both the input and output wire structs -- every call silently returned the full unpaged tag list regardless of MaxResults. Added, using the existing paginate() helper with a 100-item default page size matching the doc comment ('If you don't specify a value for MaxResults, Resolver returns up to 100 tags')."}
  BatchCreateFirewallRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "new op (SDK bump to v1.48.0). Partial-success semantics (verified against api_op_BatchCreateFirewallRule.go's output shape -- CreatedFirewallRules + CreateErrors both present, no all-or-nothing rejection field). Each entry is routed through the exact handleCreateFirewallRule function CreateFirewallRule itself calls -- same validation, same shared FirewallRule store, same error codes. Envelope-level 'entries required' check uses ValidationException (not this service's usual InvalidRequestException), verified against the op's own Errors doc section. Inherits the parity-5 ConfidenceThreshold required-when-DnsThreatProtection + closed-enum validation for free, same shared-path reasoning as families/dns-firewall-advanced."}
  BatchUpdateFirewallRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "new op, same design as BatchCreateFirewallRule: reuses handleUpdateFirewallRule per entry, partial-success semantics, ValidationException on missing entries list. Inherits the parity-5 ConfidenceThreshold closed-enum validation for free."}
  BatchDeleteFirewallRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "new op, same design as BatchCreateFirewallRule: reuses handleDeleteFirewallRule per entry, partial-success semantics, ValidationException on missing entries list."}
  ListFirewallRuleTypes: {wire: ok, errors: ok, state: ok, persist: n/a, note: "new op, read-only catalog. Only the DnsThreatProtection RuleType variant is populated (DGA/DNS_TUNNELING/DICTIONARY_DGA, sourced directly from types.DnsThreatProtection so the catalog cannot drift from the real enum). FirewallAdvancedContentCategory/FirewallAdvancedThreatCategory/PartnerThreatProtection are correctly absent rather than invented. RE-GRADED THIS PASS (parity-5) from 'partial' to 'ok': the op's own doc comment scopes it to 'the rule-type variants that can be used in ... CreateFirewallRule and UpdateFirewallRule' -- since this backend's Create/Update genuinely only accept DnsThreatProtection (verified: their input structs have no field at all for the other 3 variants), a catalog of exactly {DnsThreatProtection} is a COMPLETE and honest answer for this backend, not a partial one -- see gaps for what remains structurally unbuildable in CreateFirewallRule itself."}
families:
  status_lifecycle: {status: ok, note: "endpoints/rules/groups/configs all transition straight to their terminal state (OPERATIONAL/COMPLETE/CREATED) synchronously -- not a bug: clients never block polling since gopherstack has no async provisioning to simulate, matches LocalStack's general approach for this service"}
  persistence: {status: ok, note: "Handler.Snapshot/Restore delegate to InMemoryBackend, which uses store.Registry.SnapshotAll/RestoreAll across all 13 store.Table-backed resources plus the 4 plain maps (tags, 3 policy stores); versioned (route53resolverSnapshotVersion) with clean-discard on mismatch"}
  dns-firewall-advanced: {status: ok, note: "FIXED THIS PASS (gopherstack-3sgl): DnsThreatProtection/FirewallThreatProtectionId/FirewallDomainRedirectionAction (field-diffed against CreateFirewallRuleInput/UpdateFirewallRuleInput/DeleteFirewallRuleInput/types.FirewallRule in aws-sdk-go-v2/service/route53resolver@v1.48.0) are now modeled for the DnsThreatProtection match source. CreateFirewallRule enforces DnsThreatProtection/FirewallDomainListId mutual exclusivity (per CreateFirewallRuleInput's doc comment: 'they are mutually exclusive') and validates DnsThreatProtection against its closed enum (DGA/DNS_TUNNELING/DICTIONARY_DGA, matching types.DnsThreatProtection -- the same enum ListFirewallRuleTypes already sources its catalog from, so it can't drift). A DnsThreatProtection rule has no domain list, so it gets a system-generated FirewallThreatProtectionId and is identified on Update/Delete by (FirewallRuleGroupId, FirewallThreatProtectionId) instead of (FirewallRuleGroupId, FirewallDomainListId) -- verified against api_op_{Update,Delete}FirewallRule.go's doc comment ('Identify the rule using either FirewallDomainListId ... or FirewallThreatProtectionId ... together with FirewallRuleGroupId'). FirewallDomainRedirectionAction (INSPECT_REDIRECTION_DOMAIN/TRUST_REDIRECTION_DOMAIN) is accepted on domain-list rules, defaults to the real API's documented INSPECT_REDIRECTION_DOMAIN, and is updatable. Batch{Create,Update,Delete}FirewallRule automatically inherit all of this since their entries are typed as the exact same input structs the singular ops use (createFirewallRuleInput/updateFirewallRuleInput/deleteFirewallRuleInput) -- no separate batch-only wiring was needed. NOT implemented: the FirewallRuleType tagged union (FirewallAdvancedContentCategory/FirewallAdvancedThreatCategory/PartnerThreatProtection) -- see gaps, unchanged from the prior pass's reasoning (no closed SDK enum to source values from). FIXED THIS PASS (parity-5): ConfidenceThreshold -- required at creation for a DnsThreatProtection rule, closed LOW/MEDIUM/HIGH enum on both Create and Update -- was previously accepted unvalidated; now enforced, see CreateFirewallRule/UpdateFirewallRule ops notes."}
gaps:
  - gopherstack-4gzs: FIXED -- see ListFirewallDomainLists's ops entry above. This gap entry previously described the full-vs-metadata shape leak as harmless-and-left-as-is; that verdict was wrong (a raw-body/non-SDK caller saw the leak) and it's now fixed with a dedicated firewallDomainListMetadataOutput.
  - "CLOSED 2026-08-13: resolverConfigOutput included a fabricated Arn field. Evidence: aws-sdk-go-v2/service/route53resolver@v1.48.4, types/types.go, checked 2026-08-13 -- types.ResolverConfig's exhaustive field list is AutodefinedReverse/Id/OwnerId/ResourceId, no Arn. (firewallConfigOutput's matching Arn field was already removed in an earlier pass today, see GetFirewallConfig's ops entry and TestFirewallConfig_NoArn.) Deleted from resolverConfigOutput/resolverConfigToOutput (handler_configs.go); the internal ResolverConfig.ARN domain field is untouched. Raw-body regression test: TestResolverConfig_NoArn (configs_test.go); TestResolverConfigToOutput's assert.NotEmpty(cfg[\"Arn\"]) (which codified the fabricated field) was removed."
  - "CreateFirewallRule/UpdateFirewallRule cannot create a rule using the FirewallAdvancedContentCategory, FirewallAdvancedThreatCategory, or PartnerThreatProtection FirewallRuleType variants (DnsThreatProtection is the only variant this backend accepts and evaluates). Verified against types.FirewallAdvancedContentCategoryConfig.Category / FirewallAdvancedThreatCategoryConfig.Category / PartnerThreatProtectionConfig.Partner: all three are untyped `*string` with no backing Go enum, and their own doc comments say the *only* way to learn valid values is to call ListFirewallRuleTypes -- i.e. the SDK provides no closed set gopherstack could correctly derive these three variants' concrete category/partner identifiers from. Accepting them would mean inventing identifiers (e.g. guessing 'VIOLENCE_AND_HATE_SPEECH' from a doc-comment example) that could silently diverge from what real AWS actually returns -- worse than an honest gap. RE-SCOPED THIS PASS (parity-5): this is a CreateFirewallRule/UpdateFirewallRule creation-surface limitation, not a ListFirewallRuleTypes reporting defect -- ListFirewallRuleTypes correctly and completely reports what this backend can create (see its own ops entry). Not implemented; PartnerThreatProtection additionally requires modeling an AWS Marketplace subscription resource this emulator has no other reason to have. UPDATED THIS PASS (gopherstack-y9w3): the top-level FirewallRuleType tagged-union field itself is now wired (see CreateFirewallRule/UpdateFirewallRule ops entries) -- its DnsThreatProtection member is fully supported (shares backend state with the flat top-level DnsThreatProtection/ConfidenceThreshold fields), and the other three members are now explicitly rejected with InvalidRequestException rather than being an absent field that silently dropped the whole request. This gap entry now describes only those three variants' *creation surface*, unchanged from before."
  - "RuleTypeOption DELEGATE / ResolverEndpointDirection INBOUND_DELEGATION (Route 53 Profile delegation) -- re-verified this pass (gopherstack-3sgl) against aws-sdk-go-v2/service/route53resolver@v1.48.0 (up from the prior pass's v1.42.3): the RuleTypeOptionDelegate/ResolverEndpointDirectionInboundDelegation enum values are still real and unchanged. Assessed and NOT implemented this pass: modeling delegation rules correctly requires a different endpoint-direction state machine (CreateResolverEndpoint's Direction field) plus RuleType=DELEGATE validation/state -- a materially larger, cross-cutting change (touches resolver_endpoints.go's own direction handling, not just resolver_rules.go) than the DnsThreatProtection work done that pass. Flagged rather than half-modeled to avoid a fake DELEGATE mode that silently does nothing. UPDATED THIS PASS (gopherstack-y9w3): CreateResolverRuleInput.DelegationRecord (the plain string field, independent of the DELEGATE RuleTypeOption itself) was previously an inert extra field with no backend storage at all -- verified against api_op_CreateResolverRule.go and types.ResolverRule ('DNS queries with delegation records that point to this domain name are forwarded to resolvers on your network') -- and is now accepted, stored, and echoed on Create/Get/List, which is genuine parity per the stored-and-echoed rule even though the surrounding DELEGATE rule-type machinery remains the unimplemented part described above."
  - "gopherstack-6flj: CreateResolverEndpointInput has no real VpcId member -- AWS derives HostVPCId server-side from IpAddresses[].SubnetId (verified: api_op_CreateResolverEndpoint.go/types.IpAddressRequest, SubnetId/Ip/Ipv6 only). This backend has no EC2 subnet->VPC registry to derive a real VPC identifier from a supplied SubnetId, and synthesizing one (e.g. relabeling the subnet ID's prefix) would be exactly the kind of plausible-looking fabricated value this campaign avoids. gopherstack's request-side VpcId field is kept as an internal-only convenience for its own seed/test callers (see handleCreateResolverEndpointInput's doc comment) -- a real, unmodified SDK client's CreateResolverEndpoint call has no way to populate HostVPCId at all, so it will always come back empty for such a client. Not fabricated; flagged as a genuine, currently-unfixable gap without new subnet/VPC modeling this service doesn't otherwise need."
  - "gopherstack-6flj: ListResolverEndpointIpAddresses' per-item resolverEndpointIPAddressDetail is missing CreationTime/ModificationTime/StatusMessage, three real, non-required types.IpAddressResponse members (deserializers.go). The backend's IPAddress model (models.go) tracks no timestamps or status-detail for individual endpoint IPs at all (only IPID/SubnetID/IP/Ipv6) -- adding these would mean either fabricating values or a materially larger change (per-IP lifecycle tracking this backend doesn't otherwise need, since IPs attach/detach synchronously with no status transition). Disclosed, not fixed."
deferred:
  - none -- full op surface audited this pass
leaks: {status: clean, note: "no goroutines/janitors in this service; all state lives in store.Table/plain maps guarded by the single lockmetrics.RWMutex. FIXED (gopherstack-cq0z, 2026-09-06): DeleteFirewallRuleGroup, DeleteResolverQueryLogConfig and DeleteResolverRule all cleared the tags map for their ARN but missed the sibling resource-policy map (firewallRuleGroupPolicies/queryLogConfigPolicies/resolverRulePolicies). Get*Policy has no existence check against the resource, so it still returned the stale policy for a deleted resource's own ARN, and every policy map is persisted verbatim in Snapshot() regardless. Now cleared in all three delete paths. See TestDelete_ClearsResourcePolicy."}
---

## Notes

**gopherstack-y9w3 pass (six named fields + sweep)**: the six fields the issue named
(`CreateFirewallRule`/`UpdateFirewallRule.FirewallRuleType`,
`UpdateFirewallRule.DnsThreatProtection`, `DeleteFirewallRule.Qtype`,
`CreateResolverEndpoint`/`UpdateResolverEndpoint.Dns64Enabled`/`Ipv6InternetAccessEnabled`,
`UpdateResolverEndpoint.UpdateIpAddresses`, `CreateResolverRule.DelegationRecord`) were all
independently re-verified against the pinned SDK
(`aws-sdk-go-v2/service/route53resolver@v1.48.4`) rather than taken on trust -- confirmed
present on the real request structs by reading `api_op_*.go` directly. A scripted sweep
(diffing every registered op's wire-input struct fields against its botocore/SDK request
shape's top-level members) additionally found two more: `FirewallRuleType` was also missing
from `CreateFirewallRule` specifically (the issue's phrasing named both Create and Update, both
confirmed), and `ListTagsForResource` was missing `MaxResults`/`NextToken` entirely -- every
call silently returned the full unpaged tag list. All fixed; see each op's entry above for
specifics and citations. `DeleteFirewallRule`'s `Qtype` is the one genuine "gate behavior" fix
in this batch (see its ops entry) -- implemented as a precondition on the existing
`(FirewallRuleGroupId, FirewallDomainListId)` lookup rather than a new identity key, since
`api_op_DeleteFirewallRule.go`'s own doc comment does not list `Qtype` as part of the rule's
identity and `UpdateFirewallRule.Qtype` is a genuinely mutable field per its doc comment (not an
identity selector), so relaxing `CreateFirewallRule`'s one-rule-per-domain-list uniqueness to
accommodate a hypothesized multi-rule-per-domain-list model would have made `UpdateFirewallRule`
ambiguous without a matching change there -- flagged as a conservative reading, not asserted as
verified fact, since the SDK doc comments don't fully resolve it either way.

Protocol: awsjson1.1 (single POST, `X-Amz-Target: Route53Resolver.<Op>`). Route matcher
(`RouteMatcher`/`ExtractOperation`) is a simple prefix match/trim and correctly covers all
72 registered ops (68 + the 4 added this pass, see `TestHandlerOpsLen`) -- verified by
iterating `GetSupportedOperations()` against the real SDK's op list, no mismatches.

**2026-07-25 pass: Batch{Create,Update,Delete}FirewallRule + ListFirewallRuleTypes (SDK
bump v1.42.3 -> v1.48.0)**: the SDK bump added four new operations gopherstack had not yet
implemented (`TestSDKCompleteness` caught them). Target strings confirmed against the real
generated `serializers.go`: `Route53Resolver.BatchCreateFirewallRule`,
`Route53Resolver.BatchDeleteFirewallRule`, `Route53Resolver.BatchUpdateFirewallRule`,
`Route53Resolver.ListFirewallRuleTypes`.

*Shared state, not a parallel code path*: `BatchCreateFirewallRule`/`BatchUpdateFirewallRule`/
`BatchDeleteFirewallRule` parse each entry into `createFirewallRuleInput`/
`updateFirewallRuleInput`/`deleteFirewallRuleInput` -- the *exact same* input structs the
singular `CreateFirewallRule`/`UpdateFirewallRule`/`DeleteFirewallRule` ops already parse into
(their field sets are an exact match for `types.{Create,Update,Delete}FirewallRuleEntry` within
this service's existing declared scope) -- and call `handleCreateFirewallRule`/
`handleUpdateFirewallRule`/`handleDeleteFirewallRule` directly, per entry. There is no
independent batch-only validation or storage path to drift out of sync with the singular ops;
a batch entry gets identical validation, identical error codes, and reads/writes the identical
`firewallRules`/`firewallRulesByRegion` store.

*Atomicity -- partial success, not all-or-nothing*: determined from the real output shape
(`BatchCreateFirewallRuleOutput` carries both `CreatedFirewallRules` -- the successful subset --
and `CreateErrors` -- the failed subset, each entry echoed back with `Code`/`Message`), and
confirmed by AWS's own published example response (a 2-entry request where one entry could have
been rejected instead returns `CreateErrors: []` alongside both created rules in a single 200).
Implemented accordingly: each entry is processed independently, in request order, through the
same handler function the singular op uses; a failing entry is recorded in the errors list and
processing continues, with no rollback of entries that already succeeded earlier in the same
batch. `TestBatchCreateFirewallRule`/`TestBatchUpdateFirewallRule`/`TestBatchDeleteFirewallRule`
(`firewall_rules_test.go`) each include a `partial_failure_*` case asserting the valid entry is
both present in the response's `Created/Updated/DeletedFirewallRules` *and* visible via a
follow-up `ListFirewallRules` call against the same backend, proving the write actually landed
in the shared store rather than being rolled back alongside the failed entry.

*Batch size limit*: none found. Checked the API reference pages for all three batch ops (Request
Parameters sections), the `CreateFirewallRuleEntry`/`UpdateFirewallRuleEntry`/
`DeleteFirewallRuleEntry` type pages (which do document field-level `Length Constraints` for
string members, so the docs generator does surface such constraints when AWS publishes them),
the AWS CLI reference page, and the generated `validators.go` (`validateOpBatchCreateFirewallRuleInput`
et al. only check `!= nil`, no length/count check). No documented or SDK-enforced count limit
exists for `CreateFirewallRuleEntries`/`UpdateFirewallRuleEntries`/`DeleteFirewallRuleEntries` --
gopherstack does not fabricate one.

*Entries do not have to share a rule group*: `CreateFirewallRuleEntry`/`UpdateFirewallRuleEntry`/
`DeleteFirewallRuleEntry` each carry their own `FirewallRuleGroupId`; the batch envelope
(`BatchCreateFirewallRuleInput` etc.) holds only the entries list, with no batch-level group ID.
A single batch can therefore legitimately target multiple rule groups; group existence is
validated per-entry (inherited for free from the reused `CreateFirewallRule` path, which already
404s on an unknown group).

*New error class -- `ValidationException`, not this service's usual `InvalidRequestException`*:
every singular Firewall Rule op in this service raises `InvalidRequestException` for a
missing/invalid request field (see `ErrValidation`). The three batch ops are documented
differently: their API reference Errors sections list `AccessDeniedException`,
`InternalServiceErrorException`, `LimitExceededException`, `ThrottlingException`, and
`ValidationException` -- no `InvalidRequestException`. Added a dedicated `ErrBatchValidation`
sentinel (`errors.go`) and a new `handleError` branch (`handler.go`) so a missing/empty
`CreateFirewallRuleEntries`/`UpdateFirewallRuleEntries`/`DeleteFirewallRuleEntries` returns
`ValidationException`, matching the documented behavior instead of silently reusing the wrong
exception type.

*ListFirewallRuleTypes catalog is SDK-derived, not hand-written*: `types.FirewallRuleType` (the
tagged union used by the `FirewallRuleType` member on `Create`/`UpdateFirewallRuleEntry` and on
`types.FirewallRule` itself) has exactly four members --
`DnsThreatProtection`/`FirewallAdvancedContentCategory`/`FirewallAdvancedThreatCategory`/
`PartnerThreatProtection` -- matching the four documented values of
`ListFirewallRuleTypesInput.RuleType`. Of those, only `DnsThreatProtection` has a genuine closed
Go enum backing it (`types.DnsThreatProtection`: `DGA`/`DNS_TUNNELING`/`DICTIONARY_DGA`), reused
directly (`r53rtypes.DnsThreatProtectionDga` etc.) so the catalog can never drift from the real
enum. The other three variants' concrete identifiers
(`FirewallAdvancedContentCategoryConfig.Category`, `FirewallAdvancedThreatCategoryConfig.Category`,
`PartnerThreatProtectionConfig.Partner`) are untyped `*string` in the SDK with no backing enum --
their own doc comments say the only way to learn valid values is to call `ListFirewallRuleTypes`
itself. Rather than invent plausible-looking category/partner names from doc-comment examples
(`VIOLENCE_AND_HATE_SPEECH`, `PHISHING`, ...), gopherstack returns real data for
`DnsThreatProtection` only and an empty result for the other three RuleType filter values. This
was originally treated as the reason for an A -> A- grade change, on the theory that the catalog
was "incomplete" relative to AWS's full four-variant universe.

**parity-5 correction**: re-read `ListFirewallRuleTypes`'s own doc comment -- it retrieves "the
rule-type variants that can be used in the `FirewallRuleType` field of `CreateFirewallRule` and
`UpdateFirewallRule`", i.e. its contract is scoped to *this backend's own creatable set*, not
AWS's global catalog. Since `CreateFirewallRule`/`UpdateFirewallRule` here only ever accept
`DnsThreatProtection` (their input structs have no field at all for the other three variants),
a catalog of exactly `{DnsThreatProtection}` is a complete and honest answer to what the op is
actually documented to report -- not a partial one. The remaining gap is properly scoped to
`CreateFirewallRule`/`UpdateFirewallRule`'s creation surface (see `gaps`), not to this read-only
listing op, and the grade has been raised back to A on that basis. While re-verifying this,
found and fixed a real, independent bug the old "structural, A-" framing had been sitting next
to without catching: `ConfidenceThreshold` (closed `LOW`/`MEDIUM`/`HIGH` enum, required at
creation for a `DnsThreatProtection` rule per `CreateFirewallRuleInput`'s doc comment) was
accepted completely unvalidated on both `CreateFirewallRule` and `UpdateFirewallRule` -- a real
SDK client omitting it, or sending garbage, was silently accepted. Fixed in
`firewall_rules.go` (`validateFirewallRuleConfidenceThreshold`/`validateConfidenceThreshold`).

**Timestamps**: All Route53Resolver `*Time` fields are ISO8601 strings (RFC3339 via
`currentTime()` in backend.go), matching the real SDK's `*string` (not epoch-number) fields.
Confirmed against `aws-sdk-go-v2/service/route53resolver/types` -- every `CreationTime` /
`ModificationTime` field is typed `*string`, not `*time.Time`/epoch. Do not "fix" these to
epoch numbers in a future pass.

**"OwnerID" vs "OwnerId" wire-key bug (bug class, 5 fixes)**: this service's awsjson1.1
deserializer (verified by reading the real SDK's generated `deserializers.go`) decodes the
response body into `map[string]interface{}` and then does an *exact*, case-sensitive
`switch key { case "OwnerId": ... }` -- it does NOT go through `encoding/json`'s
case-insensitive struct-tag matching. A response field literally spelled `"OwnerID"`
(capital D) is a silent no-op on the client: the map key never matches the switch case, so
the SDK's `OwnerId` field is left nil. This is a trap for future auditors here: struct-tag
JSON casing bugs in awsjson1.1 services are NOT automatically forgiven by Go's usual
case-insensitive unmarshal behavior, because the SDK uses a hand-rolled decoder, not
`encoding/json` unmarshal-into-struct. Always grep the real `deserializers.go` for the exact
`case "..."` string, don't assume "close enough" casing is fine. Fixed in:
`firewallRuleGroupOutput.OwnerID`, `resolverQueryLogConfigOutput.OwnerID`,
`firewallConfigOutput.OwnerID`, `resolverConfigOutput.OwnerID`,
`resolverDnssecConfigOutput.OwnerID`, `resolverRuleOutput.OwnerID` (6 structs, all now tag
`json:"OwnerId"` / `json:"OwnerId,omitempty"`).

**Disassociate-by-composite-key bug class (2 fixes, both critical)**: `DisassociateResolverRule`
and `DisassociateResolverQueryLogConfig` do NOT take the opaque association ID that their
sibling `Get*Association`/`List*Associations` ops return. The real API requires the caller to
re-supply the *original* identifying pair instead:
- `DisassociateResolverRule`: `ResolverRuleId` + `VPCId` (verified against
  `api_op_DisassociateResolverRule.go` -- both `This member is required`)
- `DisassociateResolverQueryLogConfig`: `ResolverQueryLogConfigId` + `ResourceId` (verified
  against `api_op_DisassociateResolverQueryLogConfig.go`)

Before this fix, gopherstack's handlers expected `ResolverRuleAssociationId` /
`ResolverQueryLogConfigAssociationId` fields that a real SDK client never sends -- every real
`DisassociateResolverRule`/`DisassociateResolverQueryLogConfig` call from an actual
`aws-sdk-go-v2` client would hit gopherstack's own "field is required" `InvalidRequestException`
100% of the time. Unit tests didn't catch this because they hand-built the (wrong) request body
to match the handler's existing (wrong) expectations -- exactly the "unit tests are not parity
proof" trap called out in parity-principles.md #3. Both backend methods now look up the matching
association by scanning `*ByRegion` index for the (ruleID/configID, vpcID/resourceID) pair,
matching real AWS semantics where the pair is the effective identity of the association.
Note the asymmetry is real, not a typo in this codebase: `DisassociateFirewallRuleGroup` and
`UpdateFirewallRuleGroupAssociation` DO correctly use `FirewallRuleGroupAssociationId` --
verified against the real SDK, this one association type keeps the opaque ID symmetric between
Associate/Get/Update/Disassociate. Don't "fix" it to match the Resolver-rule/query-log pattern.

**Missing Tags on CreateResolverRule / AssociateFirewallRuleGroup (2 fixes)**: both real API
inputs carry a `Tags []types.Tag` field (verified against `api_op_CreateResolverRule.go` and
`api_op_AssociateFirewallRuleGroup.go`); gopherstack's input structs omitted it entirely, so
tags supplied on these two specific calls were silently discarded (never landed in the tags
store, never visible via `ListTagsForResource`). All other Tag-bearing create/associate ops
(`CreateResolverEndpoint`, `CreateFirewallRuleGroup`, `CreateFirewallDomainList`,
`CreateOutpostResolver`, `CreateResolverQueryLogConfig`) already handled this correctly and were
used as the template for the fix.

**FirewallDomainList missing timestamps**: the real `types.FirewallDomainList` has
`CreationTime`/`ModificationTime`/`StatusMessage`, but the backend struct never had storage for
them -- every Get/Create/Delete/Update/Import response silently returned them empty forever
(not a "wrong value" bug, a "field literally never existed" bug). Added the fields, set on
create, bumped on `UpdateFirewallDomains`/`ImportFirewallDomains`.

**Invented `ResolverEndpoint.IpAddresses` response field (2026-07-24 pass, critical-class)**:
`resolverEndpointOutput` (the wire shape behind `CreateResolverEndpoint`,
`GetResolverEndpoint`, `ListResolverEndpoints`, `UpdateResolverEndpoint`,
`AssociateResolverEndpointIpAddress`, `DisassociateResolverEndpointIpAddress`) carried an
`IpAddresses` list. The real `types.ResolverEndpoint` (verified against
`aws-sdk-go-v2/service/route53resolver/types/types.go`) has **no such field** -- only
`IpAddressCount int32`. IP addresses are only obtainable via the separate
`ListResolverEndpointIpAddresses` call. There was even a dedicated unit test
(`TestResolverEndpoint_IPv6IPAddress` et al.) asserting the invented field's presence --
exactly the "unit tests are not parity proof" trap: the tests were written against gopherstack's
own (wrong) shape, not the real one. Harmless to real SDK clients in practice (unknown-field-
tolerant decoders ignore extra map keys), but still a fabricated field per the no-invented-shape
rule -- deleted. Added the two real-but-missing `ResolverEndpoint` fields while here:
`RniEnhancedMetricsEnabled`/`TargetNameServerMetricsEnabled` (settable on Create/Update,
verified against `api_op_CreateResolverEndpoint.go`/`api_op_UpdateResolverEndpoint.go`).

**Invented `FirewallRule.Id`/`.Arn` + wrong Delete/Update addressing (2026-07-24 pass,
CRITICAL, same bug class as the DisassociateResolverRule/DisassociateResolverQueryLogConfig
writeup above, but missed by the prior pass)**: the real `types.FirewallRule` (verified against
`types/types.go`) has **no `Id` or `Arn` member at all** -- a firewall rule has no independent
identity on the wire. It is addressed by the `(FirewallRuleGroupId, FirewallDomainListId)` pair
it was created with (verified against `api_op_DeleteFirewallRule.go` and
`api_op_UpdateFirewallRule.go`, neither of which has a `FirewallRuleId` member -- `Delete`
requires `FirewallRuleGroupId` + optional `FirewallDomainListId`/`FirewallThreatProtectionId`;
`Update` requires the same pair). gopherstack invented `Id`/`Arn` fields on the response *and*
required a `FirewallRuleId` on `DeleteFirewallRule`/`UpdateFirewallRule` requests -- a field a
real SDK client never sends. Every real `DeleteFirewallRule`/`UpdateFirewallRule` call would
have been rejected with gopherstack's own "field is required" `InvalidRequestException` 100% of
the time. There was even a dedicated test (`TestCreateFirewallRule_IdAndArnInOutput`) asserting
the invented fields' presence -- same trap as the ResolverEndpoint bug above. Fixed by:
removing `Id`/`Arn` from `firewallRuleOutput`; changing `DeleteFirewallRule`/`UpdateFirewallRule`
to take `FirewallRuleGroupId`+`FirewallDomainListId` and resolving the rule via a new
`findFirewallRule` composite-key lookup (mirrors the `DisassociateResolverRule` fix pattern);
enforcing `(FirewallRuleGroupId, FirewallDomainListId)` uniqueness on `CreateFirewallRule` so
that lookup is always unambiguous; and removing `FirewallDomainListId` from
`UpdateFirewallRuleParams`'s mutable fields (it's part of the rule's identity, not editable --
verified `UpdateFirewallRuleInput.FirewallDomainListId` doc: "The ID of the domain list to use
in the rule" is actually the *selector*, not a retarget, since there's no other way to identify
which rule to update). The internal `FirewallRule.ID`/`.ARN` fields remain for store-indexing
purposes only -- they were never wire-visible after this fix and never should be.

**`BlockOverrideDnsType`/`BlockOverrideTtl` wire-key casing bug (2026-07-24 pass, same bug
class as the OwnerID/OwnerId note above)**: `firewallRuleOutput` used
`json:"BlockOverrideDNSType"` / `json:"BlockOverrideTTL"`; the real hand-rolled awsjson1.1
deserializer (verified via `grep -A60 deserializeDocumentFirewallRule deserializers.go`) does
exact-case `case "BlockOverrideDnsType":` / `case "BlockOverrideTtl":` matching. Real SDK
clients would have silently never seen these two fields populated on `CreateFirewallRule` /
`UpdateFirewallRule` / `DeleteFirewallRule` / `ListFirewallRules` responses. Fixed.

**`ListFirewallRules` missing Action/Priority filters**: `ListFirewallRulesInput` has optional
`Action`/`Priority` filter fields (verified against `api_op_ListFirewallRules.go`); gopherstack
silently ignored both. Added.

**`USE_LOCAL_RESOURCE_SETTING` enum gap (Route 53 Profiles feature)**: `FirewallFailOpenStatus`,
`AutodefinedReverseFlag`/`ResolverAutodefinedReverseStatus`, and `Validation`/
`ResolverDNSSECValidationStatus` all gained a third `USE_LOCAL_RESOURCE_SETTING` value in the
real SDK (verified against `types/enums.go`) on top of the original ENABLE(D)/DISABLE(D) pair --
it defers the setting to whatever a Route 53 Profile attached to the VPC specifies.
`UpdateFirewallConfig`/`UpdateResolverConfig`/`UpdateResolverDnssecConfig` previously rejected
this value with a validation error. Added support: `FirewallFailOpen`/`AutodefinedReverse`
pass the literal value straight through (matching their existing no-intermediate-state
behavior); DNSSEC's `Validation` transitions to `UPDATING_TO_USE_LOCAL_RESOURCE_SETTING`,
mirroring the pre-existing ENABLE/DISABLE -> ENABLING/DISABLING transient-status pattern.

**Not bugs (verified correct, don't re-flag)**:
- Every Create* op (`CreateResolverEndpoint`, `CreateResolverRule`, `CreateFirewallRuleGroup`,
  etc.) transitions straight to its terminal status (`OPERATIONAL`/`COMPLETE`/`CREATED`)
  instead of an intermediate `CREATING` state. This is the *opposite* of the "stuck CREATING
  forever" anti-pattern the audit brief warns about -- it means clients never need to poll, and
  is intentional/harmless for a synchronous mock backend.
- CLOSED 2026-08-13: `resolverConfigOutput`/`firewallConfigOutput` no longer carry the extra
  `Arn` field the real API type lacks -- see the `GetResolverConfig`/`GetFirewallConfig` ops
  entries and the matching `gaps` entry above for the SDK citation and regression tests.
- `GetFirewallRuleGroupPolicy`/`GetResolverQueryLogConfigPolicy`/`GetResolverRulePolicy`
  returning `""` for an unset policy rather than erroring -- reasonable mock behavior for a
  void-result-style read, matches the "empty envelope after real backend logic is correct"
  guidance in parity-principles.md #4.

## 2026-08-29: write-only-state follow-up sweep (gopherstack-6flj)

Method: for each domain struct in `models.go`, enumerated stored fields and checked
which real op can read them back; then diffed every family's real SDK type
(`ResolverEndpoint`, `ResolverRule`+`TargetAddress`, `FirewallRuleGroup`,
`FirewallRuleGroupAssociation`, `FirewallDomainList`, `FirewallRule`, `OutpostResolver`,
`ResolverQueryLogConfigAssociation`, `FirewallConfig`, `ResolverConfig`,
`ResolverDnssecConfig`) field-for-field against `types/types.go` and each type's own
`awsAwsjson11_deserializeDocument<Type>` case list (exact-case keys, per this service's
hand-rolled awsjson1.1 decoder -- see the OwnerID/OwnerId note above).
`enumcheck`/`acceptguard`/`zeroguard`/`xmlitemwrap` (repo-wide, grepped for this service)
found nothing.

**`TargetAddress.ServerNameIndication` silently dropped (both directions):** verified
against `types/types.go:1682` and both `serializers.go:4838`
(`awsAwsjson11_serializeDocumentTargetAddress`, request-side) and `deserializers.go:13705`
(`awsAwsjson11_deserializeDocumentTargetAddress`, response-side) -- a real, always-real
field ("The Server Name Indication of the DoH server that you want to forward queries to.
This is only used if the Protocol of the TargetAddress is DoH"). Neither gopherstack's
`targetIP` wire struct (`handler_resolver_rules.go`) nor its `TargetIP` domain model
(`models.go`) had a field for it at all -- a real SDK client setting
`TargetIps[].ServerNameIndication` on `CreateResolverRule`/`UpdateResolverRule` had the
value accepted (unknown-field-tolerant JSON decode) and discarded; `GetResolverRule`/
`ListResolverRules` never echoed it back. This is the "accepted from a request and never
stored" write-only-state pattern, one level deeper than the top-level fields this
campaign's earlier passes checked -- `ResolverRule` itself was already clean, but its
nested `TargetAddress` member type was not independently re-verified until this pass.
Fixed: added `ServerNameIndication` to both structs (same field order, so the existing
`targetIP(t)`/`TargetIP(t)` typed conversions still compile). Proven by
`TestResolverRule_TargetIps_ServerNameIndicationRoundTrip`
(`wire_field_fixes_test.go`) -- a real `aws-sdk-go-v2` client sets it on
`CreateResolverRule` and reads it back via `GetResolverRule`; hand-reverted (confirmed
failing against `HEAD`), restored.

**`OutpostResolver.CreationTime`/`ModificationTime`/`StatusMessage` never tracked at
all:** verified against `types/types.go:1078` and `deserializers.go:12034`
(`awsAwsjson11_deserializeDocumentOutpostResolver`, 11 cases: `Arn`, `CreationTime`,
`CreatorRequestId`, `Id`, `InstanceCount`, `ModificationTime`, `Name`, `OutpostArn`,
`PreferredInstanceType`, `Status`, `StatusMessage`). gopherstack's `OutpostResolver`
domain model and `outpostResolverOutput` wire struct had neither timestamp field at all
-- every `Create`/`Get`/`List`/`Update`/`Delete` response left them permanently empty
regardless of backend state, the same "field literally never existed" class already
fixed for `FirewallDomainList` (see the 2026-07-24-era note above). Fixed:
`CreationTime`/`ModificationTime` set at creation (`currentTime()`, same convention as
every other family), `ModificationTime` bumped on `UpdateOutpostResolver`. `StatusMessage`
is wired through but genuinely dormant -- this backend's Outpost Resolver `Status`
transitions straight to `OPERATIONAL` synchronously and never produces an
error/detail message, so no code path yet writes a non-empty value; same reasoning as
the pre-existing `ResolverRuleAssociation.StatusMessage` dormant fix. Proven by
`TestOutpostResolver_TimestampsRoundTrip` (`wire_field_fixes_test.go`) for the two
timestamps; hand-reverted (confirmed failing), restored.

**Confirmed clean by this pass's re-derivation** (not re-litigating prior passes, but
independently re-checked field-for-field against the same pinned SDK):
`FirewallRuleGroup` (11 fields), `FirewallRuleGroupAssociation` (13 fields, `StatusMessage`
already present), `FirewallDomainList` (12 fields, `Category`/`ManagedListType`
structurally absent per the existing gap), `FirewallRule` (20 fields, `Status`/
`StatusMessage`/`Id`/`Arn` correctly absent, matching the pre-existing disclosed note),
`ResolverQueryLogConfigAssociation` (7 fields), `FirewallConfig`/`ResolverConfig`/
`ResolverDnssecConfig` (4 fields each, no `Arn` on any of the three, matching the
2026-08-13 fix).

## 2026-08-30 (wrapper-key sweep): exhaustive request-field-read audit, no new bugs

Method: derived the operation list from the 13 `opsXxx()` map-literal registrations
(`buildOps`, handler.go) rather than trusting this file's prose -- 69 real operations
(the ALL_CAPS strings alongside them, e.g. `"DOMAIN_NAME"`/`"TYPE"`, are filter-name
enum values consumed by `list_filters.go`'s alias tables, not operation names; excluded).
For every `*Input` request struct across every non-test `.go` file, cross-referenced each
JSON-tagged field against a combined-text search of the whole non-test package for
`.FieldName` usage anywhere. Confirmed protocol directly from the pinned SDK:
`awsAwsjson11_*` prefix throughout `route53resolver@v1.48.4/deserializers.go` -- plain
JSON-RPC 1.1 over `X-Amz-Target`, no legacy/query path for this service.

**Result: zero unread request fields found.** Read `ListResolverRules` end-to-end
(`handler_resolver_rules.go`, `list_filters.go`) as a representative filter+pagination op:
`Filters`/`NextToken`/`MaxResults` are all consumed, filtering happens strictly before
`paginate()` (matching the "filter, then paginate" rule), and an unrecognized `Filter.Name`
is rejected with `InvalidParameterException` rather than silently ignored (`applyFilters`,
`list_filters.go:64-67`) -- matches the real op's modelled error, not fabricated. Its filter
resume-cursor (`b.rulesByRegion`, a `*store.Index[ResolverRule]`) is `Index.Get()`,
documented (pkgs memory) as insertion-ordered/stable -- a tie-prone sort (by `Name`, not
unique) over this call-stable input needs no added tiebreak, consistent with this file's own
2026-08-30 pagination-tie-sweep entry above having found no bug on the `Name`-sorted List ops
for the identical reason.

**Negative checks, explicitly:**
- **Listing that never consults its store**: one apparent candidate,
  `ListFirewallRuleTypes` (`handler_firewall_rules.go:747`), which never calls `h.Backend`.
  Confirmed NOT a bug: real AWS's `ListFirewallRuleTypes` returns a fixed AWS-managed
  catalog of DNS-threat-protection rule types, not account-specific data (the same shape
  as e.g. RDS's `DescribeDBEngineVersions` defaults), and gopherstack backs it with a real
  populated `firewallRuleTypeCatalog()` (with working `RuleType` filter + pagination), not
  an empty stub. Every other `handle(List|Get)*` reaches `h.Backend.*`.
- **Handler that discards its entire request**: none -- every `handle*(ctx, in *Type)`
  function references at least one `in.Field`, scripted check across every `handler_*.go`,
  zero exceptions.

No code changed this pass. Gates: `go build ./services/route53resolver/...`, `go vet
./services/route53resolver/...` and `go vet ./...` (repo-wide), `go test -race -count=1
./services/route53resolver/...`, `golangci-lint run ./services/route53resolver/...`.

## 2026-08-30 (gopherstack-4shm WrapOp request-field re-scan, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

This service dispatches every op through `service.WrapOp`, assembled from
13 per-family `map[string]service.JSONOpFunc` literals merged in
`buildOps()` (`handler.go`). A field scan anchored on literal decode calls
alone -- what earlier passes reporting "0 of N request shapes flagged" ran
-- resolves **0 of 72 operations (0%)**: this service was entirely
invisible to that method, gopherstack-4shm's exact class, and any prior
"zero misread keys" verdict measured against it was measuring nothing.

The new `cmd/reqfieldscan` tool (resolves `WrapOp`'s second type parameter
directly from each handler's own signature, falling back to a
case-insensitive `handle` + opName match for the 3 ops whose Go handler
name capitalizes an AWS acronym the operation name itself does not --
`handleAssociateResolverEndpointIPAddress` for
`AssociateResolverEndpointIpAddress`, etc.) reaches **72 of 72 (100%)**,
213 fields across 70 distinct request types.

**Result: zero unread fields.** The earlier field-sweep's "no misread
keys" verdict, and this file's own "handler that discards its entire
request: none" negative check above, both hold under the corrected
WrapOp-aware scan -- the blind spot was in measurement coverage, not in
this service's own handlers.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run`
-- all clean (`./services/route53resolver/...` and
`./cmd/reqfieldscan/...`). No code changed in this service this pass.

## 2026-08-31 (value-semantics pass, gopherstack-uox6): clean, no code changed

Targeted this service for bd `gopherstack-uox6`'s class -- a filter that is
read and applied but implements the wrong semantics (a negation prefix taken
literally, a documented default silently widened when omitted, a comparison
one value off from what "inclusive"/"greater than" documents, etc) --
invisible to the field-read/enum/wire-key scanners this service's prior
passes already ran clean under. This axis (behaviour, not shape) was
explicitly unexamined before this pass.

Checked every optional filter across all 12 List operations against
`types.Filter`'s own doc comment (`aws-sdk-go-v2/service/route53resolver@
v1.48.4 types/types.go`) and each operation's own input struct, operation by
operation rather than trusting a sibling's verdict (the `PatchOrchestratorFilter`
failure mode this class has produced elsewhere):

- The five `types.Filter`-based ops (`ListResolverEndpoints`,
  `ListResolverRules`, `ListResolverRuleAssociations`,
  `ListResolverQueryLogConfigs`, `ListResolverQueryLogConfigAssociations`):
  every documented `Name` value for every op is matched by field, by exact
  equality/membership (`slices.Contains`/`containsAny`), with no operator
  grammar, no wildcard, no negation, and no range/date filter documented
  anywhere in `types.Filter` -- so those sub-shapes of this bug class are
  structurally absent here, not merely unaudited. `list_filters.go`'s
  `applyFilters` combining rule (AND across filters, OR within one filter's
  `Values`) matches the standard AWS list-filter convention and is shared
  correctly by all five. An empty `Values` list matching nothing (rather than
  degrading to "no filter") is the documented-absent case correctly handled
  conservatively, per that function's own doc comment.
- `ListFirewallRuleTypes`'s `RuleType` ("An optional filter... If omitted,
  definitions across all variants are returned") -- `handler_firewall_rules.go`
  correctly returns the full catalog when `in.RuleType == ""` and narrows only
  when set.
- `ListFirewallRuleGroupAssociations`'s `Status` ("If you don't specify this,
  then DNS Firewall returns all associations, regardless of status"),
  `Priority`, `FirewallRuleGroupId`/`VpcId` ("Leave this blank to retrieve
  associations for any [rule group/VPC]") -- all four correctly no-op on
  their zero value, in both the handler (`Status`/`Priority`) and the backend
  (`VpcId`/`FirewallRuleGroupId`), confirmed by reading
  `ListFirewallRuleGroupAssociations` (`firewall_rule_groups.go`) end to end.
- `ListFirewallRules`'s `Action`/`Priority` ("Optional additional filter") --
  both correctly no-op on absence in `handleListFirewallRules`.
- `ListOutpostResolvers`'s `OutpostArn` -- no omission language in the SDK
  doc, and the handler correctly treats `""` as no filter.
- `ListResolverConfigs`/`ListFirewallConfigs`/`ListFirewallDomainLists`/
  `ListFirewallDomains`/`ListResolverEndpointIpAddresses`/
  `ListTagsForResource`: no filter parameter beyond pagination in the pinned
  SDK's own input struct (checked field-by-field, not assumed) -- structurally
  outside this bug class's surface.
- `ListResolverDnssecConfigs` takes `types.Filter` but the SDK doesn't
  document any valid `Name` for it (absent from both `types.Filter`'s
  per-operation enumeration and AWS's own API reference page for this op) --
  `matchNoDnssecConfigFilter` rejecting every filter name via `applyFilters`'s
  existing unrecognized-name path is the correct, already-in-place behaviour,
  not a gap.

No bug found. No web page fetched this pass -- everything resolved from the
pinned `aws-sdk-go-v2/service/route53resolver@v1.48.4` module cache. No code
changed in this service.

Gates: `go build`, `go vet ./...` (repo-wide), `go test -race -count=1
./services/route53resolver/...`, `golangci-lint run
./services/route53resolver/...` -- all clean.

## 2026-08-31 error-target audit (`cmd/errtargetaudit`, gopherstack-6flj/uox6)

`go run ./cmd/errtargetaudit -dir route53resolver` reported 32 class A
findings (a real, correctly-spelled error code sent to an operation whose
own SDK deserializer doesn't declare it) before this pass, 0 after. Protocol
confirmed as `awsAwsjson11` (`func awsAwsjson11_deserializeOpError<Op>`
switches in `deserializers.go`) -- the older per-op switch shape, not the
newer `rpc2_deserializeOpError<Op>` shape.

**Root cause, all 32 findings:** `ErrValidation` (`errors.go`) is a shared
sentinel mapping to `InvalidRequestException`, correct for the singular
Resolver* ops. But the entire Firewall/Outpost op family (and
`GetResolverConfig`) uses `ErrValidation` too, even though their own
deserializers model `ValidationException` instead -- confirmed per-op by
reading each op's `awsAwsjson11_deserializeOpError<Op>` switch, not assumed
from the family pattern. This matches the pre-existing family-split comment
in `handler.go`'s `handleError` (which was actually only half-applied in
code before this pass) and extends `ErrBatchValidation` -- previously scoped
in its doc comment to just the three `Batch*FirewallRule` ops -- to its true,
now-verified scope (see the corrected comment in `errors.go`).

**Fix shape: did not touch `ErrValidation` itself** (still correct for every
Resolver* caller -- re-confirmed `AssociateResolverRule`,
`CreateResolverEndpoint`, `CreateResolverQueryLogConfig`,
`AssociateResolverQueryLogConfig`, `DisassociateResolverQueryLogConfig`,
`UpdateResolverConfig`, `GetResolverDnssecConfig`,
`UpdateResolverDnssecConfig` all still declare `InvalidRequestException`).
Overrode at each Firewall/Outpost/GetResolverConfig call site instead,
swapping to the already-existing `ErrBatchValidation` sentinel
(`ValidationException`): `firewall_rules.go`, `handler_firewall_rules.go`,
`firewall_rule_groups.go`, `handler_firewall_rule_groups.go`,
`firewall_domain_lists.go`, `handler_firewall_domain_lists.go`,
`firewall_configs.go`, `handler_outpost_resolvers.go`, and
`handler_configs.go`'s `getSimpleConfig`/`updateSimpleConfig` call sites
(added a `validationErr error` parameter to `requireResourceID`/
`getSimpleConfig`/`updateSimpleConfig` in `handler.go` so the three bare-
resourceID config families -- FirewallConfig/ResolverConfig needing
`ErrBatchValidation`, ResolverDnssecConfig needing `ErrValidation` -- can
each pass their own correct sentinel through the shared helper instead of
one hardcoded default). `firewallRuleBatchErrorCode`'s per-item batch error
`Code` field (`handler_firewall_rules.go`) was also corrected from
`"InvalidRequestException"` to `"ValidationException"` for the same
`ErrBatchValidation` case, so a failing entry inside
BatchCreate/Update/DeleteFirewallRule reports the same code its standalone
counterpart now does.

**One non-family-split bug, also fixed:** `CreateFirewallRule`'s
`validateFirewallRuleDomainListUnique` (`firewall_rules.go:209`) used
`ErrAlreadyExists` (`ResourceExistsException`) for a duplicate
(FirewallRuleGroupId, FirewallDomainListId) pair. `ResourceExistsException`
is exclusively a Resolver*-association error in this SDK (confirmed: only
`AssociateResolverEndpointIpAddress`, `AssociateResolverQueryLogConfig`,
`AssociateResolverRule`, `CreateResolverEndpoint`,
`CreateResolverQueryLogConfig`, `CreateResolverRule` declare it) --
`CreateFirewallRule` doesn't. Swapped to `ErrBatchValidation`. This was
`ErrAlreadyExists`'s only production call site in the entire service; it is
now unused as a producer (its one remaining reference is a now-dead
defensive case in `firewallRuleBatchErrorCode`, left in place -- harmless,
and `ResourceExistsException` genuinely going unmodeled for the six
Resolver* association/creation ops that could produce it is a separate,
larger gap: those ops have no duplicate-detection logic at all, which is a
"never emits a code it should" gap, not this pass's "emits a code it
shouldn't" class -- not fixed here, recorded for a future pass).

**Six findings were refused a code swap because no declared type fits**
(the operation's own model declares no type for the condition): an empty
required ID/ARN reaching `GetFirewallDomainList`, `DeleteFirewallDomainList`,
`GetFirewallRuleGroup`, `GetFirewallRuleGroupAssociation`, and
`GetResolverRuleAssociation` -- none of these five declare
`ValidationException` *or* `InvalidRequestException` in their real
deserializers. For all five the handler-level pre-check was removed
entirely rather than reclassified: each backend `Get`/`Delete` call already
does a natural map lookup by ID that misses on an empty string and returns
`ErrNotFound` (`ResourceNotFoundException`), a type every one of the five
does declare -- confirmed by reading each backend method
(`firewall_domain_lists.go`, `firewall_rule_groups.go`,
`rule_associations.go`). `GetResolverRulePolicy`/`PutResolverRulePolicy`
had no such natural fallback (their backends are blind map
read/write with no not-found path), so their empty-`Arn` checks were
instead pointed at `ErrInvalidParameter` (`InvalidParameterException`,
"One or more parameters in this request are not valid" -- both ops declare
it, confirmed).

**Reachability note:** as with xray's `PutResourcePolicy` finding this same
pass, `validateOp<Op>Input` in the pinned SDK's `validators.go` only checks
`!= nil` for these required string fields, never non-empty -- so
`aws.String("")` passes client-side validation and every one of these paths
is genuinely reachable by a real, correctly-signed client, not just by raw
HTTP.

Zero web pages fetched this pass -- everything resolved from the pinned
`aws-sdk-go-v2/service/route53resolver@v1.48.4` module cache.

Gates: `go build ./...`, `go vet ./...` (repo-wide), `go test -race
-count=1 ./services/route53resolver/...`, `golangci-lint run
./services/route53resolver/...` -- all clean (see session notes for exact
output).

## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in
`ef0eef041`. Built the unpatched `cmd/reqfieldscan`/`cmd/reqfielddiff` from
`ef0eef041~1` in a worktree, ran both five times against this package, and
diffed against HEAD.

`cmd/reqfieldscan`: byte-identical across all 5 old runs and HEAD (72/72
dispatch coverage, no unread fields). `cmd/reqfielddiff`: 0 findings in
every one of the 5 old runs and at HEAD (72 SDK operations resolved, 843
emulator-declared fields, no undeclared SDK input fields). ZERO DAMAGE.

## 2026-09-04: association-duplicate-detection sweep (parity-sweep-2026-09-03)

Followed up on the 2026-08-31 error-target audit's disclosed-not-fixed gap:
"ResourceExistsException genuinely going unmodeled for the six Resolver*
association/creation ops that could produce it... those ops have no
duplicate-detection logic at all." Scoped this pass to the two ops with an
unambiguous, already-established duplicate identity (the same pair their own
Disassociate op already keys on), rather than the three Create* ops whose
CreatorRequestId idempotency semantics are not spelled out in the SDK doc
comments and would require inventing a matching-vs-conflicting-parameters
rule not verifiable from the model (left as a gap, see below).

**AssociateResolverRule** (`rule_associations.go`): calling it twice with the
same (ResolverRuleId, VPCId) silently created two associations instead of
being rejected. `AssociateResolverRule`'s own deserializer models
`ResourceExistsException` (confirmed against `deserializers.go`'s
`awsAwsjson11_deserializeOpErrorAssociateResolverRule`), and the AWS API
reference's Errors section (fetched this pass, not in the SDK doc comment)
states: "ResourceExistsException: The resource that you tried to create
already exists." `DisassociateResolverRule` already treats
(ResolverRuleId, VPCId) as this association's identity. Added a duplicate
check before creating the association.

**AssociateResolverQueryLogConfig** (`query_log_associations.go`): same bug,
same fix shape, keyed on (ResolverQueryLogConfigId, ResourceId) --
`DisassociateResolverQueryLogConfig`'s existing identity pair. Confirmed
`ResourceExistsException` modelled on this op's deserializer too; AWS API
reference Errors section quotes the identical sentence.

**Found while wiring the fix: `handler.go`'s central `handleError` switch had
no `case` for `ErrAlreadyExists` at all** -- it fell through to the default
`InternalServiceErrorException`/500 branch. This was invisible before this
pass because the 2026-08-31 error-target audit had removed `ErrAlreadyExists`'s
only production call site (`CreateFirewallRule`'s duplicate-domain-list check,
swapped to `ErrBatchValidation` since `ResourceExistsException` doesn't apply
to that op), so the sentinel had zero live producers reaching `handleError`
between that pass and this one. Added the missing `case errors.Is(err,
ErrAlreadyExists)` branch (ResourceExistsException/400, consistent with the
AWS reference's "HTTP Status Code: 400").

Not fixed, left as a gap: `CreateResolverEndpoint`/`CreateResolverQueryLogConfig`/
`CreateResolverRule` also model `ResourceExistsException`, and all three
accept a required `CreatorRequestId` ("allows failed requests to be retried
without the risk of running the operation twice") that gopherstack stores
but never checks for reuse. The idempotent-retry-vs-conflicting-duplicate
distinction real AWS applies is not stated in the SDK doc comments for these
three ops, and getting it wrong (e.g. treating every reused CreatorRequestId
as an error, breaking legitimate retries) would be worse than the current
honest gap. Needs a real verified source before it's implemented.

New test: `TestAssociateDuplicate_ResourceExistsException_RealClient`
(`error_target_fixes_test.go`), table-driven over both ops, driving the real
aws-sdk-go-v2 client so the assertion (`errors.As` into
`*types.ResourceExistsException`) proves both the backend duplicate check
and the `handleError` dispatch fix together.

Gates: `go build`, `go test -race -count=1 ./services/route53resolver/...`,
`golangci-lint run ./services/route53resolver/...` -- all clean (0 lint
issues, baseline was also 0 before this pass).
