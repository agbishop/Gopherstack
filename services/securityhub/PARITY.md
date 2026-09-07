---
service: securityhub
sdk_module: aws-sdk-go-v2/service/securityhub@v1.75.4
last_audit_commit: 1659d616
last_audit_date: 2026-07-25
overall: A            # parity-4: 7 new SDK ops (CSPM Connectors CRUD+List, SecurityHub V2 opt-in
                       # Feature enable/disable) implemented for real against v1.75.0, wired into
                       # existing DescribeSecurityHubV2 state; one bonus fix (DescribeSecurityHubV2's
                       # wire shape had invented CreatedAt/UpdatedAt fields instead of the real
                       # SubscribedAt); one honestly-documented lifecycle gap (CSPM Connector health
                       # can never reach CONNECTED -- see gaps). Everything else re-verified accurate
                       # against the bumped SDK; no other new families found beyond the 7 assigned ops.
ops:
  EnableSecurityHub: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableSecurityHub: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-1qf: DisableHub never checked AWS's documented precondition (api_op_DisableSecurityHub.go) that the account isn't currently the Security Hub administrator -- CreateMembers is this backend's only path to that relationship (Organizations delegated admin never creates Member records). Now refused with InvalidAccessException while any member is non-Removed. See Notes."}
  DescribeHub: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateSecurityHubConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFindings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- SortCriteria is now applied (sortFindings), see Notes. ALSO FIXED this pass (gopherstack-uox6 value-semantics sweep) -- matchesStringFilter combined every entry of a field's []StringFilter list with a strict AND; types.StringFilter's doc comment documents CONTAINS/EQUALS/PREFIX entries on the same field joined by OR and NOT_CONTAINS/NOT_EQUALS/PREFIX_NOT_EQUALS joined by AND, the two groups then AND'd together. A real client's `Title CONTAINS X OR Title CONTAINS Y`-shaped filter (the documented example) matched nothing under the old code. Also affects BatchUpdateFindings/UpdateFindings, which share matchesFindingFilters. See Notes."}
  BatchImportFindings: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "re-import preserves Note/UserDefinedFields/VerificationState/Workflow per AWS's documented semantics. gopherstack-1qf: also now evaluates every ENABLED automation rule's Criteria against each imported finding and applies matching FINDING_FIELDS_UPDATE actions (ascending RuleOrder, stops at first terminal match) -- previously automation rules were pure CRUD with zero call sites evaluating Criteria/Actions against findings. See Notes."}
  BatchUpdateFindings: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateFindings: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFindingHistory: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass -- BatchImportFindings/BatchUpdateFindings/UpdateFindings now record real FindingHistoryRecord entries (findingHistory map, snapshot-persisted); GetFindingHistory returns them filtered by StartTime/EndTime and paginated. See Notes."}
  CreateInsight: {wire: ok, errors: ok, state: ok, persist: ok}
  GetInsights: {wire: ok, errors: ok, state: ok, persist: ok}
  GetInsightResults: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-1qf: ResultValues was always empty regardless of Filters/GroupByAttribute/finding count -- fixed via aggregateInsightResults, which reuses matchesFindingFilters/findingFieldString (same field-name-mapped subset GetFindings already supports) to group matching findings by GroupByAttribute and count. See Notes."}
  UpdateInsight: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteInsight: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchEnableStandards: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-muzq (2026-08-21): stamped StandardsStatus PENDING and nothing else in this backend ever advanced it -- EnableHub's own default-standards subscriptions are stamped the terminal READY directly at creation (no async work modeled for those either), which is exactly the sibling-resource contrast this bug class hides behind. Confirmed no async mechanism anywhere in the package (no ticker/goroutine/janitor/work.After/runDelayed/reconciler; grepped all non-test .go files). BatchDisableStandards's DELETING stamp is NOT this bug: it deletes the record synchronously and returns the transitional value on the removed copy, so a later GetEnabledStandards correctly omits it -- the ephemeral-response-literal shape, not a stall. Fixed via GetEnabledStandards, see below."}
  BatchDisableStandards: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETING is an ephemeral response literal returned after a synchronous delete (record is removed from the table in the same call) -- a later GetEnabledStandards correctly no longer returns it. Not the gopherstack-muzq stall pattern; left as-is."}
  GetEnabledStandards: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-muzq (2026-08-21): now advances any PENDING subscription to READY on first poll (new unexported StandardsSubscription.pollCount field), mirroring the reap-on-read pattern services/omics uses for Get*-advances-Creating resources -- no generated Get*Waiter ships for this op in this SDK version, but that only means a real caller must hand-roll its own poll loop, not that an unadvancing status is correct. TestBatchEnableStandardsPath (standards_test.go) previously asserted only the initial PENDING status and stopped; strengthened with a GetEnabledStandards follow-up asserting READY. New real-SDK-client proof: TestBatchEnableStandards_ReachesReady (wire_field_fixes_test.go). Hand-reverted standards.go+models.go to git show HEAD, confirmed both tests fail with StandardsStatus stuck at PENDING, restored, md5sum byte-identical."}
  DescribeStandards: {wire: ok, errors: ok, state: ok, persist: n/a, note: "static known-standards catalog, matches AWS ARNs/names"}
  DescribeStandardsControls: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-cf4j: control status is stored/echoed only, never consulted by a check engine -- structural, see triage section."}
  UpdateStandardsControl: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-cf4j: same as DescribeStandardsControls."}
  ListStandardsControlAssociations: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-cf4j: same as DescribeStandardsControls."}
  BatchGetStandardsControlAssociations: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-cf4j: same as DescribeStandardsControls."}
  BatchUpdateStandardsControlAssociations: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-cf4j: same as DescribeStandardsControls."}
  CreateActionTarget: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeActionTargets: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateActionTarget: {wire: ok, errors: fixed, state: ok, persist: ok, note: "gopherstack-02oa: never checked b.hubEnabled, unlike CreateActionTarget/every sibling create/enable path. deserializers.go's deserializeOpErrorUpdateActionTarget (:16987) models InvalidAccessException; added the check and mapped it. See action_targets_hub_enabled_test.go."}
  DeleteActionTarget: {wire: ok, errors: fixed, state: ok, persist: ok, note: "gopherstack-02oa: same hubEnabled gap as UpdateActionTarget; deserializeOpErrorDeleteActionTarget (:4539) models InvalidAccessException. See action_targets_hub_enabled_test.go."}
  DescribeProducts: {wire: ok, errors: ok, state: ok, persist: n/a, note: "static known-products catalog"}
  ListEnabledProductsForImport: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableImportFindingsForProduct: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableImportFindingsForProduct: {wire: ok, errors: fixed, state: ok, persist: ok, note: "gopherstack-02oa: same hubEnabled gap as UpdateActionTarget; deserializeOpErrorDisableImportFindingsForProduct (:7344) models InvalidAccessException. See action_targets_hub_enabled_test.go."}
  GetSecurityControlDefinition: {wire: ok, errors: ok, state: ok, persist: n/a, note: "static known-controls catalog"}
  ListSecurityControlDefinitions: {wire: ok, errors: ok, state: ok, persist: n/a}
  BatchGetSecurityControls: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- UnprocessedSecurityControl.ErrorCode is types.UnprocessedErrorCode (types.go:19946), an enum whose members are upper-snake-case (enums.go:2086); handler emitted the free-form string \"InvalidInput\" (shared with BatchUpdateFindings' unrelated *string ErrorCode) instead of the enum member \"INVALID_INPUT\". A typed client decoded the wrong value without error. See wire_field_fixes_test.go."}
  UpdateSecurityControl: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAutomationRules: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAutomationRule: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetAutomationRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- UnprocessedAutomationRule.ErrorCode is *int32 (types.go:19904, an HTTP status code like cloudfront's identically-shaped CustomErrorResponse.ErrorCode), not a string; handler emitted a string. Before the fix, a real client's deserializer hard-failed (\"expected Integer to be json.Number, got string instead\"), confirmed by driving the real client against the unfixed handler -- not a silent drop. See wire_field_fixes_test.go."}
  BatchDeleteAutomationRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- same UnprocessedAutomationRule.ErrorCode *int32 bug as BatchGetAutomationRules; see that row and wire_field_fixes_test.go."}
  BatchUpdateAutomationRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- same UnprocessedAutomationRule.ErrorCode *int32 bug as BatchGetAutomationRules; see that row and wire_field_fixes_test.go."}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateMembers: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMembers: {wire: ok, errors: ok, state: ok, persist: ok}
  GetMembers: {wire: ok, errors: ok, state: ok, persist: ok}
  InviteMembers: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED this pass -- see Notes"}
  ListMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "onlyAssociated=true filters on MemberStatus==Enabled, which no code path ever sets (member acceptance is a cross-account operation this single-account backend can't model) -- see gaps"}
  DisassociateMembers: {wire: ok, errors: ok, state: ok, persist: ok}
  AcceptAdministratorInvitation: {wire: ok, errors: ok, state: ok, persist: ok}
  AcceptInvitation: {wire: ok, errors: ok, state: ok, persist: ok}
  DeclineInvitations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- the unprocessed-account entry (account not found) fabricated \"ErrorCode\"/\"ErrorMessage\" keys; DeclineInvitationsOutput.UnprocessedAccounts is []types.Result (types.go:18271), which declares only AccountId/ProcessingResult -- same shape members.go's CreateMembers/DeleteMembers already use correctly. A typed client silently discards unknown keys and never observes them, so only a raw-body test catches this. See wire_field_fixes_test.go."}
  DeleteInvitations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- same types.Result ErrorCode/ErrorMessage fabrication as DeclineInvitations; see that row and wire_field_fixes_test.go."}
  GetInvitationsCount: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListInvitations: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAdministratorAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  GetMasterAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateFromAdministratorAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateFromMasterAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeOrganizationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateOrganizationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableOrganizationAdminAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableOrganizationAdminAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  ListOrganizationAdminAccounts: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateFindingAggregator: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFindingAggregator: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFindingAggregators: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateFindingAggregator: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteFindingAggregator: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateConfigurationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetConfigurationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateConfigurationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConfigurationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  ListConfigurationPolicies: {wire: ok, errors: ok, state: ok, persist: ok}
  GetConfigurationPolicyAssociation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- see Notes"}
  ListConfigurationPolicyAssociations: {wire: ok, errors: ok, state: ok, persist: ok}
  StartConfigurationPolicyAssociation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- see Notes"}
  StartConfigurationPolicyDisassociation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- see Notes"}
  BatchGetConfigurationPolicyAssociations: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableSecurityHubV2: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableSecurityHubV2: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeSecurityHubV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- see Notes (parity-4): real DescribeSecurityHubV2Output is {Features, HubV2Arn, SubscribedAt}, not {HubV2Arn, CreatedAt, UpdatedAt} as previously returned; now also reports the Features map (new in v1.75.0)."}
  EnableSecurityHubFeatureV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW op (parity-4). Gated on SecurityHub V2 being enabled (matches the real API's documented \"the service must be enabled before you can enable a feature\"); features live in HubV2.Features (map[string]*HubV2Feature), so they persist/reset with the V2 hub itself -- no separate state. Idempotent: re-enabling an already-ENABLED feature is a no-op."}
  DisableSecurityHubFeatureV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW op (parity-4). Same gating as EnableSecurityHubFeatureV2. Idempotent: disabling a never-enabled or already-DISABLED feature is a no-op that leaves the Features map unchanged (matches the real API's documented no-op semantics rather than fabricating a DISABLED entry for a feature never touched)."}
  CreateAggregatorV2: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAggregatorV2: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAggregatorsV2: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAggregatorV2: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteAggregatorV2: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAutomationRuleV2: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAutomationRuleV2: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAutomationRulesV2: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAutomationRuleV2: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED this pass -- see Notes"}
  DeleteAutomationRuleV2: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateConnectorV2: {wire: ok, errors: ok, state: ok, persist: ok}
  GetConnectorV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass (gopherstack-jo2r) -- handler dropped required Health/LastUpdatedAt/ProviderDetail entirely and emitted a fabricated \"UpdatedAt\" key where the real required key is \"LastUpdatedAt\" (securityhub@v1.75.4 api_op_GetConnectorV2.go:39-79), so a real client decoded a zero-value output. Now uses a dedicated connectorV2ToGetResponse mirroring the V1 CSPM connectorToGetResponse shape: Health.ConnectorStatus/LastCheckedAt and LastUpdatedAt all reuse ConnectorV2.UpdatedAt (this backend tracks one timestamp, not separate health-check/update times); ProviderDetail echoes Provider verbatim since ProviderConfiguration and ProviderDetail share the same union member tags (Azure/JiraCloud/ServiceNow)."}
  ListConnectorsV2: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateConnectorV2: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConnectorV2: {wire: ok, errors: ok, state: ok, persist: ok}
  RegisterConnectorV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass (gopherstack-4ggy) -- handler read a fabricated body[\"ConnectorId\"]; the real RegisterConnectorV2Input carries only AuthCode and AuthState, no ConnectorId at all (securityhub@v1.75.4 api_op_RegisterConnectorV2.go:26-40). AuthState's content is opaque to any real client (minted server-side, only round-tripped verbatim); this backend's documented convention is that AuthState IS the connector ID it was minted for. AuthCode is required and validated but not persisted -- no ConnectorV2 field or RegisterConnectorV2Output member models it."}
  CreateTicketV2: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass (gopherstack-u90v) -- handler previously read a fabricated TicketConfiguration/Tags request shape and returned a fabricated TicketConfigurationArn, neither of which exist on the real wire (securityhub@v1.75.4 api_op_CreateTicketV2.go:31-63: Input is ConnectorId/FindingMetadataUid[required]/ClientToken/Mode, Output is TicketId[required]/TicketSrcUrl). Now requires ConnectorId+FindingMetadataUid (400 ValidationException if absent), validates ConnectorId against the ConnectorV2 store (404 ResourceNotFoundException if unknown), rejects any Mode other than DRYRUN, and returns a generated TicketId. TicketSrcUrl is modeled but left permanently empty -- this backend has no real ITSM integration to source a URL from. FindingMetadataUid is required and stored but not validated against a real finding, matching BatchUpdateFindingsV2's documented metadataUids gap (no OCSF ingestion path hands out real metadata.uid values here). real SDK exposes only Create for TicketV2 -- no Get/List/Update/Delete to implement."}
  GetFindingsV2: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED this pass (gopherstack-8j08) -- DateFilters/MapFilters/IpFilters/BooleanFilters/NestedCompositeFilters, previously accepted on the wire and silently ignored (worse than unsupported: a caller got zero errors and unfiltered results), are now evaluated for the field subset genuinely backed by ASFF data this store carries. NestedCompositeFilters recurses fully (AND/OR, depth-capped) rather than being half-evaluated. See Notes for the full field-by-field crosswalk and what remains unmapped (documented, not fabricated)."}
  BatchUpdateFindingsV2: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass -- request now parses the real flat wire shape (Comment/SeverityId/StatusId/FindingIdentifiers/MetadataUids, not the nonexistent \"FindingFieldsUpdate\" wrapper); FindingIdentifiers now resolve via CloudAccountUid/FindingInfoUid/MetadataProductUid mapped onto the stored finding's AwsAccountId/Id/ProductArn. MetadataUids entries always report ResourceNotFoundException (documented gap -- this mock has no OCSF ingestion path that would ever hand a caller a real metadata.uid). See Notes."}
  GetFindingStatisticsV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass (gopherstack-4ggy request side, gopherstack-jo2r sweep response side) -- request read a fabricated body[\"GroupByAttributes\"] ([]string); the real required input member is GroupByRules ([]types.GroupByRule, {GroupByField,Filters} objects -- types.go:15710-15722). Response emitted \"FindingStatistics\"; the real key is \"GroupByResults\" ([]types.GroupByResult: GroupByField+GroupByValues[{FieldValue,Count}] -- types.go:15698-15707). GroupByField now echoes the client's requested OCSF name verbatim (e.g. \"severity\") while lookups translate it via the existing ocsfStringFieldMap onto the backend's ASFF storage key (e.g. \"SeverityLabel\"). Per-rule Filters accepted but not applied (matches GetResourcesV2's pre-existing filters-ignored convention)."}
  GetFindingsTrendsV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass (gopherstack-jo2r) -- handler emitted \"FindingsTrends\" and dropped required Granularity/TrendsMetrics (securityhub@v1.75.4 api_op_GetFindingsTrendsV2.go:22-58); the backend already computed real trend data, just under the wrong key. Also found by reading the full real input: GetFindingsTrendsV2Input has no GroupByAttribute member at all, so that request-side read (always empty against a real client) was removed. Now returns one TrendsMetricsResult (Timestamp+TrendsValues.SeverityTrends) aggregating every stored finding's ASFF SeverityLabel into the 8 real bucket names; ASFF has no FATAL severity so that bucket is always 0 (documented, not fabricated). Granularity is derived from the requested time span via a documented heuristic, since the real API takes no Granularity input either -- it derives it server-side."}
  GetResourcesV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "resources derived live from V1 findings' Resources arrays -- reasonable given no separate resource ingestion API exists"}
  GetResourcesStatisticsV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass (gopherstack-4ggy request side, gopherstack-jo2r sweep response side) -- request read a fabricated body[\"GroupByAttributes\"]; real required input member is GroupByRules ([]types.ResourceGroupByRule -- types.go:17851-17862). Response emitted \"ResourceStatistics\"; real required key is \"GroupByResults\" (types.go:15698-15707). Same GroupByField-echo/lookup-translation shape as GetFindingStatisticsV2, but no OCSF->internal field map exists for resources (GetResourcesV2 has never honored Filters either), so lookups use the client's field name verbatim against this backend's ASFF Resource keys."}
  GetResourcesTrendsV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass (gopherstack-jo2r) -- handler emitted \"ResourcesTrends\" and dropped required Granularity/TrendsMetrics (securityhub@v1.75.4 api_op_GetResourcesTrendsV2.go:22-58). Also found by reading the full real input: GetResourcesTrendsV2Input has no GroupByAttribute member either, so that request-side read was removed. Now returns one ResourcesTrendsMetricsResult (Timestamp+TrendsValues.ResourcesCount.AllResources); Granularity uses the same time-span heuristic as GetFindingsTrendsV2."}
  DescribeProductsV2: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED this pass (gopherstack-jo2r) -- handler emitted \"Products\"; the real required key is \"ProductsV2\" ([]types.ProductV2 -- api_op_DescribeProductsV2.go:36-51), so a real client decoded a nil slice regardless of catalog content. Also renamed the per-item fields ProductV2 actually has (ProductV2Name, IntegrationV2Types) and dropped ProductArn, which ProductV2 has no member for at all (types.go:17113-17141); MarketplaceProductId left absent, no backing field on the shared Product model."}
  GenerateRecommendedPolicyV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tp8x (2026-08-21): returned {MetadataUid,Policy,GenerationTime}, a fabricated shape. GenerateRecommendedPolicyV2Output (securityhub@v1.75.4 api_op_GenerateRecommendedPolicyV2.go) has NO members at all besides ResultMetadata -- it only starts async generation. Fixed to return {}. NOTE: the y1zn filing's claim that this op 'is not a real operation at all' and the POST route is 'unreachable by any real client' is wrong -- verified against awsRestjson1_serializeOpGenerateRecommendedPolicyV2 (serializers.go), which sends POST /recommendedPolicyV2/{MetadataUid}, exactly this handler's route. A prior 'confirmed' verdict is not evidence; re-verify against the serializer before trusting a rejection note. Locked by TestGenerateGetRecommendedPolicyV2_RealClient."}
  GetRecommendedPolicyV2: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tp8x (2026-08-21): returned {MetadataUid,Policy,GenerationTime}; real GetRecommendedPolicyV2Output is an async-retrieval-status shape (Status/RecommendationType/ResourceArn/RecommendationSteps/Error/NextToken, deserializers.go), not a returned policy document. RecommendationSteps is a union tagged \"UnusedPermissions\" (types.RecommendationStepMemberUnusedPermissions), each step carrying RecommendedAction/RecommendedPolicy/ExistingPolicy/ExistingPolicyId/PolicyUpdatedAt (types.UnusedPermissionsRecommendationStep). This backend generates synchronously so Status is always SUCCEEDED and Error/NextToken are never populated; ResourceArn is omitted (not tracked -- no Finding-to-metadataUid linkage exists in this backend, honest gap rather than a fabricated ARN). Locked by TestGenerateGetRecommendedPolicyV2_RealClient (real-client decode of the RecommendationStepMemberUnusedPermissions union member)."}
  # CSPM Connectors (parity-4, new in v1.75.0): third-party CLOUD PROVIDER
  # connectors (currently Azure only -- CspmProviderConfiguration is a
  # single-member union) that let Security Hub CSPM ingest findings/resource
  # data from a connected Azure environment. NOT the same family as
  # ConnectorV2/RegisterConnectorV2/TicketV2 above, which are Security Hub V2's
  # ticketing-system (Jira/ServiceNow) connectors -- naming collision in the
  # real API, kept distinct here as CspmConnector (non-V2 struct) vs
  # ConnectorV2. REST-path-based (POST/GET/PATCH/DELETE /connectors[/{id}]),
  # confirmed via serializers.go SetURI("/connectors"),
  # SetURI("/connectors/{ConnectorId+}") for all 5 ops.
  CreateConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW op (parity-4). Field-diffed against api_op_CreateConnector.go + types.CspmProviderConfiguration/AzureProviderConfiguration. Required Name/Provider return 400 if missing (client-side validation middleware in the real SDK, modeled defensively here). See gaps for the connector-status lifecycle limitation."}
  GetConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW op (parity-4). Field-diffed against api_op_GetConnector.go + types.CspmHealthCheck/CspmProviderDetail/AzureDetail. Response nests health under Health{ConnectorStatus,LastCheckedAt,Message,Issues} and provider detail under ProviderDetail{Azure:{...}}, matching the real tagged-union wire shape exactly (input Provider and output ProviderDetail share the same {\"Azure\": AzureDetail} shape, confirmed via serializers.go/deserializers.go)."}
  UpdateConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW op (parity-4). Field-diffed against api_op_UpdateConnector.go + types.AzureUpdateConfiguration -- UpdateConnectorInput has NO Name field (only ConnectorId/Description/Provider), and AzureUpdateConfiguration has no AWSConfigConnectorArn (immutable after create); both are honored here (Name update silently ignored per the real shape's absence of the field; AWSConfigConnectorArn merged forward from the original CreateConnector call rather than dropped)."}
  DeleteConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW op (parity-4). Field-diffed against api_op_DeleteConnector.go -- output is EnablementStatus only (PENDING_DELETION), no ConnectorId/Arn. This mock removes the record synchronously (no background worker to model AWS's async teardown window) but still reports PENDING_DELETION on the delete response itself for wire fidelity."}
  ListConnectors: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW op (parity-4). Field-diffed against api_op_ListConnectors.go + types.CspmConnectorSummary/CspmProviderSummary -- ConnectorStatus/EnablementStatus/ProviderName filters are query params (SetQuery, confirmed via serializers.go), not a JSON body, applied as exact-match filters against the stored connector fields."}
families:
  RouteMatcher: {status: ok, note: "every classifyPath prefix cross-checked against real serializers.go SetURI paths for all ~105 ops in aws-sdk-go-v2 v1.71.2 as of the last full audit; the parity-4 pass additionally verified the 7 new ops' SetURI paths (/connectors, /connectors/{ConnectorId+}, /hubv2/feature/{FeatureName}) against v1.75.0 and confirmed pathClassifiers orders the /connectorsv2 prefix before the new plain /connectors prefix (same ordering trap as /automationrulesv2 vs /automationrules -- see Traps below). RouteMatcher's unambiguous-prefix list covers every prefix classifyPath switches on; /findings and /tags/{arn} correctly disambiguated (Authorization signing-service header / ARN service segment) from other services sharing those prefixes (e.g. Macie2). No unreachable-op bugs found."}
  Persistence: {status: ok, note: "Handler.Snapshot/Restore (persistence.go) delegate to InMemoryBackend.Snapshot/Restore (backend.go), which round-trips every store.Table via registry.SnapshotAll/RestoreAll (store_setup.go) plus the 5 plain-map fields (tags, findings, controlParams, productSubscriptions, orgAdminAccounts) and all scalar/pointer fields. Verified store_setup.go registers exactly the set of *store.Table fields declared on InMemoryBackend -- no orphaned or unregistered table."}
gaps:
  - "ListMembers(onlyAssociated=true) can never return members: filters on MemberStatus==\"Enabled\", but nothing transitions a member to Enabled because member-invitation acceptance is a cross-account action this single-account in-memory backend doesn't model (the member's own account would call AcceptInvitation against ITS OWN backend instance, not the administrator's). Not attempted this pass -- architectural, not a bug-fix-sized change; would need a multi-backend cross-account simulation this service doesn't have."
  - "GetFindingsV2 Filters.CompositeFilters evaluates String/Number/Date/Map/Ip/Boolean filters and NestedCompositeFilters (gopherstack-8j08), but only for the field-name subset in ocsfStringFieldMap/ocsfNumberFieldMap/ocsfDateFieldMap/ipFieldNetworkKeys/mapFilterCandidates (findings_v2.go) that has a genuine ASFF-backed equivalent. Any OcsfStringField/OcsfNumberField/OcsfDateField/OcsfMapField/OcsfIpField/OcsfBooleanField outside those mapped subsets is accepted on the wire but not evaluated -- deliberately, per the no-fabrication rule, rather than guessed at. Remaining unmapped, with reasons: (a) fields with no ASFF concept at all -- OcsfBooleanField compliance.assessments.meets_criteria (ASFF Compliance has no 'assessments'), OcsfMapField databucket.tags (ASFF has no databucket concept), most 'evidences.*'/vendor_attributes.*' string+number fields (ASFF has no evidences/vendor_attributes objects); (b) fields whose only ASFF analog is lossy/ambiguous -- OcsfBooleanField vulnerabilities.is_fix_available (ASFF Vulnerability.FixAvailable is three-valued YES/NO/PARTIAL; collapsing PARTIAL into a bool would misclassify findings); (c) fields that exist in ASFF only nested inside arrays this pass didn't reach -- e.g. vulnerabilities.cve.cvss.base_score (Vulnerabilities[].Cvss[].BaseScore), resources.image.*/resources.modified_time_dt (ASFF Resource has no image/per-resource-modified timestamp). class_name (its closest analog, Types, is a string array, not scalar) remains unmapped from the prior pass. A complete OCSF taxonomy crosswalk is ~70 string + ~14 number fields; this pass closed the DateFilters/MapFilters/IpFilters/BooleanFilters/NestedCompositeFilters gap specifically (the issue's stated priority) plus one bonus NumberFilter field (confidence_score -> ASFF Confidence)."
  - "BatchUpdateFindingsV2 MetadataUids-based finding identification can never resolve (always ResourceNotFoundException): this backend has no OCSF ingestion path that would ever hand a real client a metadata.uid to reference back. Only FindingIdentifiers (CloudAccountUid/FindingInfoUid/MetadataProductUid, mapped onto AwsAccountId/Id/ProductArn) can resolve a finding."
  - "(parity-4) CSPM Connector health ConnectorStatus can never leave UNKNOWN, and EnablementStatus can never reach ENABLED: unlike Connectors V2 (which has a dedicated RegisterConnectorV2 to complete an out-of-band OAuth handshake), the real CreateConnector/GetConnector/UpdateConnector/DeleteConnector/ListConnectors surface has NO companion 'complete authorization' operation at all -- establishing connectivity to the Azure account requires a purely external, provider-side step (granting the AWSConfigConnectorArn role access in the Azure portal) that this mock has no API-observable signal for. Auto-advancing a connector to CONNECTED/ENABLED without any real client action causing it would be a fabricated transition, so CreateConnector leaves it at PENDING_ENABLEMENT/UNKNOWN and UpdateConnector leaves it at PENDING_UPDATE permanently. Not attempted this pass -- architectural (no out-of-band signal exists to model), not a bug-fix-sized change."
  - "(gopherstack-uox6 value-semantics sweep) GetFindingsV2's OcsfMapFilter (findings_v2.go matchesOcsfMapFilter/compareMapFilter) does not apply the same-field CONTAINS/EQUALS-joined-by-OR, NOT_CONTAINS/NOT_EQUALS-joined-by-AND combination rule that MapFilter's own doc comment documents (the same rule fixed this pass for V1's []StringFilter in matchesStringFilter) -- multiple OcsfMapFilter entries in one CompositeFilter's MapFilters list are instead combined via that CompositeFilter's explicit Operator (AND/OR), per matchesCompositeFilterDepth. Left unresolved rather than guessed: GetFindingsV2's OcsfFindingFilters model already exposes an explicit per-CompositeFilter Operator that V1's AwsSecurityFindingFilters has no equivalent of, and neither the MapFilter doc comment nor the OcsfFindingFilters/CompositeFilter doc comments state whether the legacy implicit per-field rule still applies underneath that explicit Operator, or is superseded by it, when a field's name repeats within one CompositeFilter's MapFilters list. Not attempted this pass -- the documentation does not specify this precisely enough to implement without fabricating a rule."
deferred: []
leaks: {status: clean, note: "no goroutines, tickers, or background loops in services/securityhub -- pure request-response over an in-memory store.Registry guarded by one lockmetrics.RWMutex. New findingHistory map (findings.go/store.go) follows the same plain-map + coarse-lock pattern as findings/tags -- every read/write path holds b.mu for the duration, no separate lock, no goroutines."}
---

## Notes

### parity-4 pass (2026-07-25): 7 new SDK ops from the v1.71.2 -> v1.75.0 bump

The Go SDK module was bumped, revealing 7 operations added to
`aws-sdk-go-v2/service/securityhub` since the previous audit: `CreateConnector`,
`GetConnector`, `UpdateConnector`, `DeleteConnector`, `ListConnectors` (a new
CSPM third-party cloud-provider connector family -- see the "Traps" note
above for why this is *not* the same as the existing `ConnectorV2` family),
and `EnableSecurityHubFeatureV2`/`DisableSecurityHubFeatureV2` (opt-in feature
toggles scoped to the existing SecurityHub V2 hub state). All 7 were
implemented for real (routing, backend state, request parsing, response wire
shapes field-diffed against the SDK's own `types`/`serializers.go`/
`deserializers.go`, error codes, HTTP status, Snapshot/Restore persistence)
and added to `GetSupportedOperations()` -- none went into the
`TestSDKCompleteness` `notImplemented` list (which stayed empty).

Key design decisions:

- **`EnableSecurityHubFeatureV2`/`DisableSecurityHubFeatureV2` are wired to
  the existing `HubV2` state, not an orphan boolean.** The real API's
  `/hubv2/feature/{FeatureName}` path and its documented "the service must be
  enabled before you can enable a feature" precondition both point at the
  existing V2 hub. Features are stored as `HubV2.Features
  map[string]*HubV2Feature` (new field on the existing struct) rather than a
  separate backend field, so they persist/reset with the V2 hub's own
  lifecycle for free (no new Snapshot/Restore wiring needed) and
  `DescribeSecurityHubV2` -- the existing op -- now reports them, matching
  the real `DescribeSecurityHubV2Output.Features` field that also arrived in
  this SDK bump.
- **CSPM Connectors' authorization lifecycle is modeled honestly, not
  auto-completed.** Unlike Connectors V2 (which has `RegisterConnectorV2` to
  complete an out-of-band OAuth handshake), the real CSPM Connector surface
  has no such operation at all -- see the `gaps` entry above. A connector
  created via `CreateConnector` is left at `EnablementStatus=PENDING_ENABLEMENT`
  / health `ConnectorStatus=UNKNOWN` permanently, since no real client action
  this backend can observe would legitimately advance it further.
- **Bonus fix, found while wiring `Features` into `DescribeSecurityHubV2`:**
  its response previously returned invented `CreatedAt`/`UpdatedAt` fields;
  the real `DescribeSecurityHubV2Output` (confirmed in both v1.71.2 and
  v1.75.0, so this predates the SDK bump) is `{Features, HubV2Arn,
  SubscribedAt}`. Fixed in the same handler function this pass touched
  anyway to add `Features`.

Fresh audit (this service had no PARITY.md before the 2026-07-23 pass).
Persistence (Handler.Snapshot/Restore delegating to InMemoryBackend) was
added recently and verified intact -- no changes needed there.

### Bugs fixed this pass

1. **`handler_configpolicy.go` -- ConfigurationPolicyAssociation `TargetType`
   always empty.** `GetConfigurationPolicyAssociation`,
   `StartConfigurationPolicyAssociation`, and
   `StartConfigurationPolicyDisassociation` all read a `"TargetType"` key out
   of the request's `Target` object. The real wire shape (`types.Target` is a
   Smithy tagged union -- see `serializers.go:34632
   awsRestjson1_serializeDocumentTarget`) never sends that field; the request
   is one of `{"AccountId":...}` / `{"OrganizationalUnitId":...}` /
   `{"RootId":...}` and `TargetType` (`ACCOUNT`/`ORGANIZATIONAL_UNIT`/`ROOT`)
   must be derived from which key is present. Every association response's
   `TargetType` field was silently empty for every real SDK client. Fixed by
   adding `extractConfigPolicyTarget` (derives ID + type from the union) and
   using it at all three call sites. Covered by
   `TestParity_ConfigurationPolicyAssociation_TargetTypeDerived`
   (`parity_d_test.go`).

2. **`backend_members.go` -- `InviteMembers` never validated the account
   exists.** AWS requires `CreateMembers` before `InviteMembers`; inviting an
   account that was never created must land in `UnprocessedAccounts`. The
   previous implementation unconditionally created an `Invitation` for every
   account ID with no existence check, so `UnprocessedAccounts` was always
   empty regardless of input validity -- a disguised no-op on the validation
   path. Fixed to check `b.members.Get(id)` first and populate
   `UnprocessedAccounts` (`ResourceNotFoundException`) for unknown accounts,
   matching the same pattern already used by `DeleteMembers`/`GetMembers`.
   Covered by `TestParity_InviteMembers_UnknownAccountUnprocessed`.

3. **`backend_v2.go` -- `UpdateAutomationRuleV2` silently dropped `Actions`
   updates.** The handler passes the raw decoded JSON request body straight
   through as `updates map[string]any`. A JSON array decodes into `[]any`
   (each element `map[string]any`), but the backend asserted
   `updates["Actions"].([]map[string]any)` directly -- an assertion that can
   never succeed against `[]any`, so every `Actions` update was silently
   dropped while every other field updated fine. Fixed to convert `[]any` ->
   `[]map[string]any` element-by-element, mirroring the pattern already used
   correctly in `BatchUpdateAutomationRules` (V1) and the V2 create handler.
   Covered by `TestParity_UpdateAutomationRuleV2_ActionsApplied`.

### Bugs fixed this pass (2026-07-23 gaps sweep)

4. **`findings.go` -- `GetFindings`/`GetFindingsV2` accepted `SortCriteria`
   but silently discarded it (results returned in map-iteration order,
   effectively random).** Added `sortFindings` (stable multi-key sort over
   `types.SortCriterion`'s `Field`/`SortOrder` "asc"/"desc" wire shape),
   wired into both `GetFindings` and the new `GetFindingsV2`. Covered by
   `TestGetFindings_SortCriteria` (`findings_test.go`).

5. **`findings.go` -- `BatchImportFindings` re-import overwrote
   `Note`/`UserDefinedFields`/`VerificationState`/`Workflow` instead of
   preserving them.** AWS documents ("After a finding is created,
   `BatchImportFindings` cannot be used to update the following finding
   fields...") that these four fields are retained from the finding's
   previous version regardless of what a re-import request supplies.
   `ImportFindings` previously did a flat `maps.Copy` that let any
   subsequent import silently reset a customer's investigation
   Note/Workflow/etc. Fixed with `preserveCustomerManagedFields`, which
   restores (or deletes, if never set) these fields from the prior stored
   version after every re-import. Covered by
   `TestBatchImportFindings_PreservesCustomerManagedFields`.

6. **`findings.go` -- `GetFindingHistory` was a hardcoded stub returning
   `{Records: []}` always; no finding-update history was ever recorded.**
   Added a `findingHistory map[string][]map[string]any` store field
   (snapshot-persisted alongside `findings`, same plain-map pattern) and
   `recordFindingHistory`/`diffFindingFields` helpers. `ImportFindings` now
   records a `FindingCreated: true` entry for new findings and a field-diff
   entry for re-imports; `BatchUpdateFindings` and `UpdateFindings` each
   record a field-diff entry per mutated finding (excluding the
   `CreatedAt`/`UpdatedAt`/`FirstObservedAt`/`LastObservedAt` timestamp
   fields AWS documents as excluded from history). `GetFindingHistory` now
   filters the recorded log by `StartTime`/`EndTime` and paginates it (100
   per page, matching AWS's documented cap). Covered by
   `TestGetFindingHistory_RecordsChanges` and
   `TestGetFindingHistory_UnknownFinding`.

7. **`handler_findings.go` -- `BatchUpdateFindingsV2` read a nonexistent
   `"FindingFieldsUpdate"` wrapper key.** The real
   `BatchUpdateFindingsV2Input` wire shape
   (`aws-sdk-go-v2/service/securityhub/api_op_BatchUpdateFindingsV2.go`) is
   flat: `Comment`, `FindingIdentifiers`
   (`[]types.OcsfFindingIdentifier`), `MetadataUids`, `SeverityId`,
   `StatusId` -- there is no wrapper object, so every real client request
   was silently a no-op. Additionally, `FindingIdentifiers` uses
   `CloudAccountUid`/`FindingInfoUid`/`MetadataProductUid`
   (`types.OcsfFindingIdentifier`), not V1's `ProductArn`/`Id`, so even
   after fixing the wrapper-key bug the old delegation to V1
   `BatchUpdateFindings` could never match a stored finding. Rewrote as a
   dedicated `BatchUpdateFindingsV2` backend method
   (`findings_v2.go`) that parses the flat request fields and resolves
   `CloudAccountUid`/`FindingInfoUid`/`MetadataProductUid` against the
   stored finding's `AwsAccountId`/`Id`/`ProductArn` -- the only viable
   mapping since this mock has no separate OCSF ingestion API (findings
   only ever enter via V1 `BatchImportFindings`). Covered by
   `TestBatchUpdateFindingsV2_WireShape` and
   `TestBatchUpdateFindingsV2_UnmatchedIdentifiers`
   (`findings_v2_test.go`).

8. **`handler_findings.go` -- `GetFindingsV2` `Filters` was passed straight
   to the V1 `matchesFindingFilters`, which looks for top-level
   `Id`/`ProductArn`/etc. keys.** The real `GetFindingsV2` `Filters` wire
   shape is `types.OcsfFindingFilters`:
   `{CompositeFilters: [...], CompositeOperator: "AND"|"OR"}`, each
   `CompositeFilter` holding `StringFilters`/`NumberFilters`/etc. keyed by
   an OCSF field name (`types.OcsfStringField`/`OcsfNumberField`) plus its
   own `Operator`. None of those keys exist in the V1 filter shape, so
   every real V2 client's `Filters` was a complete no-op (matched
   everything) rather than merely "unsorted" -- worse than the PARITY.md
   entry previously on file suggested. Added `matchesFindingFiltersV2` +
   `matchesCompositeFilter`/`matchesOcsfStringFilter`/`matchesOcsfNumberFilter`
   (`findings_v2.go`), which evaluate the real nested shape against a
   field-name-mapped subset of the stored ASFF finding (see
   `ocsfStringFieldMap`/`ocsfNumberFieldMap` and the residual-gap entry
   above). `severity_id`/`status_id` NumberFilters round-trip the
   `SeverityId`/`StatusId` fields `BatchUpdateFindingsV2` itself writes (fix
   #7), giving V2 update + V2 filter a coherent, testable round trip.
   Covered by `TestGetFindingsV2_CompositeFilters`.

### GetFindingsV2 composite filter taxonomy (gopherstack-8j08, this pass)

The previous pass (fix #8 above) evaluated only `StringFilters`/`NumberFilters`
within each `CompositeFilter`; `DateFilters`, `MapFilters`, `IpFilters`,
`BooleanFilters`, and `NestedCompositeFilters` were accepted on the wire and
silently ignored -- worse than an error, since a caller got HTTP 200 and an
unfiltered result set with no indication their filter did nothing. Field-diffed
the full real taxonomy (`types.CompositeFilter`, `types.Ocsf*Filter`,
`types.Ocsf*Field` enums, `types.StringFilter`/`MapFilter`/`DateFilter`/
`IpFilter`/`BooleanFilter`/`NumberFilter`/`DateRange`,
`types.AllowedOperators`/`StringFilterComparison`/`MapFilterComparison`/
`DateRangeComparison`/`DateRangeUnit`) against `aws-sdk-go-v2/service/
securityhub@v1.75.0`'s `types/types.go` and `types/enums.go` directly (not
against this handler's own prior output).

**Filter types implemented this pass**, each restructured into its own small
result-collector (`stringFilterResults`/`numberFilterResults`/
`dateFilterResults`/`mapFilterResults`/`ipFilterResults`/
`booleanFilterResults`/`nestedCompositeFilterResults`) feeding a single
`matchesCompositeFilterDepth` combinator (decomposed to keep CodeFactor's
Complex Method check quiet -- no `nolint`):

- **DateFilters** (`ocsfDateFieldMap`): `finding_info.created_time_dt` ->
  `CreatedAt`, `finding_info.first_seen_time_dt` -> `FirstObservedAt`,
  `finding_info.last_seen_time_dt` -> `LastObservedAt`,
  `finding_info.modified_time_dt` -> `UpdatedAt` -- all genuine ASFF
  finding-level timestamps. Both comparator shapes are implemented:
  absolute `Start`/`End` bounds (`matchesDateStartEnd`), and relative
  `DateRange{Comparison: WITHIN|OLDER_THAN, Unit: DAYS, Value}`
  (`matchesDateRange`) -- `WITHIN` matches at-or-after `now - Value days`,
  `OLDER_THAN` its strict complement. `resources.image.*`/
  `resources.modified_time_dt` have no ASFF equivalent (ASFF's `Resource`
  carries no image/per-resource-modified timestamp) and are unmapped.
- **MapFilters** (`mapFilterCandidates`): `resources.tags` -> per-resource
  `Resources[].Tags`, `finding_info.tags` -> the finding-level
  `UserDefinedFields` map (the closest real ASFF analog to a finding-level
  "tag"), `compliance.control_parameters` -> `Compliance.
  SecurityControlParameters[]{Name,Value[]}`. All four `MapFilterComparison`
  values implemented (`EQUALS`/`NOT_EQUALS`/`CONTAINS`/`NOT_CONTAINS`) via
  `compareMapFilter`, with positive comparisons OR'd and negative ones AND'd
  across multiple candidate values for the same key (mirrors the documented
  same-field combination rule). `databucket.tags` has no ASFF concept at all
  and is unmapped.
- **IpFilters** (`ipFieldNetworkKeys`): `evidences.src_endpoint.ip` ->
  `Network.SourceIpV4`/`SourceIpV6`, `evidences.dst_endpoint.ip` ->
  `Network.DestinationIpV4`/`DestinationIpV6` -- ASFF has no "evidences"
  concept, but `Network`'s source/destination IP fields are the only
  genuinely analogous data this store carries. `IpFilter` has only a `Cidr`
  field (no comparator) -- CIDR containment via `net.ParseCIDR`/
  `IPNet.Contains`, with a bare IP address normalized to an exact-match
  `/32` or `/128` per AWS's documented "CIDR block or single IP" input.
- **BooleanFilters**: only `vulnerabilities.is_exploit_available` is
  evaluated -- `Vulnerability.ExploitAvailable` is a genuine two-valued ASFF
  enum (`YES`/`NO`), so it round-trips to bool cleanly; a finding matches if
  ANY entry in its `Vulnerabilities` array has a matching value.
  `vulnerabilities.is_fix_available` is deliberately NOT evaluated:
  `Vulnerability.FixAvailable` is three-valued (`YES`/`NO`/`PARTIAL`), and
  collapsing `PARTIAL` into either `true` or `false` would silently
  misclassify findings -- worse than leaving it unfiltered.
  `compliance.assessments.meets_criteria` has no ASFF backing at all (no
  "assessments" concept on `Compliance`) and is also unmapped.
- **NumberFilters bonus**: added `confidence_score` -> ASFF's own top-level
  `Confidence` (int 0-100) to `ocsfNumberFieldMap` -- a clean scalar match
  found while auditing the taxonomy, not part of the original gap list.

**NestedCompositeFilters**: recurses fully via `matchesCompositeFilterDepth`
-- each nested `CompositeFilter` is evaluated as its own sub-tree (including
its own further `NestedCompositeFilters`) and the resulting bool joins its
*parent's* result list, combined by the parent's own `Operator`. This was
chosen over half-evaluating (e.g. only reading direct filters and ignoring
nesting) because a partially-evaluated boolean tree returns **wrong**
results, not merely unfiltered ones -- see the task's own warning, confirmed
by a regression-style test case
(`NestedCompositeFilters_AND_recurses_and_requires_both_branches`): a single
finding can't have two different `AwsAccountId` values, so ANDing two
mutually-exclusive nested branches must match zero findings; before this
fix (`NestedCompositeFilters` unevaluated -> empty result list -> vacuous
match-all), that same request would have wrongly matched both seeded
findings. Recursion depth is capped at `maxNestedCompositeDepth = 5` (AWS
documents the real structure as capped at 3 layers; 5 is a defensive margin
against a pathological/hand-crafted request, not a limit real traffic
should approach). Note `types.AllowedOperators` has only `AND`/`OR` -- there
is no logical NOT combinator in the real API; negation is expressed at the
leaf via `NOT_*` comparators (`NOT_EQUALS`/`NOT_CONTAINS`/
`PREFIX_NOT_EQUALS`), not a boolean-tree NOT node, so AND/OR recursion is
the complete real semantics.

**Comparator verification**: `StringFilterComparison`
(`EQUALS`/`PREFIX`/`NOT_EQUALS`/`PREFIX_NOT_EQUALS`/`CONTAINS`/
`NOT_CONTAINS`/`CONTAINS_WORD`) was already correctly implemented by
`compareStringFilter` (reused unchanged) -- confirmed against `types.go`'s
enum values and `StringFilter`'s doc comments describing each comparator's
exact semantics (including the CONTAINS_WORD-only-in-V2-APIs note).
`MapFilterComparison` (`EQUALS`/`NOT_EQUALS`/`CONTAINS`/`NOT_CONTAINS`, no
PREFIX variant -- confirmed the enum has no PREFIX member) implemented fresh
in `compareMapFilter` following the same positive-OR/negative-AND doc
pattern. `DateRangeComparison` (`WITHIN`/`OLDER_THAN`, default `WITHIN` per
doc) and the fact `DateRangeUnit` has only `DAYS` as of this SDK version
were both confirmed directly against `enums.go`. `NumberFilter` was
reconfirmed to have no `Comparison` field at all (`Eq`/`Gt`/`Gte`/`Lt`/`Lte`
only) -- unchanged from the prior pass.

Tests: extended `TestGetFindingsV2_CompositeFilters` (existing table) with
two `confidence_score` cases, and added a new table test
`TestGetFindingsV2_CompositeFilters_DateMapIPBooleanNested` covering every
implemented filter type with paired cases that each narrow to exactly one of
two seeded findings with deliberately divergent field values (proving actual
discrimination, not a "matches everything" false pass), plus the
AND/OR nested-recursion pair described above.

### Route-matcher check

Extracted every op's HTTP method + URI template directly from
`aws-sdk-go-v2/service/securityhub@v1.71.2/serializers.go`
(`awsRestjson1_serializeOpHttpBindings*` / `SplitURI` calls) for all ~105
operations and cross-checked against `classifyPath`'s per-family
`classify*Path` functions in `handler.go`, `handler_members.go`,
`handler_configpolicy.go`, and `handler_v2.go`. All method+path pairs match.
`RouteMatcher` (`handler.go`) was separately checked to confirm every prefix
`classifyPath` switches on is also covered by `RouteMatcher`'s
unambiguous-prefix OR-chain, so no routed op is reachable by `Handler()`
directly (bypassing the matcher, as unit tests do) but unreachable through
the real Echo route registration. No route-matcher bugs found in this
service.

### Traps for the next auditor

- `/automationrulesv2` is a `strings.HasPrefix` superset of `/automationrules`
  (both share the `/automationrules` substring) -- `classifyPath`'s switch
  correctly orders the V2 case before the V1 case. Don't "simplify" that
  ordering.
- (parity-4) Same trap, new pair: `/connectorsv2` is a `strings.HasPrefix`
  superset of the new plain `/connectors` (CSPM connectors). `pathClassifiers`
  in `handler.go` orders `hasPathPrefix(pathConnectorsV2)` before
  `hasPathPrefix(pathConnectors)` -- don't reorder or collapse them. Also note:
  `CreateConnector`/`GetConnector`/etc. (this pass) and
  `CreateConnectorV2`/`GetConnectorV2`/etc. are two *entirely unrelated* real
  AWS features that happen to share the word "connector" -- CSPM connectors
  link to third-party cloud providers (Azure), Connectors V2 link to
  third-party ticketing systems (Jira/ServiceNow). Modeled as distinct Go
  types (`CspmConnector` vs `ConnectorV2`) and distinct backend/handler files
  (`connectors.go`/`handler_connectors.go` vs
  `connectors_v2.go`/`handler_connectors_v2.go`) specifically to avoid
  conflating them.
- `classifyConfigPolicyPath`'s PATCH/DELETE cases match `/configurationPolicy/`
  with explicit exclusions for `create`/`get`/`list` suffixes rather than a
  positive `{Identifier}` pattern -- this is intentional (mirrors the real
  flat-path-segment routing) and correct as long as no real
  `ConfigurationPolicyIdentifier` value is literally `"create"`, `"get"`, or
  `"list"`.
- `BatchUpdateFindings`/`ImportFindings`/`GetFindings` do **not** check
  `hubEnabled` (unlike `UpdateFindings`/insights/action-targets). This was
  investigated and left as-is: AWS's own docs don't clearly state these ops
  require the hub to be enabled, and no existing test asserts either
  behavior, so flipping it risks breaking passing integrations without clear
  spec backing. Revisit if a concrete AWS error transcript surfaces.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 3 fixed, 2 deferred

Part of the gopherstack-us9u/g479 map-literal scanner's 526-key unknown-key
bucket triage. Fixed items proven via real `aws-sdk-go-v2/service/securityhub`
client round trips or raw-body assertion (`wire_field_fixes_y1zn_test.go`),
hand-reverted, confirmed failing, restored, `md5sum`-verified byte-identical.

- `ListAggregatorsV2`: {wire: fixed} -- wrapped the list under "Aggregators";
  real member (deserializers.go's
  awsRestjson1_deserializeOpDocumentListAggregatorsV2Output) is
  "AggregatorsV2".
- `DeclineInvitations`/`DeleteInvitations`: {wire: fixed} -- each emitted an
  extra "ProcessedAccounts" key alongside the real "UnprocessedAccounts";
  neither DeclineInvitationsOutput nor DeleteInvitationsOutput has a
  ProcessedAccounts member -- success is implied by an account's absence from
  UnprocessedAccounts, not a separate echo.
- `GenerateRecommendedPolicyV2`/`GetRecommendedPolicyV2`: {wire: fixed, note:
  "confirmed real bug, then deferred to gopherstack-tp8x, now fixed
  (2026-08-21) -- see the ops entries above for the full fix. The deferral
  note's claim that GenerateRecommendedPolicyV2 'is not a real operation at
  all' was itself wrong (verified: it is real, POST
  /recommendedPolicyV2/{MetadataUid}, matching this handler's existing
  route exactly) -- a reminder that a prior pass's rejection reasoning needs
  re-verification against the serializer, same as any other claim."}

## gopherstack-wlo1 (2026-08-22): dispatch-miss error path was the one call site gopherstack-aitg left untyped

gopherstack-aitg (2026-08-11, commit 695aa1c20) added a central error path
(`typedErrorResponse`, handler.go) and audited every named call site against
securityhub's real per-operation exception lists. `handleREST`'s own
dispatch-miss fallback -- reached when `classifyPath` returns `opUnknown`,
i.e. no `classify*Path` function recognises the request's method+path --
was not one of the sites that pass touched, and unlike every genuinely
ambiguous `ErrHubNotEnabled` site in this file (each carries a comment
explaining why it's deliberately left unheadered), this one had no such
note. It wrote `{"Message": "unknown operation"}` with no
`X-Amzn-Errortype` header and no body `code`/`__type` field, so
`restjson.GetErrorInfo` (aws-sdk-go-v2's
`aws/protocol/restjson/decoder_util.go`) had nothing to read and the error
deserialized client-side as `UnknownError` regardless of the underlying
cause.

Reachability: cross-checked every op constant this package wires into
`opHandlerGroups()`'s dispatch tables (116 distinct `map[string]func()`
entries) against every `api_op_*.go` file in the pinned
`securityhub@v1.75.4` module (116 real operations) -- exact 1:1 match, zero
missing. So this fallback is structurally unreachable for any
legitimately-constructed SDK request; it can only be reached by rewriting
the request after signing (proven below), the same white-box category as
medialive/mediatailor's analogous fixes in ea67f34cf.

Fixed: `handleREST` now calls `typedErrorResponse(c, http.StatusNotFound,
"ResourceNotFoundException", "unknown operation")` -- the same helper (and
the same code) `GetSecurityControlDefinition`'s unknown-control path
already uses (handler_error_type_test.go's existing
`TestGetSecurityControlDefinition_UnknownControlSurfacesResourceNotFoundException`),
so no new exception vocabulary was introduced.

Proof: `TestGetInsightResults_UnrecognisedRouteSurfacesResourceNotFoundException`
(handler_error_type_test.go) drives a real `securityhubsdk.Client`'s
`GetInsightResults` through a Finalize-stage middleware that rewrites the
signed request's path from `/insights/results/{InsightArn+}` down to bare
`/insights` -- still inside RouteMatcher's `/insights` prefix (so the
request still reaches this package's Handler) but matching none of
`classifyInsightsPath`'s method/path cases for a GET, landing in
`handleREST`'s fallback. Hand-reverted `handler.go` to `git show HEAD` (the
pre-fix state, still carrying the bare map literal), confirmed the test
fails with `apiErr.ErrorCode() == "UnknownError"`, restored the fix,
`md5sum`-confirmed byte-identical to the pre-revert file.

Not a repeat of the `ErrHubNotEnabled` ambiguity: those sites are ambiguous
*between two real, named exceptions* a specific operation models. This site
doesn't know the operation at all (routing itself failed), so there is no
per-operation vocabulary to disambiguate between -- a generic
`ResourceNotFoundException` (already the modeled 404 shape used elsewhere
in this file, e.g. `GetSecurityControlDefinition`) is the closest fit, not
a guess among named alternatives.

Confirmed still-deliberate and left untouched: every `ErrHubNotEnabled`
bare-message site (handler_hub.go, handler_insights.go, handler_findings.go,
handler_products.go, handler_action_targets.go) -- each carries its own
comment citing the specific operation's real error list from
`securityhub@v1.75.4 deserializers.go` and the reason no single exception
can be chosen without guessing. Re-spot-checked `DisableSecurityHubV2`
(deserializers.go:7744) directly: its real error list is
`AccessDeniedException`/`InternalServerException`/`ThrottlingException`/
`ValidationException` -- no `ResourceNotFoundException`, confirming the
comment's claim -- and left as documented rather than "resolved by
elimination", since the real "not enabled" AWS status for this call is not
independently verified here.

## Race-safety sweep (2026-08-22): shallow struct copies and live-pointer returns escaping the lock

CI's `unit-tests (3)` job flagged `-race` failures in
`TestExtractOperation_SDKRouteTable` on describesecurityhubv2 and the
enable/disable-feature subtests. Root cause: `DescribeSecurityHubV2`
(hub.go) did `cp := *b.hubV2` under `RLock` and returned `&cp` --
`HubV2.Features` is `map[string]*HubV2Feature`, so the copy's `Features`
field is the *same map* as the live `b.hubV2.Features`.
`handleDescribeSecurityHubV2` (handler_hub.go) ranges over that map *after*
`RUnlock` has already run, racing against `EnableSecurityHubFeatureV2`/
`DisableSecurityHubFeatureV2`'s `b.hubV2.Features[name] = &HubV2Feature{...}`
writes under `Lock`. `Hub` (v1) has no reference fields, so `DescribeHub`'s
identical-looking `cp := *b.hub` is genuinely safe and was left alone.

Reproduced directly (not just via the flaky parallel-subtest ordering CI
hit): `TestSecurityHubV2FeatureDescribeRace` (hub_test.go) drives
`DescribeSecurityHubV2` + `Enable/DisableSecurityHubFeatureV2` concurrently
against one backend. Confirmed failing pre-fix (`runtime.mapassign_faststr`
write vs. `runtime.mapIterStart`/`mapIterNext` read), hand-reverted `hub.go`
to the shallow-copy version, re-confirmed the same failure, restored,
`md5sum`-confirmed byte-identical. Fixed with `HubV2.clone()`, which
deep-copies `Features` (new map, new `*HubV2Feature` per entry).

Audited the rest of `services/securityhub/` for the same two shapes:

1. **A struct with a map/slice field is shallow-copied (`cp := *x`) while
   that field is mutated *in place* (indexed assignment) elsewhere under
   lock**, or is aliased with a map that's mutated in place elsewhere
   (`Tags` fields are assigned the exact same map object passed to
   `b.tags[ARN] = tags` at creation time, and `TagResource`/`UntagResource`
   mutate `b.tags[ARN]` in place via `maps.Copy`/`delete`).
2. **A live, stored `*T` (or one of its map/slice fields) is returned
   directly with no copy at all**, and that same object is later mutated in
   place (by an `Update*`, or by the same `Get`-style op itself, e.g.
   `GetEnabledStandards`'s poll-to-READY advance) under a subsequent lock
   acquisition.

Fixed (added a `.clone()` deep-copy method per type, used at every point the
value crosses the lock boundary -- Create/Get/List/Batch/Update returns):

- `ConfigurationPolicy` (configuration_policies.go): `Tags` aliases
  `b.tags[Arn]`; `ConfigurationPolicy` map cloned too for consistency.
  `CreateConfigurationPolicy` also returned the live stored pointer.
- `CspmConnector` (connectors.go): `Tags` aliases `b.tags[ConnectorArn]`
  (`Provider` cloned too). `CreateConnector` returned the live pointer.
- `ConnectorV2` (connectors_v2.go): same `Tags`/`Provider` shape.
  `CreateConnectorV2` returned the live pointer; so did `UpdateConnectorV2`
  and `RegisterConnectorV2` before their `cp := *target` was replaced with
  `.clone()`.
- `AutomationRule`/`AutomationRuleV2` (automation_rules.go):
  `BatchGetAutomationRules` returned live `*AutomationRule` pointers with no
  copy at all -- `BatchUpdateAutomationRules` mutates `RuleName`/
  `RuleStatus`/`Criteria`/`Actions`/etc. on that same object in place.
  `CreateAutomationRuleV2` likewise returned the live pointer, later
  mutated by `UpdateAutomationRuleV2`.
- `StandardsSubscription` (standards.go): `BatchEnableStandards` and
  `BatchDisableStandards` returned the live, stored pointer;
  `GetEnabledStandards` returns the exact objects it just mutated in place
  (`pollCount`, `StandardsStatus`) with no copy, and those same objects can
  be mutated again later by `BatchDisableStandards`.
- `StandardsControl` (standards.go): `DescribeStandardsControls`'s override
  branch (`controls[i] = override`) assigned the live `*StandardsControl`
  stored in `b.controlOverrides` directly; `UpdateStandardsControl` mutates
  an existing override's fields in place.
- `AggregatorV2`/`FindingAggregator`: `Regions []string` is only ever
  wholesale-reassigned (never indexed into), so the existing shallow copies
  on Get/List/Update were already safe -- but `CreateAggregatorV2`/
  `CreateFindingAggregator` returned the live pointer, later mutated by
  their respective `Update*`. Fixed by copying at the Create return only.
- `Member` (members.go): `CreateMembers` appended the live pointer;
  `InviteMembers`/`DisassociateMembers` mutate `MemberStatus`/`InvitedAt` on
  that same object in place. `GetMembers`/`ListMembers` already copied
  correctly.

Confirmed safe, left unchanged, with reason:

- `Hub` (hub.go), `Invitation`/`AdminAccount` (invitations.go), `OrgConfig`
  (organizations.go), `ConfigurationPolicyAssociation`
  (configuration_policies.go), `RecommendedPolicyV2`/`TicketV2`: all-scalar
  structs, or (RecommendedPolicyV2/TicketV2) have no `Update*` that ever
  mutates an existing instance after creation.
- `knownStandards`/`knownSecurityControls`/`knownProducts`: package-level
  read-only lookup tables, never mutated after `init`; every `cp :=
  knownX[i]` copy is safe regardless of field shape.
- `BatchGetSecurityControls`'s `Parameters` field (controls.go): hands out
  `b.controlParams[id]`'s map directly with no copy, but the only writer
  (`UpdateSecurityControl`) always replaces the whole map entry
  (`b.controlParams[id] = parameters`), never indexes into an existing one --
  a previously-handed-out map is never touched again.
- `BatchGetStandardsControlAssociations`'s override branch (standards.go):
  hands out the live `*StandardsControlAssociation` from
  `b.controlAssocOverrides` directly, but the only writer
  (`BatchUpdateStandardsControlAssociations`) always `Put`s a brand-new
  struct rather than mutating an existing one in place.
- `Snapshot` (store.go): marshals every live field (including `b.tags`,
  `b.hubV2`, `b.controlParams`, ...) while still holding `RLock` for the
  entire call -- unlike the handler-side bugs above, the read never escapes
  the lock.

Proof: `go test -race -count=20 ./services/securityhub/...` clean after all
fixes; `TestSecurityHubV2FeatureDescribeRace` is the new permanent
regression test for the flagged bug specifically.

## Error-path sweep (2026-08-29): verified clean, no bugs found

Audited securityhub's failure path -- what a real typed `aws-sdk-go-v2`
client sees when a request fails -- as part of a four-service sweep
(securityhub, kafka, elbv2, stepfunctions) hunting the class of bug where
gopherstack's error-handling call site picks a sentinel/wire code the real
operation's own `deserializeOpError<Op>` switch does not model. All 116
operations' switches extracted from `deserializers.go` (securityhub@v1.75.4)
and diffed against every `typedErrorResponse(...)` call site (125 sites
across all `handler_*.go` files) and the `ErrHubNotEnabled`/`ErrNotFound`/
`ErrAlreadyExists`/etc. sentinels feeding them.

Every literal `errType` string used at a `typedErrorResponse` call site names
a real type in this SDK's `types/errors.go` (AccessDeniedException,
ConflictException, InternalException, InternalServerException,
InvalidAccessException, InvalidInputException, ResourceConflictException,
ResourceNotFoundException, ValidationException) -- no fabricated code exists
anywhere in this service. Every `ResourceNotFoundException`/
`ResourceConflictException`/`InvalidAccessException`/`ValidationException`/
`InvalidInputException` call site was cross-checked against its own
operation's modeled set (not a sibling's) and matches exactly; the classic
REST vocabulary (InvalidInputException/InternalException/
ResourceConflictException) and the newer V2-style vocabulary
(ValidationException/InternalServerException/ConflictException) are never
crossed at a call site, including the several non-"V2"-suffixed operations
(Connectors, ConnectorsV2, AutomationRulesV2, AggregatorsV2) that use the
newer vocabulary -- this distinction was already called out and correctly
handled by a prior pass (see `typedErrorResponse`'s doc comment,
handler.go:507-514), and this pass re-verified it rather than trusting the
comment.

Two call sites (`handleStartConfigurationPolicyDisassociation`,
`handleUpdateStandardsControl`) have an unreachable 500 fallback: their
backend methods never actually return an error (both silently accept any
identifier, including one that was never created, rather than validating
against a known-resource set) even though their operations model
`ResourceNotFoundException`. This is a missing-validation / structural gap,
not a wrong-sentinel-at-a-call-site bug -- fixing it would mean building a
"does this identifier correspond to a real resource" check neither op has
today, not swapping which existing sentinel a call site already picks -- so
it is reported here rather than fixed under this sweep's narrower scope.

No test changes; no source changes. Recorded as genuinely clean for this bug
class, matching several other services in this campaign.

## Error-discard sweep (2026-08-29): verified clean, no bugs found

Distinct class from the error-path sweep above: not which sentinel a call
site picks, but whether a call's own return value carrying failure
information is thrown away (`x, _ := b.Something(...)`). ~195 `, _ :=`/
`, _ =`/bare `_ = ` sites across all non-test `.go` files, triaged
individually.

The large majority are legitimate: JSON-body type assertions
(`body["Field"].(string)`) where a missing/wrong-typed value correctly
becomes the zero value; `x, _ := b.<store>.Get(id)` calls that follow a
`resolve*`/existence check in the same function (the miss case already
returned); and `strconv.Atoi(v)` best-effort query-param parses that fall
back to 0 ("use default").

All 12 `Batch*` operations checked against their backend implementations --
`BatchImportFindings`, `BatchUpdateFindings`, `BatchUpdateFindingsV2`,
`BatchGetSecurityControls`, `BatchGetAutomationRules`,
`BatchDeleteAutomationRules`, `BatchUpdateAutomationRules`,
`BatchEnableStandards`, `BatchDisableStandards`,
`BatchGetStandardsControlAssociations`,
`BatchUpdateStandardsControlAssociations`,
`BatchGetConfigurationPolicyAssociations` -- each correctly threads its
per-item unprocessed/failed list (or an `err` return) into the response.

Two things worth recording, neither a bug:

- `handleBatchEnableStandards`/`handleBatchDisableStandards`
  (handler_standards.go:57,76) discard `BatchEnableStandards`/
  `BatchDisableStandards`'s second return (a `[]map[string]any` of
  failures). Left as-is: `BatchEnableStandardsOutput`/
  `BatchDisableStandardsOutput` (securityhub@v1.75.4
  api_op_BatchEnableStandards.go / api_op_BatchDisableStandards.go) carry
  only `StandardsSubscriptions` -- there is no per-item failure field on the
  real wire shape to put it in. `BatchEnableStandards`'s own failure branch
  (empty `StandardsArn`) is additionally unreachable via a real typed
  client: `StandardsArn` is `// This member is required` on
  `types.StandardsSubscriptionRequest` and enforced by
  `validateStandardsSubscriptionRequest`/`validateOpBatchEnableStandardsInput`
  (validators.go) before the request leaves the client.
- `handleCreateAggregatorV2`'s `_ = h.Backend.TagResource(...)`
  (handler_aggregators_v2.go:46): `TagResource` (tags.go:5) unconditionally
  returns nil, so no real error is being suppressed.
- `handleCreateMembers`'s `_ = created` (handler_members.go:74): correct per
  wire shape -- `CreateMembersOutput` (api_op_CreateMembers.go) has only
  `UnprocessedAccounts`, no created-members field to populate.

No test changes; no source changes. Recorded as genuinely clean for this bug
class.

## 2026-08-30 pagination arithmetic sweep

Audited every paginated listing for the five known gopherstack
pagination-arithmetic bug classes (panic on stale offset, infinite loop on
stale equality-matched cursor, guarded-but-unused index, encoder/decoder
disagreement, unsorted collection). Census: one shared offset-token helper
(`store.go`'s `paginateSlice`, 15 call sites) plus two supporting helpers
(`filterOrAll`, `sortFindings`) feed every List/Describe/Get* op in this
service; no inline `for i, x := range all { if x.ID == token { start = i } }`
site exists outside `store.go`. `paginateSlice` itself was already correct
(clamped offset decode, no equality search — all seven checks pass).

**This service came back with a real, repo-wide Class E problem, not clean.**
11 of the 15 `paginateSlice` call sites fed it a collection read straight
from a `map` or a `store.Table.All()` (explicitly documented as unspecified
iteration order) with no sort in between:

- `filterOrAll`'s "return everything" branch (`arns` empty) called
  `t.All()` — affects `DescribeActionTargets` and `GetEnabledStandards`.
- `sortFindings` was a no-op when `sortCriteria` was empty (`if
  len(criteria) == 0 { return }`) — affects `GetFindings` and
  `GetFindingsV2`, whose backing store (`b.findings`) is itself a
  `map[string]map[string]any`, so the *common* no-sort-criteria call shape
  hit this on every listing.
- 8 more `.All()`-straight-into-`paginateSlice` sites with zero sort:
  `ListAutomationRulesV2`, `ListAggregatorsV2`, `ListInvitations`,
  `ListConnectors` (CSPM), `ListConnectorsV2`, `ListFindingAggregators`,
  `ListConfigurationPolicies`, `ListConfigurationPolicyAssociations`,
  `ListMembers`.
- 2 sites ranging a raw (non-`store.Table`) map with zero sort:
  `ListOrganizationAdminAccounts` (`b.orgAdminAccounts`), `GetResourcesV2`
  (a locally-built `map[string]map[string]any` keyed by resource Id).

All are Class E: a plain two-page walk with no deletion or tampering drops
or duplicates results whenever Go's map iteration reorders between the two
calls (confirmed empirically — reverting one fix and rerunning its
regression test failed 5/5 times).

Fixed 9 of the `store.Table`-backed sites by swapping `.All()` for
`.Snapshot()` (same package, sorted by the table's own key, already the
established idiom in this repo for exactly this purpose). Fixed the 2
raw-map sites with an explicit `sort.Slice` by account ID / resource Id.
Fixed `filterOrAll` the same way (`.Snapshot()`). Fixed `sortFindings` by
removing the empty-criteria early return and adding a final deterministic
tiebreak (`ProductArn|Id`, both ASFF-required fields) that always runs,
whether or not the caller supplied real sort criteria — this also make the
existing sort well-defined on ties within real criteria, which previously
had no tiebreak either.

Safe-by-construction pattern applied throughout: **default a miss/no-sort
case to a genuinely sorted read** (`Table.Snapshot()`, or an explicit
`sort.Slice` for the two raw-map sites) — the same pattern already used
correctly elsewhere in this repo. No threshold-search or found-flag pattern
was applicable here since none of these sites use an equality-matched
cursor (offset tokens throughout).

7 checks run against `paginateSlice` directly (all pass, both before and
after — it was never the bug) plus a stale-cursor probe on `filterOrAll` and
a tied-order probe on `sortFindings`, both of which failed against the
pre-fix code and pass post-fix. 10 end-to-end boundary-walk regression tests
drive the real exported backend methods (23 items, page size 5, non-dividing
count) for a representative sample: `ListAggregatorsV2`,
`ListAutomationRulesV2`, `ListFindingAggregators`,
`ListConfigurationPolicies`, `ListMembers`, `ListOrganizationAdminAccounts`,
`ListConnectorsV2`, `ListConnectors`, `DescribeActionTargets`, `GetFindings`
(no SortCriteria). `ListInvitations`, `ListConfigurationPolicyAssociations`,
and `GetResourcesV2` got the identical, already-proven `.Snapshot()`/explicit-sort
fix but no bespoke end-to-end test — lower priority given the pattern was
independently verified nine other times in this same sweep; flagged here for
anyone auditing this note.

New tests: `services/securityhub/pagination_arithmetic_test.go` (internal,
unexported-helper unit tests), `services/securityhub/pagination_arithmetic_e2e_test.go`
(external, real-API boundary walks).

Gates: `go build ./services/securityhub/...` (clean), `go vet
./services/securityhub/...` (clean, no signature changes), `go test -race
-count=1 ./services/securityhub/...` (pass). Work left uncommitted per this
pass's instructions.

**2026-08-30 (negative-continuation-token sweep)**: `store.go`'s `decodeToken` used a bare
`fmt.Sscanf(token, "%d", &offset)` with no bounds check at all; `paginateSlice`'s `start >=
len(results)` guard does not catch a negative `start`, so `results[start:end]` panicked given
`"-5"` as a NextToken, across all 15 call sites (`action_targets.go`, `aggregators_v2.go`,
`connectors.go`, `finding_aggregators.go`, `configuration_policies.go` x2, `connectors_v2.go`,
`automation_rules.go`, `findings.go`, `findings_v2.go`, `invitations.go`, `resources_v2.go`,
`organizations.go`, `members.go`, `standards.go`). Fixed at the decode site: `decodeToken` now
returns 0 for a negative offset, so all 15 callers inherit the fix. The existing
`TestPaginateSlice_SevenChecks` table in `pagination_arithmetic_test.go` exercised stale/
past-end/malformed-non-numeric tokens but never a negative one.

Proof: the added `negative offset token` subtest of `TestPaginateSlice_SevenChecks`
(`pagination_arithmetic_test.go`) confirmed panicking pre-fix, passes now. Gates: `go build
./services/securityhub/...`, `go vet ./services/securityhub/...`, `go test -race -count=1
./services/securityhub/...`, `golangci-lint run ./services/securityhub/...` (0 issues). Work
left uncommitted per this pass's instructions.

**2026-08-30 (gopherstack-r3pr fabricated-error-code re-audit, no code change)**:
`store.go:31`'s `errCodeInvalidInput` ("InvalidInput") re-checked against
`cmd/errcodeaudit`. All three call sites (`standards.go:95,149`,
`findings.go:458`) set it as a free-form `ErrorCode` map value inside a
`Failures`/`UnprocessedFindings` array on an ordinary 200 response
(`BatchEnableStandards`/`BatchDisableStandards`/`BatchUpdateFindings`), never
as an HTTP error envelope's `__type` — same shape as the already-known
false-positive class (glue/macie2/ce/xray free-form success-response
`ErrorCode` fields), confirmed not a wire-error-envelope bug. Aside, not
fixed here (out of scope for this class): the SDK doc comment on
`BatchUpdateFindingsUnprocessedFinding.Code` (types.go) lists
`FindingNotFound` as the specific documented value for the not-found case
`findings.go:458` covers, which differs from the `InvalidInput` used there —
a real inaccuracy, but a different bug class with no `errors.As` ground
truth, deliberately not chased this pass per campaign scope.

**2026-08-30 (gopherstack-uox6 value-semantics sweep, one bug fixed)**:
Audited every finding filter/matcher in this service against its SDK doc
comment (V1 `matchesFindingFilters`/`matchesStringFilter`/`compareStringFilter`
in `findings.go`; V2's `matchesFindingFiltersV2`/`matchesCompositeFilter*`/
`matchesOcsf*Filter` family in `findings_v2.go`; `filterOrAll` in `store.go`).

**Bug found and fixed**: `matchesStringFilter` (`findings.go`) combined every
entry of a field's `[]StringFilter` list with a strict AND. `types.StringFilter`'s
doc comment (`securityhub@v1.75.4` types.go:19655) documents the opposite for
same-field entries: CONTAINS/EQUALS/PREFIX are joined by OR ("a finding
matches if it matches any one of those filters" — the doc's own worked
example is `Title CONTAINS CloudFront OR Title CONTAINS CloudWatch`),
NOT_CONTAINS/NOT_EQUALS/PREFIX_NOT_EQUALS are joined by AND, and the two
groups then combine by AND ("Security Hub CSPM first processes the PREFIX
filters, and then the NOT_EQUALS ... filters" — the doc's second worked
example, `ResourceType PREFIX AwsIam` + `PREFIX AwsEc2` +
`NOT_EQUALS AwsIamPolicy` + `NOT_EQUALS AwsEc2NetworkInterface`). Under the
old AND-everything code, either worked example returned zero results against
a real matching finding: an under-match, invisible to any shape-based sweep
since the field is read and the comparator values are legal enum members —
only the combination across multiple entries was wrong. Affects `GetFindings`
and, via the shared `matchesFindingFilters`, `BatchUpdateFindings`.
No prior test passed a multi-entry filter on the same field (existing
`TestBackend_MatchesStringFilter`/`TestGetFindings_FiltersApplied` cases all
use exactly one `StringFilter` entry per field), so the bug was invisible to
the existing suite — "a filter test passing a single value cannot see a
multi-value bug."

Fixed by splitting entries into positive/negative groups (`isNegativeStringComparison`)
and combining `!hasPositive || positiveMatched` (OR over positives, defaulting
to "no restriction" when there are none) AND'd with every negative entry
passing. Both of the SDK doc's own worked examples now pass as tests.

Also checked and confirmed correct: `matchesFindingFiltersV2`'s composite
AND/OR (`CompositeOperator`) and `matchesCompositeFilterDepth`'s per-filter
`Operator` (both against `types.OcsfFindingFilters`/`types.CompositeFilter`'s
doc comments, matched field-for-field: `NestedCompositeFilters` three-layer
structure, `AllowedOperators` AND/OR with no NOT combinator since negation is
expressed at the leaf comparator); `matchesOcsfNumberFilter`'s
Eq/Gt/Gte/Lt/Lte against `types.NumberFilter`; `matchesDateRange`'s
WITHIN/OLDER_THAN against `types.DateRange` (default WITHIN); `compareMapFilter`'s
EQUALS/NOT_EQUALS/CONTAINS/NOT_CONTAINS against `types.MapFilter`; `ipInCIDR`'s
bare-address-normalizes-to-/32-or-/128 against `types.IpFilter`'s documented
"CIDR block or IP address" acceptance; `matchesWholeWord`'s word-boundary
regex for `CONTAINS_WORD` (documented V2-only); the lifecycle-rule-style AND
combination across *different* filter fields in both V1 and V2 (correct in
both — the bug was specifically the same-field, multi-entry case).

One gap recorded, not fixed: whether `OcsfMapFilter`'s same-field
CONTAINS/EQUALS-OR / NOT_CONTAINS/NOT_EQUALS-AND rule (documented on the
shared `MapFilter` type) still applies underneath a `CompositeFilter`'s
explicit `Operator`, or is superseded by it, is not stated by either doc
comment — left open rather than guessed (see gaps).

`GetResourcesV2`'s `filters` parameter (`resources_v2.go`) is read nowhere
(`//nolint:revive // existing issue` already marks it) and `GetInsightResults`
never evaluates `insight.Filters` at all (documented in-code: "no real
aggregation in mock") — both are pre-existing, already-flagged completeness
gaps (an unread field, not a wrong algorithm on a read one), not new findings
of this class, so left as-is.

No AWS web pages were fetched this pass — every comparator/operator set
needed was fully specified in the pinned SDK's Go doc comments
(`securityhub@v1.75.4`), unlike the SNS/EventBridge instances of this bug
class from the prior pass.

Tests: added `TestGetFindings_MultiValueSameFieldCombination` (2 subtests,
`findings_test.go`) driving the real filter shape through the HTTP handler
end to end and asserting the exact ID set returned (not just a count), using
the SDK doc's own two worked examples. Both subtests confirmed failing
(0 results each) against the unmodified `matchesStringFilter` before the fix,
passing after. No existing test was weakened; assertion count increased by 2
new subtests, 0 removed.

Gates: `go build ./services/securityhub/...`, `go vet ./services/securityhub/...`,
`go test -race -count=1 ./services/securityhub/...`, `golangci-lint run
./services/securityhub/...`. Work left uncommitted per this pass's
instructions.

## gopherstack-1qf (2026-09-04): three ACCEPTED-BUT-NEVER-DONE gaps fixed

Full-service audit for AWS parity/correctness. Independently re-verified
PARITY.md's own claims per the campaign's "don't treat prior audits as
ground truth" instruction rather than trusting them; found three genuine
"accepted but never done" bugs (pattern class (b)) plus one missing delete
precondition (class (f)), none previously flagged as bugs in this file's
`gaps`/`ops` (two were explicitly noted as *deliberate, acceptable* mock
limitations, which this pass disagrees with -- see each entry).

1. **Automation rules were pure CRUD -- `Criteria`/`Actions` were stored and
   echoed back but zero call sites in the package ever evaluated them
   against a finding** (`automation_rules.go`/`findings.go`). Same shape as
   guardduty's dead filter Action. Strong evidence this was a real gap, not
   a deliberate omission: `findings.go`'s own `findingCustomerManagedFields`
   doc comment already states `BatchImportFindings` cannot set
   `Note`/`UserDefinedFields`/`VerificationState`/`Workflow` "since they're
   managed by Security Hub customers/**automation rules**" -- automation
   rules are the only mechanism that manages those fields, so the comment's
   own premise was false until this fix. Added `applyAutomationRules`
   (`automation_rules.go`), called from `ImportFindings` after
   `preserveCustomerManagedFields`: evaluates every `RuleStatus=="ENABLED"`
   rule in ascending `RuleOrder` (ties broken by `RuleArn`) via
   `matchesFindingFilters` against `rule.Criteria` (the same field-name-mapped
   subset already used for `GetFindings`/`GetInsights` filters -- AWS's
   `AutomationRulesFindingFilters`, securityhub@v1.75.4 types.go:575, has
   additional `NumberFilter`/`DateFilter`/`MapFilter` members this file has no
   evaluator for; left unevaluated per the no-fabrication rule, same as the
   existing V2 filter gaps), and applies each match's `FINDING_FIELDS_UPDATE`
   action (the only real `AutomationRulesActionType`, enums.go:119) via
   `maps.Copy` -- the same mechanism `BatchUpdateFindings` already uses,
   since `AutomationRulesFindingFieldsUpdate` (types.go:524) has the identical
   field set. Stops at the first `IsTerminal` match. Proof:
   `TestBatchImportFindings_AutomationRuleFires` (2 subtests,
   `automation_rules_test.go`) -- confirmed failing (finding's Severity
   unchanged) against `git show HEAD`, passing after, restored.

2. **`DisableSecurityHub` never checked AWS's documented precondition.**
   `api_op_DisableSecurityHub.go`'s doc comment: "You can't disable Security
   Hub CSPM in an account that is currently the Security Hub CSPM
   administrator." `DisableHub` (`hub.go`) only checked `hubEnabled`. Fixed:
   refuses (new `ErrHubIsAdministrator` sentinel, mapped to
   `InvalidAccessException`/400 -- one of `DisableSecurityHub`'s five modeled
   error types, deserializers.go:7544, and the same code this file already
   uses for analogous `hubEnabled`-gap fixes on `UpdateActionTarget`/
   `DeleteActionTarget`/`DisableImportFindingsForProduct`) while any member
   (`CreateMembers` is this backend's only path to the administrator
   relationship -- Organizations delegated admin never creates `Member`
   records, see `organizations.go`) has `MemberStatus != "Removed"`. Proof:
   `TestDisableSecurityHub_RefusedWhileAdministrator` (2 subtests,
   `hub_test.go`) -- confirmed failing (200 instead of 400) against
   `git show HEAD`, passing after, restored, md5sum-confirmed byte-identical.

3. **`GetInsightResults` always returned empty `ResultValues`.** PARITY.md
   previously called this "acceptable mock behavior, not a stub since
   Insight itself is real" -- this pass disagrees: `Insight.GroupByAttribute`/
   `Filters` are real, stored, client-supplied values with a well-specified
   real aggregation (`InsightResults`/`InsightResultValue`, types.go:15875-15912,
   is just `{GroupByAttributeValue, Count}` per distinct value), and the
   infrastructure to compute it (`matchesFindingFilters`, `findingFieldString`)
   already existed in this same package for `GetFindings`/`GetInsights` --
   returning it unconditionally empty is functionally indistinguishable from
   a stub to any real client. Fixed via new `aggregateInsightResults`
   (`insights.go`): filters `b.findings` by `insight.Filters`
   (`matchesFindingFilters`, deliberately the same mapped-subset limitation
   as every other reuse of that function in this file -- not a new gap),
   groups by `findingFieldString(f, insight.GroupByAttribute)` (already
   resolves the `SeverityLabel`/`WorkflowStatus`/`ComplianceStatus`
   nested-field cases), counts, and returns values sorted for determinism.
   Proof: `TestBackend_GetInsightResults_AggregatesFindings`
   (`insights_test.go`) -- confirmed failing (all counts 0) against
   `git show HEAD`, passing after, restored, md5sum-confirmed byte-identical.

**Checked and confirmed correct (no bug), contra this pass's own
speculation before reading the code:**

- `BatchUpdateFindings`/`handleBatchUpdateFindings`'s "copy every body key
  except FindingIdentifiers into updates" (`handler_findings.go`) looked
  like an unbounded-field-write risk at first glance, but
  `BatchUpdateFindingsInput` (api_op_BatchUpdateFindings.go) only ever
  defines nine possible fields besides `FindingIdentifiers`
  (`Confidence`/`Criticality`/`Note`/`RelatedFindings`/`Severity`/`Types`/
  `UserDefinedFields`/`VerificationState`/`Workflow`) -- a real typed client
  is structurally incapable of sending anything else, so this is CLEAN for
  the client-observability bar this campaign uses, not a bug.
- `DeleteMembers`'s missing check for AWS's documented "can't delete
  Organizations-org members" restriction is moot in this backend:
  `organizations.go` never creates `Member` records (delegated-admin
  enable/disable is separate bookkeeping with no member-creation side
  effect), so no org-managed member can ever exist here to violate the
  restriction. Not a bug; architecturally inapplicable.
- `DeleteInsight`/`DeleteActionTarget`/`DeleteFindingAggregator`/
  `BatchDeleteAutomationRules`/`DisassociateMembers` all correctly validate
  existence (or, for `DisassociateMembers`, correctly have nothing to
  validate against -- `DisassociateMembersOutput` has zero members besides
  `ResultMetadata`, so silently skipping an unknown id matches the real
  wire shape exactly).
- `CreateFindingAggregator`/cross-Region aggregation and
  `BatchEnableStandards`/`BatchUpdateStandardsControlAssociations` control
  status are bookkeeping, not derived from real cross-region replication or
  finding-vs-control evaluation -- structural (single-backend-instance mock
  has no second region to replicate into, and no config-rule-evaluation
  engine), same category as `DescribeStandards`'/`DescribeProducts`' static
  catalogs, not a new finding. This does NOT extend to finding-level
  `Compliance.Status`: that field is never fabricated by this backend (see
  gopherstack-cf4j triage below) -- it's not part of this bullet's claim,
  only the control/standard bookkeeping is. Prior wording here read
  "...compliance status are bookkeeping" as one run-on list, which is
  ambiguous about which "compliance" it means; corrected.

**Structural/absent, checked rather than assumed, not fixed this pass:**

- **No cross-service integration.** Grepped `services/guardduty`,
  `services/inspector*`, `services/macie*` for any call into
  `services/securityhub` -- none exists. GuardDuty/Inspector/Macie appear in
  this service only as static `DescribeProducts` catalog entries
  (`products.go`); a real finding provider (or gopherstack's own emulated
  GuardDuty/Inspector/Macie) must call `BatchImportFindings` itself for
  findings to appear here. No EventBridge event publication on finding
  create/update either (only one unrelated code comment mentions "change
  events"; zero `events.`/`Publish` call sites in non-test files).
  Organizations delegated admin (`organizations.go`) is standalone
  bookkeeping with no cross-check against gopherstack's own Organizations
  service state. All matches this file's existing "findings only ever enter
  via BatchImportFindings" structural note -- reported here as the
  cross-service-integration angle this audit brief specifically asked to
  verify rather than assume.
- Findings generation is import-only (`BatchImportFindings`/`ImportFindings`
  is the only path that creates a finding) -- structural, already documented
  elsewhere in this file, re-confirmed.

**Performance**: read GetFindings/GetFindingsV2's filter+sort path and
`paginateSlice` (already the subject of a dedicated 2026-08-30 sweep in this
file). No quadratic loops found; every filter/aggregate op here (including
the two new ones this pass added) is a single O(n) pass under the backend's
one coarse lock, consistent with every other listing op in this service.
Not independently re-benchmarked.

**LocalStack parity**: NOT CHECKED -- no LocalStack instance available this
pass.

**Resource leaks**: re-confirmed the existing `findingHistory` ghost-row
finding from a prior pass is a deliberate append-only audit log, not a leak
(per this task's brief, not re-litigated). No new maps were added by this
pass's fixes (automation-rule application mutates the finding map already
being written; insight aggregation reads `b.findings` transiently, storing
nothing new).

Gates: `GOTOOLCHAIN=go1.26.6 go build ./services/securityhub/...`,
`go vet ./services/securityhub/...`, `go test -race -count=1
./services/securityhub/...` (all pass), `golangci-lint run
./services/securityhub/...` (0 issues), `gofmt -l services/securityhub/`
(clean).

## gopherstack-cf4j (2026-09-07): triage -- standards/control associations ARE bookkeeping (correctly); compliance status is NOT synthesized

Filed title-only, empty description: "securityhub: standards and control
associations are bookkeeping; compliance status is synthesized." Re-derived
both claims from the code since none of the specifics existed in the issue.
Verdict: **structural for claim 1, factually wrong for claim 2** -- no code
change. This section is the missing triage note plus the correction.

### Claim 1: standards/control associations are bookkeeping -- TRUE, and correct

`BatchEnableStandards`/`BatchDisableStandards` (standards.go) create/delete a
`StandardsSubscription` record. `DescribeStandardsControls` returns a static
`defaultControls()` list overridable per-arn via `UpdateStandardsControl`
(`b.controlOverrides`). `BatchGetStandardsControlAssociations`/
`BatchUpdateStandardsControlAssociations`/`ListStandardsControlAssociations`
read/write `b.controlAssocOverrides` the same way. All four are pure
CRUD-on-a-map: stored on write, echoed on read, consulted by nothing else.

Confirmed by grep, not assumed: `ImportFindings` (the only function that
creates a finding -- interfaces.go:14, called from exactly one call site,
handler_findings.go:71 `handleBatchImportFindings`) is never called from
standards.go, controls.go, handler_standards.go, or handler_controls.go.
Enabling a standard, disabling a control, or updating a control association
never produces, withdraws, or touches a single finding.

That is the honest behavior, not a gap, because the real AWS semantics this
mirrors require a check-evaluation engine gopherstack doesn't have.
`StandardsControl.ControlStatus`'s doc comment (types.go:19299-19301,
securityhub@v1.75.4) says outright:

> The current status of the security standard control. Indicates whether the
> control is enabled or disabled. Security Hub CSPM does not check against
> disabled controls.

"Does not check against disabled controls" presupposes Security Hub CSPM
*does* check against enabled ones -- a continuous compliance engine that
inspects real resource state per control and emits/withdraws findings as
`ControlStatus`/`AssociationStatus` change. Gopherstack's `securityhub`
package has no such engine (per the 2026-08-29 error-path sweep, confirmed
again this pass: zero cross-service call sites from guardduty/inspector/
macie into securityhub, and the only finding-creation path is client-driven
`BatchImportFindings`). Given that, `ControlStatus`/`AssociationStatus` have
exactly one honest implementation available: store what the caller set and
echo it back. Building a real per-control resource evaluator is out of
reach without picking a source of truth for "what does S3.1 check" across
every emulated service and re-running it on every toggle -- a project-wide
feature, not a securityhub fix.

**What would have to exist first**: a resource-evaluation engine that maps
each `SecurityControlId` (controls.go's `knownSecurityControls`, e.g.
`S3.1` "S3 Block Public Access setting should be enabled") to a real check
against the corresponding emulated service's stored state (e.g. query
`services/s3`'s bucket public-access-block config), runs it when a control's
`AssociationStatus`/`ControlStatus` is `ENABLED`, and creates/updates
`Compliance.Status` findings via the existing `ImportFindings` path when the
check result changes. That's new cross-service infrastructure, not a
securityhub-local fix. Not building it; echo-only bookkeeping is correct
until it exists.

### Claim 2: compliance status is synthesized -- FALSE, checked against the code

`types.ComplianceStatus` (enums.go:237,241-244) has four values: `PASSED`,
`WARNING`, `FAILED`, `NOT_AVAILABLE`.

Grepped every non-test `.go` file in this package for all four literals and
for any `math/rand` import: zero hits. Nothing in this backend ever writes
a `ComplianceStatus` value. The only two places `Compliance`/`Compliance.Status`
appear in non-test code are reads: `findings.go:347` (`nestedFindingString`,
used by `GetFindings`/`GetFindingsV2`'s `ComplianceStatus` filter) and
`findings_v2.go:570` (`GetFindingsV2` composite-filter evaluation). Neither
writes a value; both return `""` when the field is absent on the stored
finding (`nestedFindingString`, findings.go:357-360) -- absence stays
absent, it is never defaulted to an enum member.

The only place a finding (and therefore any `Compliance` object) is created
is `ImportFindings` (findings.go:104-148), which copies the caller's ASFF
map verbatim (`maps.Copy(stored, f)`, findings.go:133) into storage. The one
list of fields explicitly protected from being overwritten by a client's
re-import -- `findingCustomerManagedFields` (findings.go:21-23): `Note`,
`UserDefinedFields`, `VerificationState`, `Workflow` -- does **not** include
`Compliance`, which is correct: AWS's own docs place `Compliance` with the
finding-provider-owned fields a re-import is expected to refresh, not the
customer-managed set. So on every `BatchImportFindings` call, whatever
`Compliance.Status` the caller supplies is exactly what gets stored,
overwriting the prior value -- matching real AWS, where the finding
*provider* (a real CSPM check, GuardDuty, a third-party integration) is the
only party that ever sets `Compliance.Status`; Security Hub itself doesn't
invent one.

`BatchUpdateFindings` (findings.go:480-527) *could* in principle be a second
write path -- `handleBatchUpdateFindings` (handler_findings.go:80-98)
collects `updates` from every raw JSON body key except
`FindingIdentifiers` (handler_findings.go:91-98), with no field allowlist,
and `maps.Copy(f, updates)` applies it verbatim. This was already investigated by the 2026-08-29
error-discard sweep (PARITY.md, "Checked and confirmed correct... contra
this pass's own speculation") and found clean: `BatchUpdateFindingsInput`
(api_op_BatchUpdateFindings.go, securityhub@v1.75.4) defines exactly nine
fields besides `FindingIdentifiers` -- `Confidence`, `Criticality`, `Note`,
`RelatedFindings`, `Severity`, `Types`, `UserDefinedFields`,
`VerificationState`, `Workflow` -- and has no `Compliance` member at all. A
real typed `aws-sdk-go-v2` client is structurally incapable of sending
`Compliance` through `BatchUpdateFindings`; only a hand-crafted raw HTTP
request could exploit the missing allowlist, and even then it would be
replaying attacker-supplied input, not the backend inventing a status.
Re-confirmed this pass, not just cited: still true, not treating it as new
scope for gopherstack-cf4j.

**Conclusion**: `Compliance.Status` falls in the "copied from client input
on `BatchImportFindings`" category the audit brief calls out as legitimate,
not the "invented" category. Nothing here resembles the accessanalyzer/
personalize undisclosed-confident-answer bug class (gopherstack-xyu4/h3th)
-- there is no code path that fabricates a value the caller never supplied.
The issue title's second half does not hold up against the code as written.

### Files changed

- `services/securityhub/PARITY.md`: added `note:` fields to the
  `DescribeStandardsControls`/`UpdateStandardsControl`/
  `ListStandardsControlAssociations`/`BatchGetStandardsControlAssociations`/
  `BatchUpdateStandardsControlAssociations` table rows pointing here;
  tightened the pre-existing but ambiguous "...compliance status are
  bookkeeping" sentence (2026-08-29 sweep section) that conflated
  control-association bookkeeping with finding-level `Compliance.Status` in
  one run-on clause, and added a forward pointer to this section. No `.go`
  files touched -- both claims resolve to "no code defect," not "no code
  reviewed."

### Suggested bd disposition

Close gopherstack-cf4j as **not a bug / documentation-only**, or re-file if
the maintainer wants the "what would have to exist first" cross-service
check-evaluation engine tracked separately (it would be a new, large,
multi-service feature, not a securityhub-local fix). Suggested bd close
text below.
