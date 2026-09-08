---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: macie2
sdk_module: aws-sdk-go-v2/service/macie2@v1.54.4
last_audit_commit: da77e2959
last_audit_date: 2026-08-29
overall: A                # all 5 prior gaps + both deferred field audits closed this pass; zero gaps/deferred remain
                          # CORRECTED 2026-08-30 (gopherstack-3qg6): SearchResources' own row was `wire:
                          # gap` at the time this A was recorded (BucketCriteria/SortCriteria/pagination
                          # all discarded) even though `gaps: []` below claimed zero gaps -- the row and
                          # the gaps list had drifted apart. Now fixed (see SearchResources row) and gaps:
                          # [] is accurate again.
                          # 2026-08-21 (gopherstack-c8ge): fixed two singleton-config-with-no-Create-op
                          # merge bugs -- UpdateSensitivityInspectionTemplate wholesale-assigned
                          # Description/Excludes/Includes even when a request omitted them (all three are
                          # independently-optional on the real input), and UpdateClassificationScope
                          # wholesale-replaced Excludes instead of honoring the real
                          # ADD/REMOVE/REPLACE Operation discriminator (types.S3ClassificationScopeExclusionUpdate).
                          # See the op rows below.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  EnableMacie: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableMacie: {wire: ok, errors: ok, state: fixed, persist: ok, note: "own doc comment (api_op_DisableMacie.go:11) is 'deletes all settings and resources for a Macie account'; previously only nil'd the session, leaving classification jobs/findings/findingsFilters/customDataIDs/allowLists/s3Buckets/classScopes/resourceProfiles/sensitivityTemplates/revealConfig/classExportConfig/findingsPubConfig/autoDiscoveryConfig/tags as ghost rows. Now cleared. Cross-account org structure (members, administrator, invitations, orgConfig, autoDiscoveryAccounts) is left intact -- not a per-account 'setting or resource'."}
  GetMacieSession: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateMacieSession: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAllowList: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAllowList: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAllowList: {wire: fixed, errors: ok, state: ok, persist: ok, note: "route method was PATCH; real SDK sends PUT /allow-lists/{id} -- unreachable via real client before fix"}
  DeleteAllowList: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAllowLists: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCustomDataIdentifier: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now accepts severityLevels (real CreateCustomDataIdentifierInput field) and threads it through to storage/Get/BatchGet"}
  GetCustomDataIdentifier: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added 'deleted' and 'severityLevels' fields (real GetCustomDataIdentifierOutput has both). Also fixed a real-behavior bug: Get on a soft-deleted identifier previously 404'd -- real AWS soft-deletes (DeleteCustomDataIdentifier never hard-deletes), so Get must keep succeeding with deleted:true; only a never-existed ID 404s now."}
  DeleteCustomDataIdentifier: {wire: ok, errors: ok, state: ok, persist: ok}
  ListCustomDataIdentifiers: {wire: ok, errors: ok, state: ok, persist: ok}
  TestCustomDataIdentifier: {wire: ok, errors: ok, state: ok, persist: n/a}
  BatchGetCustomDataIdentifiers: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "response wrapper key was 'items'; real key is 'customDataIdentifiers', so a real SDK client always deserialized an empty slice. Also added notFoundIdentifierIds, and (this pass) stopped silently excluding soft-deleted identifiers -- now returned with deleted:true, matching BatchGetCustomDataIdentifierSummary's real soft-delete field."}
  CreateFindingsFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFindingsFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateFindingsFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteFindingsFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFindingsFilters: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFindings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Finding was missing count/partition/sample/schemaVersion/classificationDetails/resourcesAffected (real Finding shape); Severity.score was a float defaulting to 5.0 -- real types.Severity.Score is an int64 1-3, so 5.0 was out-of-range/not wire-compatible with real client expectations. All added; see also CreateSampleFindings note on the 'SENSITIVE_DATA' category bug."}
  ListFindings: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "criteria matching supports eq/neq on a handful of fields only -- acceptable reduced-scope emulation, not a stub. FIXED (constraint sweep): SortCriteria was parsed by the handler but never passed to the backend (always sorted by finding ID) -- now applies count/createdAt/updatedAt/type/severity.score (types.SortCriteria's doc-listed AttributeName values backed by this model); resourcesAffected and policyDetails.action.apiCallDetails.firstSeen/lastSeen are also documented values but have no comparable scalar on this model, left as no-ops rather than invented."}
  CreateSampleFindings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Category was hardcoded to the INVENTED value 'SENSITIVE_DATA', which is not a valid FindingCategory (real enum is CLASSIFICATION/POLICY) -- deleted and replaced with prefix-derived CLASSIFICATION/POLICY. Findings now also populate count/partition/sample/schemaVersion and, for CLASSIFICATION findings, classificationDetails+resourcesAffected with realistic sample S3 bucket/object data, matching real Macie's sample-finding behavior of using non-empty example data."}
  GetFindingStatistics: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (constraint sweep): FindingCriteria was parsed by the handler and passed to the backend, but the backend method discarded it into `_` and grouped/counted every finding regardless of the filter."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateClassificationJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "response was missing jobArn entirely (real CreateClassificationJobOutput has only JobArn+JobId); ClassificationJob had no Arn field at all. Added Arn (json:jobArn), computed via arn.Build, threaded through Create+Describe. This pass: also added allowListIds/customDataIdentifierIds/managedDataIdentifierIds/managedDataIdentifierSelector (real CreateClassificationJobInput fields, previously dropped), and now writes create-time tags into the shared tags map (was only echoed on the job struct, so TagResource-added tags worked but Create-time tags never showed up via ListTagsForResource)."}
  DescribeClassificationJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now includes jobArn (see CreateClassificationJob note). This pass, full field audit vs DescribeClassificationJobOutput closed the deferred item: added allowListIds/customDataIdentifierIds/managedDataIdentifierIds/managedDataIdentifierSelector/lastRunErrorStatus/statistics/userPausedDetails. lastRunErrorStatus is always {code:NONE} (no error-injection exists in this emulator); statistics is a static {numberOfRuns:1, approximateNumberOfObjectsToProcess:0} (no execution engine simulates real run progress); userPausedDetails is populated (jobPausedAt/jobExpiresAt, 30-day window) only while jobStatus is USER_PAUSED, matching the real conditional-presence contract, and cleared on any other transition."}
  ListClassificationJobs: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "filterCriteria (includes/excludes, EQ/NE comparators on jobType/jobStatus/name/createdAt -- same reduced-scope emulation as ListFindings' criteria matching) and maxResults/nextToken now actually filter and page instead of always returning every job in one page. JobSummary also gained bucketCriteria/bucketDefinitions (extracted from the stored s3JobDefinition), lastRunErrorStatus, and userPausedDetails to match the real JobSummary shape."}
  UpdateClassificationJob: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "transitioning jobStatus to USER_PAUSED now populates userPausedDetails; transitioning away from it clears userPausedDetails again (see DescribeClassificationJob note). Also: the op previously accepted any jobStatus transition unconditionally; UpdateClassificationJobInput.JobStatus's own doc comment (api_op_UpdateClassificationJob.go:37-58) states CANCELLED is valid only from IDLE/PAUSED/RUNNING/USER_PAUSED, RUNNING only from USER_PAUSED, and USER_PAUSED only from IDLE/PAUSED/RUNNING -- now enforced, returning ConflictException (409) on a disallowed transition and ValidationException (400) on an unrecognized target status."}
  CreateMember: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Member had no Arn field (real GetMemberOutput always has 'arn'); added Arn, computed via arn.Build"}
  GetMember: {wire: fixed, errors: ok, state: ok, persist: ok, note: "json tag for MasteredBy was 'masteredBy'; real wire key is 'masterAccountId' -- fixed"}
  DeleteMember: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMembers: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "onlyAssociated/maxResults/nextToken query params now parsed and honored (real default onlyAssociated=true, hiding DISASSOCIATED members unless the client passes onlyAssociated=false); previously the handler ignored the query entirely and always called ListMembers(false), i.e. always showing every member regardless of association status. Now paginated via the same listPaginated/page.NewHMAC helper as ListAllowLists."}
  DisassociateMember: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateMemberSession: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateInvitations: {wire: ok, errors: ok, state: ok, persist: ok}
  AcceptInvitation: {wire: ok, errors: ok, state: ok, persist: ok}
  DeclineInvitations: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteInvitations: {wire: ok, errors: ok, state: ok, persist: ok}
  GetInvitationsCount: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListInvitations: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAdministratorAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateFromAdministratorAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  GetMasterAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateFromMasterAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableOrganizationAdminAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableOrganizationAdminAccount: {wire: fixed, errors: ok, state: ok, persist: ok, note: "query param was read as 'accountId'; real SDK sends 'adminAccountId' -- was always empty, so DisableOrganizationAdminAccount always 404'd for a real client"}
  ListOrganizationAdminAccounts: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeOrganizationConfiguration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "OrgConfig already had maxAccountLimitReached (added in a prior, undocumented change) -- this pass, deleted OrgConfig.DataSources/Features, two INVENTED fields (a 'dataSources' map and a 'features' array) that are NOT part of the real DescribeOrganizationConfigurationOutput/UpdateOrganizationConfigurationInput shapes (verified against both) and were entirely dead code (never written anywhere in the backend). Real output is exactly {autoEnable, maxAccountLimitReached}."}
  UpdateOrganizationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAutomatedDiscoveryConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAutomatedDiscoveryConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAutomatedDiscoveryAccounts: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchUpdateAutomatedDiscoveryAccounts: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeBuckets: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (constraint sweep): three bugs. (1) Criteria was read under keys (bucketName/region -> {\"value\": ...}) the real wire never carries at all -- real BucketCriteriaAdditionalProperties uses eq/neq/gt/gte/lt/lte/prefix (serializers.go:6840), so a real client's filters were always silently ignored regardless of content. Rewired to the real operator set for bucketName/accountId/region/sharedAccess/publicAccess.effectivePermission (string, eq/neq/prefix) and objectCount/sizeInBytes/classifiableObjectCount/classifiableSizeInBytes (int64, gt/gte/lt/lte); other documented properties (jobDetails.*, replicationDetails.*, objectCountByEncryptionType.*) have no backing model field and are left unfiltered. (2) maxResults/nextToken were never parsed at all -- every bucket always came back on one page. (3) sortCriteria was never parsed -- always hardcoded ascending by bucketName; now applies accountId/bucketName/classifiableObjectCount/classifiableSizeInBytes/objectCount/sizeInBytes (sensitivityScore is a documented AttributeName this backend has no score to sort by, left a no-op). Two existing tests (TestBuckets_DescribeBuckets_FilterByRegion, TestBuckets_DescribeBuckets_FilterByName) sent the old fabricated {\"value\": ...} shape and only passed because the handler shared their mistake -- corrected to eq/prefix."}
  GetBucketStatistics: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "route method was GET with accountId as a query param; real SDK sends POST /datasources/s3/statistics with accountId in the JSON body -- unreachable via real client before fix. accountId itself is still unused by the (intentionally global, single-account) stats aggregation. 2026-08-15 pass: response key 'classifiableBucketCount' does not exist on the real GetBucketStatisticsOutput at all (real key is 'classifiableObjectCount', a summed object count, not a bucket count) -- a real client's ClassifiableObjectCount was always 0. Also added 'objectCount'/'sizeInBytes' aggregate fields, summed from per-bucket S3BucketMetadata.ObjectCount/SizeInBytes the backend already tracks but never rolled up. 'lastUpdated'/'sizeInBytesCompressed'/'bucketStatisticsBySensitivity' remain unmodeled (no compression/sensitivity-scan tracking in this backend) -- disclosed, not fixed."}
  GetClassificationExportConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  PutClassificationExportConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GetClassificationScope: {wire: ok, errors: ok, state: ok, persist: ok}
  ListClassificationScopes: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateClassificationScope: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "2026-08-21 (gopherstack-c8ge): singleton with no Create op. Real UpdateClassificationScopeInput.S3 is types.S3ClassificationScopeUpdate{Excludes: *S3ClassificationScopeExclusionUpdate{BucketNames, Operation}} -- an explicit ADD/REMOVE/REPLACE discriminator, not a replacement list -- but the handler decoded S3 as the same freeform map[string]any Excludes used for Get/List and wholesale-replaced the stored value with whatever the request carried, so an ADD call silently dropped every bucket a prior ADD had added. Modeled ClassificationScopeS3Update/ClassificationScopeS3ExclusionUpdate distinct from the Get/List-side ClassificationScopeS3/ClassificationScopeS3Exclusion (now BucketNames []string, not a map) and implemented real ADD/REMOVE/REPLACE list semantics. See TestUpdateClassificationScope_ExcludedBucketsSurviveIndependentAdds."}
  GetFindingsPublicationConfiguration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-30 (gopherstack-4a8v, reqfieldscan anonymous-struct-decode pass): FindingsPublicationConfig fabricated top-level publishClassificationFindings/publishPolicyFindings members (no omitempty, so emitted on every response) -- confirmed against api_op_GetFindingsPublicationConfiguration.go/api_op_PutFindingsPublicationConfiguration.go and types.SecurityHubConfiguration that both real fields live ONLY nested under securityHubConfiguration; neither Input nor Output has a top-level member of either name. Removed the two fabricated fields; a pre-existing test (TestFindingsPublicationConfig/get_put_publication_config) asserted the fabricated top-level shape as correct and was fixed to assert the real nested shape plus their absence. ClientToken (real PutFindingsPublicationConfigurationInput member, idempotency-only, no member on Output) was being stored via the struct's whole-value copy and echoed back on a later Get; now explicitly discarded after decode, matching this codebase's existing accept-then-drop convention for idempotency tokens (see glue/handler_catalogs.go, inspector2/handler_connectors.go)."}
  PutFindingsPublicationConfiguration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "see GetFindingsPublicationConfiguration row -- same fix, same commit."}
  GetResourceProfile: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-15 pass: response key 'sensitivityScoreOverride' does not exist on the real GetResourceProfileOutput (real key is 'sensitivityScoreOverridden', past participle) -- a real client's SensitivityScoreOverridden was always false even after UpdateResourceProfile set a manual override. Also fixed ResourceStatistics's 'totalDetectionsWithoutSuppression'->'totalDetectionsSuppressed' and 'totalItemsSkippedPermissionError'->'totalItemsSkippedPermissionDenied' (real deserializers.go field names); ResourceStatistics is always the zero-value struct in this backend (nothing populates real numbers), so the value itself is currently unobservable -- key names fixed and disclosed as untested rather than given a hollow test. 'totalItemsSensitive' remains entirely unmodeled."}
  UpdateResourceProfile: {wire: ok, errors: ok, state: ok, persist: ok}
  ListResourceProfileArtifacts: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListResourceProfileDetections: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateResourceProfileDetections: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRevealConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateRevealConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSensitiveDataOccurrences: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetSensitiveDataOccurrencesAvailability: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetSensitivityInspectionTemplate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29: response ID field emitted wire key 'id' -- real GetSensitivityInspectionTemplateOutput uses 'sensitivityInspectionTemplateId' (distinct from the list-view SensitivityInspectionTemplatesEntry's 'id' key). See TestGetSensitivityInspectionTemplate_RealClient."}
  ListSensitivityInspectionTemplates: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateSensitivityInspectionTemplate: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "route method was PATCH; real SDK sends PUT /templates/sensitivity-inspections/{id} -- unreachable via real client before fix. 2026-08-21 (gopherstack-c8ge): singleton with no Create op. Real UpdateSensitivityInspectionTemplateInput carries Description/Excludes/Includes as independently-optional pointers, but the handler wholesale-assigned all three every call (Description as a bare string, indistinguishable omitted-vs-empty), so updating just one wiped the other two. Description is now decoded as *string and all three merge only when actually provided. See TestUpdateSensitivityInspectionTemplate_FieldsSurviveIndependentUpdates."}
  GetUsageStatistics: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetUsageTotals: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "query param was read as 'currencyCode' (not a real GetUsageTotalsInput field at all); real key is 'timeRange' -- fixed extraction/naming. Backend still ignores the value and returns static zeroed totals, matching a no-billing emulator; low functional impact."}
  ListManagedDataIdentifiers: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-13 (gopherstack-jqh2 pass 2): parseManagedDataIDsPath (handler_custom_data_identifiers.go) required http.MethodGet for POST /managed-data-identifiers/list -- confirmed against awsRestjson1_serializeOpListManagedDataIdentifiers, real SDK sends POST -- so the op, despite a complete handler and backend, was permanently unroutable by a real client. A pre-existing unit test (handler_usage_test.go) encoded the same wrong GET method and passed anyway (it drives h.Handler() directly); fixed to POST alongside the routing fix. Caught by the new handler_sdk_route_table_test.go (TestExtractOperation_SDKRouteTable, full 81/81 SDK-path coverage)."}
  SearchResources: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "FIXED 2026-08-30 (gopherstack-3qg6): backend now honors BucketCriteria (Includes/Excludes And[]{SimpleCriterion|TagCriterion}, real And-join per types.SearchResourcesCriteriaBlock's doc comment), SortCriteria (ACCOUNT_ID/RESOURCE_NAME/S3_CLASSIFIABLE_OBJECT_COUNT/S3_CLASSIFIABLE_SIZE_IN_BYTES, types.SearchResourcesSortAttributeName), and maxResults/nextToken (via the same pkgs/page-backed paginate() helper DescribeBuckets uses) -- see search_resources.go, a new criteria engine mirroring but distinct from DescribeBuckets' flat-map one (SearchResources' shape is Includes/Excludes blocks of AND'd SimpleCriterion|TagCriterion, not a flat per-property map). SimpleCriterion filters on ACCOUNT_ID/S3_BUCKET_NAME/S3_BUCKET_EFFECTIVE_PERMISSION/S3_BUCKET_SHARED_ACCESS (all real S3BucketMetadata fields); AUTOMATED_DISCOVERY_MONITORING_STATUS is a real key with no backing field on S3BucketMetadata (models.go) and is left unfiltered rather than invented, same convention bucketStringField already uses for unmodeled DescribeBuckets properties -- not fixed, see gaps. TagCriterion matches bkt.Tags entries by \"key\"/\"value\" (the real SDK's KeyValuePair wire casing, types/types.go:1764), which is the casing DescribeBuckets' own tags pass-through already implicitly commits to. MatchingBucket in the response emits only fields this backend tracks (accountId/bucketName/classifiableObjectCount/classifiableSizeInBytes/objectCount/sizeInBytes); automatedDiscoveryMonitoringStatus/errorCode/errorMessage/jobDetails/lastAutomatedDiscoveryTime/objectCountByEncryptionType/sensitivityScore/sizeInBytesCompressed/unclassifiableObjectCount/unclassifiableObjectSizeInBytes have no backing data (no error simulation, no per-bucket encryption breakdown, no sensitivity scan) and are omitted, not fabricated. Proven via TestSearchResources_FiltersByBucketCriteria/_ExcludesByBucketCriteria/_SortCriteria/_Pagination (search_resources_test.go), real aws-sdk-go-v2 client round trips against decoded responses, all confirmed failing pre-fix."}
# Families audited as a group (when per-op is impractical):
families:
  route_matcher: {status: fixed, note: "RouteMatcher path-prefix matching verified against all serializers.SplitURI() calls in the SDK; found 3 method mismatches (UpdateAllowList PATCH->PUT, UpdateSensitivityInspectionTemplate PATCH->PUT, GetBucketStatistics GET->POST) that made those ops unreachable via a real SDK client despite passing unit tests that called h.Handler() directly with the (wrong) method the handler itself expected."}
  tags: {status: fixed, note: "isKnownARN recognized allow-list/custom-data-identifier/findings-filter resource types but NOT classification-job, even though CreateClassificationJob computes/stores a real jobArn -- fixed this pass: isKnownARN now also recognizes classification-job/{id} ARNs, and CreateClassificationJob writes create-time tags into the shared tags map (same pattern as CreateAllowList/CreateCustomDataIdentifier) so ListTagsForResource sees them immediately."}
# gaps/deferred: empty. All 5 gaps and both deferred field audits from the
# 2026-07-12 pass were closed in the 2026-07-23 pass -- see the per-op notes
# above (GetCustomDataIdentifier, CreateClassificationJob/DescribeClassification
# Job/ListClassificationJobs/UpdateClassificationJob, ListMembers,
# DescribeOrganizationConfiguration, GetFindings/CreateSampleFindings) and the
# tags family note. Nothing reclassified to ok without a real field-diff.
gaps: []
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; all state is coarse-lock-guarded maps/tables behind lockmetrics.RWMutex, reset via registry.ResetAll(). Every backend method that took a lock this pass (CreateClassificationJob, DescribeClassificationJob, ListClassificationJobs, UpdateClassificationJob, ListMembers, GetCustomDataIdentifier, BatchGetCustomDataIdentifiers) releases it via defer; no classification-job-runner goroutine/ticker exists (jobs never actually execute in this emulator, so there is nothing to Shutdown-drain). FIXED (gopherstack-cq0z, 2026-09-06): DeleteAllowList and DeleteFindingsFilter never cleared their entry in the tags map. isKnownARN gates TagResource/ListTagsForResource by resource existence, so the leak is not reachable through those ops post-delete; it is persisted verbatim in Snapshot() regardless, so it grows the persisted file without bound on create/delete churn. Now cleared in both delete paths. DeleteCustomDataIdentifier is unaffected: it soft-deletes (Deleted flag, entry retained), so its tags entry is intentionally kept, matching AWS's own reference-retention behavior for custom data identifiers. See TestMacie2_Delete_ClearsTags."}
---

## Notes

- macie2 is restjson1. Verified every op's (method, path) pair against
  aws-sdk-go-v2/service/macie2@v1.51.4's `serializers.go` (grepped every
  `httpbinding.SplitURI(...)` + following `request.Method = ...` pair) --
  this is the authoritative source of truth for route matching, not the
  handler's own parseRESTPath tables (which had drifted from it in 3 places,
  see route_matcher family note above).
- Wire wrapper keys: verified handler response envelope keys against every
  `deserializers.go` `awsRestjson1_deserializeOpDocument<Op>Output` `case "key":`
  block for the high-traffic families (session, allow lists, custom data
  identifiers, findings filters, findings, tags, classification jobs,
  members, buckets, usage). Most wrapper keys already matched; the two
  breaking mismatches were BatchGetCustomDataIdentifiers ("items" vs real
  "customDataIdentifiers") and the two Arn-omission bugs (Member, ClassificationJob).
- GetMacieSession/UpdateMacieSession timestamps are ISO8601 strings
  (`__timestampIso8601` in the SDK, parsed via `smithytime.ParseDateTime`),
  NOT epoch-seconds -- this service does not need `pkgs/awstime.Epoch`.
  `time.RFC3339` is wire-compatible; verified, not a bug.
- GetUsageTotals real query param is `timeRange` (values like
  `MONTH_TO_DATE`/`PAST_30_DAYS`); there is no `currencyCode` input field in
  the real API at all -- the emulator's static zeroed UsageTotal response
  (all `"USD"`, `"0"`) already matches real "no usage data" behavior, so this
  was purely a dead/misnamed query-param-extraction bug, not a data bug.
- Member.Arn wire value follows the real "account-root-style" macie2 ARN
  convention: `arn:PARTITION:macie2:REGION:MEMBER_ACCOUNT_ID:` (no resource
  part, trailing colon) via `arn.Build("macie2", region, accountID, "")`.
  Classification job ARNs use the `classification-job/{jobId}` resource
  suffix, matching the allow-list/custom-data-identifier/findings-filter
  convention already used elsewhere in this backend.
- CreateClassificationJobOutput/JobSummary do NOT have a `jobStatus` field on
  create (only DescribeClassificationJobOutput does); the handler's existing
  `"jobStatus": "RUNNING"` in the CreateClassificationJob response is extra
  (client-ignored) data, not wrong per se -- left as-is rather than removing,
  since removing it risks nothing and adding jobArn was the actual bug.

## 2026-07-23 pass notes (closed all 5 gaps + both deferred field audits)

- **Deleted an invented enum value**: `Finding.Category` was hardcoded to
  `"SENSITIVE_DATA"` in `CreateSampleFindings`, which is not a member of the
  real `FindingCategory` enum (`CLASSIFICATION` / `POLICY` are the only two
  values, per `types/enums.go`). Fixed to derive `CLASSIFICATION` vs
  `POLICY` from the finding type's `"Policy:"` prefix, matching how real
  Macie categorizes findings.
- **Deleted invented dead fields**: `OrgConfig.DataSources`
  (`map[string]any`) and `OrgConfig.Features` (`[]map[string]any`) do not
  exist anywhere in `DescribeOrganizationConfigurationOutput` or
  `UpdateOrganizationConfigurationInput`, and were never written by any
  handler -- pure dead invented surface. Deleted; `OrgConfig` is now exactly
  the real two-field shape.
- **Real behavior-changing fix, not just a field addition**:
  `GetCustomDataIdentifier`/`BatchGetCustomDataIdentifiers` on a
  soft-deleted identifier used to 404. Real Macie never hard-deletes a
  custom data identifier (`DeleteCustomDataIdentifier` docs: "Amazon Macie
  doesn't delete it permanently... it soft deletes the identifier"), and the
  real output shape's nullable `Deleted *bool` field only makes sense if Get
  can still succeed post-delete. Fixed: Get/BatchGet now always succeed for
  a real ID (deleted or not) and report `deleted:true`/`false`; only a
  never-existed ID 404s. Two existing tests
  (`GetCustomDataIdentifier on deleted CDI returns 404`,
  `TestCustomDataIdentifierNoDeletedField`) encoded the old, incorrect
  behavior and were rewritten to assert the corrected behavior
  (`TestCustomDataIdentifierDeletedField`).
- `types.Severity.Score` is `*int64` (values 1-3: Low/Medium/High) in the
  real SDK, not an arbitrary float -- `defaultFindingScore` was `5.0`
  (out of range). Fixed `Severity.Score` to `int64` and the default to `2`
  (Medium).
- `ListJobsFilterCriteria`/`ListMembersInput.OnlyAssociated` wire shapes
  verified against `serializers.go`
  (`awsRestjson1_serializeDocumentListJobsFilterCriteria`/`Term`,
  `onlyAssociated` query param) and `deserializers.go`
  (`awsRestjson1_deserializeDocumentJobSummary`) -- `createdAt` on
  `JobSummary` is ISO8601 (`smithytime.ParseDateTime`), same as
  `DescribeClassificationJob`, not epoch-seconds.
- Classification-job runtime fields (`lastRunErrorStatus`, `statistics`,
  `userPausedDetails`) are populated with realistic-but-static values since
  this emulator has no job-execution engine: `lastRunErrorStatus` is always
  `{code: NONE}`, `statistics` is always `{numberOfRuns: 1,
  approximateNumberOfObjectsToProcess: 0}`. This is an intentional,
  documented reduced-scope emulation (jobs never actually run), not a stub --
  every field that CAN be driven by real request/state data (allowListIds,
  customDataIdentifierIds, managedDataIdentifierIds,
  managedDataIdentifierSelector, userPausedDetails' pause/expiry timestamps)
  is.
## 2026-08-15 pass notes (gopherstack-6flj wrapper-key/nested-shape sweep)

Full layer-1+2 sweep of all 40 List/Describe/Get ops (the L+D+G surface
tracked by gopherstack-6flj) against `macie2@v1.54.4` deserializers.go,
one op at a time, plus a check of every Create/Update op whose response or
request shares a type with a List/Get op. Protocol: restjson1,
case-sensitive (confirmed via the sole `awsRestjson1_` deserializer prefix
and a spot-check that all 503 `EqualFold` hits in `deserializers.go` are
`errorCode` header/query matching, not body-field casing). Dead-deserializer
trap checked and does NOT apply (`HandleDeserialize` calls
`awsRestjson1_deserializeOpDocument<Op>Output` directly for every op
spot-checked, e.g. `ListFindings`, `GetBucketStatistics`).

2 real bugs found and fixed, both silent-wrong-key (correct outer shape,
wrong scalar key name) rather than missing wrapper keys -- see the
`GetBucketStatistics`/`GetResourceProfile` op notes above for detail and
citations. Both are values the backend already tracked (per-bucket
ObjectCount/SizeInBytes; the resource-profile override flag) that either
never reached the wire or reached it under a name no real client's field
would ever match.

Sibling-trap check: `GetAdministratorAccount`/`GetMasterAccount` both wrap
the real `Invitation` type, whose `relationshipStatus` field name IS
correct for macie2 (unlike securityhub's analogous
`GetAdministratorAccount`/`GetMasterAccount`, which wrap a different type
using `MemberStatus` -- confirmed as two genuinely different real shapes,
not the same sibling trap recurring here). No other version/generational
pairs exist in this service (no V1/V2 op families).

3 ratifying tests found and fixed, all "wrong key/value asserted as
correct": `handler_buckets_test.go` (4 assertion sites across 3 tests
using the pre-fix `classifiableBucketCount` key/semantic) and
`handler_resource_profiles_test.go` (1 assertion site using the pre-fix
`sensitivityScoreOverride` response key). Zero found in the
too-weak-to-fail shape.

Phantom ops: none (all 96 op consts have a real `api_op_*.go`, cross-checked
during the sweep). False-positive rate: 0 -- every finding cites the real
`deserializeOpDocument<Type>`/`deserializeDocument<Type>` function reached
from `HandleDeserialize`, file+line, or the real `api_op_*.go` struct
definition for fields never reached by the generated switch (e.g.
`AllowListSummary` has no `tags` member at all).

Harmless-extra-field non-bugs confirmed (real client ignores unknown JSON
keys, so these are not fixed): `AllowListSummary.tags`,
`FindingsFilterListItem`'s extra `description`/`position`,
`Member.updatedAt`, `CreateClassificationJobOutput`'s extra `jobStatus`,
`AutomatedDiscoveryAccount`'s extra `email`, `GetResourceProfile`'s extra
`resourceArn`. Structural/unmodeled gaps disclosed, not fixed (would need
new backend simulation, not a key-name fix): `Finding.policyDetails`,
`ClassificationDetails.detailedResultsLocation`,
`AffectedS3Bucket`/`AffectedS3Object`'s many real-but-untracked fields
(versioning, encryption detail, sensitivity score, ...),
`GetAutomatedDiscoveryConfiguration`'s
`classificationScopeId`/`disabledAt`/`firstEnabledAt`/`lastUpdatedAt`/
`sensitivityInspectionTemplateId`, `ResourceStatistics.totalItemsSensitive`,
`ListResourceProfileArtifacts`'s always-empty result (no artifact
classification simulated) and its item shape's missing
`classificationResultStatus`/extra `type`.

Every fix hand-reverted individually (no git, per this session's hard
no-git-mutation constraint), confirmed to fail with the exact predicted
symptom against a real SDK client, then restored and diffed byte-identical.
2 new real-`aws-sdk-go-v2`-client tests added in
`services/macie2/wire_field_fixes_test.go`
(`TestGetBucketStatistics_RealClient`,
`TestUpdateResourceProfile_SensitivityScoreOverridden_RealClient`).

Gates (scoped `go build`/`go vet`/`go test -race`/`go fix -diff`/
`golangci-lint run`, 0 issues, no cyclop/gocyclo/gocognit/funlen nolints)
green for `services/macie2`; `go test -race ./pkgs/...` green.

- `PolicyDetails` (the policy-finding counterpart to `ClassificationDetails`)
  was intentionally left unimplemented: `CreateSampleFindings` now correctly
  categorizes `"Policy:"`-prefixed findings as `POLICY` and gives them a
  bucket-only `resourcesAffected` (no `s3Object`, matching policy findings
  being bucket-level), but does not synthesize a `policyDetails` value. If a
  future pass needs it, `PolicyDetails`/`FindingAction`/`FindingActor` in
  `types/types.go` are the reference shapes.

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, unbumped retype

`macie2SnapshotVersion` bumped 1 -> 2. `1217df451` retagged `ResourceProfile.
SensitivityScoreOverride` (the registered `resourceProfiles` table's value type) to the
real deserializer's `sensitivityScoreOverridden`, without bumping the snapshot version.
`UpdateResourceProfile` genuinely sets this flag, so it is real, backend-controlled state;
a pre-fix (v1) snapshot's `"sensitivityScoreOverride": true` no longer matches the new key
at all and silently decodes as `false` on restore -- not an error, a quiet loss of a real
user-triggered flag.

The sibling rename in the same commit, `ResourceStatistics`'s two field renames
(`TotalDetectionsWithoutSuppression`->`TotalDetectionsSuppressed`,
`TotalItemsSkippedPermissionError`->`TotalItemsSkippedPermissionDenied`), is not a
compatibility concern: the commit's own doc comment discloses that struct is never
populated with real data by this backend (`GetResourceProfileOutput.Statistics` always
decodes/encodes as the zero-value struct), so no user data was ever stored under either
name. Two further candidates examined and disqualified the same way: `17237c95e`'s
`Severity.Score` float64->int64 retype is compatible in practice -- the only value this
backend ever sets is the constant `5.0`, which `encoding/json` renders as the bare number
`5`, so old data decodes into the new int64 field without error (verified with a standalone
round-trip); and `c37164f25`'s `ClassificationScopeS3.Excludes` map->struct retype keeps the
same `bucketNames` wire key, so the meaningful data survives the decode (the map's other key,
the `operation` discriminator from the pre-fix wholesale-replace bug this same commit fixed,
is silently dropped, but it never represented durable state a user would notice losing).

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration.

**Proof:** `TestInMemoryBackend_RestoreV1SensitivityScoreOverrideDiscarded`
(persistence_test.go) builds a v1-shaped `resourceProfiles` snapshot with
`sensitivityScoreOverride: true` and asserts `GetResourceProfile` synthesizes a fresh
default (override `false`) after restore, not a silently-decoded record with the override
lost but the score retained. Hand-reverted to version 1: the same test then shows exactly
that split symptom -- `SensitivityScore` correctly restores as `50` while
`SensitivityScoreOverridden` silently decodes `false` -- confirming the predicted failure;
restored and `md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).

## 2026-08-29 write-only-state sweep (gopherstack-6flj / gopherstack-21my)

Forward+reverse write-only-state sweep of every backend file (administrator, allow_lists,
automated_discovery, buckets, classification_jobs, custom_data_identifiers, enablement,
findings, findings_filters, members, organization, resource_profiles, reveal_configuration,
sensitivity_inspection, tags, usage) against `macie2@v1.54.4`, on top of the already-thorough
2026-08-15 wrapper-key/nesting sweep and 2026-08-21 dropped-field sweep. One new bug found:

- **`GetSensitivityInspectionTemplate`** emitted the template ID under wire key `"id"`.
  `GetSensitivityInspectionTemplateOutput`'s deserializer
  (`deserializers.go:7839`, function
  `awsRestjson1_deserializeOpDocumentGetSensitivityInspectionTemplateOutput`) reads it under
  `"sensitivityInspectionTemplateId"` instead -- a genuinely different key from the one the
  list-view `SensitivityInspectionTemplatesEntry` shape uses (`"id"`,
  `deserializers.go:21230`, feeding `ListSensitivityInspectionTemplatesOutput`). A real
  client's `GetSensitivityInspectionTemplateOutput.SensitivityInspectionTemplateId` was
  always `nil` regardless of the template's actual ID. Fixed by retagging
  `SensitivityInspectionTemplate.ID`'s json tag (that Go type backs only the Get response;
  the List response uses the separately-tagged `SensitivityInspectionTemplateSummary`, which
  was already correct). One ratifying test fixed
  (`handler_sensitivity_inspection_test.go`'s `TestSensitivityInspectionTemplates` asserted
  `getResp["id"]`, the pre-fix wrong key, as correct). New real-client round-trip test:
  `TestGetSensitivityInspectionTemplate_RealClient` (`wire_field_fixes_test.go`) -- confirmed
  failing against unmodified code (`SensitivityInspectionTemplateId` decoded empty instead of
  the real ID) before the fix, passing after.

Fields checked and confirmed readable/computable (not write-only): `ClassificationJob`'s
`ClientToken` (stored, surfaces on `DescribeClassificationJobOutput`, matching the real
shape); `ResourceProfileDetection.Suppressed` (set by `UpdateResourceProfileDetections`, read
by `ListResourceProfileDetections`); `AllowListCriteria`/`Regex`/`S3WordsList` (round-trip
through Create/Get/Update unchanged); `ClassificationScope` ADD/REMOVE/REPLACE merge (already
fixed 2026-08-21, re-verified clean); `GetFindingStatistics`'s four `groupBy` keys all compute
from stored `Finding` fields, not a stub.

Observed, not fixed (reduced-scope/structural, matches existing PARITY.md disclosures):
`CreateClassificationJob`'s `ClientToken` is not used for idempotent dedup (a repeat call
with the same token creates a second job) -- this is request-level idempotency semantics, not
a wire-shape bug, and no other op in this service enforces client-token dedup either;
`GetUsageStatistics`/`GetUsageTotals` remain all-zero/empty (no billing engine, already
disclosed); `UpdateSensitivityInspectionTemplate`'s handler additionally accepts an
undocumented `name` field with no effect on any real client (the real
`UpdateSensitivityInspectionTemplateInput` has no `Name` member at all) -- harmless
extra-acceptance, not a drop.

**Tool output:** `enumcheck` flagged `GetSensitiveDataOccurrences`'s `"status": "SUCCESS"` as
not matching any single candidate enum family; confirmed against
`types.RevealRequestStatus.Values()` (`SUCCESS`/`PROCESSING`/`ERROR`) that `SUCCESS` is valid
-- false positive, the tool just can't disambiguate which of several same-named-field enums
applies. `acceptguard`/`zeroguard`/`xmlitemwrap` had zero macie2 findings.

**Not reached this pass:** `store.go`/`store_setup.go`/`persistence.go`/`provider.go` (read
only incidentally, not independently audited this session); `handler_buckets.go`,
`handler_administrator.go`, `handler_members.go`, `handler_organization.go`,
`handler_usage.go`, `handler_tags.go` handler-layer files were not re-read line-by-line this
pass (their backends were, in `buckets.go`/`administrator.go`/`members.go`/`organization.go`/
`usage.go`/`tags.go`, and matched their existing `ok` PARITY rows with no new findings).

**Gates:** `go build ./services/macie2/...`, `go vet ./services/macie2/...`,
`go test -race -count=1 ./services/macie2/...` (pass), `golangci-lint run --fix
./services/macie2/...` (0 issues, reformatted one test call's line wrap only).

## 2026-08-30: paginated-listing reproducibility sweep (unstable page-boundary drop)

Targeted class: every `List*`/`DescribeBuckets` op routed through the shared
`listPaginated`/`mapSortPaginate`/`paginate` helpers (`store.go`) -- an offset-based
`page.NewHMAC` cursor over a `*store.Table` map walk re-sorted fresh on every call. Read
all 15 `sort.Slice` sites in the service.

**Found and fixed, 6 sites** (all: sort by a non-unique attribute with no tiebreak, fed
into an offset cursor over an unstable map-walk source):
- `sortBuckets` (`buckets.go`, feeds `DescribeBuckets`) -- default and every
  `sortCriteria.attributeName` branch (`accountId`/`objectCount`/`sizeInBytes`/
  `classifiable*`/`bucketName`) compared only the requested attribute; nothing stops two
  buckets sharing an `ObjectCount` (or any of the others). Fixed: tiebreak on `BucketArn`,
  this table's key.
- `sortJobSummaries` (`classification_jobs.go`, feeds `ListClassificationJobs`) -- default
  and every `sortBy.AttributeName` branch (`createdAt`/`jobStatus`/`name`/`jobType`)
  likewise untied; job names have no uniqueness constraint. Fixed: tiebreak on `JobID`.
- `ListCustomDataIdentifiers` (`custom_data_identifiers.go`) -- sorted by `Name` alone;
  `CreateCustomDataIdentifier` never checks for an existing `Name`. Fixed: tiebreak on `ID`.
- `ListAllowLists` (`allow_lists.go`) -- same shape, `CreateAllowList` never checks `Name`
  either. Fixed: tiebreak on `ID`.
- `ListFindingsFilters` (`findings_filters.go`) -- sorted by `Position`, which
  `CreateFindingsFilter` defaults to `1` for every filter that doesn't specify one, so
  multiple filters commonly tie by default. Fixed: tiebreak on `ID`.
- `sortFindings`'s `sortBy != nil` branch (`findings.go`, feeds `ListFindings`) --
  `count`/`createdAt`/`updatedAt`/`type`/`severity.score` all untied (the `sortBy == nil`
  default path was already safe, sorting by `ID`). Fixed: tiebreak on `ID`.

Each proven with a dedicated test in the new `pagination_tie_test.go`
(`TestDescribeBuckets_TiedObjectCount_NoDropOrDupAcrossPages`,
`TestListClassificationJobs_TiedName_NoDropOrDupAcrossPages`,
`TestListCustomDataIdentifiers_TiedName_NoDropOrDupAcrossPages`,
`TestListAllowLists_TiedName_NoDropOrDupAcrossPages`,
`TestListFindingsFilters_TiedPosition_NoDropOrDupAcrossPages`,
`TestListFindings_TiedType_NoDropOrDupAcrossPages`), each looped 30x (map-iteration
dependent) -- all six confirmed failing against unmodified code (a genuine subset
dropped, not a test artifact -- verified via the actual diff output before fixing), all
six passing after. The `ListFindingsFilters`/`ListAllowLists` fixes made their two
wrapper functions structurally identical enough to trip `dupl`; resolved with a paired
`//nolint:dupl` (precedented 145x elsewhere in the repo, e.g.
`services/backup/copy_jobs.go`/`backup_jobs.go`), not by weakening either fix.

**Immune by construction, two mechanisms**: `ListClassificationScopes`
(`classification_jobs.go:361`, sorts by `Name`) and `ListSensitivityInspectionTemplates`
(`sensitivity_inspection.go:41`, sorts by `Name`) both back onto tables that
`ensureDefaultScope`/`ensureDefaultTemplate` populate with exactly one row and nothing
else ever calls `.Put` on -- confirmed via `grep -n "classScopes.Put\|sensitivityTemplates.Put"`,
one hit each, both inside the guarded singleton-seed function. A one-row collection can't
have a page boundary. `ListManagedDataIdentifiers` (`custom_data_identifiers.go`) returns
a hardcoded, deterministic built-in catalog slice, never a map walk -- same shape as
medialive's `offerings` catalog, immune. The `GroupKey` sort in `GetFindingStatistics`
(`findings.go:314`) is built from a local Go map's own keys (`counts[key]++`, then
`result = append(result, {GroupKey: k, ...})` for each `k`) -- unique by construction, no
tiebreak needed.

**Confirmed ignoring pagination entirely** (a different, disclosed completeness gap, not
this pass's target): `ListAutomatedDiscoveryAccounts`, `ListOrganizationAdminAccounts`,
`ListInvitations`, `ListResourceProfileArtifacts`, `ListResourceProfileDetections`,
`ListTagsForResource` accept no `limit`/`token` at all and always return everything
unbounded -- can't drop a record at a page boundary that never truncates. Left as-is.

**Test-suite gap this pass filled**: `TestBuckets_DescribeBuckets_SortOrder` and
`TestBuckets_StableSortOrder` (`handler_buckets_test.go`) only ever used distinct bucket
names and repeated an unpaginated call -- no existing test in the service constructed a
tie or compared item identity across a paginated walk before this pass.

Gate output (this pass, `services/macie2/` only): `go build ./services/macie2/...` clean;
`go vet ./services/macie2/...` clean; `go test ./services/macie2/... -race -count=1` --
`ok`; `golangci-lint run ./services/macie2/...` -- `0 issues.` (one `dupl` finding caused
by this pass's own edits, confirmed by temporarily reverting `allow_lists.go`/
`findings_filters.go` and re-running lint clean, then resolved as described above).

## enumcheck confident-tier fix (2026-08-30)

`cmd/enumcheck`'s CONFIDENT tier flagged three `RelationshipStatus` literals
in `members.go` as not members of `types.RelationshipStatus`. Real
`RelationshipStatus` is mixed-case (`Enabled`/`Paused`/`Invited`/`Created`/
`Removed`/`Resigned`/... -- macie2@v1.54.4 types/enums.go:811), unlike this
service's other status-shaped fields (`MacieStatus`, `RevealStatus`), which
really are all-caps `ENABLED`/`PAUSED`/`DISABLED`. All three were genuine
value bugs:

- `CreateMember`: `"CREATED"` -> `"Created"`.
- `CreateInvitations`: `"INVITED"` -> `"Invited"`.
- `AcceptInvitation`: reused the shared `statusEnabled` constant
  (`"ENABLED"`, correct for `MacieStatus`/`RevealStatus`) for this
  `RelationshipStatus` field too -- switched to a literal `"Enabled"` at
  this one call site rather than changing the shared constant, which is
  still correct everywhere else it's used.

`GetInvitationsCount`'s own `inv.RelationshipStatus == "INVITED"` comparison
had to be updated to `"Invited"` in the same pass -- it filters the same
`Invitation.RelationshipStatus` field `CreateInvitations` now sets, so the
literal-value fix alone would have silently broken invitation counting.

**Left unfixed, out of scope for the confident tier** (both are direct field
mutations on an existing struct, not one of the three literal/composite
positions `cmd/enumcheck` covers, so the tool never flagged them):
`DisassociateMember` sets `RelationshipStatus = "DISASSOCIATED"`, and
`DeclineInvitations` sets `"RESIGNED"` -- neither is a real
`RelationshipStatus` member either (real values are `Removed` and
`Resigned` respectively). Flagged here for a future pass.

Covered by `TestCreateMember_RelationshipStatus_RealClient`,
`TestCreateInvitations_RelationshipStatus_RealClient`, and
`TestAcceptInvitation_RelationshipStatus_RealClient` (all in
`wire_field_fixes_test.go`), each driven through the real SDK client and
asserted against the real `types.RelationshipStatus` constants.

## reqfieldscan anonymous-struct-decode pass (2026-08-30, bd gopherstack-4a8v)

`cmd/reqfieldscan`'s new anonymous-inline-struct decode path (see
`handler_findings.go`'s `var req struct{...}` shapes) surfaced 7 previously
invisible unread-request-field flags. Hand-verified each against
`macie2@v1.54.4`'s own serializers:

- **Real bug, fixed**: `FindingsPublicationConfig.PublishClassificationFindings`/
  `PublishPolicyFindings` (`models.go`) were fabricated top-level fields --
  neither `PutFindingsPublicationConfigurationInput` nor
  `GetFindingsPublicationConfigurationOutput` has a member of either name;
  both real fields live only nested under `SecurityHubConfiguration`
  (`types.SecurityHubConfiguration`). Both booleans lacked `omitempty`, so
  every `Get`/`Put` response carried two keys no real client ever sends.
  Removed. See the `GetFindingsPublicationConfiguration`/
  `PutFindingsPublicationConfiguration` rows above for the full note.
- **Real bug, fixed**: `FindingsPublicationConfig.ClientToken` was stored via
  the handler's whole-struct copy and echoed back on a later `Get`, even
  though `GetFindingsPublicationConfigurationOutput` has no such member.
  Now explicitly discarded post-decode.
- **Tool false positive (whole-struct-copy shape)**:
  `FindingsPublicationConfig.SecurityHubConfiguration` reads as unread
  because `handlePutFindingsPublicationConfiguration`/
  `PutFindingsPublicationConfiguration` thread it through via `cp := *cfg`
  struct-copy assignments, never a per-field selector -- functionally
  correct and observable via `Get`, just invisible to the tool's
  whole-struct-*conversion* (`SomeType(x)` call-expression) suppression
  rule, which does not recognize a dereference-assignment copy.
- **Honest gap, matches this codebase's established idempotency-token
  convention** (see `glue/handler_catalogs.go`'s
  `putDataCatalogExportConfigurationInput`, `inspector2/handler_connectors.go`'s
  `createConnectorRequest`): `CreateAllowList.ClientToken`
  (`handler_allow_lists.go`) and `CreateFindingsFilter.ClientToken`
  (`handler_findings_filters.go`) are accepted, never stored, never echoed
  -- an idempotency-retry aid with no backend dedup window to honor. Neither
  response type has a field to echo it into either.
- **Honest gap, already documented above** (`GetUsageStatistics` row, "no
  billing engine"): `GetUsageStatistics.SortBy` (`handler_usage.go`) is
  dropped along with `FilterBy`/`MaxResults`/`NextToken` -- the backend
  returns an unconditionally empty `[]UsageRecord{}`, so there is nothing
  for any of these to filter or sort.

No other findings in this slice; `go build`/`go vet`/`go test -race
-count=1 ./services/macie2/...`/`golangci-lint run ./services/macie2/...`
all clean after the fix.

## errcodeaudit fabricated-error-code pass (2026-08-30, bd gopherstack-r3pr)

`cmd/errcodeaudit` flagged 2 confident findings.

- **Real bug, fixed**: `handler.go`'s `RESTRouter.BadRequestBody` (fires
  only on a request-body *read* failure, e.g. a body over
  `httputils.MaxRequestBodyBytes`, before any operation is dispatched)
  wrote `"BadRequestException"`, a code `macie2@v1.54.4`'s SDK models
  nowhere (its 8 exception types are AccessDenied/Conflict/
  InternalServer/ResourceNotFound/ServiceQuotaExceeded/Throttling/
  UnprocessableEntity/Validation) -- a real client's
  `errors.As(*types.ValidationException)` could never match it. Switched
  to the existing `errValidation` ("ValidationException") constant already
  used elsewhere in this package; `types.ValidationException`'s own doc
  ("an error that occurred due to a syntax error in a request") is the
  right fit. See `TestCreateAllowList_RealClient_OversizedBody`
  (`error_codes_fix_test.go`), which drives the real SDK client with an
  oversized `CreateAllowList` body and confirmed failing pre-fix.
- **Tool false positive (free-form field, not a wire error code)**:
  `classification_jobs.go:60`'s `JobLastRunErrorStatus{Code: "NONE"}` is a
  status field inside a classification-job resource returned by a
  *successful* `Create`/`Describe`/`List` response, not a wire error
  envelope -- no `errors.As` ground truth applies. Same class as
  `glue/jobs.go:471`, `ce/cost_allocation_tags.go:64`,
  `xray/handler_trace_segments.go:43` (bd gopherstack-r3pr).
