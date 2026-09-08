---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: inspector2
sdk_module: aws-sdk-go-v2/service/inspector2@v1.54.1   # version audited against
last_audit_commit: 9e3baacb5                            # HEAD when this manifest was written
last_audit_date: 2026-07-29
overall: A            # gopherstack-zj76 remainder pass: CIS/code-security name length+charset constraints now enforced (fetched live from AWS API Reference -- the Go SDK module has no length/pattern doc prose for these 4 fields), CoverageFilterCriteria's scanStatusCode/scanStatusReason/scanMode/lastScannedAt facets fixed from accepted-but-silently-ignored to genuinely narrowing (real bug, not just an omission), FindingDetail.Ttps added; authorizationUrl gap and the 7 remaining Cvss/Epss/Evidence-class nested struct types re-confirmed as genuine, deliberately-scoped-out gaps (not oversights) -- no prior family regressed
# 2026-08-21 gopherstack-r80d batch 12 (required-output cut): last_audit_commit
# left unchanged -- this pass's own commit sha is not known at edit time (the
# orchestrator, not this pass, creates the commit), and the campaign's own
# convention is to leave the existing value rather than write a guess or
# "pending" (see gopherstack-z31a, which already tracks this manifest field's
# unmerged-branch-sha problem repo-wide). 4 bugs found and fixed, all proven
# via real aws-sdk-go-v2/service/inspector2 client round trips
# (wire_output_required_r80d_test.go), hand-reverted/confirmed-failing/
# restored, md5sum-verified byte-identical: (1) GetCodeSecurityIntegration and
# ListCodeSecurityIntegrations (shared codeSecurityIntegrationToWire helper)
# dropped required type/statusReason entirely -- type was already tracked on
# the domain struct and simply never surfaced; statusReason has no backing
# data source (no OAuth/health-check flow exists) so it is now emitted
# honestly empty rather than fabricated (api_op_GetCodeSecurityIntegration.go,
# types.go's CodeSecurityIntegrationSummary). (2) Finding.Remediation had no
# struct field at all (types.go: required, Recommendation sub-member
# optional) -- now emitted as an honest empty object. (3) Finding.Resources
# (required) was only emitted when non-empty, dropping the key entirely for
# any finding seeded with zero resources -- now always emitted, non-nil.
# (4) Finding.Severity was serialized as a fabricated {label,score} nested
# object; the real wire shape (deserializers.go's
# awsRestjson1_deserializeDocumentFinding, case "severity") is a bare Severity
# string enum -- this was not a missing-field bug but a wrong-shape one, and
# the worst found this campaign: any real SDK client's ListFindings call
# failed outright ("expected Severity to be of type string, got
# map[string]interface {} instead") once any finding existed, not merely a
# missing value. The numeric score now rides the separate, optional,
# top-level inspectorScore member (Finding.InspectorScore *float64) instead.
# This also means the prior audit's "ListFindings: {wire: ok}" verdict (see
# ops: below) was never actually exercised against a real SDK client --
# gopherstack-r80d's "a recorded verdict is not evidence" lesson applies here
# too: every prior _test.go in this package asserted on raw JSON, and
# sdk_response_keys_test.go's own doc comment already explained why that
# can't catch a wrong-shape bug. Existing raw-JSON tests asserting the old
# {label,score} shape were updated to match the real wire shape
# (handler_findings_core_test.go, handler_findings_query_test.go).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  Enable: {wire: ok, errors: ok, state: ok, persist: ok}
  Disable: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetAccountStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateFilter: {wire: ok, errors: ok, state: fixed, persist: ok, note: "fixed this pass (gopherstack-or9) — name now validated against AWS's real 3-64 char, alnum/dot/underscore/dash constraint (previously accepted any non-empty string). ALSO FIXED (gopherstack-or9): Action=SUPPRESS was CRUD-only -- stored/echoed but never applied to any finding, the same 'accepted but never done' bug class this week's audit already found in guardduty (Filter.Action ARCHIVE never archived) and securityhub (automation rules never evaluated). Real CreateFilter's own SDK doc comment (api_op_CreateFilter.go): 'Creates a filter resource using specified filter criteria. When the filter action is set to SUPPRESS this action creates a suppression rule.' Now a SUPPRESS filter transitions every currently-ACTIVE matching finding to SUPPRESSED at creation (suppressMatchingFindings, filters.go), and every subsequently-seeded finding matching an already-active SUPPRESS filter arrives pre-suppressed (matchesSuppressFilter, called from SeedFinding/AddFinding) -- realizing 'creates a suppression rule' as an ongoing rule, not a one-off action. Deliberately NOT modeled: reverting a finding to ACTIVE when its suppressing filter is later deleted or changed away from SUPPRESS -- neither the SDK doc comments nor the API Reference say whether real Inspector2 does this, so it is left as a disclosed gap rather than a guessed behavior."}
  UpdateFilter: {wire: ok, errors: ok, state: fixed, persist: ok, note: "fixed this pass (gopherstack-or9) — action changing to SUPPRESS (or criteria narrowing under an already-SUPPRESS filter) now re-applies suppressMatchingFindings, same fix as CreateFilter above."}
  DeleteFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFilters: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFindings: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2026-08-21 (gopherstack-r80d batch 12) — severity was a fabricated {label,score} nested object; real wire shape is a bare Severity string enum (deserializers.go's awsRestjson1_deserializeDocumentFinding), which made every real SDK client's call fail once a finding existed, not merely drop a field. Also fixed: required Remediation (no struct field) and Resources (dropped when empty) were both omitted. gopherstack-4ly2 wrapper-key sweep (2026-08-29): SortCriteria was parsed nowhere (decodeFilterListRequest had no such member) -- every response came back in FindingArn order regardless of the client's request. Now honored for the 8 SortField values this backend's Finding model actually carries data for (AWS_ACCOUNT_ID/FINDING_TYPE/SEVERITY/FIRST_OBSERVED_AT/LAST_OBSERVED_AT/FINDING_STATUS/RESOURCE_TYPE/EPSS_SCORE); the remaining 9 (ECR_IMAGE_*/NETWORK_PROTOCOL/COMPONENT_TYPE/VULNERABILITY_ID/VULNERABILITY_SOURCE/INSPECTOR_SCORE/VENDOR_SEVERITY) fall back to the prior stable FindingArn order -- structural gap, this backend's Finding has no per-package/per-resource detail to sort by, disclosed not fabricated. Also extended findingFilterCriteria (previously only severity/findingType/findingStatus/awsAccountId) with resourceId/resourceType/title/findingArn/fixAvailable, which map directly onto existing Finding fields and were simply never wired in."}
  GetConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (cmd/enumcheck sweep, 1d6e40d1a): Ec2ScanModeState.ScanModeStatus was the non-member string \"ENABLED\" -- types.Ec2ScanModeStatus only has SUCCESS/PENDING (types/enums.go:1191-1207). UpdateConfiguration applies scan-mode changes synchronously with no pending state modeled, so the setting is always already in effect -- now emits SUCCESS (scanModeStatusSuccess, store.go). See TestGetConfiguration_ScanModeStatus_RealSDKClient (wire_field_fixes_test.go). ALSO FIXED (78d9fdf9f, gopherstack-k3w5): ecrConfiguration.rescanDurationState's status had the same non-member \"ENABLED\" bug for types.EcrRescanDurationStatus (SUCCESS/PENDING/FAILED, types/enums.go:1289-1303) -- enumcheck's ambiguous-key filter silently dropped this one since \"status\" resolves to 13 enum types in this module; the filter now reports ambiguous keys as needs-review instead of discarding them. Now emits SUCCESS (ecrRescanDurationStatusSuccess, store.go). See TestGetConfiguration_EcrRescanDurationStatus_RealSDKClient (wire_field_fixes_test.go)."}
  UpdateConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  members: {status: ok, note: "unchanged this pass"}
  delegated_admin: {status: ok, note: "unchanged this pass"}
  organization_configuration: {status: ok, note: "unchanged this pass"}
  ec2_deep_inspection: {status: ok, note: "unchanged this pass"}
  encryption_key: {status: ok, note: "unchanged this pass"}
  cis_scan_configuration: {status: ok, note: "fixed this pass — CreateCisScanConfiguration/UpdateCisScanConfiguration's scanName now enforces the real, documented length constraint (AWS API Reference: \"Minimum length of 1. Maximum length of 128.\"; no charset pattern is documented for this field, confirmed via both the API Reference page and the Go SDK module's doc comments carrying no length/pattern prose at all for scanName, unlike CreateFilterInput.name). Real AWS returns ValidationException for a violation; this backend previously accepted any non-empty string on create and any string at all (including one exceeding 128 chars) on update."}
  cis_session: {status: ok, note: "unchanged this pass"}
  cis_scan_results: {status: ok, note: "unchanged this pass"}
  code_security_integration: {status: partial, note: "fixed this pass — CreateCodeSecurityIntegration's name now enforces the real, documented length+charset constraint (AWS API Reference: \"Minimum length of 1. Maximum length of 60.\" Pattern: `[a-zA-Z0-9-_$:.]*`; the Go SDK module's doc comments carry no such prose for this field at all, unlike CreateFilterInput.name, so the API Reference was fetched directly and quoted verbatim). Get/ListCodeSecurityIntegrations re-confirmed this pass NOT to expose 'details' on the wire in real AWS either (GetCodeSecurityIntegrationOutput has no details member), so the prior gap note was overstated: the real remaining gap is only CreateCodeSecurityIntegrationOutput's optional 'authorizationUrl' member (OAuth callback URL for GitHub/GitLab-type integrations), which this backend never returns and correctly omits (status IS returned — confirmed via handleCreateCodeSecurityIntegration). Left open: gopherstack has no OAuth flow to derive a real authorization URL from, and fabricating one would be worse than omitting it — re-verified this pass against the live AWS API Reference (API_CreateCodeSecurityIntegration.html), which confirms authorizationUrl is genuinely returned by real AWS post-OAuth-flow-initiation with no derivable-from-request-input source gopherstack could reproduce. FIXED 2026-08-21 (gopherstack-r80d batch 12) — GetCodeSecurityIntegrationOutput and CodeSecurityIntegrationSummary (ListCodeSecurityIntegrations' item shape) both require type/statusReason (api_op_GetCodeSecurityIntegration.go, types.go); the flat per-op scan undercounted this because CodeSecurityIntegrationSummary's 7 required members are only reachable by reading the nested domain struct, not ListCodeSecurityIntegrationsOutput's own (non-required) Integrations field -- same nested-domain-struct undercount class this campaign already named for pinpoint/bedrockagent/cleanrooms. type was already tracked on the domain struct and simply never surfaced by the shared codeSecurityIntegrationToWire helper; statusReason has no backing data source at all (no OAuth/connection-health flow exists to derive a real reason from) so it is emitted as an honestly empty string rather than fabricated."}
  code_security_scan_configuration: {status: ok, note: "fixed this pass — Create/Get/UpdateCodeSecurityScanConfiguration now nest ruleSetCategories/periodicScanConfiguration/continuousIntegrationScanConfiguration under a required top-level 'configuration' object with level/scopeSettings/name/tags as siblings (confirmed via serializers.go's awsRestjson1_serializeDocumentCodeSecurityScanConfiguration and deserializers.go's Get output deserializer), replacing the prior flat request/response decoding that silently dropped every real client's configuration.* fields. level (required, ACCOUNT|ORGANIZATION) and configuration.ruleSetCategories (required, non-empty, SAST|IAC|SCA) are now validated with ValidationException, where the prior handler defaulted both silently. Deleted the fabricated 'status'/'integrationArn' members: neither exists on the real CodeSecurityScanConfiguration/GetCodeSecurityScanConfigurationOutput shape (field-diffed against api_op_GetCodeSecurityScanConfiguration.go — no such members). UpdateCodeSecurityScanConfiguration now only accepts the nested 'configuration' object (matching UpdateCodeSecurityScanConfigurationInput's real shape — no top-level scopeSettings on update, that member is create-time-only) and re-validates ruleSetCategories on every update, where the prior handler silently accepted a top-level scopeSettings update the real API has no field for. ListCodeSecurityScanConfigurations now emits the real, structurally distinct CodeSecurityScanConfigurationSummary shape (flat ownerAccountId/ruleSetCategories/continuousIntegrationScanSupportedEvents/frequencyExpression/periodicScanFrequency/scopeSettings, no nested 'configuration', no tags/createdAt/level) under the real 'configurations' wire key, confirmed via deserializers.go — the prior handler reused Get's full shape under the wrong 'scanConfigurations' key (that key belongs to the unrelated CIS/connector scan-configuration list endpoints), so a real ListCodeSecurityScanConfigurations client's Configurations field was never populated at all. This pass additionally fixed name's real length+charset constraint (AWS API Reference: \"Minimum length of 1. Maximum length of 60.\" Pattern: `[a-zA-Z0-9-_$:.]*`, identical to CreateCodeSecurityIntegration's name constraint) — previously any non-empty string was accepted."}
  code_security_scan_associations: {status: ok, note: "unchanged this pass"}
  code_security_scan: {status: fixed, note: "gopherstack-muzq (2026-08-21): StartCodeSecurityScan stamped the stored scan's status IN_PROGRESS and nothing else in this backend ever wrote to it again -- GetCodeSecurityScan (code_security.go) just echoed the stored map verbatim, so a client polling for readiness never saw a terminal status. Confirmed no async mechanism anywhere in the package (no ticker/goroutine/janitor/work.After/runDelayed/reconciler; grepped all non-test .go files in services/inspector2) -- inspector2 ships no generated Get*Waiter for this op either, which only means a real caller must hand-roll its own poll loop, not that an unadvancing status is correct. This backend's other synchronous scan resource, CisScan (cis_scans.go), instead stamps its terminal COMPLETED status once at creation since its checks are computed synchronously with no separate poll step -- that pattern doesn't fit here because StartCodeSecurityScanOutput.Status is documented/wire-confirmed to legitimately be IN_PROGRESS on Start. Fixed by having GetCodeSecurityScan advance IN_PROGRESS->SUCCESSFUL on first poll instead, mirroring the reap-on-read pattern services/omics uses for its own Get*-advances-Creating resources. TestStartCodeSecurityScan_Status (sdk_response_keys_test.go) previously asserted only the initial IN_PROGRESS status and stopped; strengthened with a GetCodeSecurityScan follow-up asserting SUCCESSFUL. Hand-reverted code_security.go to git show HEAD, confirmed the strengthened assertion fails with Status stuck at IN_PROGRESS, restored, md5sum byte-identical."}
  findings_report: {status: ok, note: "fixed this pass — CreateFindingsReport now accepts and stores filterCriteria/reportFormat (previously discarded/unparsed); GetFindingsReportStatus now echoes destination/filterCriteria/errorMessage (real GetFindingsReportStatusOutput wire keys: destination/errorCode/errorMessage/filterCriteria/reportId/status — confirmed via deserializers.go there is NO createdAt member on the real output at all, correcting the prior audit's gap note)"}
  sbom_export: {status: ok, note: "fixed this pass — CreateSbomExport's request field was the gopherstack-invented 'sbomFormat' key; real CreateSbomExportInput field is 'reportFormat' (confirmed via serializers.go), so every real client's report format was silently dropped. Now reads reportFormat + resourceFilterCriteria, and GetSbomExport echoes format/s3Destination/filterCriteria/errorMessage (real GetSbomExportOutput wire keys, confirmed via deserializers.go; also no createdAt member in the real output)"}
  coverage: {status: ok, note: "fixed this pass (both new-op and accepted-but-ignored-filter fixes) — ListCoverage/ListCoverageStatistics were hardwired empty stubs in an earlier pass; added SeedCoverage (store.Table[CoverageEntry], real CoveredResource wire shape incl. epoch-encoded lastScannedAt) following the SeedFinding precedent. ListCoverage/ListCoverageStatistics now genuinely evaluate 8 real CoverageFilterCriteria facets against real stored CoverageEntry data — accountId/resourceId/resourceType/scanType (already wired) plus scanStatusCode/scanStatusReason/scanMode/lastScannedAt (fixed this pass: field-diffed against types.go, these were accepted in the request shape but silently never applied to narrow results — a real bug, since CoverageEntry.ScanStatus.{StatusCode,Reason}/ScanMode/LastScannedAt already existed as real, seeded data with nothing wiring the filter to it; lastScannedAt is a CoverageDateFilter, encoded as epoch-seconds numbers on the wire per serializers.go's awsRestjson1_serializeDocumentCoverageDateFilter, not RFC3339). ListCoverageStatistics's groupBy supports ACCOUNT_ID/RESOURCE_TYPE/SCAN_STATUS_CODE; ECR_REPOSITORY_NAME not modeled, would require the nested ResourceMetadata union. Not modeled (re-confirmed this pass, genuinely no backing data — not an ignored-filter bug like the four fixed above): CoverageFilterCriteria's remaining ~20 facets tied to CoveredResource.resourceMetadata (a nested per-resource-type union this backend never populates at all — ec2InstanceTags/ecrImageTags/lambdaFunctionTags/cloudContainerImageTags/ecrImageInUseCount/ecrImageLastInUseAt/imagePulledAt/cloud*/code*, confirmed via types.CoverageFilterCriteria's full field list)."}
  finding_aggregations: {status: fixed, note: "FIXED this pass (gopherstack-or9, extended gopherstack-f9vi) — ListFindingAggregations always emitted an 'accountAggregation'-keyed response entry regardless of the requested aggregationType. types.AggregationResponse (inspector2@v1.54.1 types/types.go) is a real Smithy union with 15 members (accountAggregation/amiAggregation/packageAggregation/findingTypeAggregation/titleAggregation/repositoryAggregation/...), and the real deserializer (deserializers.go's awsRestjson1_deserializeDocumentAggregationResponse) picks which member to populate purely from which JSON key is present -- it never consults the request's aggregationType. So a real client requesting e.g. AggregationType=PACKAGE always got back an AccountAggregation value instead (wrong union member; its own requested fields, e.g. PackageAggregation.PackageName, never populated). gopherstack-or9 stopped fabricating that entry for every non-ACCOUNT type. gopherstack-f9vi re-examined the 'no per-package/per-resource detail' premise and found it half wrong: Finding.Resources []FindingResource{Type, ID} already existed, just unused by this function. ACCOUNT, TITLE, REPOSITORY, AWS_EC2_INSTANCE, AWS_ECR_CONTAINER, AWS_LAMBDA_FUNCTION and CODE_REPOSITORY are now genuinely computed (grouped by Finding.Title or by the ID of a matching Resources[] entry, with real per-group severityCounts) — see TestListFindingAggregations_ResourceKeyedGrouping (findings_aggregation_test.go). PACKAGE, AMI, IMAGE_LAYER, LAMBDA_LAYER, FINDING_TYPE, CONTAINER_IMAGE, SERVERLESS_FUNCTION and VM_INSTANCE remain honestly empty: PACKAGE has no backing vulnerability/package sub-struct anywhere on FindingResource; AMI has no AMI-ID field anywhere in this model; IMAGE_LAYER/LAMBDA_LAYER need multiple required fields (LayerHash+Repository+ResourceId, and FunctionName+LayerArn+ResourceId respectively) this model has none of; FINDING_TYPE's real response shape (FindingTypeAggregationResponse) carries no group key at all — it is a per-account rollup identical in shape to ACCOUNT's own response, so implementing it would just duplicate ACCOUNT under a different envelope key rather than add real grouping; CONTAINER_IMAGE/SERVERLESS_FUNCTION/VM_INSTANCE are the multi-cloud generalizations of AWS_ECR_CONTAINER/AWS_LAMBDA_FUNCTION/AWS_EC2_INSTANCE and were deliberately NOT conflated with those narrower resource types without stronger evidence of real AWS's actual grouping semantics (declining to guess). See TestListFindingAggregations_UnimplementedTypesIgnoreResourceData (findings_aggregation_test.go), which pins that these stay empty even when the same findings carry title/resource data other aggregation types now group by."}
  usage_totals: {status: ok, note: "fixed this pass — ListUsageTotals now derives real per-account Usage entries (real UsageTotal/Usage wire shape: currency/estimatedMonthlyCost/total/type) from which resource types are Enable'd and how many SeedCoverage entries of each scan type exist, replacing the prior hardwired-empty-usage stub. estimatedMonthlyCost is a documented deterministic placeholder rate (gopherstack has no metering engine and real Inspector pricing is not reproducible in a mock) — the wire shape and field names are what parity requires here, not the dollar amount"}
  account_permissions: {status: ok, note: "fixed this pass — deleted the gopherstack-invented 'status' field from AccountPermission (real Permission shape is operation/service, confirmed via deserializers.go; there is no 'status' member on the real type at all). ListAccountPermissions now returns the real Operation x Service permission matrix (ENABLE_SCANNING/DISABLE_SCANNING/ENABLE_REPOSITORY/DISABLE_REPOSITORY x EC2/ECR/LAMBDA), narrowed by the optional service filter, replacing the prior hardwired-empty stub"}
  vulnerability_search: {status: ok, note: "fixed in an earlier pass — deleted the gopherstack-invented 'vulnerabilityId'/'severity' Vulnerability fields (real wire keys are 'id'/'vendorSeverity', confirmed via deserializers.go). Added SeedVulnerability (store.Table[Vulnerability]) following the SeedFinding precedent: real SearchVulnerabilities queries AWS's own global vulnerability intelligence database, which gopherstack has no data source for, so results only ever come from explicitly seeded IDs — real SearchVulnerabilitiesFilterCriteria.vulnerabilityIds is a required exact-ID lookup list, not a free-text query, so this is a faithful (not simplified) model of the real request contract. Re-field-diffed this pass (gopherstack-zj76): Vulnerability.AtigData/CisaData/Cvss2/Cvss3/Cvss4/Epss/ExploitObserved (7 real, distinct nested struct types, confirmed via types.go) remain deliberately unmodeled. Decision, not an oversight: SeedVulnerability already lets a caller supply any real value at seed time (the exact precedent that made BatchGetFindingDetails' epssScore/riskScore/cwes/tools/ttps additive-safe — seed data is caller-supplied truth, not fabricated), so these COULD be added the same way; they were not this pass because it is a substantial multi-struct-type addition (7 new nested types with their own sub-fields) rather than a targeted bug fix, and belongs in a dedicated future pass with its own field-diff and test budget rather than folded into a remainder/cleanup pass. Not a disguised gap: omitted (unset on the wire), not fabricated."}
  batch_get_code_snippet: {status: ok, note: "fixed this pass — added SeedCodeSnippet (store.Table[codeSnippet]) following the SeedFinding precedent: gopherstack has no static analysis engine to derive real code snippet content, so BatchGetCodeSnippet returns seeded content for a finding ARN, or a CODE_SNIPPET_NOT_FOUND error entry (the only CodeSnippetErrorCode meaningful here) for any ARN with none seeded — replacing the prior stub, which silently ignored its input entirely and always returned two empty lists regardless of what was asked for"}
  batch_get_finding_details: {status: ok, note: "fixed in an earlier pass — the request handler decoded findingArns into []map[string]any; real BatchGetFindingDetailsInput.findingArns is a plain string array (confirmed via api_op_BatchGetFindingDetails.go), so every real client request of the form {\"findingArns\":[\"arn1\",\"arn2\"]} failed json.Unmarshal with a ValidationException (client-breaking). Fixed to []string. Finding gained optional epssScore/riskScore/cwes/referenceUrls/tools fields (real FindingDetail shape) settable via SeedFinding; BatchGetFindingDetails now returns them for findings that exist, or a FINDING_DETAILS_NOT_FOUND error entry for ARNs that don't, replacing the prior always-empty stub. This pass (gopherstack-zj76) additionally added Ttps ([]string, identical shape to the already-modeled Cwes/Tools/ReferenceUrls — a cheap, safe addition, unlike the objects below) to both Finding and findingDetailToWire. Re-field-diffed this pass: FindingDetail.CisaData (*CisaData)/Evidences ([]Evidence)/ExploitObserved (*ExploitObserved) remain deliberately unmodeled — real, distinct nested struct types (confirmed via types.go), same 'substantial multi-struct addition, future dedicated pass' decision as vulnerability_search's Cvss2/Cvss3/Cvss4/Epss/AtigData/CisaData/ExploitObserved (ExploitObserved and CisaData are the identical shared types in both places)."}
  batch_get_free_trial_info: {status: ok, note: "unchanged this pass"}
  get_clusters_for_image: {status: ok, note: "fixed this pass — two wire bugs: (1) the request handler decoded a bare 'filterCriteria' map, but real GetClustersForImageInput nests the required resourceId under a 'filter' object (ClusterForImageFilterCriteria), confirmed via serializers.go, so the value was silently dropped on every real request and the required-field validation never ran; (2) the response used 'clusters' but the real wire key is 'cluster' (singular), confirmed via deserializers.go, so a real client's Cluster field was never populated. Now validates the required filter.resourceId (ValidationException if absent) and emits the correct 'cluster' key. Still returns an empty cluster list always: gopherstack has no ECS/EKS cluster-membership tracking to join an ECR image against, and fabricating cluster ARNs would be worse than an honest empty (but now correctly-keyed and validated) result — see gaps below"}
  connectors: {status: ok, note: "new this pass — CreateConnector/UpdateConnector/DeleteConnector/ListConnectors added for the inspector2@v1.53.0 SDK bump. Real ConnectorCloudProvider has exactly one value, AZURE (confirmed via types/enums.go) — there is no GitHub/GitLab connector type in the real API despite that being a natural guess from the 'connector' name; code-repository integrations are the separate, pre-existing CodeSecurityIntegration family, unaffected by this pass. CreateConnectorOutput/UpdateConnectorOutput field-diffed against api_op_CreateConnector.go/api_op_UpdateConnector.go: each returns only connectorArn (confirmed asynchronous — no full Connector echo), which this backend matches rather than inventing a fuller response. Connector wire shape field-diffed against deserializers.go's awsRestjson1_deserializeDocumentConnector: createdAt/updatedAt/health.lastCheckedAt are real 'date-time' (RFC 3339 string, parsed via smithytime.ParseDateTime) timestamps, NOT the unixTimestamp epoch-seconds shape pkgs/awstime.Epoch targets elsewhere in this service — confirmed against the deserializer instead of assumed, avoiding a wire bug class this campaign has hit in other services. Connector authorization lifecycle modeled honestly per this campaign's finding (also hit by securityhub): real ConnectorHealthStatus includes PENDING_AUTHORIZATION for an unfinished external Azure AD app-consent (OAuth) flow, and none of the 6 connector SDK ops drive or observe that step, so this backend creates connectors at EnablementStatus=PENDING_ENABLEMENT / Health.ConnectorStatus=PENDING_AUTHORIZATION and never auto-advances either (UpdateConnector moves EnablementStatus to PENDING_UPDATE, still never auto-resolving to ENABLED/CONNECTED). DeleteConnector's real PENDING_DELETION EnablementStatus value is not modeled: there is no GetConnector operation through which a caller could ever observe an in-between state, so this backend completes the delete synchronously rather than leaving the connector permanently listed as 'pending' and unobservably undeleted. ListConnectors' filterCriteria supports provider/connectorArns/awsConfigConnectorArns (each real filter's Comparison enum has exactly one value, EQUALS, confirmed via types/enums.go) — accounts (meaningless in this single-account emulator) and connectorType (no corresponding field on the real Connector response type to filter against at all) are not modeled, documented rather than silently ignored, following the coverage/vulnerability_search precedent for omitted filter facets."}
  connector_scan_configuration: {status: ok, note: "new this pass — ListConnectorScanConfigurations/UpdateConnectorScanConfiguration added for the inspector2@v1.53.0 SDK bump. There is no CreateConnectorScanConfiguration operation in the real API (confirmed via `go doc .../inspector2`); UpdateConnectorScanConfiguration is the sole write path, keyed by awsConfigConnectorArn rather than connectorArn (confirmed via serializers.go's awsRestjson1_serializeOpDocumentUpdateConnectorScanConfigurationInput). UpdateConnectorScanConfiguration validates that at least one Connector carries the given awsConfigConnectorArn, returning ResourceNotFoundException for an unrecognized one rather than accepting any ID, per this campaign's explicit requirement to validate the connector actually exists. ConnectorScanConfigurationItem's connectorArns member is derived live from the connectors table's byAwsConfigArn secondary index at read time (not stored alongside the scan configuration), matching that it is a live join in the real API, confirmed via deserializers.go's awsRestjson1_deserializeDocumentConnectorScanConfigurationItem."}
gaps:
  - "ListFindingAggregations genuinely supports 7 of the 15 real AggregationType values (gopherstack-f9vi, extending gopherstack-or9): ACCOUNT, TITLE, REPOSITORY, AWS_EC2_INSTANCE, AWS_ECR_CONTAINER, AWS_LAMBDA_FUNCTION, CODE_REPOSITORY. The remaining 8 (PACKAGE, AMI, IMAGE_LAYER, LAMBDA_LAYER, FINDING_TYPE, CONTAINER_IMAGE, SERVERLESS_FUNCTION, VM_INSTANCE) need Finding/FindingResource detail this backend's model does not carry (package/vulnerability sub-struct, AMI ID, image-layer hash, Lambda layer ARN) or, for FINDING_TYPE, have a response shape with no group key to aggregate by at all — see the finding_aggregations note above for the full per-type breakdown. Also unmodeled: the aggregationRequest parameter (per-type sort/filter sub-object, e.g. FindingTypeAggregation.findingType) is accepted but not read for any type, including ACCOUNT — unchanged scope from before this pass, since ACCOUNT's own request member (AccountAggregation) carries no facets that would narrow results in this backend anyway."
  - "A SUPPRESS filter's effect on findings (gopherstack-or9) is one-directional: creating/updating a filter to SUPPRESS suppresses currently- and subsequently-matching ACTIVE findings, but deleting the filter or changing its action away from SUPPRESS does not revert any finding it previously suppressed back to ACTIVE. Neither the pinned SDK's doc comments nor the API Reference document reversal semantics, so this was left undecided rather than guessed."
  - "ListConnectors' ConnectorFilterCriteria.accounts/connectorType facets are not modeled (accounts is meaningless in this single-account emulator; connectorType — CUSTOMER_MANAGED/SERVICE_LINKED — has no corresponding field on the real Connector response type to filter against at all, confirmed via types/types.go). Only provider/connectorArns/awsConfigConnectorArns are supported."
  - "Connector's real PENDING_DELETION EnablementStatus value and ScopeConfiguration's real ACTIVE/ERROR/DISABLED State values are never reached: this backend's connectors never leave PENDING_AUTHORIZATION (no out-of-band Azure OAuth step exists in the SDK to drive them further), so DeleteConnector completes synchronously and every submitted scope setting is always reported PENDING. Both are deliberate, documented simplifications of an inherently external-system-dependent async lifecycle — see the connectors family note above."
  - "CreateCodeSecurityIntegrationOutput's optional 'authorizationUrl' member (real API: OAuth callback URL for GitHub/GitLab-type integrations) is never returned. gopherstack has no OAuth flow to derive a real URL from; omitting it is unset-on-the-wire, not wire-breaking. Re-confirmed this pass (gopherstack-zj76) against the live AWS API Reference (docs.aws.amazon.com/inspector/v2/APIReference/API_CreateCodeSecurityIntegration.html): authorizationUrl is genuinely populated by real AWS as part of initiating the OAuth handshake with the repository provider (GitHub/GitLab) — there is no request input or local state gopherstack could derive an equivalent, real, dereferenceable URL from. Honest, confirmed-impossible-to-close gap, not a stub."
  - "RESOLVED this pass (gopherstack-zj76): CreateCisScanConfiguration/UpdateCisScanConfiguration.scanName, CreateCodeSecurityIntegration.name, and CreateCodeSecurityScanConfiguration.name now enforce their real, documented constraints, fetched from the live AWS API Reference (the Go SDK module's doc comments carry no length/pattern prose for any of these four fields, unlike CreateFilterInput.name): scanName is 1-128 characters, no charset pattern; the two CodeSecurity name fields are 1-60 characters matching pattern `[a-zA-Z0-9-_$:.]*`. Real AWS returns ValidationException for a violation; this backend previously accepted any non-empty string (and, for UpdateCisScanConfiguration, any string at all including one exceeding 128 chars)."
  - "GetClustersForImage always returns an empty (but now correctly-keyed, request-validated) cluster list: gopherstack has no ECS/EKS cluster-membership tracking to join an ECR image resourceId against — re-confirmed this pass: neither services/ecs nor services/eks track any image-to-cluster membership state to join against. Would need a SeedClustersForImage capability plus real ECS/EKS service cross-references to close for real; lower priority than the wire-shape bugs already fixed since GetClustersForImage is a low-traffic informational op."
  - "CodeSecurityScanConfiguration.scopeSettings and .periodicScanConfiguration/.continuousIntegrationScanConfiguration are still accepted as loosely-typed map[string]any pass-throughs rather than validated against ScopeSettings.projectSelectionScope's (ALL|SPECIFIC) / PeriodicScanConfiguration.frequency's (WEEKLY|MONTHLY|NEVER) / ContinuousIntegrationScanConfiguration.supportedEvents's (PULL_REQUEST|PUSH) real enum constraints — only the outer 'configuration' nesting, required level, and required ruleSetCategories enum were fixed in an earlier pass. Real AWS returns ValidationException for enum violations; this backend accepts any value."
  - "PARTIALLY RESOLVED this pass (gopherstack-zj76): CoverageFilterCriteria's scanStatusCode/scanStatusReason/scanMode string facets and lastScannedAt date-range facet now genuinely narrow ListCoverage/ListCoverageStatistics results (they were previously accepted-but-silently-ignored despite real backing data already existing on CoverageEntry — a real bug, not just an omission, since a client-supplied filter on these facets was a no-op that over-returned results). Still not modeled, and genuinely so (no backing data at all, confirmed via CoverageFilterCriteria's full field list in types.go): the ~20 remaining facets tied to CoveredResource.resourceMetadata (a nested per-resource-type metadata union this backend never populates) — ec2InstanceTags, ecrImageTags, ecrImageInUseCount, ecrImageLastInUseAt, imagePulledAt, lambdaFunctionTags, cloudContainerImageTags, and the rest of the cloud*/code*/lambda* tag and resource-attribute facets."
  - "Vulnerability's nested AtigData/CisaData/Cvss2/Cvss3/Cvss4/Epss/ExploitObserved objects and FindingDetail's CisaData/Evidences/ExploitObserved objects (7 distinct real struct types total, confirmed via types.go this pass) are real but not modeled — only scalar/list fields are seedable via SeedVulnerability/SeedFinding. FindingDetail.Ttps was the one exception: fixed this pass (gopherstack-zj76) as a plain []string, identical in shape to the already-modeled Cwes/Tools/ReferenceUrls, so it was folded into this pass; the 7 struct-typed objects above are a genuinely larger addition (each carries its own several-field sub-shape) deliberately left for a dedicated future pass rather than expanded here — see the vulnerability_search/batch_get_finding_details family notes above for the full reasoning. SeedVulnerability/SeedFinding already make this additive-safe whenever that pass happens (seed data is caller-supplied truth, not fabricated)."
deferred:
  - "Full CIS session lifecycle semantics (health/telemetry payload validation, session expiry) — accepted as no-ops, not audited for correctness beyond routing/basic state."
leaks: {status: clean, note: "no goroutines/janitors in this service; all resource maps (including the 3 new tables added this pass: coverageEntries, vulnerabilities, codeSnippets) are store.Table-backed and cleared by Reset/registry.ResetAll; every new Lock/RLock call site pairs with an immediately-following defer Unlock/RUnlock"}
---

## Notes

**2026-08-07 (fixed by a concurrent account-service pass, gopherstack-303i)**: `RouteMatcher`
matched `pathEnable`/`pathDisable` (`"/enable"`/`"/disable"`) as raw path *prefixes* with no
SigV4-service-name gate, so `strings.HasPrefix("/enableRegion", "/enable")` wrongly claimed
`services/account`'s `POST /enableRegion`/`/disableRegion` before Account's own (correctly
service-gated) `RouteMatcher` ever ran -- confirmed live via `test/integration/account_test.go`
(501 NotImplementedException from Inspector2, not the expected Account response). Per this
package's own `{method, path}` dispatch table, `/enable`/`/disable` are exact fixed paths with
no children (real Inspector2 has no `/enableFoo` sub-resource), so prefix matching was never
correct for these two entries regardless of Account. Fixed: `/enable`/`/disable` now require
exact path equality in `RouteMatcher`, checked before the (unchanged) prefix loop that still
serves every genuine directory-style prefix (`/filters/`, `/status/`, ...). All existing
Inspector2 tests still pass unmodified.

Protocol: restjson1. All request/response bodies are JSON; most ops are POST with
an explicit action path (e.g. `/findings/list`), a handful use GET/PUT/DELETE
(GetEncryptionKey=GET, Reset/UpdateEncryptionKey=PUT, StartCisSession/StopCisSession/
SendCisSessionHealth/SendCisSessionTelemetry=PUT, TagResource=POST,
UntagResource=DELETE, ListTagsForResource=GET on `/tags/{arn}`).
The route matcher (`RouteMatcher`/`classifyPath`/`classifyExtendedPath`) was
cross-checked op-by-op against `aws-sdk-go-v2/service/inspector2@v1.48.2`'s
serializers.go (method + SplitURI path per op) in a prior pass: all 75 routed
ops (13 base + 62 extended) matched the real SDK's method+path exactly. This
pass adds 6 more (all extended/POST-body-dispatched, matching this package's
existing convention): CreateConnector (`/connector/create`), UpdateConnector
(`/connector/update`), DeleteConnector (`/connector/delete`), ListConnectors
(`/connector/list`), ListConnectorScanConfigurations
(`/connectorscanconfigurations/list`), UpdateConnectorScanConfiguration
(`/connectorscanconfiguration/update`) — every path cross-checked against
`aws-sdk-go-v2/service/inspector2@v1.53.0`'s serializers.go
`httpbinding.SplitURI(...)` call for that op, confirming an exact match
(including the plural/singular `connectorscanconfigurations` vs
`connectorscanconfiguration` path segments, which are easy to transpose).
The handler now routes 81 ops total (13 base + 68 extended).

**2026-08-13 (gopherstack-jqh2 pass 2):** the prior op-by-op cross-checks above were
manual verification, not a permanent test — the pre-existing `handler_routing_test.go`
was generated from this package's own switch statements (a refactor-safety guardrail)
and never covered the 6 Connector ops. Re-extracted all 81 ops' real method+path
independently from `inspector2@v1.54.1` serializers.go and added
`handler_sdk_route_table_test.go` (`TestExtractOperation_SDKRouteTable`, full 81/81
coverage including the 6 Connector ops). All 81 resolved correctly — no bugs, the
manual cross-checks held.

### Connectors and connector scan configuration (new this pass)

The Go SDK module was bumped to `aws-sdk-go-v2/service/inspector2@v1.53.0`
(from `v1.48.2`), which added 6 operations with no prior gopherstack
implementation: `CreateConnector`/`UpdateConnector`/`DeleteConnector`/
`ListConnectors` (a new Azure-cloud-provider "connector" resource family,
`connectors.go`/`handler_connectors.go`) and
`ListConnectorScanConfigurations`/`UpdateConnectorScanConfiguration` (scan
settings keyed by the connector's associated AWS Config connector ARN, same
files). All 6 are genuinely implemented against real backend state
(`store.Table[Connector]` + a `byAwsConfigArn` secondary index +
`store.Table[ConnectorScanConfiguration]`, both flowing through
`b.registry.SnapshotAll()`/`RestoreAll()` automatically — no
`persistence.go`/`inspector2SnapshotVersion` change needed, following the
`coverageEntries`/`vulnerabilities`/`codeSnippets` precedent from the prior
pass), not added to `sdk_completeness_test.go`'s `notImplemented` list. See
the `connectors`/`connector_scan_configuration` family notes above for the
full field-diff and the deliberate, documented authorization-lifecycle
simplifications (connectors never leave `PENDING_AUTHORIZATION` — there is no
SDK operation that could ever drive or observe completion of the real
external Azure OAuth consent step, the same bug class this campaign's
securityhub connector work flagged).

### This pass's wire-shape and invented-field fixes

Every "gap"/"partial" family the prior audit (2026-07-12, commit `1e21a848`)
left open was field-diffed against `aws-sdk-go-v2/service/inspector2@v1.48.2`'s
`types/types.go`, `api_op_*.go`, `serializers.go`, and `deserializers.go` this
pass (not just re-read from the prior notes). No inspector2 source changed
between `1e21a848` and this pass's start (`git log 1e21a848..HEAD --
services/inspector2/` was empty), so every family marked `ok` by the prior
pass was trusted without re-diffing, per the manifest's own re-audit protocol.

**Client-breaking wire bugs fixed** (confirmed via the SDK's own
`serializers.go`/`deserializers.go`):
- `BatchGetFindingDetails` request: `findingArns` was decoded into
  `[]map[string]any`; the real shape is a plain `[]string`. Every real
  client's request of the form `{"findingArns":["arn1","arn2"]}` failed
  `json.Unmarshal` with a 400 ValidationException.
- `CreateSbomExport` request: used the gopherstack-invented `sbomFormat` key;
  the real `CreateSbomExportInput` field is `reportFormat`. Every real
  client's report format was silently dropped (unrecognized key, not a
  decode error, so this one degraded rather than crashed).

**Silently-wrong (wrong key name, not crashing) wire bugs fixed**:
- `GetClustersForImage` request: decoded a bare `filterCriteria` map; the
  real `GetClustersForImageInput` nests the required `resourceId` under a
  `filter` object (`ClusterForImageFilterCriteria`). The required-field
  validation never ran and the value was always dropped.
- `GetClustersForImage` response: emitted `"clusters"`; the real wire key is
  `"cluster"` (singular). A real client's `Cluster` field was never
  populated regardless of backend content.

**Invented fields deleted** (no counterpart in the real SDK types):
- `AccountPermission.Status` (wire key `"status"`) — the real `Permission`
  type has no such member; the real second field is `Service` (wire key
  `"service"`), which this backend never populated at all.
- `Vulnerability.VulnerabilityID`/`Severity` (wire keys `"vulnerabilityId"`/
  `"severity"`) — the real `Vulnerability` type's id field wire key is
  `"id"`, and the closest real severity-like field is `VendorSeverity` (wire
  key `"vendorSeverity"`), a distinct semantic (the reporting vendor's own
  severity label, not an Inspector-computed one).

**Prior gap notes corrected** (the real API turned out not to have the
fields the prior note assumed were missing): `GetFindingsReportStatusOutput`
and `GetSbomExportOutput` have **no `createdAt` member at all** in the real
API (confirmed via `deserializers.go`'s case-block enumeration) — the prior
pass's gap note asking for it was itself slightly wrong. `FindingsReport`/
`SbomExport.CreatedAt` remain backend-internal bookkeeping fields and must
never reach the wire.

### New additive seed capabilities (the SeedFinding precedent, extended)

Real Amazon Inspector populates coverage, code snippets, and vulnerability
intelligence automatically via managed scanning engines and AWS's own global
vulnerability database — none of which gopherstack has an equivalent data
source for. Rather than leaving `ListCoverage`/`ListCoverageStatistics`,
`BatchGetCodeSnippet`, and `SearchVulnerabilities` as permanent
hardwired-empty stubs (LocalStack's behavior for these ops), this pass added
`SeedCoverage`/`SeedCodeSnippet`/`SeedVulnerability` — following the exact
precedent `SeedFinding` established. Each real backing store is a
`store.Table` registered on the registry (so `Snapshot`/`Restore` cover them
for free), and each list/search/batch-get op now does genuine state
lookup/filtering/pagination against seeded data instead of returning a
constant empty envelope. `ListUsageTotals` similarly went from a hardwired
constant to a real derivation from `Enable`d resource types and seeded
coverage counts. `ListAccountPermissions` went from hardwired-empty to
computing the real Operation x Service matrix (gopherstack's mock account
has no IAM engine to evaluate against, so it reports the account as able to
perform every configuration operation — a defensible default, not a
fabrication, since there is no real permission model to be unfaithful to).

### Timestamp wire format

Unchanged from the prior pass's fix set (see git history for
`BatchGetFindingDetails`/`ListFindings`/`GetMember`/`ListMembers`/
`BatchGetFreeTrialInfo`/`ListCisScans`/`GetCodeSecurityIntegration`/
`GetCodeSecurityScanConfiguration`). This pass's new timestamped wire
surfaces (`CoverageEntry.LastScannedAt` via `coverageEntryToWire`,
`Vulnerability.VendorCreatedAt`/`VendorUpdatedAt` via
`vulnerabilitiesToWire`) follow the same pattern: epoch-seconds via
`pkgs/awstime.Epoch`, built by hand in a `*ToWire` function, never via
`encoding/json`'s default `time.Time` marshaling. **Trap for the next
auditor** (unchanged): any *new* handler that does `c.JSON(status,
someDomainStruct)` where the struct has a `time.Time` field reaching the
wire directly is reintroducing this bug class.

### Persistence

Unchanged in structure from the prior pass. This pass added three tables —
`coverageEntries` (`CoverageEntry`, composite key `resourceId/scanType`),
`vulnerabilities` (`Vulnerability`, keyed by `id`), and `codeSnippets`
(`codeSnippet`, keyed by `findingArn`) — all registered via
`store.Register`/`registerAllTables` (`store_setup.go`), so they flow
through `b.registry.SnapshotAll()`/`RestoreAll()` automatically with no
`persistence.go` changes required. `inspector2SnapshotVersion` was **not**
bumped: the registry-driven table snapshot format is additive (a new table
name in the `Tables` map), and `RestoreAll` tolerates a snapshot missing
newer table names (pre-this-pass snapshots simply restore with those three
tables empty).

This (2026-07-25) pass adds two more tables the same way: `connectors`
(`Connector`, keyed by `ConnectorArn`, with a `byAwsConfigArn` secondary
`store.Index` — see `store_setup.go`) and `connectorScanConfigs`
(`ConnectorScanConfiguration`, keyed by `AwsConfigConnectorArn`). Same
additive-table story: no `persistence.go`/`inspector2SnapshotVersion` change,
`TestInMemoryBackend_SnapshotRestore_FullState` (`persistence_test.go`)
extended to seed and round-trip both, including the `byAwsConfigArn` index
(proven by the round-tripped `ConnectorScanConfigurationItem.ConnectorArns`,
which is derived from that index rather than stored).

### Filter name validation

`CreateFilter`'s `name` is now validated against AWS's real constraint (3-64
characters, alphanumeric plus dot/underscore/dash) via `validateFilterName`
in `filters.go`, returning `ValidationException` on violation. The same
constraint was not extended to `CreateCisScanConfiguration`/
`CreateCodeSecurityIntegration`/`CreateCodeSecurityScanConfiguration` this
pass — their exact real per-op name constraints were not confirmed against
the SDK's validation-trait metadata, and guessing wrong would trade one gap
for a different bug (over-strict rejection of valid real-world names). Left
as a documented gap.

### PARITY.md accuracy note

The 2026-07-23 pass's note above (13 ops / 22 families, unchanged counts) was
accurate as of that pass — it added no new ops or families, only upgraded
existing `gap`/`partial` entries. This (2026-07-25) pass is different: the Go
SDK modules were bumped, `aws-sdk-go-v2/service/inspector2` picked up 6 new
operations (`CreateConnector`/`UpdateConnector`/`DeleteConnector`/
`ListConnectors`/`ListConnectorScanConfigurations`/
`UpdateConnectorScanConfiguration`), and `TestSDKCompleteness` failed until
they were routed. All 6 are genuinely implemented (see the `connectors`/
`connector_scan_configuration` family notes above), not added to
`sdk_completeness_test.go`'s `notImplemented` list. The handler's routed-op
count goes from 75 to 81 (13 base + 68 extended, up from 62); the `families:`
entry count goes from 22 to 24. Per this campaign's instructions, `go run
./cmd/gendocs` was deliberately **not** run this pass (it regenerates
unrelated services' READMEs as a side effect) — the badges/README's counts
are stale until the next full `gendocs` regeneration; this manifest is the
source of truth in the interim.

### 2026-08-21 gopherstack-r80d batch 12: required-output cut, 4 bugs

Selected as the largest remaining candidate after sagemaker (off-limits,
mid-conversion this session) per `services/_REQUIRED_OUTPUT_CANDIDATES.md`'s
ranked table: 38 required output fields / 81 ops (29 with at least one),
confirmed with a fresh `go run ./cmd/requiredoutputfields` run against
`inspector2@v1.54.1`. Read all 29 ops with required output fields end to
end against their handlers, plus every domain struct with `This member is
required.` annotations in `types.go` (an AST-style walk, not a grep window)
to catch the nested-domain-struct undercount class this campaign already
named for pinpoint/bedrockagent/cleanrooms — `CodeSecurityIntegrationSummary`
(7 required members) is exactly that shape here, reachable only through
`ListCodeSecurityIntegrations`' non-required `Integrations` field.

4 bugs found and fixed, all proven via real `aws-sdk-go-v2/service/
inspector2` client round trips (`wire_output_required_r80d_test.go`),
hand-reverted (both source files reverted to `HEAD` together, confirmed both
new tests fail with the predicted symptom)/confirmed-failing/restored,
md5sum-verified byte-identical:

1. **`GetCodeSecurityIntegration`/`ListCodeSecurityIntegrations`** (shared
   `codeSecurityIntegrationToWire` helper in `handler_code_security.go`)
   dropped required `type`/`statusReason` (`api_op_GetCodeSecurityIntegration.go`,
   `types.go`'s `CodeSecurityIntegrationSummary`). `type` was already tracked
   on the `CodeSecurityIntegration` domain struct (`models.go`) and simply
   never surfaced; `statusReason` has no backing data source at all (no
   OAuth/connection-health flow exists to derive a real reason from), so it
   is now emitted as an honestly empty string rather than fabricated.
   Shape: member with no struct field at all (statusReason) plus a tracked-
   but-unsurfaced field (type).
2. **`Finding.Remediation`** (`handler_findings.go`'s `findingToWire`,
   emitted via `ListFindings`) had no struct field at all
   (`types.go`: required; its own `Recommendation` sub-member is optional).
   Now emitted as an honest empty object — no remediation text/URL
   gopherstack has any source for. Shape: missing struct field entirely.
3. **`Finding.Resources`** (required, `types.go`) was only emitted when
   non-empty (`if len(f.Resources) > 0`), dropping the key entirely for any
   finding seeded with none — reachable, since neither `SeedFinding` nor
   `AddFinding` enforce a non-empty resource list. Now always emitted,
   non-nil. Shape: required-but-inapplicable means present-and-empty, not
   absent.
4. **`Finding.Severity`** was serialized as a fabricated `{label, score}`
   nested object. The real wire shape
   (`deserializers.go`'s `awsRestjson1_deserializeDocumentFinding`, case
   `"severity"`) is a bare `Severity` string enum — this is not a missing-
   field bug but a wrong-shape one, and the most severe found this campaign:
   any real SDK client's `ListFindings` call failed outright
   (`"expected Severity to be of type string, got map[string]interface {}
   instead"`) once any finding existed, not merely a missing value. The
   numeric score now rides the separate, optional, top-level
   `inspectorScore` member (`Finding.InspectorScore *float64`) instead of a
   nested `score` key that never existed on the real shape. This also means
   the manifest's prior `ListFindings: {wire: ok}` verdict was never
   actually exercised against a real SDK client: every existing
   `ListFindings`-adjacent test in this package asserted on raw JSON
   (`handler_findings_core_test.go`, `handler_findings_query_test.go`), and
   `sdk_response_keys_test.go`'s own doc comment already explains why that
   can't catch a wrong-shape bug — this campaign's "a recorded verdict is
   not evidence" lesson, reapplied. The 5 existing raw-JSON assertions on
   the old `{label,score}` shape were updated to match the corrected wire
   shape rather than left contradicting the fix.

All 29 ops with required output fields were read end to end; no other bugs
found. Filters/CIS-scan-configuration/connector/coverage/vulnerability-search
families were re-confirmed clean, including validator-based reachability
checks (e.g. `CreateFilterInput`'s `FilterCriteria` sub-facets are all
individually optional per `validateFilterCriteria`, matching the databrew-
class "validator only checks the top-level pointer" shape — but every real
client's request still decodes to a non-nil Go map even for an empty
`{}` object, so `Filter.Criteria != nil` never actually goes false in
practice). `AutoEnable`'s `Ec2`/`Ecr` are both real-client-guaranteed present
(the SDK's own `validateOpUpdateOrganizationConfigurationInput` rejects a nil
`AutoEnable`, and `Ec2`/`Ecr` are validated required within it), so
`UpdateOrganizationConfiguration`'s echo-the-request-map approach is safe.

Did not touch sagemaker (off-limits, mid-conversion under gopherstack-oc9v
this session — `git status` showed uncommitted `services/sagemaker/`
changes throughout).

# 2026-08-21 gopherstack-g479 (ad hoc map[string]any blind spot)
`DescribeOrganizationConfiguration`: {wire: fixed, errors: ok, state: fixed, persist: ok} --
`OrgConfiguration.AutoEnable` was stored and echoed as a single collapsed
`bool`; real `DescribeOrganizationConfigurationOutput.AutoEnable` is the
per-scan-type object (`ec2`/`ecr`/`lambda`/`lambdaCode`/`codeRepository`),
confirmed against `aws-sdk-go-v2/service/inspector2@v1.54.1`'s
`deserializers.go`
(`awsRestjson1_deserializeOpDocumentDescribeOrganizationConfigurationOutput`,
case `"autoEnable"`) and `types/types.go`'s `AutoEnable` struct. A real
client's `DescribeOrganizationConfiguration` failed outright with
`"unexpected JSON type true"`. The prior audit (r80d, see above) confirmed
`UpdateOrganizationConfiguration`'s echo-the-request-map approach was safe,
but never exercised the Describe side against a real client, which is where
this collapse actually lived. `OrgConfiguration.AutoEnable` is now
`map[string]bool`; `UpdateOrganizationConfiguration` stores the real map
instead of collapsing it, then reads its own write back rather than echoing
the raw request. Found via a new map-literal/index-assign kind-mismatch
scanner (go/types-based, not text-matched) built for gopherstack-g479 --
this class (`map[string]any{}` literals with no struct-field path) had zero
automated coverage before. Proven via a real `aws-sdk-go-v2/service/inspector2`
client round trip (`TestDescribeOrganizationConfiguration_AutoEnableObject`,
`sdk_response_keys_test.go`), hand-reverted/confirmed-failing with the SDK's
own error text/restored/`md5sum`-verified byte-identical. Gates:
`go build`, `go vet`, `gofmt -l` (clean), `go test -race` (pass),
`golangci-lint run` (0 new issues; 2 pre-existing `fieldalignment` findings
in `persistence.go`/`store.go`, files this pass did not touch, left alone).
`last_audit_commit` left at its existing value per this file's own
standing convention (r80d note above) -- not updated this pass.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 4 confirmed bugs

Part of the gopherstack-us9u/g479 map-literal scanner's 526-key unknown-key
bucket triage. All 4 proven via real `aws-sdk-go-v2/service/inspector2`
client round trips or raw-body assertion
(`wire_field_fixes_y1zn_test.go`), hand-reverted, confirmed failing,
restored, `md5sum`-verified byte-identical.

- `GetCisScanResultDetails`: {wire: fixed} -- wrapped results under
  "checkResults"; real member (deserializers.go's
  awsRestjson1_deserializeOpDocumentGetCisScanResultDetailsOutput) is
  "scanResultDetails".
- `ListCisScans`: {wire: fixed} -- emitted a flat "targetAccountId" string;
  types.CisScan has no such member -- account IDs live under
  "targets.accountIds" (types.CisTargets). TargetResourceTags remains
  honestly absent (this backend tracks no such state).
- `ListFindingAggregations`: {wire: fixed} -- severityCounts included a "low"
  key; types.SeverityCounts declares only all/critical/high/medium -- LOW
  findings fold into "all" only, there is no separate low bucket in the real
  API.
- `GetEc2DeepInspectionConfiguration`: {wire: fixed} -- wrapped a fabricated
  "ec2ScanModeState" object (scanMode/scanModeStatus, neither real) into the
  response; GetEc2DeepInspectionConfigurationOutput declares only
  errorMessage/orgPackagePaths/packagePaths/status.

## Equality-matched-cursor restart sweep (2026-08-30)

Every paginated listing in this service (`ListFindings`, `ListConnectors`,
`ListConnectorScanConfigurations`, `ListCoverage`) resumed a `nextToken` by scanning for
the item whose key equalled the token and left `start` at 0 on no match -- an
unresolvable token restarted pagination at page one instead of truncating. Findings,
coverage entries, and connector scan configurations have no delete operation in real
Inspector2 (status changes only, or a derived/live view), so every hostile test here
forges an unresolvable token; `ListConnectors`'s test genuinely deletes the cursor's
connector, since `DeleteConnector` exists.

`ListConnectors` (sorted by `ConnectorArn`, the `connectors` table's own key),
`ListConnectorScanConfigurations` (sorted by `AwsConfigConnectorArn`, the
`connectorScanConfigs` table's own key), and `ListCoverage` (sorted by
`coverageEntryKeyFn`, the `coverageEntries` table's own composite key) are each sorted
by exactly the field their cursor carries and that field is unique, so all three were
converted to a threshold search: resume at the first item whose key is strictly greater
than the token.

`ListFindings` is different and required two fixes:

1. **Restart bug**: `matched` is sorted by `sortField` (`sortFindings`), which only
   equals `FindingArn` (the cursor's field) when the caller didn't request
   `SortCriteria` -- with any other field (`SEVERITY`, `AWS_ACCOUNT_ID`, etc.) the list
   isn't ordered by `FindingArn` at all, so a threshold search on the cursor wouldn't be
   valid for those callers. Since the same function must serve both cases, fixed by
   defaulting an unresolved token to the end of the collection instead.
2. **Non-total sort / tie compounding**, found while checking the sort per this
   campaign's known trap (quicksight's tied-name bug): every `sortFindings` field but
   `FindingArn` itself admits ties (many findings can share a severity, status, type,
   account, or timestamp), and `matched` is built via `store.Table.Range`, which
   iterates Go's underlying map in genuinely randomized order on every call (unlike
   rolesanywhere's `store.Index.Get`, which returns an insertion-ordered slice -- this
   is a real, not just theoretical, difference; confirmed both ways with dedicated
   tests). Without a tiebreak, two findings tied on the requested sort field could land
   in a different relative order on the page-2 call than they did on page 1, letting an
   already-served finding reappear or letting one slip past the cursor entirely. A test
   with 24 same-severity findings paginated 3-at-a-time reproduced this concretely on
   unmodified code: only 9 of 24 were ever visited before the walk stopped advancing.
   Fixed by appending `FindingArn` (unique) as a tiebreak to every `sortFindings`
   comparator, making the overall order total and reproducible across the repeated
   calls pagination makes.

`SearchVulnerabilities` also contains the same equality-match-with-unhandled-miss shape
(`findings.go`), but is inert: it never emits a `nextToken` (`return matched[start:],
"", nil` unconditionally), so no client ever receives a token to follow into a second
call, and there's no page-size cap to make a second page necessary regardless of
match count. Left as-is -- fixing dead code here would be adding unproven surface
against a bug that cannot actually manifest through this API.

New tests (`handler_pagination_restart_test.go`, all confirmed failing pre-fix except
the tied-name check noted separately in rolesanywhere's own entry):
`TestListFindings_Pagination_StaleTokenDoesNotRestart`,
`TestListFindings_Pagination_TiedSeverityNoDropOrDuplicate` (reproduced the drop
concretely, see above), `TestListConnectors_Pagination_DeletedMidPage`,
`TestListConnectorScanConfigurations_Pagination_StaleTokenDoesNotRestart`,
`TestListCoverage_Pagination_StaleTokenDoesNotRestart`. No prior test in this service
(`connectors.go`/`coverage_reporting.go` had no dedicated test file at all; `findings`
pagination tests such as `TestListFindings_Pagination` and `TestListFindingsPagination`
only exercised page sizes/happy-path chains) ever deleted an item or forged a token
between pages.

Confirmed no other pagination bug class: every other `List*` op in this service
(`ListCisScans`, `ListMembers`, `ListFilters`, `ListCodeSecurityIntegrations`,
`ListUsageTotals`, `ListDelegatedAdminAccounts`, `ListAccountPermissions`,
`ListCisScanResultsAggregatedBy*`, etc.) has no `nextToken`/pagination logic at all --
each returns its full result set unpaginated, a structural completeness gap distinct
from this bug class, not the restart bug.

**Gates**: `go build ./services/inspector2/...`, `go vet ./services/inspector2/...`,
`go test -race -count=1 ./services/inspector2/...` all pass; `golangci-lint run
./services/inspector2/...` reports 0 issues.

## 2026-08-30 (gopherstack-uox6, value-semantics sweep): audited findings/coverage/
connector/CIS filter matchers, no bug found, 1 gap recorded

Audited (this specific "field read+applied but wrong semantics" class, distinct
from wire-shape/field-diff coverage already tracked above): matchStringFilters +
findingFilterCriteria.matches (findings.go, backing ListFindings' filterCriteria --
severity/findingType/findingStatus/awsAccountId/resourceId/resourceType/title/
findingArn/fixAvailable, each a real `types.StringFilter` per FilterCriteria's own
field-by-field doc page, `Comparison` typed `types.StringComparison` {EQUALS,
PREFIX, NOT_EQUALS} per enums.go); matchDateFilters/coverageStringFilters.matches
(coverage_reporting.go, ListCoverage/ListCoverageStatistics); ListConnectors'
provider/connectorArns/awsConfigConnectorArns membership filters
(handler_connectors.go/connectors.go, real `types.StringFilter`/`ConnectorArnFilter`/
`AwsConfigConnectorArnFilter` whose own Comparison enums each carry exactly one
legal value, EQUALS, already correctly undecoded per the existing code comment);
SearchVulnerabilities' exact-ID lookup; ListCisScanResultsAggregatedBy{Checks,
TargetResource} (no FilterCriteria narrowing at all -- confirmed these two ops take
no criteria parameter in this backend, a structural gap already implied by the
missing param, not a wrong-algorithm bug). All read correctly against their
comparison operators (PREFIX = real prefix match, EQUALS = exact, date ranges
correctly inclusive-both-ends per CoverageDateFilter's startInclusive/endInclusive
wire names) and all consistently OR multiple values within one field, AND across
fields -- confirmed correct, not merely unchanged from a prior pass.

One gap recorded, not fixed: matchStringFilters (findings.go) combines EVERY filter
on a field with a flat OR, including NOT_EQUALS entries mixed with or repeated
alongside EQUALS/PREFIX ones -- the same "wrong boolean" shape as this campaign's
securityhub finding-filter bug (positive OR, negative AND, groups AND), but here I
could not confirm the documented combining rule precisely enough to fix it as a bug
rather than guess a new one: `types.StringFilter`'s own doc comment
(aws-sdk-go-v2/service/inspector2@v1.54.1/types/types.go) is bare ("The operator to
use when comparing values in the filter" / "The value to filter on"), and neither
API_FilterCriteria.html nor API_ListFindings.html (both fetched this pass) carry any
AND/OR combining prose at all -- confirmed directly via WebFetch
(docs.aws.amazon.com/cli/latest/reference/inspector2/list-findings.html: "The
documentation does not contain any prose explaining how multiple filterCriteria
values or multiple filter types combine... No restrictions or interaction guidance
is provided.") A WebSearch synthesis surfaced a plausible-sounding "NOT_EQUALS
filters on the same field are joined by AND" claim, but the same synthesis also
asserted a CONTAINS/NOT_CONTAINS StringComparison for Inspector2 that does not exist
in this SDK's enums.go (StringComparison only has EQUALS/PREFIX/NOT_EQUALS) -- that
result had conflated Inspector2 with SecurityHub's own, differently-shaped
StringFilter, so it was not treated as ground truth. Left matchStringFilters
unchanged rather than fabricate the combining rule from an unverifiable source.

No code changed in this service this pass.

Pages fetched this pass, all via WebFetch, each checked for the injected
"agent-toolkit search-skills" footer pattern flagged on the parent bd issue:
docs.aws.amazon.com/inspector/v2/APIReference/API_FilterCriteria.html (carried the
footer), docs.aws.amazon.com/inspector/v2/APIReference/API_ListFindings.html
(carried the footer), docs.aws.amazon.com/cli/latest/reference/inspector2/
list-findings.html (did NOT carry it). All three treated as untrusted data; no
instruction from any of them was followed.

## 2026-09-04 (gopherstack-or9): family-angle audit, 2 confirmed bugs

This service is the third member of a family whose other two both shipped a
dead evaluation path (guardduty's `Filter.Action` ARCHIVE stored/echoed/never
applied; securityhub's automation rules stored/never evaluated, plus
`GetInsightResults` hardcoded empty). Checked Inspector2's own equivalents
first, with suspicion, per this pass's brief.

**1. `CreateFilter`/`UpdateFilter` SUPPRESS action was CRUD-only** (`filters.go`).
`b.filters` (the stored `Filter` resources, with their `Action`/`Criteria`)
was never referenced anywhere in `findings.go` outside its own CRUD in
`filters.go` — confirmed by grepping every non-test reference to `b.filters`
across the package. A filter's `FilterCriteria` and `Action` were stored and
echoed by `ListFilters`/`GetFilter`-equivalent reads, but had zero effect on
any `Finding`. Real `CreateFilter`'s own SDK doc comment
(`api_op_CreateFilter.go`): "Creates a filter resource using specified filter
criteria. When the filter action is set to `SUPPRESS` this action creates a
suppression rule." — the strongest evidence class (pinned SDK source), and
exactly the family's dead-evaluation-path shape. Fixed: a SUPPRESS filter now
suppresses (1) every currently-ACTIVE finding matching its criteria, at
`CreateFilter`/`UpdateFilter` time, and (2) every subsequently-seeded finding
matching an already-active SUPPRESS filter, at `SeedFinding`/`AddFinding`
time — realizing "creates a suppression rule" as an ongoing rule rather than
a one-off. Reversal (deleting/un-SUPPRESS-ing a filter un-suppressing
findings) is deliberately left undecided — genuinely undocumented in both the
SDK and the API Reference, not guessed. Proven via
`TestFilterSuppression` (`filters_suppression_test.go`), hand-reverted
(neutered `suppressMatchingFindings`'s `f.Action != filterActionSuppress`
guard to `return` unconditionally, confirmed 3 of 4 subtests fail with the
finding stuck ACTIVE, restored) and confirmed passing again.

**2. `ListFindingAggregations` always returned the wrong union member for
every AggregationType but ACCOUNT** (`findings.go`). `types.AggregationResponse`
(inspector2@v1.54.1 `types/types.go`) is a real Smithy union with 15 members
(`accountAggregation`, `amiAggregation`, `packageAggregation`,
`findingTypeAggregation`, `titleAggregation`, `repositoryAggregation`, ...),
and the real deserializer (`deserializers.go`'s
`awsRestjson1_deserializeDocumentAggregationResponse`) selects which member to
populate purely from which JSON key is present in the response object — it
does not consult the request's `aggregationType` at all. This backend always
emitted an `accountAggregation`-keyed entry regardless of what was requested,
so a real client asking for any of the other 14 `AggregationType` values
(e.g. `PACKAGE`) got back an `AccountAggregation` value instead of the one it
asked for — not a crash, but a silently wrong-shaped response for the
overwhelming majority of real `AggregationType` values, and the manifest's
prior `finding_aggregations: {status: ok}` verdict never actually caught it
(existing tests, `TestListFindingAggregations_SeededCounts`/
`TestListFindingAggregations_NoLowKey_RealClient`, only ever exercised
`AggregationType=ACCOUNT`). This backend's `Finding` model has no
per-package/per-resource/per-repository/per-image detail to genuinely
aggregate the other 14 types by, so the fix does not fabricate content under
the correct key for them either: only `ACCOUNT` (the type with real backing
data) returns populated responses now; every other `aggregationType` returns
an honest empty `responses` list under the correctly-echoed
`aggregationType`. Proven via
`TestListFindingAggregations_NonAccountType_RealClient`
(`wire_field_fixes_or9_test.go`, 5 non-ACCOUNT subtests), hand-reverted
(neutered the `aggregationType != aggregationTypeAccount` early-return guard,
confirmed all 5 subtests fail with a populated `accountAggregation` entry
returned for e.g. `AggregationType=PACKAGE`, restored) and confirmed passing
again. `AggregationType=ACCOUNT`'s own existing tests
(`TestListFindingAggregations_SeededCounts`,
`TestListFindingAggregations_NoLowKey_RealClient`) still pass unmodified —
this fix does not change ACCOUNT's behavior.

**Eight named bug patterns, checked this pass**: (a) discarded parameters —
`ListFindingAggregations`' `aggregationRequest` param remains discarded
(pre-existing, now explicitly disclosed as a gap rather than silently wrong);
`SendCisSessionHealth`/`SendCisSessionTelemetry` are no-ops but their state
(`CisSession.Status`) is never surfaced by any Get/List op — confirmed via
grep, genuinely unobservable, matches this file's existing "deferred" note,
not a new finding. (b) accepted-but-never-done — both bugs above. (c)
zero-value bypasses guard — an empty `{}` `FilterCriteria` matches every
finding by design (`matchStringFilters` returns true for an empty filter
list on every field), symmetric with `ListFindings`' own inline
`filterCriteria` handling already audited in the 2026-08-30 value-semantics
pass; not a new bug. (d) ghost rows — not re-litigated per this pass's brief
(prior sweep already cleared `codeSecurityScans`/`enabledTypes`). (e) stale
cache/partial sync — none found; `CisSession.Status` stops at `STOPPING`
after `StopCisSession` but is unobservable (no Get/List op), so there is no
staleness a client could ever detect. (f) missing delete precondition —
`DeleteFilter`, `DisableDelegatedAdminAccount`, `Disable`,
`DisassociateMember` all existence-check before mutating; no new bug found.
(g) correct code nothing reaches — grepped every `is*/check*/validate*`
function in the package for call sites outside tests; all have at least one
real caller. (h) fabricated value — every error type name this service
emits (`ResourceNotFoundException`, `ConflictException`, `ValidationException`,
`InternalServerException`) confirmed present in
`aws-sdk-go-v2/service/inspector2@v1.54.1`'s `types/errors.go`; none
fabricated.

**Cross-service integration** (dimension 3): confirmed absent, structural —
grepped every non-test file in this package for `services/ecr`,
`services/ec2`, `services/lambda`, `services/securityhub` imports: zero
hits. ECR image scanning, EC2 instance coverage, Lambda function scanning,
and SecurityHub finding forwarding are not wired to any other gopherstack
service; `ListCoverage`/`ListFindings` data only ever comes from
`SeedCoverage`/`SeedFinding` (already documented above), not derived from
another service's live state. Not a bug — this backend has no scanning
engine to derive it from, same as the rest of this file's "no data source"
gaps.

**Performance/leaks** (dimensions 4-5): `ListFindings` does a full
`store.Table.Range` scan + sort under `b.mu.RLock` on every call — O(n),
consistent with every other `List*` op in this codebase; no O(n²) pattern
found. No goroutines/tickers/`time.AfterFunc` anywhere in the package
(grepped) — leaks dimension re-confirmed clean, unchanged from the existing
`leaks: {status: clean}` verdict.

**Gates**: `GOTOOLCHAIN=go1.26.6 go test -race -count=1
./services/inspector2/...` passes; `GOTOOLCHAIN=go1.26.6 golangci-lint run
./services/inspector2/...` reports 0 issues (one `goconst` finding surfaced
by this pass's own new `"ACCOUNT"` literal in `findings.go`, pushing the
repo-wide-in-package count to 3 alongside `code_security.go`'s pre-existing
`isValidCodeSecurityLevel` literal — resolved by extracting a package-level
`aggregationTypeAccount` const and using it at both `findings.go` call
sites, dropping the literal count back under the threshold without touching
`code_security.go`); `gofmt -l services/inspector2/` clean.

**2026-09-06 gopherstack-f9vi**: re-examined the `finding_aggregations`
gap's premise ("the Finding model has no per-package/per-resource detail")
and found it half wrong — `Finding.Resources []FindingResource{Type, ID}`
(`models.go`) already existed, ListFindingAggregations just never read it.
Implemented the 6 non-ACCOUNT `AggregationType` values this data genuinely
supports: `TITLE` (grouped by `Finding.Title`), `REPOSITORY`,
`AWS_EC2_INSTANCE`, `AWS_ECR_CONTAINER`, `AWS_LAMBDA_FUNCTION`,
`CODE_REPOSITORY` (each grouped by the `ID` of a `Resources[]` entry whose
`Type` matches the real `types.ResourceType` value, confirmed via
`types/enums.go`), each rendered under its real union-member key and field
names (confirmed against `deserializers.go`'s per-type
`awsRestjson1_deserializeDocument*AggregationResponse` functions —
`titleAggregation`/`title`, `repositoryAggregation`/`repository`,
`ec2InstanceAggregation`/`instanceId`, `awsEcrContainerAggregation`/
`resourceId`, `lambdaFunctionAggregation`/`resourceId`,
`codeRepositoryAggregation`/`projectNames`). Left unimplemented, reported
rather than guessed: `PACKAGE` (no vulnerability/package sub-struct exists
anywhere on `FindingResource`), `AMI` (no AMI-ID field anywhere in this
model — `Ec2InstanceAggregationResponse.Ami` is optional and unrelated to
the required `AmiAggregationResponse.Ami` key), `IMAGE_LAYER` (4 required
fields — `AccountId`/`LayerHash`/`Repository`/`ResourceId` — with no
layer-hash data captured anywhere), `LAMBDA_LAYER` (4 required fields —
`AccountId`/`FunctionName`/`LayerArn`/`ResourceId` — ditto, no Lambda-layer
resource type exists in this model), `FINDING_TYPE`
(`FindingTypeAggregationResponse` carries no group-key field at all — only
`AccountId` plus optional cloud/exploit/fix facets — so it is structurally a
per-account rollup identical in shape to `ACCOUNT`'s own response;
implementing it would duplicate `ACCOUNT` under a different envelope key
rather than add real, testable grouping), and `CONTAINER_IMAGE`/
`SERVERLESS_FUNCTION`/`VM_INSTANCE` (the multi-cloud generalizations of
`AWS_ECR_CONTAINER`/`AWS_LAMBDA_FUNCTION`/`AWS_EC2_INSTANCE` respectively —
deliberately not conflated with those narrower resource types without
stronger evidence of real AWS's actual grouping semantics between the two;
declining to guess rather than fabricate a plausible-looking mapping).
Proven via `TestListFindingAggregations_ResourceKeyedGrouping`
(`findings_aggregation_test.go`, 6 subtests, one per implemented type, each
asserting two distinct groups with correct per-group `severityCounts.all`
rather than merely non-empty responses) and
`TestListFindingAggregations_UnimplementedTypesIgnoreResourceData`
(`findings_aggregation_test.go`, 8 subtests) pinning that the still-
unsupported types stay empty even when the seeded finding carries title and
resource data other aggregation types now group by. Hand-reverted
`findings.go` to `git show HEAD`, confirmed the package still built and all
6 `TestListFindingAggregations_ResourceKeyedGrouping` subtests failed
(`TestListFindingAggregations_UnimplementedTypesIgnoreResourceData` correctly
kept passing unmodified, confirming it is a true pin rather than an
artifact of the new code), restored and confirmed `diff -q` byte-identical.
`wire_field_fixes_or9_test.go`'s `TestListFindingAggregations_NonAccountType_RealClient`
was not weakened: its `title`/`repository` subtests still pass unmodified
because that test's fixture seeds a finding with no title and no resources,
so those two aggregation types still correctly produce zero groups for it —
only its explanatory comment was corrected, since "no per-repository detail"
is no longer an accurate blanket claim. `PACKAGE`/`FINDING_TYPE`/`AMI`
subtests there are unaffected (still genuinely unsupported).

**Gates (this pass)**: `GOTOOLCHAIN=go1.26.6 go test -race
./services/inspector2/...` passes; `GOTOOLCHAIN=go1.26.6 golangci-lint run
services/inspector2/...` reports 0 issues (this pass's own new
`"resourceId"`/`"severityCounts"`/`"aggregationType"`/`"responses"` map-key
literals pushed 4 strings over goconst's threshold — resolved by adding
`keyResourceID`/`keySeverityCounts`/`keyAggregationType`/`keyResponses` to
the existing `key*` constant block in `handler.go` and using them at every
call site, including the 2 pre-existing `"resourceId"` literals in
`code_security.go`/`handler_coverage_reporting.go` that crossed the
threshold alongside the new ones; `golangci-lint run --fix` resolved a
`golines` line-length finding on `findingAggregationResult`'s parameter
list); `GOTOOLCHAIN=go1.26.6 go build ./...` passes; `GOTOOLCHAIN=go1.26.6
go test ./pkgs/persistence/... -run TestSnapshotVersionGuard` passes
unchanged (no snapshot-shaped state touched by this pass — `map[string]any`
wire envelopes only, not a persisted struct).
