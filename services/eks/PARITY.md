---
# PARITY MANIFEST SCHEMA — see services/_PARITY_TEMPLATE.md for the schema doc.
service: eks
sdk_module: aws-sdk-go-v2/service/eks@v1.98.0
last_audit_commit: 7c297a53  # gopherstack-uult (2026-08-13) fixed after this hash was recorded; hash not yet known at edit time
last_audit_date: 2026-08-13
# ERROR path verified 2026-08-29 (wrapper-key-sweep pass): extracted every
# op's deserializeOpError<Op> switch (eks@v1.90.4 deserializers.go, 65 ops
# N-of-N). Handler.handleError is one global 4-sentinel table applied to all
# ops -- found systemic bug: ErrValidation's code was "InvalidParameterValueException",
# which does not exist anywhere in this SDK (0 occurrences); fixed to
# "InvalidParameterException", the code every op that models parameter
# validation actually uses (both errors.go and the handler.go literal fixed).
# Also fixed 4 wrong-code call sites where a real code was used but the
# specific op does not model it: CreateFargateProfile's cluster-not-found and
# duplicate-profile paths, CreateCapability's cluster-not-found path, and
# CreateNodegroup's cluster-not-found path all emitted ResourceNotFoundException/
# ResourceInUseException, unmodeled by those 3 ops -- now ErrValidation
# (InvalidParameterException), the only client-fault code each models.
# TagResource/UntagResource/ListTagsForResource route through a dedicated
# handleTagError instead of the global table: their own switches model only
# BadRequestException/NotFoundException, an entirely different exception
# family from the rest of this service. See error_sentinel_fixes_test.go
# (real-SDK errors.As assertions, each confirmed failing pre-fix).
# fargate_profiles_test.go/node_groups_test.go had 3 pre-existing tests
# asserting the old wrong status codes as correct; corrected alongside the fix.
overall: A            # route-matcher pass (prior audit) + gaps/deferred closeout pass (this audit)
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateCluster: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tp8x (2026-08-21): fixed the y1zn-deferred kubernetesNetworkConfig/networkingConfig split. Merged ElasticLoadBalancing *ElasticLoadBalancingConfig into the KubernetesNetworkConfig model/JSON struct as a sibling of IPFamily/ServiceIPv4CIDR/ServiceIPv6CIDR (matching types.KubernetesNetworkConfigRequest/Response, eks@v1.90.4 types/types.go:1597,1645); deleted the separate NetworkingConfig type and Cluster.NetworkingConfig field entirely, across clusters.go (ClusterOptionalConfig, resolveClusterOptionalConfig -- now returns 3 values not 4, new cloneKubernetesNetworkConfig helper for the nested-pointer deep copy), models.go, and handler_clusters.go (kubernetesNetworkConfigJSON gained ElasticLoadBalancing, networkingConfigJSON type deleted, createClusterBody.NetworkingConfig field deleted, buildClusterOptConfig's NetworkingConfig-building block deleted, appendClusterOptionalInfra's separate networkingConfig emission deleted, clusterNetConfigJSON now emits elasticLoadBalancing as part of the same object). A real client's ElasticLoadBalancing setting inside kubernetesNetworkConfig now round-trips both directions. Locked by TestCreateDescribeCluster_ElasticLoadBalancing_RealClient (real SDK client, both CreateCluster and DescribeCluster) and TestNetworkingConfig_RoundTrip (rewritten -- the old version sent/asserted the wrong top-level 'networkingConfig' key, ratifying the bug). gopherstack-tp8x (2026-08-21, follow-up): the Cluster shape change above went in without bumping eksSnapshotVersion, so a pre-fix snapshot's Cluster.NetworkingConfig.ElasticLoadBalancing would have silently vanished on restore into the new shape instead of the mismatch being caught. Bumped eksSnapshotVersion 1->2 (persistence.go) to force discard of any snapshot from before this shape changed."}
  DescribeCluster: {wire: fixed, errors: ok, state: ok, persist: ok, note: "see CreateCluster's gopherstack-tp8x note -- same clusterNetConfigJSON fix, shared by both ops."}
  ListClusters: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "now supports maxResults/nextToken pagination via pkgs/page (was returning the full list in one page). gopherstack ignored-parameter sweep (2026-08-29): Include (blank vs 'all') was declared by ListClustersInput but never read -- every cluster, including ones registered via RegisterCluster, was always returned. Now blank Include excludes clusters with a non-nil ConnectorConfig (connected/external clusters); Include=[all] includes them, matching the SDK doc. Backend ListClusters signature gained an includeExternal bool param"}
  DeleteCluster: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateClusterConfig: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "was routed as bare-path PUT /clusters/{name}; real path is POST /clusters/{name}/update-config. gopherstack-muzq (2026-08-21): the returned Update record was stamped InProgress and never advanced -- DescribeUpdate polled InProgress forever; now scheduled to Successful via scheduleUpdateTransition"}
  UpdateClusterVersion: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "was routed at fictional POST /clusters/{name}/update-version; real path is POST /clusters/{name}/updates (shared with ListUpdates GET). gopherstack-muzq (2026-08-21): same InProgress-forever bug and fix as UpdateClusterConfig"}
  RegisterCluster: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was routed at /clusters/{placeholder}/register; real path is global POST /cluster-registrations (name comes from body, always did)"}
  DeregisterCluster: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was routed as POST /clusters/{name}/deregister; real path is DELETE /cluster-registrations/{name}"}
  DescribeClusterVersions: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "gopherstack-g479 (2026-08-21): endOfStandardSupportDate/endOfExtendedSupportDate were static YYYY-MM-DD strings in a hand-built map[string]any table; real deserializers.go parses json.Number via ParseEpochSeconds. Confirmed against aws-sdk-go-v2/service/eks@v1.90.4's deserializers.go; failed with 'expected Timestamp to be a JSON Number, got string instead' pre-fix. Found via a new go/types-based map-literal kind scanner (map[string]any{} literals had zero automated coverage before this pass)."}
  AssociateEncryptionConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-g479 (2026-08-21): the returned Update.params was a hand-built {\"encryptionConfig\": ...} object; real Update.Params is an array of {type, value} pairs (deserializers.go's deserializeDocumentUpdate, case \"params\") with UpdateParamTypeEncryptionConfig = \"EncryptionConfig\". Failed with 'unexpected JSON type map[...]' pre-fix."}
  CreateNodegroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeNodegroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ListNodegroups: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now supports maxResults/nextToken pagination"}
  DeleteNodegroup: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateNodegroupConfig: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "was reachable on a bare POST to the nodegroup path with no suffix check, so real SDK traffic to .../update-config fell through with a corrupted nodegroupName (the literal suffix baked in); now requires the real /update-config suffix. gopherstack-muzq (2026-08-21): the Update record built in the handler was stamped InProgress and never advanced; now scheduled to Successful"}
  UpdateNodegroupVersion: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-muzq (2026-08-21): the returned Update record was stamped InProgress and never advanced -- DescribeUpdate polled InProgress forever; now scheduled to Successful via scheduleUpdateTransition"}
  CreateAddon: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "2026-08-21 (gopherstack-y1zn): addonToJSON (shared by Create/Describe/Delete) emitted \"marketplaceVersion\" and \"resolveConflicts\"; types.Addon has neither (real Marketplace field is the nested \"marketplaceInformation\" object, not tracked by this backend; resolveConflicts is CreateAddon/UpdateAddon request-only, never echoed). Both removed. Proven via TestAddon_NoMarketplaceVersionOrResolveConflicts_RealClient, hand-reverted/confirmed-failing/restored/md5sum-verified. 2026-09-07 (gopherstack-bs4t): CreateAddonInput.PodIdentityAssociations was declared by the model but never read by createAddonBody, so create-time associations were silently dropped. Fixed -- see the gopherstack-bs4t/wmuv note below."}
  DescribeAddon: {wire: fixed, errors: ok, state: ok, persist: ok, note: "see CreateAddon's gopherstack-y1zn note -- same addonToJSON fix."}
  ListAddons: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now supports maxResults/nextToken pagination"}
  DeleteAddon: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "see CreateAddon's gopherstack-y1zn note -- same addonToJSON fix. 2026-09-07 (gopherstack-wmuv): did not clean up the deleted add-on's owned pod identity associations, leaking a tags handle and an orphaned association. Fixed -- see the gopherstack-bs4t/wmuv note below."}
  UpdateAddon: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "was PUT to bare addon path; real op is POST .../addons/{addonName}/update. 2026-09-07 (gopherstack-tu95): PodIdentityAssociations was declared on the wire but never read -- its documented tri-state (absent=no change, []=delete all, populated=replace) was unimplemented. Addon also never surfaced its owned associations back (types.Addon.PodIdentityAssociations), so a delete could not be observed. See PodIdentityAssociation tri-state entry below."}
  DescribeAddonVersions: {wire: fixed, errors: ok, state: n/a, persist: n/a, note: "path was /addon-versions; real path is /addons/supported-versions — was completely unreachable by the real SDK client"}
  DescribeAddonConfiguration: {wire: fixed, errors: ok, state: n/a, persist: n/a, note: "path was /addon-configuration; real path is /addons/configuration-schemas — was completely unreachable. gopherstack-g479 (2026-08-21): configurationSchema was ALSO a nested JSON object where the real member (deserializers.go, case \"configurationSchema\": value.(string)) is the schema as a raw JSON string; failed with 'expected String to be of type string, got map[string]interface {} instead' pre-fix. Found via a new go/types-based map-literal kind scanner."}
  CreateAccessEntry: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAccessEntry: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added ModifiedAt (real aws-sdk-go-v2/service/eks/types.AccessEntry.ModifiedAt was entirely unmodeled); set on create and every update"}
  ListAccessEntries: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "now supports maxResults/nextToken pagination. gopherstack ignored-parameter sweep (2026-08-29): AssociatedPolicyArn was declared by ListAccessEntriesInput ('only the access entries associated to that access policy are returned') but never read -- every access entry in the cluster was always returned. Now filters via a per-entry ListAssociatedAccessPolicies lookup"}
  DeleteAccessEntry: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAccessEntry: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was routed as PUT; real method is POST to the same leaf path. Also now sets ModifiedAt"}
  AssociateAccessPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateAccessPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAssociatedAccessPolicies: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "now supports maxResults/nextToken pagination"}
  ListAccessPolicies: {wire: fixed, errors: ok, state: n/a, persist: n/a, note: "wire key for each entry was 'policyArn'; real aws-sdk-go-v2/service/eks/types.AccessPolicy field (deserializers.go's awsRestjson1_deserializeDocumentAccessPolicy) is 'arn' -- 'policyArn' is the correct key for AssociatedAccessPolicy elsewhere in this API but was wrong here. Also now supports maxResults/nextToken pagination"}
  CreateFargateProfile: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added Health (real aws-sdk-go-v2/service/eks/types.FargateProfile.Health was entirely absent from the wire response, not just unmodeled in the struct)"}
  DescribeFargateProfile: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same Health fix as CreateFargateProfile"}
  ListFargateProfiles: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now supports maxResults/nextToken pagination; no UpdateFargateProfile exists in the real API either"}
  DeleteFargateProfile: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePodIdentityAssociation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added ModifiedAt/ExternalId/Policy/DisableSessionTags -- all real aws-sdk-go-v2/service/eks/types.PodIdentityAssociation fields that were entirely unmodeled"}
  DescribePodIdentityAssociation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same field additions as CreatePodIdentityAssociation"}
  ListPodIdentityAssociations: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "was emitting the FULL PodIdentityAssociation shape (roleArn/createdAt/tags included); real ListPodIdentityAssociations returns the PodIdentityAssociationSummary shape which deliberately omits those fields -- verified against types.PodIdentityAssociationSummary. Also now supports maxResults/nextToken pagination. gopherstack ignored-parameter sweep (2026-08-29): Namespace/ServiceAccount were declared by ListPodIdentityAssociationsInput but never read -- every association in the cluster was always returned regardless"}
  DeletePodIdentityAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePodIdentityAssociation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was routed as PUT; real method is POST to the same leaf path. Now also accepts Policy/DisableSessionTags and sets ModifiedAt"}
  AssociateIdentityProviderConfig: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "now captures groupsPrefix/usernamePrefix/requiredClaims (previously dropped) and generates a real ARN (previously unset). gopherstack-muzq (2026-08-21): Status was stamped CREATING and nothing ever advanced it -- no ticker, no later call, while sibling cluster/addon/nodegroup resources transition correctly; now scheduled to ACTIVE mirroring scheduleClusterActivation. gopherstack-i8lo (2026-08-22): oidc.identityProviderConfigName (OidcIdentityProviderConfigRequest, eks@v1.90.4 types/types.go:2120, required) was decoded but never validated -- a missing name silently defaulted to clientId instead of being rejected; ClientId/IssuerUrl (types.go:2115,2132) were already validated. Now rejects a missing identityProviderConfigName with InvalidParameterException."}
  DescribeIdentityProviderConfig: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "response was a flat {clusterName,name,type,status,oidc,createdAt} object; real shape (aws-sdk-go-v2/service/eks/types.IdentityProviderConfigResponse) nests the full OidcIdentityProviderConfig under an 'oidc' key with identityProviderConfigName/identityProviderConfigArn/clientId/issuerUrl/usernameClaim/usernamePrefix/groupsClaim/groupsPrefix/requiredClaims/tags/status fields, none of which matched gopherstack's flat shape. Route-match looseness (any 3rd path segment) is unchanged, still intentional"}
  ListIdentityProviderConfigs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now supports maxResults/nextToken pagination (envelope shape {name,type} pairs was already correct)"}
  DisassociateIdentityProviderConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCapability: {wire: fixed, errors: fixed, state: fixed, persist: fixed, note: "was a GLOBAL (non-cluster-scoped) resource keyed only by 'name' at POST /capabilities, which does not exist in the real API at all (fabricated path). Real Capability is cluster-scoped (unique CapabilityName per cluster) at /clusters/{clusterName}/capabilities and requires ClusterName+CapabilityName+Type+RoleArn+DeletePropagationPolicy. Rebuilt: composite-keyed store (capabilityKey), cluster-scoped route, required-field validation, capabilityName/clusterName/arn/type/roleArn/deletePropagationPolicy/createdAt/tags on the wire (was emitting only name/version/status under the wrong field name 'name' instead of 'capabilityName'). This pass additionally added ModifiedAt/Health/Configuration and accepts (but does not persist for idempotency) ClientRequestToken"}
  DescribeCapability: {wire: fixed, errors: fixed, state: fixed, persist: fixed}
  ListCapabilities: {wire: fixed, errors: fixed, state: fixed, persist: fixed, note: "was returning bare capability-name strings; real ListCapabilities returns CapabilitySummary objects (capabilityName/arn/status/type/version/createdAt/modifiedAt) -- verified against types.CapabilitySummary. Also now supports maxResults/nextToken pagination"}
  DeleteCapability: {wire: fixed, errors: fixed, state: fixed, persist: fixed}
  UpdateCapability: {wire: fixed, errors: fixed, state: fixed, persist: fixed, note: "was PUT; real method is POST to the same leaf path. ModifiedAt now set on every update; Health/Configuration were added to the model (see CreateCapability note) -- Configuration remains a passthrough map (no per-capability-type ArgoCd/Ack/Kro schema validation)"}
  CreateEksAnywhereSubscription: {wire: fixed, errors: fixed, state: fixed, persist: fixed, note: "path was /subscriptions; real path is /eks-anywhere-subscriptions — was completely unreachable. Also now validates the required 'term' field (unit must be MONTHS, duration must be 12 or 36 -- verified against types.EksAnywhereSubscriptionTerm) and models autoRenew/effectiveDate/expirationDate, none of which were previously modeled at all"}
  DescribeEksAnywhereSubscription: {wire: fixed, errors: ok, state: ok, persist: ok}
  ListEksAnywhereSubscriptions: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "now supports maxResults/nextToken pagination. gopherstack ignored-parameter sweep (2026-08-29): IncludeStatus was declared by ListEksAnywhereSubscriptionsInput but never read -- every subscription was always returned regardless of status"}
  DeleteEksAnywhereSubscription: {wire: fixed, errors: ok, state: ok, persist: ok}
  UpdateEksAnywhereSubscription: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was PUT; real method is POST to the same leaf path"}
  DescribeInsight: {wire: ok, errors: ok, state: n/a, persist: n/a, note: "content is synthetic/fabricated (pre-existing; AWS's real insight analysis cannot be emulated) but is now reachable at the correct path"}
  ListInsights: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "was GET; real method is POST (carries an optional filter body) — was unreachable by the real SDK client. Now also reads maxResults/nextToken from the POST body (not query params, since ListInsights carries no query string) and paginates. Was also emitting the FULL Insight shape (recommendation, plus the invented clusterName that neither Insight nor InsightSummary carries on the wire — the cluster is already identified by the URL path); real ListInsights returns types.InsightSummary, which omits recommendation/additionalInfo/categorySpecificSummary/resources entirely -- verified against types.InsightSummary. DescribeInsight's response still includes the invented clusterName (separate pre-existing bug, out of scope for this pass). kubernetesVersion/name (InsightSummary members) have no honest source in this backend's Insight model and are left absent rather than fabricated. gopherstack ignored-parameter sweep (2026-08-29): the body's 'filter' key (InsightsFilter.categories/statuses/kubernetesVersions) was not parsed at all. Now filters by categories/statuses (both modeled on this backend's synthetic Insight); kubernetesVersions is left unapplied -- Insight has no version field to filter against, and fabricating one was rejected"}
  StartInsightsRefresh: {wire: fixed, errors: ok, state: n/a, persist: n/a, note: "was routed/shaped as a per-insight, per-refresh-id nested resource (/insights/{id}/refresh); real API is a cluster-level singleton at /clusters/{name}/insights-refresh with no id at all. Response was also wrongly nested under an 'insightsRefresh' envelope key; real fields (message/status/startedAt/endedAt) are at the response root"}
  DescribeInsightsRefresh: {wire: fixed, errors: ok, state: n/a, persist: n/a, note: "same fixes as StartInsightsRefresh"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "extended to find Capability ARNs too"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "genuinely has no maxResults/nextToken in the real API (ListTagsForResourceInput has neither field) -- not a gap"}
  DescribeUpdate: {wire: ok, errors: ok, state: ok, persist: ok}
  ListUpdates: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "now supports maxResults/nextToken pagination. gopherstack ignored-parameter sweep (2026-08-29): NodegroupName was declared by ListUpdatesInput but never read; added Update.NodegroupName (backend-internal, json:\"-\", not part of the real wire shape) populated by UpdateNodegroupVersion/UpdateNodegroupConfig, and ListUpdates now filters by it. AddonName/CapabilityName remain unfixed -- no Update record is ever created for UpdateAddon/UpdateCapability in this backend (they return the mutated Addon/Capability directly, not an async Update the way the real API does), so there is nothing yet to filter; fixing those needs a separate, larger change to UpdateAddon/UpdateCapability's response shape"}
  CancelUpdate: {wire: fixed, errors: fixed, state: fixed, persist: fixed, note: "implemented for real: POST /clusters/{name}/updates/{updateId}/cancel-update. Real EKS only performs cancellation for VersionRollback update types that are still InProgress (Kubernetes version rollback on EKS Auto Mode clusters, per the op's doc comment); any other type/status now returns InvalidRequestException (new ErrInvalidRequest sentinel) rather than silently no-opping or 404ing. On success sets Status=Cancelled and a Cancellation{Status,Reason} record, matching types.Update.Cancellation/types.Cancellation. No public op creates a VersionRollback update in this SDK version (it is an AWS-internal transition), so the success path is only reachable by seeding an update via the existing exported StoreUpdate — tests exercise this directly"}
gaps:
  - "ListUpdates.AddonName/CapabilityName filters are unimplemented: UpdateAddon/UpdateCapability never create an Update record in this backend (they return the mutated resource directly), so there is no addon/capability-scoped Update to filter over yet"
  - "ListInsights.Filter.kubernetesVersions is unimplemented: Insight has no version field on this backend's synthetic model"
  - "Capability Configuration remains an untyped passthrough map — no per-CapabilityType (ArgoCd/Ack/Kro) schema validation of Configuration/UpdateCapabilityConfiguration, unlike the real API's discriminated CapabilityConfigurationResponse/UpdateCapabilityConfiguration union types"
  - "Insight/DescribeInsight content is fabricated/synthetic, not derived from real cluster analysis (pre-existing, inherent emulator limitation -- there is no real cluster to analyze)"
  - "types.InsightSummary/types.Insight's kubernetesVersion and name members have no honest source in this backend's Insight model and are left absent from both DescribeInsight and ListInsights rather than fabricated"
  - "DescribeInsight still emits an invented clusterName field that neither types.Insight nor types.InsightSummary carries on the wire (the cluster is already identified by the URL path); ListInsights was fixed to drop it this pass (gopherstack-uult) but DescribeInsight was out of scope for that fix"
  - "ClientRequestToken (CreateCapability, CancelUpdate, CreatePodIdentityAssociation, etc.) is accepted on the wire for shape parity but never used for idempotency dedup, matching this backend's in-memory non-durable nature; a real duplicate-request-with-same-token replay will create two resources instead of returning the first one"
deferred:
  - "AWS error-code granularity beyond ResourceNotFoundException/ResourceInUseException/InvalidParameterValueException/InvalidRequestException (added this pass for CancelUpdate's not-cancellable case) — ClientException/ResourceLimitExceededException/ServerException are not modeled/reachable anywhere in this backend; a full sweep of which ops can plausibly return them was not done this pass"
leaks: {status: clean, note: "worker.Group timers (cluster/nodegroup/fargate/addon CREATING->ACTIVE transitions) stopped via Handler.Shutdown->Backend.Close->work.Stop(); tags.Tags Prometheus-label objects closed on Delete/Reset for every resource type including Capability (closeIDPAndSubscriptionTagsLocked and DeleteCluster's cascade). No new goroutines/tickers introduced this pass -- CancelUpdate and pagination are synchronous request/response paths"}
---

## Notes

**2026-08-13 (gopherstack-jqh2 pass 3):** re-extracted all 65 ops' real
method+path directly from `eks@v1.90.4` serializers.go and drove them
through `ExtractOperation` via the new `handler_sdk_route_table_test.go`
(`TestExtractOperation_SDKRouteTable`, one subtest per op, `t.Parallel()`).
All 65 resolved correctly, including the several same-path/different-method
collisions this service's routing depends on (`/clusters/{name}/updates`
GET/POST, `/clusters/{clusterName}/insights-refresh` GET/POST,
`/clusters/{clusterName}/{access-entries,capabilities,
pod-identity-associations,eks-anywhere-subscriptions}/{id}`
GET/DELETE/POST). No pre-existing table existed to check. This confirms the
extensive 2026-07-12/07-23 route-matcher fixes documented below held under
the strong per-op SDK diff method — no new routing bugs found. This test is
now the permanent regression guard for route-table drift.

Protocol: REST-JSON (restjson1). All wire-shape and route facts in this file were
verified directly against `aws-sdk-go-v2/service/eks@v1.89.0`'s `serializers.go`
(`httpbinding.SplitURI(...)` + `request.Method = "..."` per
`awsRestjson1_serializeOp<Op>.HandleSerialize`) and `deserializers.go`
(`awsRestjson1_deserializeOpDocument<Op>Output` field switch statements), not
against gopherstack's own output — per the parity-principles memory.

### This pass (2026-07-23): gaps/deferred closeout

Starting point was the prior route-matcher/wire-shape audit's 5 gaps + 2
deferred items. All 5 gaps and both deferred items were addressed:

1. **CancelUpdate** — implemented for real (route, backend state transition,
   `Cancellation` record, `InvalidRequestException` for non-cancellable
   updates). See the `CancelUpdate` ops entry above for the full writeup.
2. **Pagination** — every List op that supports `maxResults`/`nextToken` in
   the real API (all of them except the genuinely-unpaginated
   `ListTagsForResource`) now does, via a shared `eksPaginationParams`/
   `eksPageResponse` helper pair in `helpers.go` built on `pkgs/page`.
   `ListInsights` is POST-only or so its pagination params come from the JSON
   body, not query params — handled as a special case.
3. **CreateEksAnywhereSubscription 'term'** — now required and validated
   (`unit` must be `MONTHS`, `duration` must be 12 or 36); the subscription
   record also now models `autoRenew`/`effectiveDate`/`expirationDate`, none
   of which existed before.
4. **Capability Configuration/Health/ModifiedAt/ClientRequestToken** —
   `ModifiedAt` and `Health` (always an empty-issues object, since this
   backend never generates real capability health problems) are now modeled
   and set on create/update. `Configuration` is modeled as an untyped
   passthrough map (see gaps: no per-type schema). `ClientRequestToken` is
   accepted on the wire but not used for idempotency (see gaps).
5. **Insight fabricated content** — left as-is; this is an inherent emulator
   limitation (there is no real EKS control plane to analyze), not something
   fixable by more wire-shape work. Documented as a permanent gap rather than
   silently dropped.

Deferred item 1 (full field-by-field audit of AccessEntry/FargateProfile/
PodIdentityAssociation/IdentityProviderConfig) turned up real wire bugs, not
just missing-but-harmless fields:

- **AccessEntry**: `ModifiedAt` was completely absent (real
  `types.AccessEntry.ModifiedAt`).
- **FargateProfile**: `Health` was completely absent from the wire response
  (real `types.FargateProfile.Health`), not merely unmodeled internally.
- **PodIdentityAssociation**: `ModifiedAt`/`ExternalId`/`Policy`/
  `DisableSessionTags` were absent. More importantly, **ListPodIdentityAssociations
  was emitting the wrong shape entirely** — the full `PodIdentityAssociation`
  object (including `roleArn`/`createdAt`/`tags`) instead of the real API's
  `PodIdentityAssociationSummary`, which deliberately omits those fields.
- **IdentityProviderConfig**: **DescribeIdentityProviderConfig's response
  shape was wrong** — gopherstack returned a flat
  `{clusterName,name,type,status,oidc,createdAt}` object; the real API nests
  everything under `{identityProviderConfig: {oidc: {...}}}` with
  `identityProviderConfigName`/`identityProviderConfigArn`/`clientId`/
  `issuerUrl`/`usernameClaim`/`usernamePrefix`/`groupsClaim`/`groupsPrefix`/
  `requiredClaims`/`tags`/`status` fields inside the `oidc` object. None of
  gopherstack's flat top-level keys matched what a real SDK client expects to
  unmarshal.
- **ListAccessPolicies** (found incidentally while touching this area): each
  entry's ARN field was keyed `"policyArn"`; the real
  `types.AccessPolicy` wire key is `"arn"` (`"policyArn"` is correct for the
  *different* `AssociatedAccessPolicy` type used by
  `ListAssociatedAccessPolicies`, which was already correct).
- **ListCapabilities** (found incidentally): was returning bare capability-name
  strings; the real API returns `CapabilitySummary` objects.

Deferred item 2 (error-code granularity) got a narrow, real fix: `CancelUpdate`
on a non-cancellable update now returns `InvalidRequestException` (new
`ErrInvalidRequest` sentinel, `awserr.ErrConflict`-backed) rather than a
generic `InvalidParameterValueException` or silently succeeding — this
matches the real API's documented "cancellation is only performed if the
update can be cancelled" behavior. Broader error-code coverage
(`ClientException`, `ResourceLimitExceededException`, `ServerException`)
remains deferred; see `deferred:` above.

### Route-matcher bug class (prior pass, 2026-07-12)

This service had a large number of the "whole family unroutable by a real SDK
client" bug class the audit brief called out (the same class that hit
services/backup). Two flavors:

1. **Wrong path.** `pathSubscriptions` was `/subscriptions` (real:
   `/eks-anywhere-subscriptions`), `pathAddonVersions` was `/addon-versions`
   (real: `/addons/supported-versions`), `pathAddonConfiguration` was
   `/addon-configuration` (real: `/addons/configuration-schemas`),
   `RegisterCluster`/`DeregisterCluster` were nested under
   `/clusters/{name}/register` / `/deregister` (real: global
   `/cluster-registrations` POST, `/cluster-registrations/{name}` DELETE — no
   cluster-scoping in the real API since the cluster doesn't exist in EKS yet
   when you register it), `UpdateClusterConfig` was a bare-path PUT on
   `/clusters/{name}` (real: POST `/clusters/{name}/update-config`),
   `UpdateClusterVersion` was posted to a fictional `/update-version` segment
   (real: POST `/clusters/{name}/updates`, the same path `ListUpdates` GETs),
   `UpdateNodegroupConfig`/`UpdateAddon` were missing their real
   `/update-config` / `/update` path suffixes entirely, and `Capability` was a
   fabricated **global** (non-cluster-scoped) resource at `/capabilities` when
   the real API is cluster-scoped at `/clusters/{clusterName}/capabilities`.
   Every one of these was **completely unreachable** by a real
   `aws-sdk-go-v2` client — the SDK would send requests to paths gopherstack's
   `RouteMatcher`/`parseEKSPath` never recognized.

2. **Wrong method.** Every `Update*` op whose real path is shared with its
   sibling `Describe`/`Delete` op (`UpdateAccessEntry`,
   `UpdateEksAnywhereSubscription`, `UpdatePodIdentityAssociation`,
   `UpdateCapability`) is **POST** in the real API, not PUT — gopherstack had
   routed all four as PUT. `ListInsights` is **POST** (it carries an optional
   filter body), not GET — gopherstack required GET.

`StartInsightsRefresh`/`DescribeInsightsRefresh` additionally had a wrong
*shape*: real EKS insights-refresh is a cluster-level singleton (no
per-refresh id at all, `/clusters/{name}/insights-refresh`) with response
fields (`message`, `status`, `startedAt`, `endedAt`) directly at the response
root; gopherstack invented a nested `insights/{id}/refresh[/{refreshId}]`
resource and wrapped the response in a nonexistent `insightsRefresh` envelope
key.

### Traps for the next auditor

- `DescribeIdentityProviderConfig`'s route match is intentionally loose (any
  POST to the 3rd path segment after `identity-provider-configs`, not just
  literal `describe`) — this is over-permissive but harmless since no other
  real op uses that path shape; don't "fix" it without checking test coverage
  first.
- `RegisterCluster`'s cluster name comes from the **request body**, never the
  URL — the handler already read `in.Name` from the body even before this
  pass's fix; only the route *path* was wrong (pointed at a
  `/clusters/{name}/register` shape that made the URL-derived name irrelevant
  anyway).
- `ListAccessPolicies`, `DescribeAddonVersions`, `DescribeAddonConfiguration`,
  `DescribeClusterVersions` are genuinely **global, static** endpoints (no
  `clusterName` in the path) — don't try to cluster-scope them.
- Timestamps are epoch-seconds JSON numbers everywhere (`createdAt`,
  `startedAt`, `endedAt`, etc.) via `.Unix()`, matching the SDK's
  `smithytime.ParseEpochSeconds` deserializers — already correct throughout,
  not something this pass needed to touch.
- `CancelUpdate`'s success path (VersionRollback + InProgress) cannot be
  reached through any other public op in this SDK version — AWS generates
  VersionRollback updates internally when a node rollback is triggered on an
  Auto Mode cluster, not via a documented Create/StartRollback op. Tests seed
  it directly via `Handler.Backend.StoreUpdate`.
- `page.New`'s `nextToken` is the empty string (omitted from the JSON
  response body via `eksPageResponse`) once a List's results are exhausted —
  don't expect a literal `null` in the map like the real SDK's Go struct
  (`*string` marshals to `null`); gopherstack's map-based JSON just omits the
  key, which decodes identically on the client side (`NextToken` stays nil).

## gopherstack-muzq (2026-08-21): resources stuck in a transitional status forever

Continues gopherstack-oc9v/gopherstack-muzq's cross-service sweep for resources
stamped with a transitional status at construction that nothing in the backend
ever advances -- a client polling for readiness never exits its loop, even
though the emulator answers 200 with a well-formed body every time.

**Two confirmed instances in this service, both fixed:**

- `AssociateIdentityProviderConfig` stamped `IdentityProviderConfig.Status` as
  `CREATING` and nothing else in the backend ever wrote to that field --
  `clusters.go`/`addons.go`/`node_groups.go`'s sibling `CREATING`→`ACTIVE`
  transitions (via `b.work.After` + a `*TransitionDelay` constant) are the
  correct pattern in this exact service; `identity_providers.go` alone never
  had one. Fixed by adding `identityProviderTransitionDelay` and scheduling the
  transition the same way, right after `Put`. The existing
  `TestIDPConfigCreatesAsCreating` asserted `CREATING` immediately after
  associate -- true, but it never checked the status ever changed -- and was
  strengthened with a `require.Eventually` block confirming `ACTIVE`.
- `UpdateClusterConfig`/`UpdateClusterVersion`/`UpdateNodegroupVersion`, plus
  the node-group-config handler's inline `Update` construction, all stamped
  the returned `Update.Status` as `InProgress` and nothing ever advanced it --
  the only other writer of an `Update`'s `Status` is `CancelUpdate`, which only
  handles `VersionRollback`-type updates and only transitions to `Cancelled`.
  A real client's `DescribeUpdate` waiter (the standard EKS "did my update
  finish" poll) never saw a terminal status. Fixed via a new
  `scheduleUpdateTransition` helper (mirroring `scheduleClusterActivation`)
  called from all four sites, advancing `InProgress`→`Successful`.
  `TestDescribeUpdate_Status_Successful` is a striking case of this bug class:
  the test is *named* after the terminal state but its body asserted
  `InProgress` and stopped there. Also strengthened:
  `TestUpdateClusterConfig_Status_InProgress`,
  `TestUpdateNodegroupVersion_Status_InProgress`; added
  `TestNodegroup_UpdateConfig_UpdateRecordReachesSuccessful` for the handler
  path, which had no status test at all.

Both fixes reuse `b.work` (`*worker.Group`, already present on
`InMemoryBackend` and used by every sibling `*TransitionDelay` mechanism in
this service) -- no new async infrastructure was introduced.

Cleared (real advancing path found, not a bug): `CreateCluster`/`CreateAddon`/
`CreateFargateProfile`/`CreateNodegroup` all correctly schedule their own
`CREATING`→`ACTIVE` transitions already. `UpdateClusterVpcEndpoint` sets
`Successful` synchronously at creation (no async work exists for a pure
config-flag flip) -- correct as-is, not a bug.

Verified by hand-revert: each fix's file was reverted to its pre-fix `git
show HEAD:<path>` content, the corresponding test failed with the predicted
symptom (`Condition never satisfied` -- the status stayed transitional), then
was restored and confirmed `md5sum`-identical to the fixed version.

### 2026-08-22 (gopherstack-i8lo): missing required-member validation

`AssociateIdentityProviderConfig`'s `oidc.identityProviderConfigName` is
marked `// This member is required.` on `OidcIdentityProviderConfigRequest`
(`aws-sdk-go-v2/service/eks@v1.90.4` `types/types.go:2120`, alongside
`ClientId:2115` and `IssuerUrl:2132`, also both required). The handler
(`handler_identity_providers.go`) already rejected a missing `clientId`/
`issuerUrl`, but a missing `identityProviderConfigName` silently defaulted to
`clientId` instead of being rejected -- so a request omitting a required
member was accepted and produced a config keyed under a name the caller never
supplied, something real AWS's `validateOpAssociateIdentityProviderConfigInput`
(`validators.go`) would reject before the request ever reaches the service.

Fixed by adding the same `== ""` rejection used for the other two required
fields, and removing the `clientId`-fallback default.

The existing `TestEKS_AssociateIdentityProviderConfig/associate_idp_config_success`
fixture (`identity_providers_test.go`) supplied only `issuerUrl`/`clientId`
and omitted `identityProviderConfigName` while asserting `200 OK` -- exactly
the fixture-ratifies-the-defect pattern this campaign keeps finding. Fixed to
include the name, and a new `associate_idp_config_missing_config_name`
subtest (expecting `400`) was added alongside it; `cluster_not_found` and
`duplicate` subtests, which also omitted the name, were updated to supply
one so they still test what their names say. `TestParseAssocPaths`
(`handler_test.go`) sent `"configName"` (a field the handler never decodes --
the real key is `identityProviderConfigName`), which also silently relied on
the same fallback; corrected to the real field name.

Confirmed via hand-revert: reverting `handler_identity_providers.go` to
`git show HEAD:services/eks/handler_identity_providers.go` made the new
`missing_config_name` subtest fail (`expected: 400, actual: 200`); restored
and `md5sum`-verified identical to the fix.

## Map-walk pagination sweep (2026-08-30, fix/wrapper-key-sweep-rds-cloudwatch-sqs-sns)

Audited every `sort.Slice` call and every `pkgs/page.New` call site (the
handler-level offset-index pager `eksPaginationParams`/`eksPageResponse`
feed) in `services/eks` for the "sort on a tie-prone field over
`store.Table.All()` (a map walk, unstable between calls), no unique
tiebreak" bug class. Discriminator: `.All()` is the bug source;
`store.Index.Get(clusterName)` (used by every cluster-scoped List* in this
service) is insertion-ordered and stable across calls, so a tie-prone sort
over it is provably harmless.

**Structurally almost entirely clean**: of 13 `page.New` call sites
(fargate profiles, access entries, access policies, associated access
policies, identity provider configs, addons, insights, clusters, pod
identity associations, capabilities, updates, node groups, subscriptions),
12 read from a `store.Index.Get(clusterName)` lookup (stable, matches the
discriminator), a `store.Table.Snapshot()` (`ListClusters` — deterministic
key-sorted, not `.All()`), a raw per-key slice lookup (associated access
policies), or a fully hardcoded static/derived list (`ListAccessPolicies`,
`ListInsights` — no backing store at all). None of these sort on a
non-unique key over an unstable source.

**One bug found and fixed**: `ListEksAnywhereSubscriptions`
(subscriptions.go) is the *only* eks List op that reads
`b.subscriptions.All()` (a genuine `store.Table` map walk) rather than an
index — subscriptions have no cluster to scope an index by. It sorted by
`Name` alone; `CreateEksAnywhereSubscription` never checks Name for
uniqueness (unlike real AWS, which does — a separate, pre-existing parity
gap not fixed here, out of scope), so two subscriptions can legitimately
share a Name. Proven via a new `AddSubscriptionInternal`-seeded test
(`subscriptions_test.go`) constructing 12 same-named subscriptions and
walking pages of 5 through the real HTTP handler 30x; failed on iteration 0
against unmodified code (7 of 12 survived one walk). Fixed: added `ID` (the
table's own key, `uuid`-derived, always unique) as the tiebreak.

Gates: `go build ./services/eks/...`, `go vet ./services/eks/...`,
`go test -race -count=1 ./services/eks/...` (pass), `golangci-lint run
./services/eks/...` (0 issues).

## 2026-08-30 enumcheck typed-response-struct extension: 34 findings, all false positives

`cmd/enumcheck` was extended to see an enum value carried on a named
response struct's own composite literal, not only a `map[string]any` entry.
Run against `services/eks`, it surfaced 34 needs-review findings, all under
one wire key ("status" or "type") that is genuinely ambiguous SDK-wide —
shared by 11 (status) or 5 (type) unrelated real enums in
`eks@v1.90.4/types/enums.go`. Hand-checked every distinct value against the
enum its owning field's name actually indicates (`Cluster.Status` →
`ClusterStatus`, `Addon.Status` → `AddonStatus`, `Update.Status`/`.Type` →
`UpdateStatus`/`UpdateType`, `InsightsRefresh.Status` →
`InsightsRefreshStatus`, `AnywhereSubscription.Status` →
`EksAnywhereSubscriptionStatus`, etc.): every value is a real, legal member
of its true single candidate (e.g. `"InProgress"` = `UpdateStatusInProgress`,
`"AddonUpdate"` = `UpdateTypeAddonUpdate`, `"COMPLETED"` =
`InsightsRefreshStatusCompleted`) — it only fails the ambiguous-key tier's
"legal in every candidate" check because the other ~10 unrelated enums
sharing the wire key don't declare that member. No bug found; nothing
changed in this service.

## 2026-08-31 cmd/errtargetaudit sweep: 49 findings, all false positives (tool mechanism identified)

`go run ./cmd/errtargetaudit -dir eks` (65/65 operations resolved, no
coverage warning) reported 49 class-A findings — 48 operations "sending"
`NotFoundException`, one (`TagResource`) "sending" `InvalidParameterException`
— against real ground truth confirmed per-op in
`aws-sdk-go-v2/service/eks@v1.90.4/deserializers.go`
(`awsRestjson1_deserializeOpError<Op>` shape): every one of the 48 does
legitimately declare `ResourceNotFoundException`, not `NotFoundException`
(only `ListTagsForResource`/`TagResource`/`UntagResource` — the older tagging
API family — genuinely declare `NotFoundException`); `TagResource` genuinely
declares no `InvalidParameterException` (only `BadRequestException`/
`NotFoundException`).

**Traced to the actual runtime behavior first, not just the tool's static
resolution.** `handler.go`'s central `handleError` maps
`errors.Is(err, ErrNotFound)` → `"ResourceNotFoundException"` for every
non-tag operation — correct. `handler_tags.go`'s separate `handleTagError`
maps the *same* `ErrNotFound` identifier → `"NotFoundException"` for
`TagResource`/`UntagResource`/`ListTagsForResource` only — also correct,
with its own dated comment explaining why (the tagging API's deserializer
models a different exception family). Both are genuinely correct at their
own call sites; a real client hitting any of the 48 flagged operations
receives `ResourceNotFoundException`, matching what that operation declares.

**Root cause of the false positives: the tool's `sentinelCodes` builds one
flat map keyed by sentinel *identifier name* across the whole package**
(`cmd/errtargetaudit/classifiers.go`'s `sentinelCodes`/`addSwitchSentinelCodes`).
Both `handleError` and `handleTagError` contain an
`errors.Is(err, ErrNotFound)` branch; since both use the identifier
`ErrNotFound`, the second file scanned overwrote the first's entry for the
whole package, so every one of the 48 non-tag call sites got attributed the
*tag* mapper's code. The `TagResource` finding is the mirror case: the
`constructorCode` classifier resolved `validateTagMap`'s `return
ErrValidation` to the *service-wide* `ErrValidation`→`InvalidParameterException`
entry (correct for the resource-family ops that legitimately use it), but at
`TagResource`'s actual call site (`handler_tags.go`'s
`handleTagResource`) `validateTagMap`'s error is never dispatched through
`errors.Is`/`handleError` at all — it's caught by a bare `!= nil` check and
answered with a hardcoded `"BadRequestException"` literal, so
`ErrValidation`'s mapped code is unreachable from this operation regardless
of what the sentinel resolves to.

This is a **new, more precise instance** of the shared-mapper/unreachable-
branch false-positive shape from the prior calibration pass (`gopherstack-uox6`'s
CLASS-A ERROR SWEEP 4): not one mapper with an unreachable branch, but *two
separate mapper functions* colliding on the same sentinel identifier, where
the tool has no way to know which mapper a given operation's dispatch chain
actually reaches. Filed as a P2 tool-precision issue rather than acted on
further here (this pass's scope is the three named services, not the tool).

**Verdict: eks is clean.** No source changes. All 49 findings verified false
via the mechanism above, not dismissed by pattern alone — re-running the
tool post-investigation (no code changed) reproduces the identical 49,
confirming the tool's static resolution, not eks's runtime behavior, is the
source.

No web pages fetched this pass — everything came from the pinned SDK module
cache.

Gates: no source changes to eks; `go build ./services/eks/...`, `go vet
./services/eks/...`, `go test -race -count=1 ./services/eks/...`,
`golangci-lint run ./services/eks/...` all re-confirmed clean (unchanged from
before this pass).

## 2026-09-07 (gopherstack-tu95): UpdateAddon.PodIdentityAssociations tri-state was unimplemented

`UpdateAddonInput.PodIdentityAssociations` (aws-sdk-go-v2/service/eks@v1.90.4
`api_op_UpdateAddon.go`) is documented tri-state: "If this value is left
blank, no change. If an empty array is provided, existing associations owned
by the add-on are deleted." `updateAddonBody` in `handler_addons.go` never
declared the field at all, so it was silently dropped on every request
regardless of value. `types.Addon.PodIdentityAssociations []string` (the
association IDs owned by the add-on) was also never surfaced by `Addon` or
`addonToJSON`, so even a correct delete could not have been observed via
DescribeAddon.

Fixed both. `updateAddonBody.PodIdentityAssociations` is now
`[]addonPodIdentityAssociationBody` (roleArn/serviceAccount, matching
`types.AddonPodIdentityAssociations` -- no namespace member); its absent-vs-
`[]`-vs-populated tri-state is read directly off Go's `encoding/json`
nil-vs-non-nil-slice unmarshal behavior (a `[]T` field is left nil when its
JSON key is absent, and set to a non-nil, possibly-empty slice when the key
is present), the same idiom already used by
`services/backup/handler_frameworks.go`'s `UpdateFramework`/
`FrameworkControls`. The handler converts a non-nil `[]addonPodIdentityAssociationBody`
into `*[]PodIdentityAssociationSpec` (nil pointer = no change) and passes it
to `Backend.UpdateAddon`, which -- when non-nil -- deletes every
`PodIdentityAssociation` with `OwnerARN == addon.ARN` and creates one per
spec, recording the resulting association IDs on `addon.PodIdentityAssociations`.
`addonToJSON` now always emits `podIdentityAssociations` (empty array when
nil) so DescribeAddon can observe the result.

Addon-owned associations always land in the `kube-system` namespace
(`addonPodIdentityNamespace` const): this backend does not track a per-addon
`NamespaceConfig` override (CreateAddon's own `NamespaceConfig` field is
separately unwired -- see open item below), so there is no per-addon
namespace to use instead. `CreateAddon`'s own `PodIdentityAssociations`
field remains unwired (it has no tri-state semantics on Create, just a plain
create-time list) -- out of scope for gopherstack-tu95, filed as a follow-up.

Regression tests (`addon_pod_identity_test.go`), all HTTP-handler-driven,
each proven failing against unmodified code before this fix (reverted the
three production files, ran, confirmed 4 failures, restored):
- `TestAddon_UpdateAddon_PodIdentityAssociations_AbsentLeavesUnchanged`
- `TestAddon_UpdateAddon_PodIdentityAssociations_EmptyDeletesOnlyThisAddon`
  (creates associations on two addons, deletes one via `[]`, asserts the
  other addon's associations survive unchanged and ListPodIdentityAssociations
  returns exactly the survivor)
- `TestAddon_UpdateAddon_PodIdentityAssociations_PopulatedReplaces`
- `TestAddon_UpdateAddon_PodIdentityAssociations_RejectsMissingFields`
  (a populated entry missing roleArn/serviceAccount is rejected
  InvalidParameterException, no partial associations created)

`Addon` gained the `PodIdentityAssociations []string` field; the
`pkgs/persistence` snapshot-version guard confirms this is additive-only
(every existing field unchanged) and needs no `eksSnapshotVersion` bump --
only its golden `testdata/snapshot_inventory.json` fixture needs a
`-update` refresh, deliberately left for a separate pass.

## 2026-09-07 (gopherstack-bs4t, gopherstack-wmuv): CreateAddon dropped PodIdentityAssociations; DeleteAddon leaked its owned associations

Two follow-ups from gopherstack-tu95's UpdateAddon fix above, both in
`services/eks/addons.go`.

**gopherstack-bs4t.** `CreateAddonInput.PodIdentityAssociations`
(`api_op_CreateAddon.go`) is documented as a plain create-time list: "An
array of EKS Pod Identity associations to be created. Each association maps
a Kubernetes service account to an IAM role." -- no tri-state wording at
all, unlike `UpdateAddonInput`'s field. `createAddonBody` in
`handler_addons.go` never declared the field, so create-time associations
were silently dropped. Fixed: `createAddonBody` now carries
`PodIdentityAssociations []addonPodIdentityAssociationBody` (the same body
type `updateAddonBody` already used), converted to `[]PodIdentityAssociationSpec`
and passed as a new trailing parameter to `Backend.CreateAddon`, which
validates it (`validatePodIdentityAssociationSpecs`, extracted from
`UpdateAddon`'s inline check and now shared by both) and reuses
`replaceAddonPodIdentityAssociationsLocked` to create one association per
spec (its delete-owned-first step is a no-op on a brand-new addon).

**gopherstack-wmuv.** `DeleteAddonInput.Preserve`'s doc comment: "Specifying
this option preserves the add-on software on your cluster but Amazon EKS
stops managing any settings for the add-on. If an IAM account is associated
with the add-on, it isn't removed." The `DeleteAddon` op doc comment itself
says nothing about pod identity associations; `Preserve`'s doc is the only
evidence, and its wording only makes sense if the default (`preserve=false`)
path *does* remove the associated IAM account -- confirming wmuv's claim
by negative implication. `DeleteAddon` never read a `preserve` flag at all
and never touched `PodIdentityAssociation`, so owned associations survived
their deleted owner and leaked a `tags.Tags` Prometheus-label handle. Fixed:
`DeleteAddonInput`'s `preserve` query parameter (serializers.go's
`awsRestjson1_serializeOpHttpBindingsDeleteAddonInput` -- `encoder.SetQuery("preserve")`,
only emitted when true) is now read in `handleDeleteAddon` via
`strconv.ParseBool` and passed to `Backend.DeleteAddon`, which -- unless
`preserve` is set -- calls `replaceAddonPodIdentityAssociationsLocked` with
a nil spec list to delete every association owned by the add-on (`OwnerARN
== addon.ARN`) before removing it, reusing the exact same helper as
gopherstack-tu95 and gopherstack-bs4t rather than a third parallel
implementation.

Both `CreateAddon` errors that podIdentityAssociations validation feeds
(missing `roleArn`/`serviceAccount`) confirmed declared for `CreateAddon` in
`deserializeOpErrorCreateAddon` (`InvalidParameterException`, already the
shared `ErrValidation` sentinel).

Regression tests (`addon_pod_identity_test.go`), all HTTP-handler-driven:
- `TestAddon_CreateAddon_PodIdentityAssociations_Populated` (create with two
  associations, asserts DescribeAddon reports exactly those two, each
  association's roleArn/serviceAccount and ownerArn verified via
  DescribePodIdentityAssociation/ListPodIdentityAssociations) -- fails
  against unmodified code (0 associations instead of 2)
- `TestAddon_CreateAddon_PodIdentityAssociations_Absent` (create without the
  field, asserts `[]`)
- `TestAddon_CreateAddon_PodIdentityAssociations_RejectsMissingFields`
  (a populated entry missing serviceAccount is rejected
  InvalidParameterException and leaves no partially created addon) -- fails
  against unmodified code (200 instead of 400, addon exists)
- `TestAddon_DeleteAddon_CleansUpOwnedPodIdentityAssociations` (creates a
  second addon with its own association, deletes the first, asserts via
  ListPodIdentityAssociations that only the second addon's association
  survives) -- fails against unmodified code (3 survivors instead of 1)
- `TestAddon_DeleteAddon_Preserve_KeepsOwnedPodIdentityAssociations`
  (deletes with `?preserve=true`, asserts both associations still present)

`Addon`/`PodIdentityAssociationSpec` gained no new fields (bs4t/wmuv reuse
the `PodIdentityAssociations []string` field gopherstack-tu95 already added
to `Addon`); the `pkgs/persistence` snapshot-version guard was re-run
read-only and reports no drift.

Gates: `go build ./...`, `go vet ./services/eks/...`,
`go test -race -count=1 ./services/eks/...`, and
`golangci-lint run services/eks/...` all clean.

## 2026-09-07 (gopherstack-gala): CreateAddonInput.NamespaceConfig was unwired; addon-owned pod identity always landed in kube-system

Follow-up from gopherstack-tu95's "see open item below" note above.
`CreateAddonInput.NamespaceConfig *types.AddonNamespaceConfigRequest`
(`api_op_CreateAddon.go`): "The namespace configuration for the addon. If
specified, this will override the default namespace for the addon." --
confirmed a real, create-only field: `UpdateAddonInput` (`api_op_UpdateAddon.go`)
has no such member, so a real client cannot change an add-on's namespace
after creation. `Addon.NamespaceConfig *types.AddonNamespaceConfigResponse`
(`types.go:141`) echoes it back; both are `{"namespace": string}` on the wire
(`serializers.go` `awsRestjson1_serializeDocumentAddonNamespaceConfigRequest`,
`deserializers.go` `awsRestjson1_deserializeDocumentAddonNamespaceConfigResponse`,
confirmed against `request_snapshot_test.go`/`response_snapshot_test.go`).
`AddonInfo.DefaultNamespace` (`types.go:207`, on `DescribeAddonVersions`'
output) documents that the *default* namespace is genuinely per-addon ("if no
custom namespace is specified"), not uniformly `kube-system` -- `kube-system`
is simply the correct default for the AWS-managed add-ons this backend's
`DescribeAddonVersions` enumerates (vpc-cni, coredns, kube-proxy,
aws-ebs-csi-driver, aws-efs-csi-driver all install there in real EKS).

`createAddonBody` never declared `NamespaceConfig`, so it was silently
dropped, `Addon` never carried a namespace, and
`replaceAddonPodIdentityAssociationsLocked` (`addons.go`) always used the
`addonPodIdentityNamespace` constant. Fixed: `Addon` gained `Namespace
string` (`models.go`); `CreateAddon` takes it as a new parameter and stores
it; `createAddonBody` gained `NamespaceConfig *addonNamespaceConfigBody`;
`addonToJSON` emits `namespaceConfig: {namespace: ...}` when set;
`replaceAddonPodIdentityAssociationsLocked` now uses `addon.Namespace` when
non-empty, falling back to `addonPodIdentityNamespace` otherwise --
preserving the `kube-system` default gopherstack-tu95 chose. `UpdateAddonInput`
correctly has no `NamespaceConfig`, so `updateAddonBody` was deliberately left
without one: an inbound `namespaceConfig` key on an update body is silently
ignored by `encoding/json`, matching the real API's immutability.

Pod identity association resolution is affected (which namespace an
addon-owned association is installed into), not authorization/policy
resolution itself -- no IAM/trust-policy semantics changed.

Regression tests (`addon_namespace_test.go`), each proven failing against
unmodified code (captured before the fix):
- `TestAddon_NamespaceConfig_RoundTrip` (real `ekssdk.Client`
  CreateAddon+DescribeAddon) -- failed: `NamespaceConfig` nil on readback
  (`Expected value not to be nil`)
- `TestAddon_NamespaceConfig_PodIdentityAssociationUsesCustomNamespace` --
  failed: association namespace was `"kube-system"`, wanted `"efs-csi"`
- `TestAddon_NamespaceConfig_ImmutableOnUpdate` -- failed: `addon` had no
  `"namespaceConfig"` key at all yet (`NamespaceConfig` wasn't wired on
  create either)
- `TestAddon_NamespaceConfig_Absent_OmitsNamespaceConfig` and
  `TestAddon_NamespaceConfig_Absent_PodIdentityAssociationDefaultsToKubeSystem`
  passed unmodified (locking in the preserved default, not new behavior)
- `TestAddon_NamespaceConfig_PersistenceRoundTrip` (new, passes against the
  fix; see below)

`Addon` gained the `Namespace string` field: additive-only (every existing
field unchanged), snapshotted generically as plain JSON by
`pkgs/store/registry.go`'s `Registry.SnapshotAll`/`RestoreAll`, so no
`eksSnapshotVersion` bump. As with gopherstack-tu95's `PodIdentityAssociations`
addition, only the shared `pkgs/persistence/testdata/snapshot_inventory.json`
golden fixture needs a `-update` refresh (`go test ./pkgs/persistence/...
-run TestSnapshotVersionGuard -update`); left for a separate pass since that
file is outside `services/eks/` and also carries an unrelated pre-existing
`codedeploy` drift on this branch not caused by this change.

Gates: `go vet ./services/eks/...`, `go test -race ./services/eks/...`, and
`golangci-lint run ./services/eks/...` all clean.
