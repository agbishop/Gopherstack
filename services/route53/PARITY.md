---
service: route53
sdk_module: aws-sdk-go-v2/service/route53@v1.65.6
last_audit_commit: ee7d2bae
last_audit_date: 2026-07-23
overall: A          # this pass: closed BOTH tracked gaps (AssociateVPCWithHostedZone
                    # duplicate-VPC idempotency, CreateReusableDelegationSet HostedZoneId
                    # mode) and 3 of the 4 deferred items — CreateKeySigningKey InvalidKMSArn
                    # validation, alias-cycle depth-guard stress tests, and a genuine
                    # routing-policy bug found while re-deriving TestDNSAnswer's selection
                    # algorithm: GeoProximityLocation- and CidrRoutingConfig-routed record
                    # sets were never recognised by classifyRouting at all and silently fell
                    # through to plain first-by-SetIdentifier answers. Implemented real
                    # selectGeoProximity (bias-scaled great-circle distance) and selectCIDR
                    # (longest-prefix-match against CIDR collection locations, "*" default
                    # fallback) routing. Also removed all 6 banned cyclop/gocognit/funlen
                    # nolints in the service by decomposition (map-dispatch table for
                    # selectAnswer's routing-kind switch, extracted validate*/merge*/apply*
                    # helpers preserving exact error/precedence order). Ran the SDK-driven
                    # route53/route53resolver integration test suite against the real
                    # aws-sdk-go-v2 client (Dockerized binary) — all 45 tests pass, closing
                    # the prior pass's "unit tests only" gap.
ops:
  CreateHostedZone: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: CallerReference reuse with different Name/Comment/PrivateZone now returns HostedZoneAlreadyExists (409) instead of silently returning the wrong zone; fixed this pass: DelegationSetId was parsed off the wire and then silently dropped — every zone got the same hardcoded default name servers regardless of what was requested. Now accepts a reusable delegation set (bare or /delegationset/-prefixed ID), validates it exists (NoSuchDelegationSet), and both the CreateHostedZone/GetHostedZone DelegationSet response element and the zone's auto-seeded NS/SOA records use the linked set's real name servers"}
  DeleteHostedZone: {wire: ok, errors: ok, state: ok, persist: ok}
  GetHostedZone: {wire: ok, errors: ok, state: ok, persist: ok, note: "DelegationSet response element now reflects the zone's actual linked reusable delegation set (Id + NameServers) instead of always the fixed default pair — see CreateHostedZone"}
  ListHostedZones: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-r80d) — Marker, a required output member (api_op_ListHostedZones.go: 'the value that you specified for the marker parameter in the request that produced the current response'), was never echoed back; the response struct only carried the optional NextMarker (next-page cursor). Prior wire: ok was false — see 2026-08-14 pass. FIXED (2026-08-29 list-filter-params pass) — DelegationSetId and HostedZoneType, both real query-bound filters (api_op_ListHostedZones.go), were never read by the handler at all; every call returned every zone regardless. Now filters by the zone's stored DelegationSetID/PrivateZone. FIXED (2026-08-30 wrapper-key-sweep pagination-reproducibility pass) — pagination was not reproducible across calls: source is b.zones.All() (store.Table map walk, unspecified order) and the result was sorted only by Name, which real Route53 allows to repeat across distinct hosted zones (distinct CallerReference, same domain name -- CreateHostedZone/matchExistingHostedZone only reject a CallerReference collision, never a bare name collision). Paging in small windows dropped or duplicated a same-named zone at the page boundary between two otherwise-identical calls. Fixed by tiebreaking on ID (the zone's own store.Table key) after Name, matching ListHostedZonesByName's existing Name-then-ID order. See TestListHostedZones_PaginationStableAcrossDuplicateNames (list_hosted_zones_pagination_test.go), hand-reverted to confirm it fails against the unfixed sort, then restored."}
  ListHostedZonesByName: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateHostedZoneComment: {wire: ok, errors: ok, state: ok, persist: ok}
  GetHostedZoneCount: {wire: ok, errors: ok, state: ok, persist: ok}
  ChangeResourceRecordSets: {wire: ok, errors: ok, state: ok, persist: ok, note: "CREATE/DELETE/UPSERT, exact-match DELETE validation, all record-type value validators, routing-policy mutual exclusion, batch validated atomically before any mutation applied — see Notes"}
  ListResourceRecordSets: {wire: ok, errors: ok, state: ok, persist: ok, note: "Name/Type/SetIdentifier lexicographic sort + pagination cursors"}
  CountResourceRecordSets: {wire: ok, errors: ok, state: ok, persist: ok}
  GetChange: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateHealthCheck: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: CallerReference reuse with a different HealthCheckConfig now returns HealthCheckAlreadyExists (409); fixed: CALCULATED HealthThreshold > len(ChildHealthChecks) now rejected (InvalidInput)"}
  GetHealthCheck: {wire: ok, errors: ok, state: ok, persist: ok}
  ListHealthChecks: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-r80d) — same missing required Marker echo as ListHostedZones/ListReusableDelegationSets. Prior wire: ok was false — see 2026-08-14 pass"}
  GetHealthCheckCount: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteHealthCheck: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateHealthCheck: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: HealthCheckVersion was entirely missing from the wire (CreateHealthCheck/GetHealthCheck/ListHealthChecks/UpdateHealthCheck responses never emitted it, even though it's a required field in the real HealthCheck shape). Now every health check carries a Version starting at 1, incremented on each successful update; UpdateHealthCheck's optional request-side HealthCheckVersion is checked for optimistic concurrency and returns HealthCheckVersionMismatch (409) on a stale value"}
  GetHealthCheckStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  GetHealthCheckLastFailureReason: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: no longer silently returns empty tags for a nonexistent hosted zone/health check — now validates existence and returns NoSuchHostedZone/NoSuchHealthCheck (404)"}
  ListTagsForResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2 bugs: (1) HTTP route was unreachable (handler checked a bare /2013-04-01/tags path that can never match; real AWS URI is POST /2013-04-01/tags/{ResourceType}), (2) same missing-existence-check bug as ListTagsForResource"}
  ChangeTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: the handler discarded ChangeTagsForResource's error return (setTags/removeTags used `_ = ...`), so tagging a nonexistent resource silently 200'd instead of 404ing; also fixed: resource tags (b.tags) were never wired into Snapshot/Restore and were lost across a backend restore"}
  CreateKeySigningKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: duplicate name in a zone now returns KeySigningKeyAlreadyExists (409) instead of generic InvalidInput; fixed this pass: KeyManagementServiceArn was never validated — any string (including empty) was accepted. Now checked against a KMS customer-managed-key ARN pattern (arn:{aws|aws-cn|aws-us-gov}:kms:<region>:<12-digit-account>:key/<id>) and rejected with InvalidKMSArn (400, confirmed against the CreateKeySigningKey API reference's Errors section) when malformed"}
  ActivateKeySigningKey: {wire: ok, errors: ok, state: ok, persist: ok}
  DeactivateKeySigningKey: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteKeySigningKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: deleting an ACTIVE KSK returned a fabricated 'KeySigningKeyNotInactive' code that doesn't exist in the AWS API; now returns the real InvalidKeySigningKeyStatus (400)"}
  EnableHostedZoneDNSSEC: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableHostedZoneDNSSEC: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDNSSEC: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateVPCWithHostedZone: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: re-associating a VPC already associated with the same zone now returns success (idempotent no-op) instead of a fabricated InvalidInput error. AWS's documented error list has no duplicate-association error, and the one association-conflict error it does document (ConflictingDomainExists) is explicitly scoped to a *different* hosted zone with the same name, ruling it out for this case — confirmed against the AssociateVPCWithHostedZone API reference's Errors section"}
  DisassociateVPCFromHostedZone: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: VPC not associated now returns VPCAssociationNotFound (404) instead of generic InvalidInput; LastVPCAssociation guard already correct"}
  ListVPCAssociations: {wire: ok, errors: ok, state: ok, persist: ok}
  ListHostedZonesByVPC: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-r80d) — MaxItems, a required output member (api_op_ListHostedZonesByVPC.go:36-40), was absent from the response struct entirely (not merely unset); the SDK always decoded a nil *int32. Handler now parses the optional maxitems query param (default 100, maxHZByVPC) and echoes it. Prior wire: ok was false — see 2026-08-14 pass. FIXED (2026-08-29 list-filter-params pass) — MaxItems was parsed and echoed in the response but never actually applied: the backend call dropped it entirely, so the constraint was decorative only. Now truncates the result to maxItems. FIXED (2026-08-29 wrapper-key-sweep pass, corrects the two entries above's state: ok) — truncation had no cursor at all: the response carried no NextToken and no IsTruncated, so anything past the first page was silently and permanently unreachable, with no way for a client to even detect truncation. api_op_ListHostedZonesByVPC.go confirms the real continuation field is NextToken on both Input and Output (not NextMarker, unlike ListHostedZones/ListHealthChecks/ListReusableDelegationSets), wire element also \"NextToken\" (deserializers.go). Backend now returns pkgs/page.Page[HostedZone] (index-cursor, same shape ListHostedZones/ListHealthChecks already use) instead of a bare truncated slice; handler reads/echoes nexttoken and emits NextToken when truncated. See TestListHostedZonesByVPC_Pagination (creates more items than one page, follows the cursor, asserts the remainder arrives exactly once). FIXED (2026-08-30 wrapper-key-sweep pagination-reproducibility pass) — same not-reproducible-across-calls bug as ListHostedZones above, on b.vpcAssociations (a plain map keyed by zone ID, unspecified walk order) sorted only by Name: two private zones associated with the same VPC can share a Name (CreateHostedZone allows duplicate names; AssociateVPCWithHostedZone has no name-collision check), so a tied pair could drop or duplicate at a page boundary between calls. Fixed identically: tiebreak on ID after Name. See TestListHostedZonesByVPC_PaginationStableAcrossDuplicateNames (list_hosted_zones_by_vpc_pagination2_test.go), hand-reverted to confirm it fails against the unfixed sort, then restored."}
  CreateVPCAssociationAuthorization: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteVPCAssociationAuthorization: {wire: ok, errors: ok, state: ok, persist: ok}
  ListVPCAssociationAuthorizations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30 gopherstack-kwzs) — MaxResults/NextToken (api_op_ListVPCAssociationAuthorizations.go, no IsTruncated member) were parsed nowhere; every call returned every authorization. Now truncates via pkgs/page.New (b.vpcAssocAuthorizations[zoneID] is an append-only slice, already call-stable, no sort/tiebreak needed) and echoes NextToken. Default MaxResults is 50 per the SDK doc comment (new vpcAssocAuthDefaultMaxResults, distinct from this service's usual 100). See TestListVPCAssociationAuthorizations_Pagination."}
  CountAssociatedVPCs: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCidrCollection: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: duplicate collection name now returns CidrCollectionAlreadyExistsException (400) instead of allowing an unbounded number of same-named collections"}
  ChangeCidrCollection: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: added the optional CollectionVersion request field; when supplied it is checked against the collection's current Version and a mismatch returns CidrCollectionVersionMismatchException (409)"}
  DeleteCidrCollection: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: real AWS requires a CIDR collection to be empty (no locations/CIDR blocks) before it can be deleted; gopherstack previously deleted non-empty collections unconditionally. Now returns CidrCollectionInUseException (400) when Locations is non-empty"}
  ListCidrCollections: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30 gopherstack-kwzs) — MaxResults/NextToken (api_op_ListCidrCollections.go, no IsTruncated member) were never applied; the response always returned every collection and the existing (unset) NextToken struct field, plus a fabricated IsTruncated field the real op doesn't have, were both dead weight. Now paginates via pkgs/page.New (sorted by ID, unique, so the b.cidrCollections.All() map walk admits no tie) and the fabricated IsTruncated field was removed rather than wired to a value with no wire meaning."}
  ListCidrLocations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "code fix: NoSuchCidrCollection -> NoSuchCidrCollectionException (real AWS shape name has the Exception suffix, confirmed against aws-sdk-go-v2 types/errors.go — unlike every other Route53 NoSuch* error). FIXED (2026-08-30 gopherstack-kwzs) — MaxResults/NextToken (api_op_ListCidrLocations.go, no IsTruncated member) never applied; same fabricated-IsTruncated-field removal and pkgs/page.New pagination as ListCidrCollections. collections.SortedKeys(col.Locations) was already deterministic, no tiebreak needed."}
  ListCidrBlocks: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30 gopherstack-kwzs) — MaxResults/NextToken (api_op_ListCidrBlocks.go, no IsTruncated member) never applied; same fabricated-IsTruncated-field removal and pkgs/page.New pagination as ListCidrCollections. col.Locations[locationName] is an append-only slice, already call-stable across calls, no tiebreak needed."}
  CreateQueryLoggingConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "status fix: QueryLoggingConfigAlreadyExists 400 -> 409"}
  GetQueryLoggingConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteQueryLoggingConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  ListQueryLoggingConfigs: {wire: ok, errors: ok, state: ok, persist: ok, note: "CORRECTION (2026-08-29 wrapper-key-sweep pass): a prior note claimed this op already honoured every declared filter/marker; false — MaxResults/NextToken (api_op_ListQueryLoggingConfigs.go) are never read, response always returns every matching config with no IsTruncated/NextToken. Not fixed this pass (out of the two-service scope); see deferred list."}
  CreateReusableDelegationSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "status fix (prior pass): NoSuchDelegationSet 404 -> 400. Fixed this pass: the HostedZoneId param (real AWS's 'mark an existing hosted zone's delegation set as reusable' mode, confirmed against the CreateReusableDelegationSet API reference) was parsed off the wire and silently discarded. Now validates the zone exists (HostedZoneNotFound, 400 — a distinct wire code from NoSuchHostedZone, confirmed against the same reference), rejects private zones (a reusable delegation set can't be associated with a private hosted zone, per the operation's own doc text), rejects a zone whose delegation set was already extracted this way (DelegationSetAlreadyReusable, 400), and returns a new reusable set carrying the zone's real name servers (tracked via a backend-internal, non-wire HostedZone.DelegationSetSourceUsed bookkeeping field, confirmed to survive Snapshot/Restore). Also fixed a second, previously-untracked bug found while auditing this op: reusing a CallerReference across two CreateReusableDelegationSet calls silently created two unrelated delegation sets instead of erroring — now returns DelegationSetAlreadyCreated (400, confirmed against the same API reference), matching real AWS's non-idempotent CallerReference-reuse behavior for this specific operation (unlike CreateHostedZone/CreateHealthCheck's idempotent-retry semantics)"}
  GetReusableDelegationSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteReusableDelegationSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: now returns DelegationSetInUse (400) if any hosted zone is still linked to the set, instead of deleting it out from under live zones"}
  ListReusableDelegationSets: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-r80d) — same missing required Marker echo as ListHostedZones/ListHealthChecks; handler didn't even read the marker query param. Prior wire: ok was false — see 2026-08-14 pass. FIXED (2026-08-30 gopherstack-kwzs) — Marker was echoed but never actually applied, and MaxItems was hardcoded to the literal string \"100\": every call returned every reusable delegation set regardless of MaxItems, and NextMarker/IsTruncated never appeared at all. Now paginates via pkgs/page.New (sorted by ID, unique, so the b.reusableDelegationSets.All() map walk admits no tie). See TestListReusableDelegationSets_Pagination."}
  CountZonesByReusableDelegationSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: previously always returned 0 (hosted zones were never linked to delegation sets at all); now counts real linked zones"}
  TestDNSAnswer: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed this pass: classifyRouting never recognised GeoProximityLocation or CidrRoutingConfig at all (only Weight/Region/GeoLocation/Failover/MultiValueAnswer), so geoproximity- and CIDR-routed record sets silently fell through to routingSimple and TestDNSAnswer answered from whichever candidate sorted first by SetIdentifier instead of running real proximity/CIDR selection — a genuine wrong-answer bug, not just an unverified-but-correct algorithm. Implemented selectGeoProximity (great-circle distance from awsRegionCoords/parsed lat-lon, scaled by (1 - Bias/100) per AWS's documented bias direction — exact geometry is AWS-undocumented, so this is a faithful approximation, not a re-derivation of a public spec) and selectCIDR (longest-prefix-match against the CIDR collection's location blocks, reserved \"*\" location as the catch-all default, matching AWS's documented CIDR-routing specificity rule). Weighted/latency/failover/geolocation/multivalue selection re-read against AWS's routing-policy documentation this pass and found already correct; not fully re-derived against non-public AWS source, see deferred"}
  CreateTrafficPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "status fix: TrafficPolicyAlreadyExists 400 -> 409"}
  CreateTrafficPolicyVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateTrafficPolicyInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: allowed unlimited duplicate instances for the same (hostedZoneID, name); now returns TrafficPolicyInstanceAlreadyExists (409)"}
  UpdateTrafficPolicyInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateTrafficPolicyComment: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteTrafficPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTrafficPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteTrafficPolicyInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTrafficPolicyInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTrafficPolicies: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-lx5h) — response dropped TrafficPolicyIdMarker, a required member on ListTrafficPoliciesOutput (deserializers.go's ListTrafficPoliciesOutput switch) that AWS always serializes, not just when truncated. This backend is single-page (IsTruncated always false), so the marker is emitted as an always-present empty string rather than a fabricated next-page ID. Prior wire: ok was false. FIXED (2026-08-30 gopherstack-kwzs) — this service is no longer single-page: MaxItems was hardcoded \"100\" and the marker was never applied. Query key is \"trafficpolicyid\" (serializers.go's awsRestxml_serializeOpHttpBindingsListTrafficPoliciesInput), NOT \"trafficpolicyidmarker\" as the field name would suggest -- verified from the pinned SDK rather than inferred. Paginates via pkgs/page.New, sorted by ID (unique, no tie). See TestListTrafficPolicies_Pagination."}
  ListTrafficPolicyVersions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-lx5h) — same TrafficPolicyVersionMarker gap and fix as ListTrafficPolicies' TrafficPolicyIdMarker above. Prior wire: ok was false. FIXED (2026-08-30 gopherstack-kwzs) — same never-truncates bug as ListTrafficPolicies. Query key is \"trafficpolicyversion\", not \"trafficpolicyversionmarker\". b.trafficPolicies[id] is an append-only slice in ascending version order, already call-stable, no tiebreak needed. See TestListTrafficPolicyVersions_Pagination."}
  ListTrafficPolicyInstances: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30 gopherstack-kwzs) — MaxItems hardcoded \"100\", HostedZoneIdMarker/TrafficPolicyInstanceNameMarker/TrafficPolicyInstanceTypeMarker entirely absent from the response struct, marker never applied. api_op_ListTrafficPolicyInstances.go's three marker fields collapse to a single opaque pkgs/page.New token carried in HostedZoneIdMarker (query key \"hostedzoneid\"); the other two marker fields are decorative, matching the simplification ListHostedZonesByVPC already makes over AWS's real per-field marker semantics. Sorted by ID (unique), no tie. See TestListTrafficPolicyInstances_Pagination."}
  ListTrafficPolicyInstancesByHostedZone: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30 gopherstack-kwzs) — two bugs, one more severe than the filed pagination issue: (1) the HostedZoneId FILTER itself was read from query key \"hostedzoneid\", but serializers.go's awsRestxml_serializeOpHttpBindingsListTrafficPolicyInstancesByHostedZoneInput binds it to \"id\" -- a real aws-sdk-go-v2 client's filter was silently ignored and this op always returned nothing to a real caller (the pre-existing test that appeared to cover this used the same wrong \"hostedzoneid\" key the handler read, so test and bug agreed -- corrected to \"id\", not weakened). (2) MaxItems hardcoded \"100\", markers never applied. This op has no HostedZoneIdMarker (redundant with the now-fixed HostedZoneId filter), so TrafficPolicyInstanceNameMarker carries the pkgs/page.New opaque token instead. See TestListTrafficPolicyInstancesByHostedZone_Pagination."}
  ListTrafficPolicyInstancesByPolicy: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30 gopherstack-kwzs) — same class of bug as ListTrafficPolicyInstancesByHostedZone, more severe than the filed pagination issue: TrafficPolicyId/TrafficPolicyVersion (the FILTER, not the pagination marker) were read from query keys \"trafficpolicyid\"/\"trafficpolicyversion\", but serializers.go's awsRestxml_serializeOpHttpBindingsListTrafficPolicyInstancesByPolicyInput binds them to \"id\"/\"version\" -- a real client's filter was always empty/zero and this op always returned nothing. \"hostedzoneid\" is genuinely HostedZoneIdMarker here (distinct from the filter, unlike ByHostedZone which has none), now the pkgs/page.New opaque cursor. MaxItems hardcoded \"100\" and markers never applied, also fixed. See TestListTrafficPolicyInstancesByPolicy_Pagination."}
families:
  record_types: {status: ok, note: "A/AAAA/CNAME/MX/TXT/SPF/NS/SOA/PTR/SRV/CAA/DS/NAPTR value-format validators verified against RFC-shaped regexes; HTTPS/SVCB/SSHFP/TLSA intentionally accept any value (no AWS-documented format constraint enforced by the real service either)"}
  routing_policies: {status: ok, note: "Weighted(SetIdentifier+Weight 0-255)/Latency(Region)/Failover(PRIMARY|SECONDARY)/Geolocation/Multivalue/Geoproximity(exactly one of AWSRegion|Coordinates|LocalZoneGroup, Bias -99..99, lat/lon range-checked)/CIDR routing all validated for mutual exclusion and SetIdentifier requirement per AWS rules at ChangeResourceRecordSets time. fixed this pass: TestDNSAnswer's selection algorithm (classifyRouting/selectAnswer) never actually ran geoproximity or CIDR selection at all despite validating those fields — see TestDNSAnswer note in ops table. Weighted/latency/geo/failover/multivalue selection re-checked against AWS's public routing-policy docs and found correct (all-zero weights split equally, exact-region-match short-circuits latency, PRIMARY-healthy-else-SECONDARY failover, most-specific geolocation match, up-to-8-record multivalue cap)"}
  dnssec: {status: ok, note: "EnableHostedZoneDNSSEC requires >=1 ACTIVE KSK (KeySigningKeyWithActiveStatusNotFound), KSK lifecycle (create/activate/deactivate/delete) state machine verified"}
  errCodeLookup: {status: ok, note: "every route53 sentinel error's wire code + HTTP status cross-checked this pass against aws-sdk-go-v2/service/route53@v1.62.3 types/errors.go and the botocore api-2.json httpStatusCode field — see fixes in ops table above"}
gaps: []  # both tracked gaps (gopherstack-8l0.5, gopherstack-8l0.3) closed this pass, see ops table
deferred:
  - selectWeighted/selectLatency/selectGeo/selectFailover/multiValueAnswer were re-checked against AWS's *public* routing-policy documentation this pass (see routing_policies family note) and found correct, but not re-derived against AWS's non-public source — Route 53's exact selection algorithm (esp. latency-routing tie-breaks and geoproximity's precise bias geometry) is not fully published, so "matches documented behavior" is the strongest verification achievable without live-AWS access
  - "2026-08-29 list-filter-params pass: pagination is hardcoded/never-truncating on 6 list ops — ListReusableDelegationSets (Marker/MaxItems never read, backend takes none), ListGeoLocations (Start*Code + MaxItems never read; static 15-row table so low real-world impact), ListCidrCollections/ListCidrBlocks/ListCidrLocations (MaxResults/NextToken never read, always IsTruncated=false), and the ListTrafficPolic{y,yInstance}* family — ListTrafficPolicies, ListTrafficPolicyVersions, ListTrafficPolicyInstances(ByHostedZone|ByPolicy) — which all hardcode MaxItems:\"100\" in the response and never truncate or apply their Marker params. Recorded as deferred rather than fixed, matching the cloudfront pass's precedent: real filter/parameter bugs (ListHostedZones) took priority over a page-size sweep across 6 ops, which is a larger piece of work than this pass. ListVPCAssociationAuthorizations similarly ignores MaxResults/NextToken but VPC-per-zone authorization counts are AWS-limited to a handful, so impact is low. **ALL SIX FIXED 2026-08-30 (gopherstack-kwzs), plus ListVPCAssociationAuthorizations** — see each op's own row above for its real marker field name(s) and test. ListGeoLocations (still not its own ops: row; it's a static compile-time table, not backend-owned data) now does threshold search on the exact (ContinentCode, CountryCode, SubdivisionCode) triple to resume — equality matching is safe here specifically because the table is immutable at runtime, unlike the equality-with-zero-default bug class this campaign otherwise warns about; see seekGeoLocationStart's doc comment (handler_record_sets.go) and TestListGeoLocations_Pagination. Two of the six (ListTrafficPolicyInstancesByHostedZone, ListTrafficPolicyInstancesByPolicy) turned out to have a second, more severe, independent bug on top of the filed pagination gap: each read its primary FILTER parameter (not the marker) from the wrong query key entirely, so a real client's filter was always silently ignored and the op always returned nothing — see each row's own note for the exact wrong-vs-real key."
  - "2026-08-29 wrapper-key-sweep pass: ListQueryLoggingConfigs was missed by the list-filter-params pass above and its ops-table row wrongly recorded as already honouring every filter (corrected in that op's row and the note two entries above). MaxResults/NextToken are never read; the account-wide case (no hostedzoneid filter) returns every config with no IsTruncated/NextToken, unbounded by the number of hosted zones in the account. Not fixed this pass (out of the cloudfront/route53 two-service scope) — same never-truncating-pagination shape as the 6 ops above, so grouped with them rather than fixed in isolation."
leaks: {status: clean, note: "no goroutines, tickers, or background timers anywhere in services/route53 (grep for 'go func|time.After|time.Sleep|Ticker' returns nothing) — all ops are synchronous request/response; Reset()/DeleteHostedZone/DeleteHealthCheck correctly cascade-delete tags/KSKs/VPC-assocs/query-logging-configs so no orphaned map entries accumulate under normal use. b.tags itself was NOT wired into Snapshot/Restore before a prior pass (fixed then) — that was a persistence gap, not a leak, since Reset() already covered it. This pass's new HostedZone.DelegationSetSourceUsed field is backend-internal (not a new map/table) and rides along with the existing zoneDataSnapshot embedding of HostedZone, confirmed to survive Snapshot/Restore by TestSnapshotRestore_DelegationSetSourceUsed — no new lock paths, no new leak surface."}
---

## Notes

### 2026-08-29 (list-filter-params sweep: parameters declared and never honoured)

Measured all 21 collection-returning operations (verified by SDK output shape, not
verb: the 17 `List*` ops, plus `ListTagsForResources`, `GetHealthCheckStatus`, and
`GetCheckerIpRanges`, which return arrays despite `Get*` names) and every constraining
parameter each declares in its own `api_op_<Op>.go` Input struct. Found and fixed 2
real bugs on `ListHostedZones`/`ListHostedZonesByVPC` (see ops table). `ListHealthChecks`,
`ListHostedZonesByName`, `ListResourceRecordSets`, and `ListTagsForResources` were
re-verified and already honour every declared filter/marker correctly. **Correction
(2026-08-29 wrapper-key-sweep pass): the claim above that `ListQueryLoggingConfigs`
"already honours every declared filter/marker" was wrong.** Its real Input
(`api_op_ListQueryLoggingConfigs.go`) declares `MaxResults` and `NextToken`; the
handler (`handler_query_logging.go`) only reads `hostedzoneid` and returns every
matching config unpaginated, with no `IsTruncated`/`NextToken` in the response at
all — the account-wide (no `hostedzoneid` filter) case can grow with the number of
hosted zones. Not fixed this pass (out of the two-service budget); added to the
never-truncating-pagination list below rather than left mis-recorded as correct.
6 further list ops have never-truncating pagination — see `deferred` above,
same shape as cloudfront's prior-pass finding, not fixed here by the same "larger piece
of work" reasoning. No parameter-parsed-then-discarded-to-`_` cases and no handler that
skips reading its request body were found in this service this pass.

### 2026-08-29 (error-path sweep: what a typed client sees on failure)

Extracted all 71 `awsRestxml_deserializeOpError<Op>` switches from route53@v1.65.6's
deserializers.go and cross-referenced every backend/handler call site raising a sentinel
error (or a literal wire code) against its own op's modeled set. `backendErrorTable`
(handler.go) — the shared sentinel-to-wire-code table every op funnels through via
`handleBackendError` — was correct and 1:1 with errors.go's sentinels (unlike quicksight,
which collapses by category; unlike this table, which maps every sentinel to its own distinct
code, matching real AWS's fine-grained Route 53 error set). No sentinel-reuse or wrong-code
bugs found across the 62 ops resolvable by direct backend-method-name call-graph tracing
(op name == `StorageBackend` interface method name here, confirmed via interfaces.go).

**Real bug found and fixed**: `UpdateHostedZoneFeatures` never validated `HostedZoneId` at
all — `updateHostedZoneFeatures` (handler_hosted_zones.go) discarded its `path` argument
(`func (h *Handler) updateHostedZoneFeatures(c *echo.Context, _ string) error`) and
unconditionally returned success. Its own deserializer models `NoSuchHostedZone` for exactly
this case (alongside `InvalidInput`/`LimitsExceeded`/`PriorRequestNotComplete`) — a
missing-error bug (returning success where AWS raises), not a wrong-code one. Fixed by parsing
the zone ID from the path (same `TrimPrefix`/`TrimSuffix` pattern as the sibling
`disassociateVPCFromHostedZone`) and validating existence via the already-available
`GetHostedZone` backend method before returning success. Covered by
`error_path_sweep_test.go` (real `aws-sdk-go-v2/service/route53` client, `errors.As` against
`types.NoSuchHostedZone`). Persisting the `EnableAcceleratedRecovery` flag itself is a separate,
larger feature gap (the `StorageBackend` interface has no such field/method) and was left
out of scope for this error-path-only pass.

**Method note**: 9 of the 71 ops (`GetAccountLimit`, `GetCheckerIpRanges`, `GetGeoLocation`,
`GetHealthCheckLastFailureReason`, `GetHostedZoneLimit`, `GetReusableDelegationSetLimit`,
`GetTrafficPolicyInstanceCount`, `ListGeoLocations`, `UpdateHostedZoneFeatures`) have no
backend method of the same name — they're implemented as handler-layer functions instead
(`getHostedZoneLimit`, `getGeoLocation`, etc., in handler_*.go), so a naive op-name-to-method
call-graph trace misses them entirely and silently under-reports. Re-traced each by its actual
handler function name. `GetGeoLocation` raises `NoSuchGeoLocation` via a direct `xmlError(...)`
call rather than a named sentinel (correct — the code was simply invisible to sentinel-based
tracing, not missing). `GetCheckerIpRanges`/`GetTrafficPolicyInstanceCount`/`GetAccountLimit`
model no core error code at all and correctly raise none.

**Protocol**: REST-XML (path/verb routing, XML request+response bodies), matching
`aws-sdk-go-v2/service/route53`'s `awsRestxml_*` (de)serializers. Namespace
`https://route53.amazonaws.com/doc/2013-04-01/` on every response root element.

**ChangeResourceRecordSets — the core op, and its traps**:
- Change-batch validation is two-phase: every `Change` in the batch is validated
  against the *pre-batch* zone snapshot first (in submission order), and only if
  *all* validate does the second phase apply every mutation. This correctly makes
  the whole batch atomic (all-or-nothing) and matches AWS's documented behavior for
  the canonical example of deleting a CNAME and creating an alias A record for the
  same name in one batch (different `Type` values ⇒ different map keys ⇒ no
  collision during validation).
- **Trap** (not a bug, verified deliberately): a batch containing a literal
  `DELETE` followed by `CREATE` for the *exact same* `(Name, Type, SetIdentifier)`
  key will fail with "record set already exists" during the CREATE's validation,
  because validation runs against the unmutated zone. This is very unlikely to be
  a real-world pattern (UPSERT already covers "replace this record's values"), and
  AWS's own documented DELETE+CREATE example always uses two different `Type`
  values, so this was deliberately left as-is rather than "fixed" without stronger
  evidence of the real batch-processing order for the same-key case. Flagged here
  so the next auditor doesn't have to re-derive this.
- DELETE requires an exact match of TTL + resource-record values (or AliasTarget)
  against the current record, unless the request omits both (a "bare" delete by
  name+type+SetIdentifier) — `deleteValuesMatch` already encodes this correctly.
- Alias vs. non-alias TTL rule: TTL is required (and range-checked to
  `2147483647`) only when `AliasTarget == nil`; alias records must omit TTL. Already
  correct.

**Error code auditing method this pass**: every sentinel error in `backend.go`'s
`var (...)` block was cross-checked against two independent AWS sources: (1)
`aws-sdk-go-v2/service/route53@v1.62.3`'s generated `types/errors.go` (gives the
literal `ErrorCode()` wire string per exception type), and (2) the botocore
`api-2.json` model's `"error":{"httpStatusCode":N}` field per shape (gives the
real HTTP status). Six concrete mismatches were found and fixed — see the `ops`
table. The most surprising one: `NoSuchCidrCollectionException`,
`NoSuchCidrLocationException`, and `NoSuchCloudWatchLogsLogGroup` are the *only*
Route 53 "NoSuch*" errors whose wire code carries the `Exception` suffix; every
other `NoSuch*` error (NoSuchHostedZone, NoSuchHealthCheck, NoSuchChange, ...) does
not. Also surprising: `NoSuchDelegationSet` is HTTP 400, not 404, unlike every
other `NoSuch*` error in the service (confirmed twice against both sources before
trusting it).

**CallerReference idempotency — the real rule**: reusing a `CallerReference` on
`CreateHostedZone`/`CreateHealthCheck` is idempotent (returns the original
resource) *only* when every other input parameter is identical to the original
request. Reusing it with *any* different parameter returns
`HostedZoneAlreadyExists`/`HealthCheckAlreadyExists` (409) — it is not a "last
write wins" or "always return the first" semantic. A prior audit pass had gotten
this backwards and encoded the wrong behavior directly into test assertions
(`same_ref_different_name_still_idempotent`); those tests were corrected this
pass rather than left as false parity proof.

**ListTagsForResources routing — real AWS request URI**: `POST
/2013-04-01/tags/{ResourceType}` (batch lookup by `ResourceType` + a list of
`ResourceId`s in the XML body) — there is no bare `/2013-04-01/tags` endpoint.
The handler previously checked for the nonexistent bare path (which additionally
could never be reached anyway, since the outer dispatcher requires the
`/2013-04-01/tags/` prefix with a trailing slash), so a real AWS SDK client's
`ListTagsForResources` call was silently misrouted into `ChangeTagsForResource`
and 404'd. Fixed by detecting "no `/` after the ResourceType segment" instead of
comparing against a bare-prefix string.

**Tag-family disguised-stub trap**: `ChangeTagsForResource`'s backend-level
existence check was always correct — but the handler called it through two
one-line wrappers (`setTags`/`removeTags`) that discarded the returned error
(`_ = h.Backend.ChangeTagsForResource(...)`), so the real validation never
surfaced over the wire. This is the "real-looking op that's actually a disguised
stub" pattern from parity-principles.md: grepping for the backend method alone
would have shown correct-looking code; the bug was purely in the handler
throwing the result away.

## 2026-07-12 re-audit (this pass)

No local drift in `services/route53/` between the prior audit commit
(`ce30166a`, which the ledger's `last_audit_commit: 017fc20a` predates on a
squashed/rebased branch — see re-audit protocol) and this pass's start.
Per the re-audit protocol, only the `partial`-rated rows the prior ledger
flagged were re-examined; all `ok` rows were trusted unchanged. Three of the
four tracked gaps were closed:

**`HealthCheckVersion` was missing from the wire entirely, not just
unchecked.** The prior ledger described this gap as "no optimistic-concurrency
check", implying the field existed but wasn't validated. In fact
`HealthCheckVersion` — a *required* field on AWS's `HealthCheck` shape,
confirmed against `aws-sdk-go-v2/service/route53@v1.62.3` `types/types.go`
— was never serialized into `CreateHealthCheck`/`GetHealthCheck`/
`ListHealthChecks`/`UpdateHealthCheck` responses at all; `HealthCheck` had no
`Version` field on the backend struct. This is the "wrong wire shape",
not just "missing error check", bug class from parity-principles.md. Fixed:
`HealthCheck.Version` now starts at 1 and increments on every successful
`UpdateHealthCheck`; the optional request-side `HealthCheckVersion` is
checked when present and returns `HealthCheckVersionMismatch` (409,
confirmed via `botocore`'s `route53/2013-04-01/service-2.json`
`error.httpStatusCode`) on a stale value.

**`ChangeCidrCollection`/`DeleteCidrCollection`** — added the optional
`CollectionVersion` optimistic-concurrency check (mirrors the
`HealthCheckVersion` fix; `CidrCollectionVersionMismatchException`, 409) and
the "collection must be empty before it can be deleted" guard
(`CidrCollectionInUseException`, 400 — confirmed via botocore: despite the
similar name/shape to the 409 version-mismatch error, this one really is
400, not 409).

**Reusable delegation set <-> hosted zone linkage** was the largest gap:
`CreateHostedZoneRequest.DelegationSetId` was parsed off the XML wire and
then silently dropped on the floor — every hosted zone got the same
hardcoded default name-server pair no matter what was requested, and
`CountZonesByReusableDelegationSet` always returned 0 / `DeleteReusableDelegationSet`
never checked for in-use sets because zones were *structurally* never linked
to delegation sets at all (no field to link them). Fixed by adding
`HostedZone.DelegationSetID`/`NameServers` fields; `CreateHostedZone` now
resolves and validates a supplied `DelegationSetId` (accepting both the bare
`N...` form and the `/delegationset/N...` form real AWS returns, matching
the existing normalization convention in `handler_completeness.go`'s
delegation-set routes — factored out into the shared `normaliseDelegationSetID`
helper), uses the linked set's real name servers for both the
`DelegationSet` response element and the zone's auto-seeded NS/SOA records,
and `DeleteReusableDelegationSet`/`CountZonesByReusableDelegationSet` now
walk live zones instead of a permanently-empty relationship.

Not fixed in that pass: `CreateReusableDelegationSet`'s `HostedZoneId` param
and `AssociateVPCWithHostedZone`'s duplicate-VPC error code — both closed in
the 2026-07-23 pass below.

## 2026-07-23 pass (this pass)

Closed both tracked gaps from the prior audit and 3 of its 4 deferred items.
Also found and fixed a genuine wrong-answer bug in `TestDNSAnswer` while
re-deriving the routing-policy selection algorithms (the deferred item this
ledger flagged for the next audit), and removed all 6 of the service's
`//nolint:cyclop|gocognit|funlen` suppressions by decomposition per the
campaign's banned-nolint sweep.

**`AssociateVPCWithHostedZone`'s duplicate-VPC re-association** — fetched
the live AWS API reference (`API_AssociateVPCWithHostedZone.html`) and
confirmed its documented error list (`ConflictingDomainExists`,
`InvalidInput`, `InvalidVPCId`, `LimitsExceeded`, `NoSuchHostedZone`,
`NotAuthorizedException`, `PriorRequestNotComplete`,
`PublicZoneVPCAssociation`) has no error for "VPC already associated with
*this* zone", and `ConflictingDomainExists`'s documented cause is
specifically "the VPC is already associated with *another* hosted zone that
has the same name" — ruling it out for this case. Changed the backend to
treat re-association as an idempotent no-op (matches the general community
understanding reflected in Terraform's Route53 VPC-association resource
design). `TestDuplicateVPC` rewritten to assert success + a stable VPC count
of 1 instead of the previously-asserted (and now-understood-to-be-wrong)
`InvalidInput` error.

**`CreateReusableDelegationSet`'s `HostedZoneId` param** — fetched the live
AWS API reference and confirmed this is real AWS's "mark an existing hosted
zone's delegation set as reusable" mode, with its own error list
(`DelegationSetAlreadyCreated`, `DelegationSetAlreadyReusable`,
`DelegationSetNotAvailable`, `HostedZoneNotFound` [not `NoSuchHostedZone` —
a distinct wire code specific to this operation, confirmed against
`aws-sdk-go-v2/service/route53@v1.62.3` `types/errors.go`'s
`HostedZoneNotFound` type], `InvalidArgument`, `InvalidInput`,
`LimitsExceeded`). Implemented: zone-existence check (`HostedZoneNotFound`,
400), private-zone rejection (per the operation's own doc text: "You can't
associate a reusable delegation set with a private hosted zone"),
double-extraction rejection (`DelegationSetAlreadyReusable`, 400, tracked via
a new backend-internal `HostedZone.DelegationSetSourceUsed` bool — not part
of the wire `HostedZone` shape, confirmed to survive Snapshot/Restore), and
real name-server inheritance from the source zone. While implementing this,
found and fixed a second, previously-untracked bug in the *same* function:
`CreateReusableDelegationSet` never checked for `CallerReference` reuse at
all, silently creating unlimited duplicate delegation sets for the same
reference — now returns `DelegationSetAlreadyCreated` (400), matching real
AWS's error-on-reuse semantics for this specific operation (unlike
`CreateHostedZone`/`CreateHealthCheck`'s idempotent-retry-on-identical-input
semantics, which a much earlier pass had already gotten right for those two
ops — `CreateReusableDelegationSet` is documented as genuinely different:
"you must use a unique CallerReference string every time").

**`TestDNSAnswer` routing-policy re-derivation — the deferred item, and what
it actually found.** Re-reading `classifyRouting` against the full list of
routing-policy fields `ResourceRecordSet` carries (checked against
`validateRoutingPolicy`'s own mutual-exclusion list, which already covered
all seven policy fields) surfaced that `classifyRouting` only recognised
five of them — `Weight`, `Region`, `GeoLocation`, `Failover`,
`MultiValueAnswer` — never `GeoProximityLocation` or `CidrRoutingConfig`.
Record sets using either fell through to `routingSimple`, meaning
`TestDNSAnswer` answered from whichever candidate sorted first by
`SetIdentifier` instead of running any proximity or CIDR matching at all —
a silent wrong-answer bug on every geoproximity- or CIDR-routed zone, not
merely an "unverified but probably fine" algorithm. This is exactly the
"real-looking op that's actually a disguised stub" pattern from
parity-principles.md: `ChangeResourceRecordSets` validated these fields
correctly (so grepping for `GeoProximityLocation`/`CidrRoutingConfig`
handling would have shown seemingly-complete code), but the read path never
consulted them. Fixed by adding `routingGeoProximity`/`routingCIDR` kinds and
two new selectors: `selectGeoProximity` (great-circle distance from
`awsRegionCoords` or parsed `Coordinates` lat/lon, scaled by
`1 - Bias/100` — AWS documents Bias's *direction* [higher bias expands a
resource's effective service area] but not its exact geometry, so this is a
faithful approximation of the documented behavior, not a re-derivation of a
public spec) and `selectCIDR` (longest-prefix-match against the referenced
CIDR collection's location blocks, with the reserved `"*"` location as the
default fallback — this *is* a fully documented AWS rule, unlike
geoproximity's bias geometry). The other five routing kinds
(weighted/latency/geo/failover/multivalue) were re-checked against AWS's
public routing-policy documentation and found already correct.

**Alias cycle/depth handling** — added `TestTestDNSAnswerAliasCycle`
covering both a self-referencing alias (`a` -> `a`) and a two-hop cycle
(`a` -> `b` -> `a`), each run with a goroutine + 5s timeout so a regression
that broke `maxAliasDepth`'s guard would fail the test instead of hanging
the suite. Both terminate correctly with an empty answer, confirming
`resolveAlias`'s existing `depth >= maxAliasDepth` guard already handles
pathological chains — no code change needed here, just proof.

**`CreateKeySigningKey` `InvalidKMSArn`** — added `reKMSArn`, a regex
matching a well-formed KMS customer-managed-key ARN across the
standard/China/GovCloud partitions
(`arn:{aws|aws-cn|aws-us-gov}:kms:<region>:<12-digit-account>:key/<id>`),
checked after the zone-existence lookup (so `create_ksk_zone_not_found`-style
requests still 404 before ever reaching ARN validation, matching this
service's existing required-field-then-existence-then-format validation
order). Every existing test's placeholder ARNs (`"arn:kms:test"` and
similar clearly-non-ARN strings) were updated to well-formed fake ARNs.

**Banned-nolint sweep** — removed all 6 `//nolint:cyclop|gocognit|funlen` in
the service:
- `cidr_collections.go`'s `ChangeCidrCollection` (`gocognit`): extracted
  `applyCidrCollectionPut`/`applyCidrCollectionDeleteIfExists`/`applyCidrCollectionChange`.
- `handler_health_checks.go`'s `updateHealthCheck` (`gocognit,cyclop,funlen`):
  extracted `mergeHealthCheckUpdate{Strings,Numeric,Flags,Collections}`.
- `handler_record_sets.go`'s `changeResourceRecordSets` (`funlen`):
  extracted `toBackendResourceRecordSet`/`toBackendChange`.
- `record_sets.go`'s `validateRoutingPolicy` (`cyclop`): extracted
  `countRoutingPolicies`/`validateRoutingPolicyCardinality`/`validateRoutingPolicyFields`.
- `record_sets.go`'s `validateChange` (`gocognit,cyclop`): extracted
  `validateChangeType`/`validateChangeTTL`/`validateChangeCNAME`/`validateChangeRecordValues`/`validateChangeActionState`.
- `record_sets.go`'s `ListResourceRecordSets` (`gocognit,cyclop`): extracted
  `sortRecordSets`/`seekRecordSetStart`/`paginateRecordSets`.

Decomposing `selectAnswer` to add the two new routing kinds pushed it over
`cyclop`'s limit on its own; replaced the routing-kind `switch` with a
`map[routingKind]singleAnswerSelector` dispatch table built once via
`sync.OnceValue` (the established `apigatewayv2`-style pattern this campaign
uses elsewhere), keeping `routingMultiValue`/`routingSimple` — which don't
fit the "one selector function" shape — as explicit early returns.
`selectCIDR`'s own longest-prefix-match loop separately tripped `gocognit`
once written; split into `cidrBlockLongestPrefix` (single-location scan) and
`cidrCandidateMatch` (per-candidate resolution) to flatten the nesting.

**SDK-driven integration-test run** — `make build-linux` (whole-repo
monolith binary; every service links into one binary, so this isn't
route53-specific and is genuinely slow in a resource-constrained sandbox —
two earlier attempts in this pass hit multi-minute wall-clock stalls before
finally completing) followed by `go test ./test/integration/... -run
Route53` against the resulting Dockerized binary. All 45 route53 and
route53resolver integration tests passed, including
`TestIntegration_Route53_ChangeResourceRecordSets`,
`TestIntegration_Route53_WeightedRouting`,
`TestIntegration_Route53_FailoverRouting`,
`TestIntegration_Route53_TestDNSAnswerWeighted`,
`TestIntegration_Route53_HealthCheck_Lifecycle`,
`TestIntegration_Route53_DeactivateDeleteKSK`,
`TestIntegration_Route53_EnableDisableDNSSEC`, and
`TestIntegration_Route53_ResourceRecordSetsChangedWaiter` — proving this
pass's fixes against a real `aws-sdk-go-v2` client round-trip, not just unit
tests, per parity-principles.md's "unit tests are not parity proof"
guidance. No dedicated `route53_parity_test.go` exists yet (the existing
coverage is spread across `route53_test.go`/`route53_audit_test.go`/
`route53_new_ops_test.go`/`route53_waiter_test.go`); creating one consolidated
file is a housekeeping task for a future pass, not a correctness gap.

## 2026-08-14 pass (gopherstack-r80d): required output member sweep

Extracted every field marked `This member is required.` at the top level of
an `<Op>Output` struct across all 71 `route53@v1.65.6` operations (parsed
directly from the pinned SDK's `api_op_*.go` files, blank-line-separated
field blocks, case-/tag-suffix-tolerant), yielding 108 required output
members across 58 of 71 ops — validated against the extraction tool's
known-answer case (kinesis's `DescribeLimits`, 4/4 exact match, matching the
bug fixed in `be789761c`) and a negative case (kinesis's `ListShards`, 0/0)
before trusting it at route53's scale.

Every one of the 58 ops was read end-to-end (not grepped) to confirm each
required field is actually written into the response, per
parity-principles.md's "grep alone shows real-looking code, read the path to
be sure" guidance. Found and fixed **4** silently-unset required output
members, all one bug class — real AWS's `Marker` element ("the value you
specified for the marker parameter in the request that produced the current
response") being conflated with the *optional* `NextMarker` next-page
cursor, or (for `ListHostedZonesByVPC`) `MaxItems` missing from the response
struct entirely:

- `ListHostedZones`, `ListHealthChecks`, `ListReusableDelegationSets`:
  response structs only carried `NextMarker`; the required `Marker` echo of
  the request's own `marker` parameter was never wired at all.
  `ListReusableDelegationSets`'s handler didn't even parse the `marker`
  query param.
- `ListHostedZonesByVPC`: `MaxItems` — required, but the response struct had
  no field for it and the handler never read the `maxitems` query param.

All four are the same silent-zero-value class as batch one's lambda finding:
a typed SDK client decodes a `nil`/`""` for a field AWS guarantees is always
present, with no error surfaced. Each fix is covered by an SDK-driven round
trip test (`wire_output_required_r80d_test.go`) that sets the corresponding
request field to a distinguishing non-empty value and asserts it comes back
unchanged (not merely non-nil) — verified to fail against the pre-fix code
by hand-reverting each change and confirming an `md5sum`-identical restore
afterward.

The remaining 104 required output fields across the other 54 ops were all
confirmed correctly populated by reading each handler's response-construction
code. **route53 is settled for this bug class**: every required output
member across every op that has one has been read and checked, not sampled.

## 2026-08-13 pass (gopherstack-l5ir): route reachability audit

All 71 real route53 ops were extracted from `route53@v1.65.6` serializers.go
(`request.Method` + `httpbinding.SplitURI(...)` in each op's
`awsRestxml_serializeOp<Op>.HandleSerialize`) and diffed against `routeRequest`'s
dispatch tree. Found and fixed **one** op that resolved to a plausible WRONG
op rather than 404ing: `GetHealthCheckLastFailureReason`
(`GET .../healthcheck/{id}/lastfailurereason`) fell through `routeHealthCheck`'s
generic method switch (which only special-cased the `/status` suffix, not
`/lastfailurereason`) and silently returned the full `HealthCheck` object --
`GetHealthCheck`'s response shape, not the failure-reason response -- for
every real client call. The implementation (`getHealthCheckLastFailureReason`)
already existed and was already correct; it was simply unreachable. This is
exactly the "resolves to a plausible wrong op, not a 404" class of bug that a
route-table diff alone (as opposed to a real per-op resolution test) misses
-- see gopherstack-4nek's cloudfront findings for the precedent. Fixed by
checking the `/lastfailurereason` suffix before the generic switch, mirroring
the existing `/status` handling. The dead `routeCompletenessLimits` branch
that appeared to handle this path (but never could, since `routeRequest`'s
top-level switch always routes any `/healthcheck/...` path to `routeHealthCheck`
first) was removed and documented rather than left as a misleading no-op.
`extractHealthCheckOperation`/`iamActionForHealthCheck` (ExtractOperation's
and IAMAction's own, separate implementations of the same shape) carried the
identical bug and were fixed identically.

All other 70 ops, including every shared-path pair method-disambiguated on
the same URL (the tags trio, hostedzone GET/DELETE/POST, trafficpolicy
GET/DELETE/POST at both the `{Id}` and `{Id}/{Version}` depths, and
GetGeoLocation/ListGeoLocations sharing one switch case across two literal
paths, `/geolocation` vs `/geolocations`, disambiguated by a
continentcode/countrycode/subdivisioncode query filter rather than a bare
flag) were confirmed correctly routed already -- route53 was, like `mgn`
audited in the same pass, essentially clean going in. No query-parameter- or
flag-discriminated pair was found to be *mis*-disambiguated.

`ExtractOperation`, previously covering roughly half of the 71 ops (many
newer families -- CIDR sub-paths, traffic-policy `{Id}/{Version}` vs `{Id}`,
TPInstance updates, info/limit endpoints -- fell through to `"Unknown"` even
though the real HTTP dispatch handled them correctly), was extended to mirror
`routeRequest`'s real dispatch tree op-for-op. This is now backed by
`TestExtractOperation_SDKRouteTable` (`handler_paths_sdk_diff_test.go`, one
subtest per op) -- 71/71 pass, and it is the permanent regression guard for
this sweep rather than a one-off report. No existing test encoded the old
wrong behavior (none tested `GetHealthCheckLastFailureReason` via HTTP at
all), so no test corrections were needed beyond the new file.

Gates: `go build`, `go vet`, `go test -race`, `go fix -diff` (no diff),
`golangci-lint run` (0 findings, after decomposing 3 new `cyclop` violations
and adding op-name constants for 6 new `goconst` violations the extended
`ExtractOperation` introduced) all clean.

## 2026-08-31 pass (gopherstack-21my): first per-item sweep

route53 had never had a per-item field-name sweep under this issue. Confirmed
`awsRestxml_` (REST-XML, `strings.EqualFold` element matching, same latent
case-only-mismatch class as query/XML) from route53@v1.65.6's own
`deserializers.go` before starting. Byte-for-byte case check against the
pinned SDK for every list op below, plus the no-`*Unwrapped`-call-site check
repo-wide against route53@v1.65.6: **zero hits**, so every route53 list is
correctly member-wrapped, not flattened.

**BUG (fixed): `ListHostedZonesByName`'s own ad-hoc `HostedZone` item builder
(`handler_hosted_zones.go`'s `listHostedZonesByName`) dropped
`Config.PrivateZone` and `ResourceRecordSetCount` entirely** -- both are real
`types.HostedZone`/`types.HostedZoneConfig` members
(`awsRestxml_deserializeDocumentHostedZone` /
`...HostedZoneConfig`), and both are backed by state this backend already
tracks correctly: `GetHostedZone` and the plain `ListHostedZones` both build
the item through a single shared `toXMLHostedZone` helper that sets both
fields from the zone record, but `listHostedZonesByName` built its own
literal instead of calling it, setting only `ID`/`Name`/`CallerReference`/
`Config.Comment`. Every hosted zone returned by `ListHostedZonesByName` had
the right count, `PrivateZone` always `false` and `ResourceRecordSetCount`
always `0` regardless of the zone's real state -- the sibling-disagreement
shape this issue's queue prioritized, on a route53 op that had never been
checked at either layer before. Fixed by replacing the ad-hoc literal with
`toXMLHostedZone(&zones[i])`, the same builder the other two ops use. Test:
`TestListHostedZonesByName_ItemShape_RealClient`
(`wire_field_fixes_r53sweep1_test.go`), creates a private hosted zone with a
comment via the real client, adds one record, and asserts
`Config.PrivateZone`, `Config.Comment` and `ResourceRecordSetCount` all
round-trip through `ListHostedZonesByName`. Verified failing pre-fix by
hand-revert (`PrivateZone` false, `ResourceRecordSetCount` 0).

**RE-VERIFIED CLEAN, byte-for-byte case included:**
- `ListHealthChecks` -- shares `xmlHealthCheck`/`xmlHealthCheckConfig` with
  `GetHealthCheck`/`CreateHealthCheck` (one struct, three call sites), all
  emitted `HealthCheckConfig` members (18 of 18) and `HealthCheck`'s own
  `Id`/`CallerReference`/`HealthCheckConfig`/`HealthCheckVersion` correctly
  named, including the flattened-list checks (`Regions>Region`,
  `ChildHealthChecks>ChildHealthCheck`) against the real deserializer's exact
  member-element names. `CloudWatchAlarmConfiguration` and `LinkedService`
  (both real top-level `HealthCheck` members) are genuine no-backing-state
  gaps -- absent from the domain model entirely -- shared identically by
  `Get` and `List`, so no sibling disagreement is possible here.
- `ListResourceRecordSets` -- `xmlResourceRecordSet` carries 14 of 15 real
  `ResourceRecordSet` members correctly, including the fully-nested
  `AliasTarget`, `CidrRoutingConfig`, `GeoProximityLocation.Coordinates`, and
  `ResourceRecords>ResourceRecord`. `TrafficPolicyInstanceId` is a genuine
  gap: nothing in this backend tags a record set with the traffic-policy
  instance that created it. The reused `xmlGeoLocation` type carries three
  extra fields (`ContinentName`/`CountryName`/`SubdivisionName`) that don't
  exist on the real per-record `GeoLocation` type -- harmless, since a real
  client's decoder silently skips unrecognized elements, and those fields
  are exactly the ones the *different* real type `GeoLocationDetails`
  (`ListGeoLocations`'s own item, verified separately below) legitimately
  needs; the type is deliberately shared, not a mismatch.
- `ListGeoLocations` -- `xmlGeoLocation` (the same struct, used here for its
  full six-field form) matches `GeoLocationDetails` exactly, wrapped
  `GeoLocationDetailsList>GeoLocationDetails` as the real (never called)
  `*Unwrapped` sibling confirms it should be.
- `ListTrafficPolicyInstances` (and its `ByHostedZone`/`ByPolicy` siblings) --
  all six call sites of the item builder share one function,
  `toXMLTPInstance`, emitting 8 of 9 real `TrafficPolicyInstance` members.
  The missing `Message` is a genuine no-backing-state gap: this backend's
  `TrafficPolicyInstance.State` is hardcoded to `"Applied"` at every creation
  site (no async-failure modeling), and `Message` is only ever populated by
  real AWS on a non-`Applied` state, so no legal input through this backend
  can ever populate it -- recorded per this issue's restraint guidance, not
  counted as a fix.
- `ListTrafficPolicies` -- `xmlTrafficPolicySummary`'s five members match
  `TrafficPolicySummary` exactly (a genuinely different, slimmer real type
  than `GetTrafficPolicy`'s `TrafficPolicy`, so no sibling disagreement is
  expected or possible).
- `ListReusableDelegationSets` -- reuses the already-clean `xmlDelegationSet`
  (`CallerReference`/`Id`/`NameServers`), wrapped
  `DelegationSets>DelegationSet` matching the real deserializer.
- `ListQueryLoggingConfigs` -- `xmlQueryLoggingConfig`'s three members
  (`CloudWatchLogsLogGroupArn`/`HostedZoneId`/`Id`) all correct.
- `ListCidrCollections`/`ListCidrBlocks`/`ListCidrLocations` --
  `xmlCidrCollectionSummary`/`xmlCidrBlockSummary`/`xmlCidrLocationSummary`
  all match `CollectionSummary`/`CidrBlockSummary`/`LocationSummary` exactly.
- `ListHostedZonesByVPC` -- `xmlHostedZoneSummary` (already carrying an
  explanatory comment tying it to `HostedZoneSummary`'s deserializer) matches
  exactly, including its "Id wire element is HostedZoneId, not Id" detail.
- `ListVPCAssociationAuthorizations` -- `xmlVPC`'s `VPCId`/`VPCRegion` match
  `types.VPC` exactly.

**NOT REACHED at this layer:** `ListTagsForResource(s)` (simple map/tag
shapes, lower yield per this issue's own priority note), and the read-side
detail of `ListCidrCollections`' companion `GetCidrCollection`-equivalent
change/version flow beyond what the existing sweep1 tests already cover.

Gates: `go build ./services/iam/... ./services/route53/...`, `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/iam/... ./services/route53/...`
(pass), `golangci-lint run ./services/iam/... ./services/route53/...` (0
issues). No `nolint` directives exist in any file this pass touched.

## 2026-08-31 pass (gopherstack-21my): closing out the per-item sweep

Continuation of the 2026-08-31 first-per-item-sweep entry above, which
explicitly named `ListTagsForResource(s)` as not reached ("simple map/tag
shapes, lower yield per this issue's own priority note"). Reconfirmed
`awsRestxml_` from route53@v1.65.6's own `deserializers.go` before starting
(unchanged from the prior entry).

**`ListTagsForResource`/`ListTagsForResources` came back CLEAN, byte-for-byte
case included.** `handler_tags.go`'s `resourceTagSet`/`xmlResourceTagSet`
match `types.ResourceTagSet` (`ResourceId`/`ResourceType`/`Tags>Tag`)
exactly, `r53Tag` matches `types.Tag` (`Key`/`Value`) exactly, and both
wrapper keys (`ResourceTagSet` on the singular op, `ResourceTagSets>
ResourceTagSet` on the batch op) match `awsRestxml_deserializeOpDocument
ListTagsForResourceOutput`/`...ResourcesOutput` exactly. No wrapping-shape
issue either: `ResourceTagSetListUnwrapped`/`TagListUnwrapped` exist in the
pinned SDK but have no call site anywhere in `services/route53`, confirming
both lists are correctly member-wrapped. This closes the one item this
issue's route53 coverage had explicitly left open; every route53 list
operation named across both sweep passes has now had a per-item field-name
check.

No bugs found, no fix, no new test — a clean verdict doesn't need a
round-trip test to prove a negative, consistent with how prior clean
per-item findings in this issue's history (e.g. sagemaker, elbv2's
non-buggy ops) were recorded without one.

No web pages fetched this pass — everything came from the pinned SDK module
cache (route53@v1.65.6) already vendored in the module cache.

Gates: `go build ./services/route53/...`, `go vet ./services/route53/...`,
`go test -race -count=1 ./services/route53/...` (pass, no test changes),
`golangci-lint run ./services/route53/...` (0 issues). No `nolint`
directives exist in this service.

## 2026-09-07 pass (gopherstack-ns7j): full backendErrorTable × ops cross-product audit

Systematic (not sampled) audit of every `backendErrorTable` row (35, not the
title's "roughly 33") against every one of route53's 71 ops (`ls
api_op_*.go` in the pinned `aws-sdk-go-v2/service/route53@v1.65.6`). Per-op
declared error codes were extracted by scripting `awk
"/^func awsRestxml_deserializeOpError<Op>\(/,/^}/"` over `deserializers.go`
for all 71 ops in one pass, `grep -oE '"[A-Za-z0-9]+"'` (digit class
included, per the issue's own warning), then dropping the `"UnknownError"`
placeholder every deserializer's default-branch initializes `errorCode` to
before the real `<Code>` is parsed off the wire (confirmed by reading
`awsRestxml_deserializeOpErrorActivateKeySigningKey`'s body — it is not a
real declared code, just a Go zero-value string).

**Reachability tracing**: for each of the 35 sentinels in `errors.go`, grep
found every backend/handler call site, the enclosing function determined the
originating backend method, and each such method's callers were traced
(directly, or via the shared helpers `checkTagResourceExists`,
`resolveZoneNameServers`/`resolveSourceZoneNameServers`,
`matchExistingHostedZone`, `CountResourceRecordSets`/`CountAssociatedVPCs`/
`CountZonesByReusableDelegationSet`) up to the handler function and from
there to its op, using this service's REST-XML path+method routing (mostly
1:1 handler-function-to-op, unlike header-dispatched JSON services). Every
call site was reachability-confirmed by reading the actual function body,
not inferred from naming.

**Result: all 35 rows' codes are declared by every op that can reach them.
Zero rows emit an undeclared code.** The 12 `Err*Record` sentinels
(`ErrInvalidARecord`, `ErrInvalidAAAARecord`, etc.) exist in `errors.go` but
correctly have no table row: `validateChangeRecordValues` stringifies them
into `ErrInvalidAction`'s message via `err.Error()` rather than `%w`-wrapping
them, so they are never `errors.Is`-matchable and never reach
`handleBackendError` — confirmed by reading `record_sets.go`.

**Sentinel-with-no-row (the KMS bug class) check: zero found.** Every
`fmt.Errorf` in the package's non-test `.go` files wraps one of the 35 table
sentinels (`grep 'fmt\.Errorf(' *.go | grep -v 'Err[A-Za-z]'` returned
nothing) — there is no code path that can fall through to the generic
`InternalError` 500 for a case AWS models with a specific code.

**Dead-row check (no op can reach it): 1 found.** `ErrNoSuchGeoLocation` /
`"NoSuchGeoLocation"` has a table row, but `getGeoLocation`
(`handler_record_sets.go`) never constructs an error that wraps it — its
not-found branch calls `xmlError(c, http.StatusNotFound, "NoSuchGeoLocation",
"the specified geographic location was not found")` directly, bypassing
`ErrNoSuchGeoLocation`/`handleBackendError`/`backendErrorTable` entirely (the
same pattern that made 2 KMS sentinels fall through to a 500 in the prior
KMS sweep — except here the hardcoded call site happens to already emit the
*correct* code, so no wire-behavior bug results). Confirmed this is the only
such case: every other direct (non-`handleBackendError`) `xmlError(c, ...)`
call site in the package emits only `"InvalidInput"` (declared by all but 3
ops, and unreachable from those 3 anyway) or `"NoSuchOperation"`/`"InternalError"`
(routing/generic, outside the op-error cross-product).

**Left unfixed, for a judgement call**: whether to wire `getGeoLocation`'s
not-found branch through `ErrNoSuchGeoLocation`/`handleBackendError` (making
the row live, matching every other op's pattern — but with no black-box wire
difference, since the emitted status/code are already correct either way),
or instead delete the now-provably-dead row from `backendErrorTable`
(shrinks the table to what's actually reachable, but removes the guard that
would currently catch a future regression if someone *does* wire
`getGeoLocation` through the sentinel later and gets the mapping wrong). No
fix applied to `handler_record_sets.go` — it is byte-identical to before
this pass.

**Regression tests added** (both pass against the *current, unfixed* code,
by construction — the finding is a dead table row, not a wire-output bug):
`TestGetGeoLocation`'s `not_found` subtest (`record_sets_routing_test.go`)
now asserts the raw XML body contains `<Code>NoSuchGeoLocation</Code>`, not
just HTTP 404. `TestGetGeoLocation_NotFound_RealClient` and
`TestGetGeoLocation_Found_RealClient` (`error_path_sweep_test.go`) drive the
handler through the real `aws-sdk-go-v2` client (`newTestRoute53Client`,
already used by this file's `TestUpdateHostedZoneFeatures_UnknownZone_RealClient`)
and assert `errors.As` into the SDK's typed `*route53types.NoSuchGeoLocation`
for the failing case, and the correct `ContinentCode` round-trip for the
succeeding case. Each assertion was neutered (wrong wire-code string, wrong
typed error, wrong expected continent) and confirmed to still compile and
fail before being restored.

Error-response shape confirmed by reading `handler.go`'s `xmlError`: writes
`Content-Type: application/xml`, then `<?xml version="1.0"
encoding="UTF-8"?>` followed by an `<ErrorResponse
xmlns="…">Error>Type>Sender</Type><Code>…</Code><Message>…</Message>` body —
quoted verbatim from a neuter-run test failure diff:
`<ErrorResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/"><Error><Type>Sender</Type><Code>NoSuchGeoLocation</Code><Message>the specified geographic location was not found</Message></Error></ErrorResponse>`.

Gates: `GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/route53/...`
(pass), `GOTOOLCHAIN=go1.26.6 golangci-lint run services/route53/...` (0
issues). Files touched this pass: `record_sets_routing_test.go`,
`error_path_sweep_test.go`, this `PARITY.md` entry — no production code
changed.

## 2026-09-08: xmlError/handleBackendError nil-on-write fall-through audit (gopherstack-246v) -- clean

Part of the sweep following the elasticache fix (gopherstack-8haq): `xmlError`
(`handler.go:910`) writes the XML error body and unconditionally `return nil`s.
`handleBackendError` (`handler.go:998`) maps a backend sentinel error to a wire code via
`backendErrorTable` and either calls `xmlError` per match or falls back to a generic
`InternalError` — always via `return xmlError(...)`. Any helper that rejects through either
and is called by code storing and checking the result would get a silent nil and fall
through past the rejection.

**Method (mechanical).** route53 is the second-largest service in this sweep (23k lines,
flat package, no subdirectories). A `go/parser`/`go/ast` script computed the fixed-point
closure seeded with `{xmlError, handleBackendError}`: find every function with a bare
`return <sink>(...)`, add it, repeat to convergence. This folds the dispatch cross-reference
into the same pass — `Handler` (`handler.go:790`) is route53's dispatch entry (an
`echo.HandlerFunc` closure registered directly), and its unrecognized-action fallback is a
direct `return xmlError(...)`, so it and every `dispatchTable()` target reachable through a
default-case direct-return landed in the closure automatically; their own call sites were
swept in the same pass rather than partitioned separately.

The closure converged at 100 functions (the 2 seeds, `Handler`, and 97 discovered op
handlers — `createHostedZone`, `changeResourceRecordSets`, `listHealthChecks`, etc.). Every
call site of every one of those 100 was re-walked and classified: 331 total call sites
(production and test). 328 are `return <fn>(...)` (direct-return, safe). The remaining 3 are
`h.Handler()` calls in test files (`handler_paths_sdk_diff_test.go:144`,
`handler_test.go:629`, `iam_enforcement_test.go:88`) obtaining the exported
`echo.HandlerFunc`-returning method — a name collision with the discovered closure, not a
stored-then-checked error from any response-writer helper.

**No instance of the broken shape exists in route53.** No code changed. Gates:
`GOTOOLCHAIN=go1.27.0 golangci-lint run ./services/route53/...` 0 issues;
`GOTOOLCHAIN=go1.27.0 go test -race ./services/route53/...` ok.
