---
service: glue
sdk_module: aws-sdk-go-v2/service/glue@v1.152.0
last_audit_commit: a7f9c5fb2  # gopherstack-uult (2026-08-13) fixed after this hash was recorded; hash not yet known at edit time
last_audit_date: 2026-08-13
# 2026-08-30 wrapper-key/sort-totality sweep (Class F: a sort that exists but is
# not total). Swept every sort.Slice/sort.Strings/slices.Sort* call site across
# this service's ~48 paginated listings for whether the sort key is unique.
# 7 genuine bugs found and fixed, all sharing the same shape -- a field that
# admits ties, re-sorted fresh from unordered store.All()/map storage on every
# call via an unstable sort, so two honest calls can disagree about the
# relative order of tied items and a record is dropped or duplicated across a
# page boundary with nothing else changed:
#   - GetBlueprintRuns (blueprints.go), ListColumnStatisticsTaskRuns
#     (column_statistics.go), ListDataQualityRuleRecommendationRuns
#     (data_quality_rulesets.go), ListMaterializedViewRefreshTaskRuns
#     (materialized_views.go), ListDataQualityEvaluationRuns
#     (data_quality_stats.go) all sorted solely on StartedOn, a
#     float64(time.Now().Unix()) value truncated to whole seconds -- any two
#     runs started within the same wall-clock second tie. Fixed by adding each
#     type's own real unique ID (RunID/ColumnStatisticsTaskRunID/
#     RecommendationRunID/TaskRunID/RunID respectively) as the final
#     comparator term.
#   - GetMLTransforms/ListMLTransforms (ml.go) sorted solely on Name; real AWS
#     MLTransform.Name is not unique (only TransformId is -- confirmed against
#     glue@v1.152.0's CreateMLTransform, which has no name-uniqueness
#     constraint). Fixed by appending TransformID as the tiebreak; this also
#     makes handler_ml.go's user-supplied-Sort path (sortTransforms, a stable
#     sort applied on top of this base order) total for STATUS/CREATED/
#     LAST_MODIFIED, none of which are unique either.
#   - SearchAssets' sortAssets (assets.go) let the caller pick the sort
#     attribute (Name/Description/AssetTypeId/CreatedAt/UpdatedAt), none of
#     which is unique across assets -- only Id is (already used as the
#     fallback for an unrecognized/empty attr, but not appended as a tiebreak
#     for the 5 named cases). Fixed by falling through to ID in every case.
# Each fix proven by a dedicated test (pagination_sort_totality_test.go) that
# creates several tied-key items, walks paginateSlice's own offset-token
# semantics repeatedly (Go's map iteration order is randomized per range, so
# repeated calls surface the instability), and asserts the concatenated ID
# set is exact -- confirmed to fail on iteration 0 against the pre-fix code
# for all 7, confirmed green post-fix across 30 iterations each.
# Also swept for Class G (two-or-more collections the API defines as one
# ordered sequence, truncated independently): none found in this service --
# every paginated response found carries exactly one truncated collection;
# no delimiter/common-prefix-style dual-list op exists here.
# Remaining sort sites reviewed and confirmed already total (unique key, no
# fix needed): every other sort.Slice/sort.Strings call in this service sorts
# on a field that is that resource's real primary key (ID/ARN/Name-as-primary-
# key/composite key) -- e.g. UsageProfile/CustomEntityType/Integration/
# SecurityConfiguration/Schema-within-Registry Name, FunctionName (scoped per
# database), CatalogID, VersionID, IndexName, ItemID -- confirmed against each
# type's own store.Table key function, not assumed from the field name alone.
# 2026-08-21 gopherstack-r80d batch 15 (required-output cut): 6 required-response-
# member bugs found and fixed at member granularity across three families --
# Catalog.Name (CreateCatalog read the name off a nonexistent CatalogInput.Name;
# the real top-level Name was never read at all), GrokClassifier.Classification/
# GrokPattern, XMLClassifier.Classification, JsonClassifier.JsonPath, and
# ColumnStatistics.ColumnType (all tagged omitempty on a required member reachably
# empty from a real client). See the dated catalogs/classifiers/column_statistics
# entries below for full detail and SDK file:line citations. 4 more findings fixed
# but NOT counted (ColumnStatistics.AnalyzedTime, integrationSummary.CreateTime,
# CustomEntityType.RegexString, EncryptionAtRest.CatalogEncryptionMode) -- all
# unreachable given this backend's own server-side computation/validation or the
# real SDK client's own non-nil-string-length validator, see the
# column_statistics/catalogs entries below.
# 2026-08-30 gopherstack-6nr4 follow-up: GetMLTaskRuns, flagged and left
# unfixed by the sweep above for budget, is now fixed. It's the deeper of the
# two variants that sweep named: not just a missing sort tiebreak but a
# request struct that declared no Filter/Sort/MaxResults/NextToken at all
# (its sibling GetMLTransforms, same file, already had all four). Confirmed
# against the pinned SDK first per the issue's own instruction, not copied
# from the sibling -- api_op_GetMLTaskRuns.go's real Input/Output shapes
# matched what GetMLTransforms already modeled closely enough (Filter/Sort/
# MaxResults/NextToken request side, NextToken added response side) that the
# same paginateSlice helper applies. See the GetMLTaskRuns op row below for
# the full fix and test detail.
overall: A            # gopherstack-q4qt (this pass): ListSchemas/ListSchemaVersions declared MaxResults/NextToken (glue@v1.152.0 api_op_ListSchemas.go / api_op_ListSchemaVersions.go) but honored neither -- both were outside gopherstack-awzv's empty-struct-input sweep because they already took a real RegistryId/SchemaId, so they were never wired; fixed via the existing paginateSlice helper, matching every other List op in this file, with new local defaultListSchemasLimit/defaultListSchemaVersionsLimit consts (25, per each op's own doc comment) matching ListRegistries' convention. Read the whole of both ops per gopherstack-7f5k's pattern of paired bugs: ListSchemas.RegistryId was checked and confirmed already applied as a real filter (registry.go's ListSchemas: `registryName == "" || s.RegistryName == registryName`), not a repeat of DescribeInboundIntegrations/GetColumnStatisticsTaskRuns's ignored-scoping-parameter bug -- new test proves it excludes a sibling registry's schema. ListSchemaVersions takes SchemaId (SchemaName+RegistryName), inherently scoped to one schema, so there was no separate filter gap to find there. Test coverage: services/glue/handler_pagination_sweep_sdk_test.go (paginationCasesSchemaRegistry, MaxResults truncation + NextToken resume for both ops) and services/glue/handler_filter_sweep_sdk_test.go (TestSDKRoundTrip_ListSchemas_ScopesByRegistry); every new assertion hand-verified to fail against the pre-fix behavior (paginateSlice call removed, and separately the RegistryId filter neutralized, each confirmed red then restored). Closes gopherstack-q4qt. gopherstack-7f5k (prior pass): DescribeInboundIntegrations had both bugs its sibling DescribeIntegrations had before gopherstack-awzv -- MaxRecords/Marker declared but never read, and its raw *Integration struct marshaled straight out so CreatedAt (time.Time) rendered as an RFC3339 string where the real wire shape is a JSON Number; fixed via paginateSlice and pkgs/awstime.Epoch, matching DescribeIntegrations. Also found while in the op: its response field was named Integrations, the real name is InboundIntegrations (api_op_DescribeInboundIntegrations.go), and TargetArn was declared on the input but never applied as a filter -- both fixed. handler_schemas.go's GetRegistry/GetSchema/ListSchemas/ListSchemaVersions/GetSchemaVersion shared ListRegistries' pre-fix CreatedTime/UpdatedTime float-vs-string bug (Schema Registry declares these *string, confirmed per-op against each deserializer's own switch, not assumed from ListRegistries) -- fixed the same way, via formatGlueTimestampString. Two more found while in these five ops: GetRegistryOutput fabricated a Tags member that doesn't exist on the real type (only CreateRegistryOutput has one) -- removed; GetSchemaOutput dropped LatestSchemaVersion/NextSchemaVersion/SchemaCheckpoint even though the backend's Schema model already tracks them (used by CreateSchema) -- added; ListSchemaVersionsOutput's field was named SchemaVersions, the real name is Schemas (api_op_ListSchemaVersions.go) -- a real client silently decoded to an always-empty slice, now fixed. Test coverage: services/glue/handler_timestamp_sweep_sdk_test.go, driven through the real aws-sdk-go-v2 client; every new assertion hand-verified to fail against the pre-fix behavior. gopherstack-uult (prior pass): ListRegistries/ListSchemas/ListSchemaVersions marshaled the raw Registry/Schema/SchemaVersion domain structs instead of scoping to types.RegistryListItem/SchemaListItem/SchemaVersionListItem -- Tags/RegistryArn/DataFormat/Compatibility/LatestSchemaVersion/NextSchemaVersion/SchemaCheckpoint/SchemaDefinition leaked across the three ops; fixed with dedicated summary structs. gopherstack-ustu (prior pass): DescribeConnectionType/ListConnectionTypes' Capabilities was fabricated as []string instead of the real *types.Capabilities struct, breaking real-SDK-client deserialization entirely for both ops; fixed, plus a second-layer ConnectionTypeBrief.Category->Categories (plural) shape bug found alongside it. See families.DescribeConnectionType/families.ListConnectionTypes and the dated note below. gopherstack-i60f (prior pass): CreateSchema can now carry the initial SchemaDefinition and creates the first version atomically, closing a silent-drop gap found right after gopherstack-j1b7 landed; CreateSchemaOutput gained the five real version fields it was missing entirely. gopherstack-j1b7 (prior pass): schema-registry Compatibility enum validation and DISABLED-mode enforcement now real (CreateSchema/UpdateSchema/RegisterSchemaVersion); BACKWARD/FORWARD/FULL/*_ALL diffing and DQDL grammar validation remain deferred, both re-confirmed genuinely package-sized, not approximated. gopherstack-vcor (prior pass): StartWorkflowRun now actually fires a workflow's entry trigger (previously a bookkeeping no-op) and links the resulting job runs/crawls to the WorkflowRun via an internal, persisted-but-not-wire field; WorkflowRunStatistics is now computed live from that link. gopherstack-dol3 (prior pass): tag-ARN dispatch fixed for Blueprint/DevEndpoint/MLTransform/UDF (plus a real creation/update tag-loss bug found alongside it); workflow Graph+LastRun derived from real trigger/run state; one real, AWS-quota-verified ResourceNumberLimitExceededException (dev endpoints) added. EvaluationMetrics, DQDL/compatibility parsing, 3 of 4 quota/idempotency exceptions, WorkflowRun.Graph's per-node run details, and BlueprintDetails remain honestly deferred -- see notes below.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateDatabase: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDatabase: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDatabases: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass (gopherstack-qd3.3): DatabaseInput/Database gained Parameters, LocationUri, CreateTableDefaultPermissions ([]PrincipalPermissions -> DataLakePrincipal), and TargetDatabase (*DatabaseIdentifier), field-diffed against types.DatabaseInput/types.Database. CreateDatabase/UpdateDatabase now clone (previously CreateDatabase returned the live map-stored pointer, same bug class as the prior pass's GetTables fix)"}
  CreateTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: added Parameters/Owner/Retention to Table+TableInput, and full StorageDescriptor (InputFormat/OutputFormat/SerdeInfo/Parameters/BucketColumns/SortColumns/Compressed/NumberOfBuckets/StoredAsSubDirectories) and Column.Parameters, all previously silently dropped"}
  GetTable: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTables: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: was returning live backend *Table pointers uncloned (lock-bypass mutation/data-race risk); now clones like GetTable/SearchTables"}
  UpdateTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "same field-completeness fix as CreateTable"}
  DeleteTable: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchDeleteTable: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTableVersions: {wire: ok, errors: ok, state: ok, persist: ok, note: "not re-verified in depth this pass; existing coverage looked correct"}
  DeleteTableVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchDeleteTableVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePartition: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: BatchCreatePartition (which CreatePartition delegates to) never checked the parent table existed, silently storing orphaned partitions against a nonexistent db/table; now returns EntityNotFoundException per AWS contract. Also added Partition/PartitionInput.Parameters + Partition.CreationTime/CatalogId"}
  BatchCreatePartition: {wire: ok, errors: ok, state: ok, persist: ok, note: "same table-existence fix as CreatePartition"}
  GetPartition: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPartitions: {wire: ok, errors: ok, state: ok, persist: ok, note: "expression filter (segment) not re-verified in depth this pass. Performance fix 2026-09-06 (gopherstack-a9rs): InMemoryBackend.GetPartitions scanned every partition across every table in the backend (store.Table.Snapshot(), full sort) under the read lock and filtered by key prefix, instead of an indexed per-table lookup. Added a partitionsByTable secondary store.Index (see store.AddIndex; keyed by dbName|tableName, populated/maintained automatically by store.Table.Put/Delete) and GetPartitions now looks up that table's group directly, still sorting the (much smaller) per-table result to preserve the pre-existing sorted-by-key order NextToken pagination depends on. Correctness (ordering, pagination, empty-table, cross-table isolation) unchanged and covered by TestGetPartitions_OrderedByValues, TestGetPartitions_EmptyTableReturnsEmpty, TestGetPartitions_DoesNotLeakOtherTables, TestPagination_GetPartitions_RoundTripOrder. BenchmarkGetPartitions_ManyTables (200 tables x 50 partitions, target table in the middle): ~51.7ms/op, ~20.9MB/op, ~1.15M allocs/op before -> ~29µs/op, ~22KB/op, 595 allocs/op after (roughly 1770x faster, ~950x less memory)."}
  BatchGetPartition: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE fix this pass: was a disguised stub — always returned an empty Partitions list regardless of backend state, with a comment falsely claiming \"the mock backend has no partition storage\". Now looks up each PartitionsToGet entry via GetPartition and reports misses in UnprocessedKeys per the real BatchGetPartitionResponse shape"}
  UpdatePartition: {wire: ok, errors: ok, state: ok, persist: ok, note: "now also persists Parameters through both the in-place and rename paths"}
  BatchUpdatePartition: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePartition: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchDeletePartition: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePartitionIndex: {wire: ok, errors: ok, state: ok, persist: ok, note: "not re-verified in depth this pass"}
  GetPartitionIndexes: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePartitionIndex: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCrawler: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass (additively, gopherstack-qd3.1/qd3.2): CreateCrawler's positional signature is called from services/cloudformation (external package) so it was kept unchanged; CreateCrawlerWithOptions(...,CrawlerOptions) now also carries CrawlerSecurityConfiguration/SchemaChangePolicy/RecrawlPolicy/LineageConfiguration/LakeFormationConfiguration. CrawlerTarget/CrawlerTargets now models all 8 real target kinds (S3/JDBC/Catalog/DynamoDB/Delta/Hudi/Iceberg/MongoDB), field-diffed against types.CrawlerTargets — previously only S3/JDBC/Catalog were modeled"}
  GetCrawler: {wire: ok, errors: ok, state: ok, persist: ok}
  GetCrawlers: {wire: ok, errors: ok, state: ok, persist: ok}
  ListCrawlers: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCrawler: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CrawlerOptions/target-kind fix as CreateCrawler; also fixed a missing CrawlerRunningException guard (UpdateCrawler previously allowed updating a RUNNING/STARTING/STOPPING crawler, unlike DeleteCrawler which already checked this)"}
  DeleteCrawler: {wire: ok, errors: ok, state: ok, persist: ok}
  StartCrawler: {wire: ok, errors: ok, state: ok, persist: ok}
  StopCrawler: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCrawlerSchedule: {wire: ok, errors: ok, state: ok, persist: ok}
  StartCrawlerSchedule: {wire: ok, errors: ok, state: ok, persist: ok}
  StopCrawlerSchedule: {wire: ok, errors: ok, state: ok, persist: ok}
  GetCrawlerMetrics: {wire: ok, errors: ok, state: ok, persist: ok, note: "not re-verified in depth this pass"}
  CreateJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: added Job.MaxCapacity + NotificationProperty (previously missing entirely — the MaxCapacity vs WorkerType/NumberOfWorkers axis named explicitly in the audit brief), plus AWS's documented mutual-exclusion validation between MaxCapacity and WorkerType/NumberOfWorkers"}
  GetJob: {wire: ok, errors: ok, state: ok, persist: ok}
  GetJobs: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "not re-verified in depth this pass"}
  UpdateJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "same MaxCapacity/NotificationProperty fix as CreateJob"}
  DeleteJob: {wire: ok, errors: ok, state: ok, persist: ok}
  StartJobRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass (gopherstack-qd3.4): StartJobRunWithOptions adds real per-run overrides (WorkerType/NumberOfWorkers/MaxCapacity/Timeout/NotificationProperty/SecurityConfiguration) on top of the job-defaults path added last pass, matching StartJobRunRequest and enforcing the MaxCapacity vs WorkerType/NumberOfWorkers mutual-exclusion rule at the run level too. Also fixed a wire-error-code bug: exceeding ExecutionProperty.MaxConcurrentRuns returned generic InvalidInputException instead of the documented ConcurrentRunsExceededException (confirmed in deserializers.go's StartJobRun error switch) — new ErrConcurrentRunsExceeded sentinel, also wired into StartWorkflowRun's new MaxConcurrentRuns check (workflows family)"}
  GetJobRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified this pass (gopherstack-s1u9): GetJobRun calls advanceStates(now) before reading JobRunState, which lazily runs reconcileLocked's STARTING->RUNNING->SUCCEEDED/TIMEOUT transitions even if the background reconciler hasn't ticked yet -- so JobRunState is already a correct, poll-ready completion signal for an external caller (e.g. a future Step Functions .sync integration, gopherstack-tdp6) with no gap to fix. No code changed here; see services/ecs/PARITY.md's matching gopherstack-s1u9 entry for the full design reasoning (a push/broadcast completion API was deliberately NOT added on top of this, for either service, absent a concrete consumer)."}
  GetJobRuns: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchStopJobRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (per-item failure sweep): BatchStopJobRunOutput.SuccessfulSubmissions (api_op_BatchStopJobRun.go) had no wire field at all, so a client could see which run IDs errored but never which ones were actually accepted for stopping. Errors was already correctly populated (EntityNotFoundException/IllegalStateException per bad run ID); only the success half of the same response was missing. Proven by TestBatchStopJobRun_ReportsSuccessfulSubmissions (fails without the fix). Per-item failure sweep also checked BatchCreatePartition, BatchDeleteConnection, BatchDeletePartition, BatchDeleteTable, BatchDeleteTableVersion, BatchGetIterableForms, BatchUpdatePartition, BatchGetPartition, BatchGetTableOptimizer, BatchPutDataQualityStatisticAnnotation, DeleteSchemaVersions: all correctly populate their failure field. CreateIntegration/DeleteIntegration/ModifyIntegration's Errors and GetColumnStatisticsFor{Table,Partition}/UpdateColumnStatisticsFor{Table,Partition}'s Errors are correctly left empty -- neither the Integration model nor column-statistics storage in this backend tracks any failure state a real client can trigger (confirmed for the column-statistics ops by three existing SDK-driven tests -- TestColumnStatisticsForTable_RequiredColumnType, TestColumnStatistics -- that already prove a client-constructed StatisticsData with no populated data member round-trips successfully, i.e. AWS does not enforce Type/data-member consistency server-side either)."}
  GetJobBookmark: {wire: ok, errors: ok, state: ok, persist: ok, note: "not re-verified in depth this pass"}
  ResetJobBookmark: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass (gopherstack-dol3): tagResource()'s ARN dispatcher (tags.go) only recognized Database/Crawler/Job/DataQualityRuleset/Connection/Trigger/Workflow; Blueprint/DevEndpoint/MLTransform/UserDefinedFunction ARNs all returned EntityNotFoundException. Added findBlueprintByARN/findDevEndpointByARN/findMLTransformByARN/findUDFByARN and wired all 4 into TagResource/UntagResource/GetTags/TaggedResources. Also found (not just dispatch): MLTransform/UserDefinedFunction had NO Tags field at all -- CreateMLTransformWithOptions/CreateUserDefinedFunction already called the internal tagResource(ARN, tags) at creation time, but it silently no-op'd against the undispatched ARN, so creation-time tags were lost entirely (not merely unreachable). Added Tags fields to both structs (json:\"-\", matching Blueprint/DevEndpoint's existing internal-only pattern -- confirmed types.MLTransform/types.UserDefinedFunction have no Tags field on the real wire either). Second, separate bug found alongside: UpdateMLTransform/UpdateUserDefinedFunction replace the whole stored record with the caller's input; neither UpdateMLTransformRequest nor UpdateUserDefinedFunctionInput carries Tags on the real wire (confirmed -- AWS updates tags only via TagResource/UntagResource), so every Update call was silently wiping any previously-set tags. Both Update methods now carry existing.Tags forward explicitly. Re-checked this pass (wrapper-key sweep) against the sfn TagResource map/array bug class: glue's TagResourceInput.TagsToAdd is map[string]string (api_op_TagResource.go:44, serializers.go:37549-37564) -- unlike sfn, a map here is correct and needed no change; confirmed via a real-client round-trip test (tag_resource_sdk_test.go)."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "see TagResource note -- same dispatch fix."}
  GetTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "see TagResource note -- same dispatch fix."}
  CreateColumnStatisticsTaskSettings: {wire: ok, errors: n/a, state: ok, persist: ok, note: "fixed (gopherstack-7rq1): request member was `RoleArn`, a gopherstack-invented name -- the real CreateColumnStatisticsTaskSettingsRequest member (glue/2017-03-31/service-2.json) is `Role`. A real client's role was silently dropped by json.Unmarshal every time (empty RoleArn stored), leaving the setting created but never actually runnable with the caller's IAM role. Fixed the json tag; existing tests only asserted HTTP 200 (used the wrong key, now corrected), new TestColumnStatisticsTaskSettings_WireRoleName asserts the value round-trips through GetColumnStatisticsTaskSettings. Schedule/SampleSize/CatalogID/SecurityConfiguration/Tags remain absent from the wire struct (deliberately unmodelled this pass -- ColumnStatisticsTaskSettings has no CatalogID/SecurityConfiguration backend state, and Schedule/SampleSize/Tags were not part of this fix's scope)."}
  UpdateColumnStatisticsTaskSettings: {wire: ok, errors: n/a, state: ok, persist: ok, note: "same RoleArn->Role fix as CreateColumnStatisticsTaskSettings."}
  GetResourcePolicies: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (cursor-pagination sweep): GetResourcePoliciesOutput.NextToken (api_op_GetResourcePolicies.go) was never populated -- the handler ignored MaxResults/NextToken entirely and returned every stored resource policy (per-resource-ARN policies plus the account-level policy, unbounded) in one response. Now routed through the shared paginateSlice helper like every other list op in this package (defaultGetResourcePoliciesLimit=100). Proven via a real aws-sdk-go-v2/service/glue client round trip seeding 3 policies with MaxResults=2 (handler_pagination_sweep_sdk_test.go's 'get resource policies' case), confirmed failing pre-fix (all 3 returned in one page, no NextToken)."}
  GetResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23 (clientcoverage-driven audit of the 228 ops never exercised by a real-SDK-client test): GetResourcePolicyOutput's CreateTime/UpdateTime (confirmed against deserializers.go's awsAwsjson11_deserializeOpDocumentGetResourcePolicyOutput case list: CreateTime/PolicyHash/PolicyInJson/UpdateTime) were dropped entirely -- not fabricated, since this backend already tracks both timestamps per policy on resourcePolicyEntry (resource_policies.go), used correctly by the sibling GetResourcePolicies op the whole time. Backend GetResourcePolicy's signature gained two return values (createTime, updateTime float64); StorageBackend interface updated, both call sites (tables_test.go, persistence_test.go) updated, `make build-check` clean. Proven via a real aws-sdk-go-v2/service/glue client round trip (wire_output_dropped_fields_test.go: TestGetResourcePolicy_ReturnsCreateAndUpdateTime), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical."}
  GetMLTaskRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23 (same audit as GetResourcePolicy): GetMLTaskRunOutput's StartedOn/CompletedOn/ExecutionTime/ErrorString/LogGroupName (api_op_GetMLTaskRun.go) were dropped entirely by a narrower hand-rolled response struct, even though MLTaskRun (models.go) already tracks all five (StartedOn set by StartMLEvaluationTaskRun/StartExportLabelsTaskRun/StartImportLabelsTaskRun/StartMLLabelingSetGenerationTaskRun; CompletedOn set by CancelMLTaskRun) -- the sibling GetMLTaskRuns (list) op was unaffected since it marshals the *MLTaskRun model directly with its own correct json tags. Not touched: TaskRun.LastModifiedOn (real SDK member) and Properties' real shape (*types.TaskRunProperties, a TaskType+4-nested-sub-struct union; this backend's MLTaskRun.Properties is map[string]string and is never populated by any code path, so it stays a documented, currently-inert modelling gap rather than a proven bug -- see gaps). Proven via a real aws-sdk-go-v2/service/glue client round trip (wire_output_dropped_fields_test.go: TestGetMLTaskRun_ReturnsRealTrackedFields), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical."}
  GetMLTaskRuns: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6nr4): GetMLTaskRunsInput declared NO Filter/Sort/MaxResults/NextToken at all -- unlike its sibling GetMLTransforms (same file), which already had all four -- so every call returned the transform's complete, unpaginated task-run set regardless of what a real client requested. Confirmed against the pinned SDK (api_op_GetMLTaskRuns.go): real GetMLTaskRunsInput carries Filter (types.TaskRunFilterCriteria: StartedAfter/StartedBefore/Status/TaskRunType), Sort (types.TaskRunSortCriteria: Column in TASK_RUN_TYPE/STATUS/STARTED, SortDirection), MaxResults, NextToken; real GetMLTaskRunsOutput adds NextToken alongside TaskRuns. Wired via matchesTaskRunFilter/sortTaskRuns/paginateSlice, the same helpers GetMLTransforms already uses. sortTaskRuns tiebreaks every column on TaskRunID: MLTaskRun.StartedOn is a whole-second time.Now().Unix() value (ml.go), so runs started in the same second tie under any real sort column, the same tie-prone-sort precondition already fixed for five other glue listings -- an untiebroken sort here would have traded a missing cursor for dropped/duplicated rows across a page boundary. Proven via TestGetMLTaskRuns_SDKPagination_TotalOrderNoTiesLost (real aws-sdk-go-v2 client, 6 same-second runs, MaxResults=2, asserts the union of every page equals the seeded set exactly) and TestGetMLTaskRuns_SDKFilter_ByStatus (Filter.Status excludes a non-matching run); both confirmed failing against pre-fix code before the fix landed."}
  GetDataQualityRuleRecommendationRun: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23 (same audit as GetResourcePolicy): GetDataQualityRuleRecommendationRunOutput.StartedOn (api_op_GetDataQualityRuleRecommendationRun.go) was dropped entirely even though DQRuleRecommendationRun (models.go) already tracks it, set by StartDataQualityRuleRecommendationRun. wire stays partial, not ok: the real output has ~10 more members (AdditionalRunOptions/CompletedOn/CreatedRulesetName/DataQualitySecurityConfiguration/DataSource/ErrorString/ExecutionTime/LastModifiedOn/NumberOfWorkers/RecommendedRuleset/Role/Timeout) with no backing state anywhere in this backend -- no rule-recommendation engine runs, matching the already-documented ml_transforms EvaluationMetrics gap class ('this backend never runs a real ML evaluation, so there is no real metric to report'); DataSource in particular can't be honestly reconstructed since this backend only stores a flat DataSourceS3Path string while the real field is a structured types.DataSource{GlueTable}, already noted in the gopherstack-awzv gap list below. Left as an honest modelling gap, not fabricated. Proven via a real aws-sdk-go-v2/service/glue client round trip (wire_output_dropped_fields_test.go: TestGetDataQualityRuleRecommendationRun_ReturnsStartedOn), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical."}
families:
  connections: {status: ok, note: "fixed this pass: field-diffed Connection/ConnectionInput against types.Connection/types.ConnectionInput and added Description, MatchCriteria ([]string), and PhysicalConnectionRequirements (AvailabilityZone/SubnetId/SecurityGroupIdList — used e.g. by NETWORK-type connections in place of ConnectionProperties), all previously silently dropped. CreateConnectionWithOptions/UpdateConnectionWithOptions added additively (CreateConnection/UpdateConnection kept for existing callers). Not modeled: AthenaProperties/SparkProperties/PythonProperties/AuthenticationConfiguration/CompatibleComputeEnvironments — newer OAuth/compute-environment fields judged out of scope for this pass (no auth-flow simulation exists anywhere in this backend)."}
  RegisterConnectionType: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass (gopherstack-u90v): handler previously read only ConnectionType/Description and dropped ConnectionProperties, ConnectorAuthenticationConfiguration, IntegrationType and RestConfiguration, all required (glue@v1.152.0 api_op_RegisterConnectionType.go:38-70) — two more (ConnectionProperties, IntegrationType) than the sweep that filed this issue caught. Response was also fabricated: real RegisterConnectionTypeOutput carries only ConnectionTypeArn (api_op_RegisterConnectionType.go:79-84), not the previous ConnectionType/Status pair. Now requires all four (InvalidInputException if absent — ValidationException is also declared for this op, but InvalidInputException is what this handler's existing ErrValidation/awserr.ErrInvalidParameter convention already maps to, and it's in the same declared switch), validates IntegrationType against the SDK's own enum (\"REST\" only), validates ConnectorAuthenticationConfiguration.AuthenticationTypes is present (its own required sub-field), and returns a real ConnectionTypeArn. ConnectionProperties/ConnectorAuthenticationConfiguration are stored as opaque documents (map[string]any, not flattened) but never echoed anywhere: neither has a matching field on DescribeConnectionTypeOutput (its ConnectionProperties is a differently-shaped map[string]Property; its AuthenticationConfiguration is *types.AuthConfiguration, a distinct type) — genuinely inert, not an omission. RestConfiguration IS the same type on both sides and is now echoed on DescribeConnectionType."}
  DescribeConnectionType: {wire: ok, errors: ok, state: ok, persist: ok, note: "RestConfiguration added gopherstack-u90v (see RegisterConnectionType note) and echoes correctly. FIXED (gopherstack-ustu): Category was removed entirely -- confirmed not a field on the real DescribeConnectionTypeOutput at all (api_op_DescribeConnectionType.go) -- and Capabilities changed from a fabricated []string of \"READ\"/\"WRITE\" to the real *types.Capabilities shape (SupportedAuthenticationTypes/SupportedComputeEnvironments/SupportedDataOperations, all required; new local connectionCapabilities struct, handler_connection_types.go, since this backend hand-rolls wire structs rather than importing SDK types). A real SDK client's deserializer previously rejected the whole response body on the array-vs-object mismatch (confirmed: TestSDKRoundTrip_RegisterConnectionType_EchoesRequiredMembers could not drive DescribeConnectionType through the real client for this reason and fell back to raw HTTP -- it now uses the real client). This backend's existing per-type READ/WRITE data (rwCaps/readCaps) maps exactly onto SupportedDataOperations (types.DataOperation's only two enum values are literally \"READ\"/\"WRITE\") and is threaded through, not discarded; SupportedAuthenticationTypes/SupportedComputeEnvironments have no backing state anywhere in this backend and are modeled as real, present, empty slices (not fabricated) when Capabilities itself is present -- Capabilities is omitted entirely (not an empty-but-present object) for connector categories with no tracked DataOperations at all (NETWORK/MARKETPLACE/CUSTOM), since Capabilities is not itself a required member on this op's output."}
  ListConnectionTypes: {wire: partial, errors: ok, state: ok, persist: n/a, note: "NOT previously tracked in this ledger. FIXED (gopherstack-ustu, second-layer find made while fixing DescribeConnectionType's Capabilities bug): ConnectionTypeBrief has the same fabricated-[]string Capabilities bug as DescribeConnectionTypeOutput (same fix, shared connectionCapabilities/toConnectionCapabilities helper), PLUS a second, distinct shape bug not called out in the issue that filed this fix -- the real field is Categories (types.ConnectionTypeBrief, glue@v1.152.0 types/types.go:2533-2564), a []string (plural), not the singular Category string this backend emitted. This backend's ConnectionTypeInfo only ever models one category per type, so it is now echoed as the one-element list that shape implies -- not fabricated into several. DisplayName/LogoUrl/Vendor/ConnectionTypeVariants are also real ConnectionTypeBrief members with no backing state anywhere in this backend -- deliberately left absent (wire: partial for this reason) rather than invented; see gaps."}
  ListEntities: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-2wvq): ConnectionName was wrongly required -- ListEntitiesInput declares no required members at all (glue@v1.152.0 api_op_ListEntities.go:29-49). With none given, this now serves the native Amazon S3 Glue Data Catalog path the op's own doc describes, off this backend's real databases/tables (GetDatabases/GetTables), not fabricated data: top level lists databases (Category DATABASES, IsParentEntity true), ParentEntityName=<database> lists that database's tables as \"database.table\" (Category TABLES) -- see DescribeEntity note for why this qualified form was chosen. Also fixed the accept-and-drop half: ParentEntityName was a real input field silently ignored by every path; it is now honored for native-catalog listing. It is NOT honored in connector (ConnectionName given) mode -- entityCatalog()'s canned CRM/COMMERCE entities model no children (only ACCOUNT/CUSTOMER set IsParentEntity, with nothing underneath), so there is nothing to filter to; inventing child entities for those two would be exactly the half-feature this issue's rule warns against, so ParentEntityName stays a documented no-op there. FIXED 2026-08-29 (cursor-pagination sweep): NextToken (declared on both input and output) was still never populated in the native-catalog path -- databases/tables are real, unbounded, user-created collections. The real op declares no MaxResults, so the page size is server-fixed (defaultListEntitiesLimit=100); now routed through paginateSlice. Connector-mode listing (entityCatalog(), a compile-time 7-entry catalogue) is provably bounded and left as-is -- see DescribeEntity. Proven via a real client round trip seeding 101 databases (entities_test.go: TestListEntities_Pagination), confirmed failing pre-fix."}
  GetEntityRecords: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-2wvq): ConnectionName was wrongly required -- GetEntityRecordsInput's only required members are EntityName and Limit (glue@v1.152.0 api_op_GetEntityRecords.go:35-48); ConnectionName is optional (line 55), and the op's own doc says why: \"query preview data from a given connection type or from a native Amazon S3 based Glue Data Catalog\". Checked the OTHER direction too (this issue's rule 1): Limit is real-SDK-required (its client-side validator, validators.go:13344-13360, rejects a call omitting it before the request is ever sent) but this handler never enforced that -- now returns InvalidInputException for Limit<=0, closing that half. With no ConnectionName, EntityName must be the \"database.table\" form ListEntities' native-catalog path advertises (chosen, not AWS-specified, since GetEntityRecordsInput has no separate database/parent field to disambiguate a bare table name against multiple databases -- stated as chosen, in code (nativeEntityName/splitNativeEntityName) and here); a bare database name or an unqualified/unknown name is EntityNotFoundException, not an empty or fabricated success. Records are synthesized the same deterministic way as the connector path (sampleRecord over an entityDefinition), but the schema is real: columnToEntityField maps each StorageDescriptor.Column and PartitionKey's Glue/Hive type string (bigint/decimal(...)/boolean/timestamp/date/etc, matched by prefix) onto the same EntityField shape DescribeEntity uses for connector entities. DescribeEntity itself is out of this issue's scope (not one of the two ops named) and still requires ConnectionName -- it does not yet support native-catalog table lookups; a real client discovering a native entity via ListEntities and then calling DescribeEntity on it would get EntityNotFoundException today. That is a real, scoped-out gap, not silently papered over."}
  triggers: {status: ok, note: "fixed this pass (gopherstack-qd4.1): Trigger gained Description, WorkflowName, and EventBatchingCondition (BatchSize/BatchWindow); TriggerCondition gained CrawlerName and CrawlState (types.Condition supports crawler-state predicates, not just job-state — was entirely unmodeled); TriggerAction gained SecurityConfiguration/NotificationProperty/Timeout (types.Action fields silently dropped). CreateTrigger/UpdateTrigger now enforce AWS's documented 'max 2 crawler actions per trigger' soft limit (about-triggers.html), returning InvalidInputException over the limit. WorkflowName is create-only (not part of TriggerUpdate, confirmed against types.TriggerUpdate) so UpdateTrigger does not accept it."}
  workflows: {status: partial, note: "fixed this pass (gopherstack-qd3.5-era fix retained): Workflow gained MaxConcurrentRuns, enforced in StartWorkflowRun, returning ConcurrentRunsExceededException. gopherstack-dol3: Workflow.Graph and Workflow.LastRun are now real, derived fields -- GetWorkflow/BatchGetWorkflows gained IncludeGraph (confirmed on GetWorkflowInput/BatchGetWorkflowsInput; Graph is only populated when set, matching AWS). Graph (WorkflowGraph{Nodes,Edges}) is built by workflowGraphLocked (workflow_graph.go) purely from real state: every Trigger with WorkflowName==this workflow becomes a TRIGGER node (with real TriggerDetails.Trigger, confirmed types.TriggerNodeDetails.Trigger), each trigger's TriggerAction.JobName/CrawlerName become downstream JOB/CRAWLER nodes+edges, each trigger's TriggerPredicate.Conditions become upstream JOB/CRAWLER nodes+edges -- no fabricated topology. Node.UniqueId is \"<kind>/<name>\" (real ID-gen algorithm not discoverable from the SDK, same simplification already accepted here for FormType.Id). LastRun is the most recent entry from real StartWorkflowRun history (b.workflowRuns), absent until a run has actually happened. NEW this pass (gopherstack-vcor): the missing link is built. Verified against aws-sdk-go-v2/service/glue@v1.152.0 that neither JobRun nor Crawl/CrawlerHistory carries a WorkflowRunId on the wire (types.go:2815-2836,2916-2946,7134-7352) -- JobRun's only real correlation field is TriggerName (types.go:7350-7351), which this backend now also populates for the first time. StartWorkflowRun now fires the workflow's entry-point trigger(s) (WorkflowName==this workflow, Predicate==nil -- AWS calls this the workflow's \"start trigger\", workflows_overview.html) and stamps the new run's ID onto the job runs/crawls those actions start, via an internal-only (non-wire) WorkflowRunID field on JobRun/CrawlHistoryEntry that persists but is stripped before GetJobRun/GetJobRuns responses (ListCrawls was already safe: its crawlHistoryOut DTO copies fields explicitly). GetWorkflowRun/GetWorkflowRuns/GetWorkflow/BatchGetWorkflows now compute WorkflowRunStatistics live from that link (never stored, so it can't go stale); ErroredActions/WaitingActions count job runs only, per the SDK's own doc comments for those two fields (\"count of job runs in the ERROR/WAITING state\", types.go:13224-13225) unlike the other fields' generic \"Actions\" wording. Two things are deliberately still not modeled: (1) conditional (predicate-gated) triggers within a workflow never fire on their own -- this backend has no predicate-evaluation engine watching job/crawler completions, so only an entry trigger's own direct actions are ever linked to a run, not a full downstream DAG execution; (2) BlueprintDetails (still structurally unreachable, unchanged from gopherstack-dol3) and WorkflowRun.Graph/GetWorkflowRun's own IncludeGraph (types.Node.JobDetails.JobRuns/CrawlerDetails.Crawls) remain unpopulated -- the link now exists to build them, but that is real additional work (converting stamped runs into per-node run-history lists) not done this pass."}
  dev_endpoints: {status: ok, note: "fixed this pass: DevEndpoint/DevEndpointInput were previously missing ~20 of ~24 real fields (RoleArn, SecurityGroupIds, SubnetId, WorkerType, GlueVersion, NumberOfWorkers/Nodes, PublicKey(s), ExtraJarsS3Path/ExtraPythonLibsS3Path, SecurityConfiguration, VpcId, AvailabilityZone, YarnEndpointAddress/PrivateAddress/PublicAddress, FailureReason, LastUpdateStatus, ZeppelinRemoteSparkInterpreterPort, CreatedTimestamp/LastModifiedTimestamp) — CreateDevEndpoint took only a bare name. Field-diffed against types.DevEndpoint/CreateDevEndpointInput/UpdateDevEndpointInput and added all of them. RoleArn is a real AWS-required field and is now validated as such (was previously accepted as empty, which real AWS rejects). UpdateDevEndpoint gained AddPublicKeys/DeletePublicKeys/PublicKey/DeleteArguments (previously only AddArguments worked). Network address fields (VpcId/YarnEndpointAddress/PrivateAddress/PublicAddress) are deterministic mock values, not real network state — there is no VPC/networking simulation in this backend, consistent with every other service. NEW this pass (gopherstack-dol3): CreateDevEndpoint now enforces AWS's real, published default quota 'Max development endpoint per account: 25' (docs.aws.amazon.com/general/latest/gr/glue.html, verified via WebFetch this pass, not from memory) via a new ErrResourceNumberLimitExceeded sentinel -> ResourceNumberLimitExceededException, confirmed present in CreateDevEndpoint's real error catalog (deserializers.go's awsAwsjson11_deserializeOpErrorCreateDevEndpoint switch). See gap-list note on the other three quota/idempotency exceptions for why only this one resource kind got a limit this pass."}
  security_configurations: {status: ok, note: "fixed this pass: EncryptionConfiguration was missing DataQualityEncryption (DataQualityEncryptionMode/KmsKeyArn), field-diffed against types.EncryptionConfiguration — CloudWatchEncryption/JobBookmarksEncryption/S3Encryption were already modeled. CreateSecurityConfiguration/GetSecurityConfiguration/DeleteSecurityConfiguration/ListSecurityConfigurations all do real state mutation; cloneSecurityConfig's shallow-copy pattern audited and confirmed safe (no field is ever mutated post-creation, same reasoning as the data_quality_rulesets finding below)."}
  schema_registry: {status: partial, note: "FIXED this pass (gopherstack-q4qt): ListSchemas/ListSchemaVersions ignored MaxResults/NextToken entirely (both declared on the real ListSchemasInput/ListSchemaVersionsInput, glue@v1.152.0 api_op_ListSchemas.go/api_op_ListSchemaVersions.go) -- unlike the gopherstack-awzv sweep's 29 ops, these two already took a real RegistryId/SchemaId so were never swept for pagination. Wired through the existing paginateSlice helper with new defaultListSchemasLimit/defaultListSchemaVersionsLimit consts (25, matching each op's own doc comment and ListRegistries' established convention). ListSchemas.RegistryId was re-verified to already be a real filter (registry.go's ListSchemas, confirmed via hand-revert), not a repeat of the DescribeInboundIntegrations/GetColumnStatisticsTaskRuns ignored-scoping-parameter bug class; ListSchemaVersions' SchemaId is inherently a single-schema scope, no separate filter gap. See the dated overall note above and services/glue/handler_pagination_sweep_sdk_test.go / handler_filter_sweep_sdk_test.go for coverage. fixed this pass: RegisterSchemaVersion never validated its SchemaDefinition against the schema's DataFormat — CreateSchema's initial definition IS validated (validateSchemaDefinition), but every subsequent RegisterSchemaVersion call silently accepted arbitrarily malformed AVRO/JSON/PROTOBUF content, a real correctness gap now fixed by reusing the same validator. GetSchemaByDefinition was already implemented for real (found not to be a stub, contrary to the prior ledger's 'still not audited' note). FIXED this pass (gopherstack-j1b7): CreateSchema/UpdateSchema silently accepted any Compatibility string (confirmed against types.Compatibility.Values(), aws-sdk-go-v2/service/glue@v1.152.0 types/enums.go:328-354 -- NONE/DISABLED/BACKWARD/BACKWARD_ALL/FORWARD/FORWARD_ALL/FULL/FULL_ALL are the only 8 legal values), now rejected with InvalidInputException. DISABLED's own documented behavior (api_op_CreateSchema.go:14-18: 'restricts any additional schema versions from being added after the first schema version') was entirely unenforced -- RegisterSchemaVersion now rejects a second version when Compatibility is DISABLED (first version always accepted regardless of mode, per api_op_RegisterSchemaVersion.go:17-18); this is a complete, zero-approximation implementation of DISABLED because it needs no schema diffing, only a version-count check. Still not modeled, deliberately: BACKWARD/FORWARD/FULL (and their _ALL variants) all require a real per-DataFormat schema-compatibility-diffing algorithm (AVRO/JSON/PROTOBUF each have distinct field-addition/type-widening rules) -- sized and deferred rather than approximated, since a diff that misses a real incompatibility is worse than no check at all (a caller trusts a pass). validateAvroSchema/validateJSONSchema/validateProtobufSchema remain surface-level (JSON well-formedness + minimal structural markers, not full grammar validation) — both that and the six diffing-based modes would require real schema-parsing libraries per format, out of scope for this pass (no new go.mod dependencies permitted, per the prior pass's ledger). FIXED this pass (gopherstack-i60f): CreateSchema had no way to carry SchemaDefinition at all -- a client doing AWS's documented one-call create-with-definition flow got a schema with zero versions and no error, a silent drop found immediately after the j1b7 fix above landed. CreateSchema now accepts an optional SchemaDefinition (aws-sdk-go-v2/service/glue@v1.152.0 api_op_CreateSchema.go:106), validated the same way as RegisterSchemaVersion; a malformed definition rejects the whole call, leaving no half-created schema (Test_CreateSchema_RejectsInvalidDefinition). When supplied, the schema and its first SchemaVersion are created atomically and CreateSchemaOutput now returns the five real version fields it was previously missing entirely -- LatestSchemaVersion/NextSchemaVersion/SchemaCheckpoint/SchemaVersionId/SchemaVersionStatus (api_op_CreateSchema.go:129-157). This closes the loop with j1b7's DISABLED enforcement: creating with a definition sets LatestSchemaVersion=1 immediately, so DISABLED correctly refuses a following RegisterSchemaVersion the same as it would a real second version; creating without one leaves LatestSchemaVersion=0 so DISABLED still permits exactly the first RegisterSchemaVersion. Both paths covered by Test_CreateSchema_DisabledCompatibility_VersionSlotInteraction."}
  data_quality_rulesets: {status: partial, note: "fixed this pass: CreateDataQualityRuleset/UpdateDataQualityRuleset silently dropped Description entirely (real CreateDataQualityRulesetInput/UpdateDataQualityRulesetInput both document it) and CreateDataQualityRuleset was also missing TargetTable (DataQualityTargetTable: TableName/DatabaseName/CatalogId) and DataQualitySecurityConfiguration — all field-diffed against types.CreateDataQualityRulesetInput and added via new CreateDataQualityRulesetWithOptions. Re-confirmed the prior pass's finding that CreateDataQualityRuleset/StartDataQualityRulesetEvaluationRun returning their live map-stored pointer is not an actual bug (handlers only read immutable identity fields). Still not modeled: DQDL syntax / rule-type validation — the Ruleset string is stored and returned verbatim with no grammar checking, would require a real DQDL parser, out of scope for this pass."}
  ml_transforms: {status: partial, note: "fixed this pass: CreateMLTransform/UpdateMLTransform silently dropped GlueVersion/WorkerType/NumberOfWorkers/MaxCapacity (the MLTransform model already had these fields from a prior pass, but neither Create nor Update ever wired them from the wire request — a genuine 'field exists on the model but is unreachable' gap) plus MaxRetries/Timeout/Schema ([]SchemaColumn)/TransformEncryption (MlUserDataEncryption+TaskRunSecurityConfigurationName), none of which existed at all. Field-diffed against types.MLTransform/CreateMLTransformRequest/UpdateMLTransformRequest. Added CreateMLTransformWithOptions plus the same MaxCapacity-vs-WorkerType/NumberOfWorkers mutual-exclusion validation used elsewhere (CreateJob/CreateCrawler/StartJobRun). Still not modeled: EvaluationMetrics (FindMatchesMetrics precision/recall/F1/confusion-matrix) — this backend never runs a real ML evaluation, so there is no real metric to report; StartMLEvaluationTaskRun creates a real task-run record but does not fabricate evaluation numbers, which would be a stub-shaped lie rather than an honest gap. Re-confirmed this pass (gopherstack-dol3): still correctly absent, still no code anywhere references EvaluationMetrics/FindMatchesMetrics. Also fixed this pass: Tags were entirely lost, both at creation (see TagResource note) and on every Update (Tags now carried forward explicitly)."}
  blueprints: {status: ok, note: "fixed this pass: CreateBlueprint took only a bare Name — real CreateBlueprintInput requires BlueprintLocation (the S3 path Glue reads the blueprint from) and also supports Description/Tags, all silently unsupported. UpdateBlueprint similarly took only Name; real UpdateBlueprintInput requires BlueprintLocation and supports Description. Blueprint (the response/Get type) was also missing BlueprintLocation/BlueprintServiceLocation/Description/ParameterSpec/ErrorMessage/CreatedOn/LastModifiedOn — field-diffed against types.Blueprint and added. BlueprintLocation is now validated as required on both Create and Update, matching AWS. Not modeled: LastActiveDefinition — this duplicates Blueprint's own top-level fields in the common case (only differs after a failed update, which this backend does not simulate), so leaving it out does not create an observable gap for any currently-modeled failure path."}
  user_defined_functions: {status: ok, note: "fixed this pass: UserDefinedFunction was missing FunctionType (types.UserDefinedFunction/UserDefinedFunctionInput both document it — was entirely unmodeled, meaning Athena/Redshift-Spectrum-style scalar-function metadata was silently dropped) and CatalogId (every other catalog-scoped resource in this backend — Database/Table/Partition — already models CatalogID; UDF was the one exception). Also fixed a wire-shape bug in the other direction: the local model had a `FunctionArn` field with `json:\"FunctionArn\"` that does NOT exist on the real wire type at all (confirmed against types.UserDefinedFunction) — a fabricated extra field that, while harmless to JSON-tolerant clients, is not real AWS-accurate shape; changed to `json:\"-\"` (internal-only, used for TagResource) so GetUserDefinedFunction/GetUserDefinedFunctions responses now match the real shape exactly. Fixed this pass (gopherstack-dol3): Tags were entirely lost, both at creation (see TagResource note) and on every Update (Tags now carried forward explicitly). Separately noted, not fixed (out of this pass's tag-dispatch scope): the wire's createUserDefinedFunctionInput.Tags field (handler_user_defined_functions.go) has no equivalent on the real CreateUserDefinedFunctionInput at all (confirmed against the pinned SDK) -- real AWS clients never send it and can only tag a UDF post-creation via TagResource, which now works correctly; the extra accepted-but-non-standard input field is pre-existing and harmless (unreachable by any real SDK client) but is not itself AWS-accurate shape."}
  resource_policy: {status: ok, note: "fixed this pass: PutResourcePolicy silently dropped PolicyExistsCondition (MUST_EXIST/NOT_EXIST) and PolicyHashCondition entirely — every call unconditionally created/overwrote the policy regardless of what a caller passed, defeating the optimistic-concurrency guard those fields exist for. Worse, DeleteResourcePolicy's PolicyHashCondition parameter was already plumbed from the wire into the backend method but the backend signature discarded it as `_ string` — any caller's hash was ignored and the policy always deleted. Both now enforce the conditions and return the documented ConditionCheckFailureException (new sentinel ErrResourcePolicyConditionFailed, mapped in handler.go's handleError) or EntityNotFoundException (MUST_EXIST-but-missing) on mismatch. Interface signature PutResourcePolicy gained two params (existsCondition, hashCondition). Fixed this pass (gopherstack-qd4.2): EnableHybrid (TRUE/FALSE) is now accepted, validated as a well-formed enum, and recorded per-policy — previously silently dropped without even being read off the wire. AWS's documented precondition ('must be TRUE if you have already used the Management Console to grant cross-account access') can never actually trigger in this backend because Lake Formation console-grant state is not modeled anywhere in gopherstack, so both TRUE and FALSE correctly succeed unconditionally, matching real AWS behavior for any account with no console grants."}
  integration_resource_properties: {status: ok, note: "fixed this pass (found while auditing the deferred families, not previously tracked in this ledger): GetIntegrationResourceProperty/CreateIntegrationResourceProperty/UpdateIntegrationResourceProperty/ListIntegrationResourceProperties and GetIntegrationTableProperties all returned the live map-stored pointer with its SourceProperties/TargetProperties (or SourceTableConfig/TargetTableConfig) maps uncloned. UpdateIntegrationResourceProperty/UpdateIntegrationTableProperties reassign those same map fields in place under the lock, while Get/Create's callers read them after the lock is released — a genuine data race, same bug class as the prior pass's GetTables fix. Fixed by cloning (new cloneIntegrationResourceProperty helper + inline clone for the table-properties Get)."}
  Integration: {status: fixed, note: "FIXED (gopherstack-lx5h), first PARITY.md entry for CreateIntegration/ModifyIntegration/DeleteIntegration. CreateIntegrationOutput carries 6 required fields (api_op_CreateIntegration.go); the handler emitted only IntegrationName/Status, dropping CreateTime (already tracked on the model as Integration.CreatedAt, just never surfaced), IntegrationArn, SourceArn, and TargetArn. The last two were structural, not cosmetic: CreateIntegrationInput itself declares SourceArn/TargetArn as required INPUT members that the handler never read at all, and the Integration model had no fields to store them — so this needed schema + request-read fixes together, not a response-key rename. Added SourceArn/TargetArn to the Integration model (validated required, InvalidInputException via the service's existing ErrValidation convention, matching this op's own declared error switch) and IntegrationArn via arn.Build(\"glue\", region, account, \"integration/\"+name), following the exact convention every other Glue resource ARN in this codebase already uses (blueprintARN/connectionARN/crawlerARN/etc.), not a fabricated pattern. CreateTime is emitted as epoch-seconds via pkgs/awstime.Epoch — JSON-RPC 1.1's IntegrationTimestamp shape (confirmed in deserializers.go's CreateIntegrationOutput switch), not an ISO string. ModifyIntegration/DeleteIntegration checked per the issue's request and found the identical 2-of-6-or-fewer gap (ModifyIntegrationOutput/DeleteIntegrationOutput share the same 6 required fields); both fixed the same way, sourcing the now-complete Integration record instead of fabricating anything. Also fixed on the input side, found while in the same code: real ModifyIntegration/DeleteIntegrationInput's own doc comments describe IntegrationIdentifier as \"The Amazon Resource Name (ARN) for the integration\", but this backend's store is keyed by IntegrationName and the handlers only ever accepted the bare name — a real SDK client passing the ARN gopherstack itself just started returning from Create would have 404'd against Modify/Delete. Added resolveIntegrationName (accepts either name or ARN) rather than switching the store's primary key, a smaller and lower-risk fix. Also fixed DescribeInboundIntegrations' IntegrationArn filter, which compared the real filter value against IntegrationName (silently matching nothing for any real client) — now compares against the real IntegrationArn field this pass added. DeleteIntegrationOutput.Status now reports DELETING (a real enum value reflecting the delete just actioned), not fabricated ACTIVE/absent. Not touched: ModifyIntegration accepts DataFilter/Description/IntegrationConfig/IntegrationName (rename) as real optional inputs but this backend still does not apply any of them to the stored record (pre-existing behavior, unchanged — the issue's ask was the response-field gap, not full Modify semantics); disclosed here as a gap, not silently left broken. gopherstack-muzq (2026-08-21): CreateIntegration stamped Status CREATING and nothing anywhere in this backend ever advanced it -- no ticker, no later call, and reconciler.go's existing background reconciler (crawler RUNNING->READY, job-run STARTING->RUNNING->SUCCEEDED) did not cover Integration at all, so every DescribeIntegrations call showed CREATING for the entire lifetime of every integration ever created. Fixed by reusing that exact reconciler (new integrationReadyAt map, a CREATING->ACTIVE branch in reconcileLocked/pendingDueLocked, and a lazy b.advanceStates call at the top of ListIntegrations, mirroring GetCrawler) rather than introducing new async infrastructure. New test TestIntegration_ReachesActive asserts the terminal ACTIVE state via DescribeIntegrations, not just the correct initial CREATING (which the pre-existing TestIntegration already asserted and stopped there)."}
  catalogs: {status: fixed, note: "FIXED 2026-08-21 (gopherstack-r80d batch 15, required-output cut): CreateCatalog read the catalog's Name off a nonexistent CatalogInput.Name field. The real CreateCatalogInput (api_op_CreateCatalog.go) has no CatalogId member at all and puts Name at the top level, a sibling of CatalogInput (confirmed against serializers.go's awsAwsjson11_serializeOpDocumentCreateCatalogInput and types.CatalogInput, which itself has no Name field) -- a catalog is created and addressed purely by Name. Every catalog created by a real client was silently stored under the empty-string key with an empty Name; a subsequent GetCatalog(CatalogId: <the name the client used>) would 404, and Catalog.Name -- required (types/types.go) -- was also tagged omitempty on the wire view, so even a successful GetCatalogs would drop the key entirely. Fixed by reading the real top-level Name (createCatalogInput gained a Name field, the fictional CatalogId field removed) and using it as both the catalog's Name and this backend's storage key, and removing CatalogEntry.Name's omitempty tag. Proven via a real aws-sdk-go-v2/service/glue client round trip (wire_output_required_r80d_test.go), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical. handler_catalogs_test.go's pre-existing TestCatalog sent CatalogId on create (a field the real wire never has) and never asserted Name at all -- corrected to send only the real top-level Name and assert it round-trips. Also fixed but NOT counted, same batch: EncryptionAtRest.CatalogEncryptionMode (models.go, used by GetDataCatalogEncryptionSettings/PutDataCatalogEncryptionSettings) is required (types/types.go) and was tagged omitempty, but unlike the members counted above, the real PutDataCatalogEncryptionSettingsInput's own client-side validateEncryptionAtRest (validators.go) checks `len(v.CatalogEncryptionMode) == 0`, not just a nil pointer -- an empty string is rejected before the request is even sent, so no real client can reach a stored EncryptionAtRest with an empty CatalogEncryptionMode. Tag removed as harmless cleanup."}
  classifiers: {status: fixed, note: "FIXED 2026-08-21 (gopherstack-r80d batch 15, required-output cut): GrokClassifier.Classification/GrokPattern, XMLClassifier.Classification and JsonClassifier.JsonPath are all required (types/types.go) but tagged omitempty on this backend's wire structs. Reachable via a real client: CreateGrokClassifierRequest/CreateXMLClassifierRequest/CreateJsonClassifierRequest's own client-side validators (validators.go) only reject a nil pointer, never an empty string, and this backend's CreateClassifier/UpdateClassifier store whatever content is supplied with no further validation -- so a real client supplying an empty (non-nil) Classification/GrokPattern/JsonPath is a fully reachable state, not a bypass of client-side checks. Fixed by removing the omitempty tags on all four members (GrokClassifier.Classification, GrokClassifier.GrokPattern, XMLClassifier.Classification, JSONClassifier.JSONPath in models.go). Proven via three real aws-sdk-go-v2/service/glue client round trips (wire_output_required_r80d_test.go, one per classifier sub-type), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical. No existing test asserted the absence of these keys, so none needed correction."}
  column_statistics: {status: fixed, note: "FIXED 2026-08-21 (gopherstack-r80d batch 15, required-output cut): ColumnStatistics.ColumnType is required (types/types.go) but tagged omitempty. Reachable via a real client: UpdateColumnStatisticsForTable/UpdateColumnStatisticsForPartition's own client-side validateColumnStatistics (validators.go) only rejects a nil ColumnType pointer, never an empty string, and this backend's UpdateColumnStatisticsForTable/UpdateColumnStatisticsForPartition (column_statistics.go) store whatever ColumnType is supplied verbatim with no further validation. Fixed by removing the omitempty tag. Proven via a real aws-sdk-go-v2/service/glue client round trip (wire_output_required_r80d_test.go: UpdateColumnStatisticsForTable with ColumnType=aws.String(\"\"), then GetColumnStatisticsForTable), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical. Two adjacent members carrying the same dead omitempty tag were fixed as harmless cleanup, NOT counted as bugs, because neither is reachably empty through any real client path: ColumnStatistics.AnalyzedTime is overwritten server-side to time.Now() on every UpdateColumnStatisticsForTable/Partition call regardless of what the client supplies, so it can never actually be zero-valued in a stored record; integrationSummary.CreateTime (handler_integrations.go, DescribeIntegrations) is likewise always derived from Integration.CreatedAt, which the sole construction site (integrations.go's CreateIntegration) always sets to time.Now().UTC(). Also fixed but not counted, same reachability reasoning: CustomEntityType.RegexString (models.go) is required (types/types.go) and tagged omitempty, and the real CreateCustomEntityTypeInput's client-side validator likewise only rejects a nil pointer -- but this backend's own CreateCustomEntityType (custom_entity_types.go) independently rejects an empty RegexString with ErrValidation before ever storing a record, so no real client can reach a stored CustomEntityType with an empty RegexString; the tag was removed as defense-in-depth, not claimed as a proven bug."}
  glossaries: {status: ok, note: "NEW this pass (parity-4, SDK bump to v1.149.0 revealed 31 new ops): CreateGlossary/GetGlossary/UpdateGlossary/DeleteGlossary/ListGlossaries and CreateGlossaryTerm/GetGlossaryTerm/UpdateGlossaryTerm/DeleteGlossaryTerm/ListGlossaryTerms field-diffed against the SDK's Create/Get/Update output shapes (Glossary reuses one struct for all three since they share exactly Id/Name/Description; same for GlossaryTerm). DeleteGlossary enforces AWS's documented 'cannot delete while it still contains terms' ConflictException (confirmed in deserializers.go's error switch). DeleteGlossaryTerm additionally disassociates the term from every asset/iterable-form-item that referenced it (not separately documented by the op's own shape, but the same referential-integrity discipline this backend already applies elsewhere, e.g. BatchDeleteTable cascading to partitions) -- covered by TestGlue_AssociateGlossaryTerms_TableDriven/deleting_glossary_term_cascades_to_asset. Glossary/GlossaryTerm IDs are opaque generated IDs (gls-/term- prefix + short uuid), matching that Name is not unique and Identifier is always used for lookup in the real shapes."}
  asset_catalog: {status: ok, note: "NEW this pass: AssetType (PutAssetType/GetAssetType/DeleteAssetType/ListAssetTypes) and Asset (PutAsset/GetAsset/UpdateAsset/DeleteAsset/SearchAssets) field-diffed against the SDK. PutAssetType validates every referenced FormTypeIdentifier exists (EntityNotFoundException) -- an inferred FK check, not explicitly documented, but matches the FormType<-AssetType ownership DeleteFormType's own ConflictException already implies. PutAsset requires an existing AssetTypeId. DeleteAssetType has NO documented ConflictException (confirmed absent from deserializers.go's error switch, unlike DeleteFormType/DeleteGlossary), so deleting an asset type still referenced by assets is allowed -- deliberately not inventing an undocumented guard. AssociateGlossaryTerms/DisassociateGlossaryTerms validate both the asset and every glossary term ID exist. SearchAssets supports SearchText (case-insensitive substring on Name/Description) plus FilterClause's full union shape (AndAllFilters/OrAnyFilters/AttributeFilter/MapFilter, all 6 SearchFilterOperator values, decoded as a plain struct rather than reproducing the SDK's Go-side interface union since this backend only ever decodes, never encodes, the filter -- see search_assets.go's file doc comment) and Sort. MapFilter is scoped to the 'Forms' map attribute (the only map-shaped Asset field); AttributeFilter covers Name/Description/Id/AssetTypeId/CreatedAt/UpdatedAt."}
  form_types_and_attachments: {status: ok, note: "NEW this pass: FormType (PutFormType/GetFormType/DeleteFormType/ListFormTypes) is upsert-keyed by Name (AWS documents 'if a form type with the given name already exists, it is updated' for the sibling PutAssetType, and PutFormType's own required-uppercase-first-letter validation strongly implies the same identity-by-name shape); FormType.Id is set equal to Name since the real ID-generation algorithm is not discoverable from the public SDK shapes alone -- the same class of simplification this file already accepts for DevEndpoint's mock network fields (see PARITY notes below). PutAttachment/DeleteAttachment attach forms either directly to an asset or (via IterableFormName+ItemIdentifier) to an item within one of the asset's iterable forms; BatchGetIterableForms/ListIterableForms are read-only per the SDK, so an iterable-form item's entire existence in this backend is derived from PutAttachment having targeted it at least once -- there is no other creation path in the 31-op surface this pass covers (see iterableFormItemRecord's doc comment in assets.go). This is modeled as a deliberately NOT-store.Table raw nested map (InMemoryBackend.iterableFormItems) because its key is a 3-level nested collection, not a single value's own field; it is still fully covered by Snapshot/Restore (see state_and_persistence)."}
  dashboard_and_session_endpoint: {status: partial, note: "NEW this pass: GetDashboardUrl (JOB/SESSION dashboard URL) and GetSessionEndpoint (interactive session Spark Connect endpoint) both do a REAL existence check against this backend's job/session tables (EntityNotFoundException on an unknown resource, InvalidInputException on a ResourceType other than JOB/SESSION) and GetSessionEndpoint additionally real-checks the session isn't STOPPED/STOPPING (IllegalSessionStateException, confirmed as a documented error for this op). The URL/auth-token VALUES themselves are deterministic mock data, not backed by a real Glue Studio console or Spark Connect listener -- the same modeling choice already established and accepted in this file for DevEndpoint's YarnEndpointAddress/PrivateAddress/PublicAddress (no VPC/networking simulation exists anywhere in gopherstack). Marked partial rather than ok only because GetSessionEndpoint's state gate had to work around a PRE-EXISTING, unrelated gap noted while implementing this: Session.Status is set to PROVISIONING on CreateSession and nothing in this backend ever advances it to READY (no reconciler transition exists for sessions, unlike crawlers/job-runs/workflow-runs), so gating GetSessionEndpoint on READY would make it permanently unreachable in this backend; it is gated on 'not STOPPED/STOPPING' instead. Flagging the missing PROVISIONING->READY session transition for a future pass rather than expanding this one's scope to fix session lifecycle."}
  error_codes_global: {status: ok, note: "SEVERE systemic fix this pass: the shared ErrValidation sentinel wired \"ValidationException\" as its wire __type — confirmed against aws-sdk-go-v2/service/glue/deserializers.go that the vast majority of Create/Update/Delete operations (CreateDatabase, CreateTable, CreateJob, CreateCrawler, CreateTrigger, CreateBlueprint, CreateCustomEntityType, CreateUsageProfile, tag validation, ...) document InvalidInputException instead. Changed the shared sentinel + handler.go's hardcoded mapping to InvalidInputException, and fixed the ~8 existing tests that had encoded the wrong wire code. Also fixed awserrFromDetail (handler_stubs.go), which always wrapped batch-operation ErrorDetail as awserr.ErrNotFound regardless of the actual ErrorCode string — so e.g. an AlreadyExistsException detail from BatchCreatePartition surfaced to CreatePartition callers as EntityNotFoundException. Not touched: IdempotentParameterMismatchException, ResourceNumberLimitExceededException, OperationTimeoutException, ConcurrentModificationException remain unused — no account-level quota/concurrency-conflict modeling exists to trigger them realistically (bd: gopherstack-qd3.5)"}
  BatchGetDataQualityRulesetEvaluationRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-05 (SDK v1.152.0, new op): in: RunIds*[]string; out: Runs[]DataQualityRulesetEvaluationRun, RunsNotFound[]string; errors: InternalServiceException/InvalidInputException/OperationTimeoutException (no EntityNotFoundException -- unknown IDs go in RunsNotFound instead, confirmed absent from the op's own error switch). Real batch lookup against the same dataQualityEvalRuns table GetDataQualityRulesetEvaluationRun already reads, following BatchGetCrawlers' found/missing split shape exactly (crawlers.go)."}
  data_catalog_export_configuration: {status: partial, note: "2026-08-05 (SDK v1.152.0, new ops): Get/PutDataCatalogExportConfiguration. Unlike DataCatalogEncryptionSettings, these ops carry no CatalogId at all (confirmed absent from both Input structs) -- modeled as one backend-global (account+region) singleton, matching GetGlueIdentityCenterConfiguration's existing pattern (identity_center.go). PutDataCatalogExportConfiguration validates ExportSetting is ENABLED or DISABLED (InvalidInputException otherwise) and really stores EncryptionConfiguration/CreatedAt/UpdatedAt; GetDataCatalogExportConfiguration returns the real DISABLED default when never configured (same rationale already documented for GetDataCatalogEncryptionSettings' empty-default return). state=partial only because Status mirrors ExportSetting SYNCHRONOUSLY: real AWS transitions through ENABLING/DISABLING before settling (an actual async S3 Tables export pipeline standing up/tearing down), which this backend has nothing to simulate -- honest immediate settlement, not a fabricated transient state, but also not the real eventually-consistent timing. S3TableBucketArn has no corresponding field anywhere in PutDataCatalogExportConfigurationInput, so it is never populated -- see gaps."}
gaps:
  - "2026-08-23 (clientcoverage-driven audit): GetUnfilteredTableMetadata/GetUnfilteredPartitionMetadata/GetUnfilteredPartitionsMetadata (Lake Formation cell/row-level-filtering integration) are all missing several real output members with no backing state anywhere in this backend -- GetUnfilteredTableMetadataOutput's CellFilters/IsMaterializedView/IsMultiDialectView/IsProtected/Permissions/QueryAuthorizationId/ResourceArn/RowFilter, and GetUnfilteredPartitionsMetadataOutput's NextToken (confirmed against api_op_GetUnfilteredTableMetadata.go / api_op_GetUnfilteredPartitionsMetadata.go) -- this backend has no Lake Formation permissions/cell-filter engine anywhere (consistent with PutResourcePolicy's existing EnableHybrid note: 'Lake Formation console-grant state is not modeled anywhere in gopherstack'). IsRegisteredWithLakeFormation is left false rather than fabricated true. Left absent/false rather than invented."
  - "2026-08-23 (clientcoverage-driven audit): CreateCatalog/UpdateCatalog/GetCatalog(s) accept/return only Description/Parameters of the real types.CatalogInput/types.Catalog; the newer Lake Formation federation members (AllowFullTableExternalDataAccess, CatalogProperties, CreateDatabaseDefaultPermissions, CreateTableDefaultPermissions, FederatedCatalog, OverwriteChildResourcePermissionsWithDefault, TargetRedshiftCatalog -- types/types.go:1067-1107) have no backing state anywhere in this backend (no federated-catalog or Lake-Formation-permissions modeling exists for any resource kind). Left absent rather than invented."
  - "2026-08-23 (clientcoverage-driven audit): GetDataQualityResult's DataQualityResult model (models.go) stores only ResultID+Score; the real GetDataQualityResultOutput's AggregatedMetrics/AnalyzerResults/CompletedOn/DataSource/EvaluationContext/JobName/JobRunId/Observations/ProfileId/RuleResults/StartedOn (api_op_GetDataQualityResult.go) have no backing state -- this backend never runs a real data-quality evaluation, the same class already documented for ml_transforms' EvaluationMetrics gap. Left absent rather than invented."
  - "2026-08-23 (clientcoverage-driven audit): MLTaskRun (models.go) has no LastModifiedOn field (real TaskRun/GetMLTaskRunOutput both have it, api_op_GetMLTaskRun.go); Properties is typed map[string]string on this backend's model but the real field is *types.TaskRunProperties (a TaskType + 4 nested per-task-kind sub-structs), and no code path in this backend ever populates it, so the type mismatch is currently inert (always omitted, never observed by a real client) rather than an active bug. See the GetMLTaskRun op note above for the fields that WERE fixed this pass."
  - "2026-08-13 (gopherstack-ustu): ListConnectionTypes' ConnectionTypeBrief.DisplayName/LogoUrl/Vendor/ConnectionTypeVariants (types.ConnectionTypeBrief, glue@v1.152.0 types/types.go:2533-2564) have no corresponding backing state anywhere in this backend (no per-connector display name/logo/vendor/variant catalog exists) -- left absent rather than invented."
  - "2026-08-05: DataCatalogExportConfiguration.S3TableBucketArn (GetDataCatalogExportConfigurationOutput field) is real AWS-managed state -- the actual S3 Tables bucket ARN backing the export -- with no corresponding input field anywhere in this API (confirmed absent from PutDataCatalogExportConfigurationInput). There is no way to honestly derive it, so it is always left empty rather than fabricated."
  - "2026-08-05: DataCatalogExportConfiguration.Status's ENABLING/DISABLING transient states (real AWS's async S3 Tables export pipeline standing up/tearing down) are not modeled -- this backend has no such pipeline, so Status settles to ENABLED/DISABLED synchronously with the Put call. Honest (no fabricated FAILED occurrences or invented settlement delay), just not eventually-consistent like real AWS."
  # All 7 gaps tracked at the start of this pass are fixed — see the ops/families
  # notes above for each. Kept here (marked FIXED) rather than deleted so the
  # bd issue IDs remain traceable; close the corresponding bd issues separately.
  - "FIXED this pass: CrawlerTarget missing DynamoDBTargets/DeltaTargets/HudiTargets/IcebergTargets/MongoDBTargets (bd: gopherstack-qd3.1)"
  - "FIXED this pass: CreateCrawler/UpdateCrawler missing SchemaChangePolicy, RecrawlPolicy, LineageConfiguration, CrawlerSecurityConfiguration, LakeFormationConfiguration (bd: gopherstack-qd3.2)"
  - "FIXED this pass: DatabaseInput/Database missing Parameters, LocationUri, CreateTableDefaultPermissions, TargetDatabase (bd: gopherstack-qd3.3)"
  - "FIXED this pass: StartJobRun has no per-run capacity/argument overrides (bd: gopherstack-qd3.4)"
  - "PARTIALLY FIXED this pass (gopherstack-dol3, was gopherstack-qd3.5): re-researched the 4 quota/idempotency exceptions properly this time (WebFetch against real AWS docs, not memory) instead of leaving all 4 blanket-open. FIXED: ResourceNumberLimitExceededException, now real and enforced for CreateDevEndpoint against AWS's actual published default quota (docs.aws.amazon.com/general/latest/gr/glue.html: 'Max development endpoint per account: 25', confirmed present in CreateDevEndpoint's real error catalog). AWS publishes real default quotas for many other Glue resource kinds too (jobs: 2,000; databases: 10,000; crawlers: 1,000; triggers: 1,000; workflows: 1,000; connections: 1,000; security configurations: 250; ML transforms: 100; functions per account/database: 100) — dev endpoints was implemented as the one cleanly testable example (small enough to create N+1 in a fast unit test); wiring the rest is a mechanical follow-up now that the real numbers are sourced and cited here, not a research problem. STILL OPEN, now with real reasoning instead of a blanket statement: (1) IdempotentParameterMismatchException — AWS's real doc text is 'The same unique identifier was associated with two different records' (verified via WebFetch against docs.aws.amazon.com/glue/latest/webapi/API_CreateJob.html); it appears in the real error catalogs of CreateCustomEntityType/CreateDevEndpoint/CreateJob/CreateMLTransform/CreateSession/CreateTrigger/UpdateDataQualityRuleset/UpdateWorkflow (verified by scanning deserializers.go's per-op error switches), but NONE of these operations have a ClientToken/RequestToken field on their real Input types (verified) unlike e.g. CreateDataQualityRuleset which DOES have ClientToken but is NOT in this exception's op list — so the exact 'two different records, same identifier' trigger condition for the ops that actually list it isn't confidently derivable from the SDK alone, and guessing wrong risks silently changing today's correct AlreadyExistsException behavior for a duplicate Name. Genuinely deferred pending real AWS testing/documentation, not skipped out of laziness. (2) ConcurrentModificationException — real text: 'Two processes are trying to modify a resource simultaneously.' Structurally unreachable in this backend by design: every operation holds the coarse per-backend b.mu.Lock for its full duration (see pkgs-catalog.md's locking rule), so two operations against the same resource can never actually race at the data level — there is no real concurrent-modification condition to detect. (3) OperationTimeoutException — real text: 'The operation timed out.' No operation in this synchronous in-memory backend can genuinely time out; would require fabricating an arbitrary timeout threshold with nothing real behind it. (bd: gopherstack-qd3.5/dol3). Note: ConcurrentRunsExceededException — a DIFFERENT, distinct exception from ConcurrentModificationException — WAS found unused and fixed in a prior pass (see StartJobRun/workflows notes above); do not conflate the two when re-auditing."
  - "FIXED this pass: Trigger/TriggerAction missing Description, EventBatchingCondition, WorkflowName, and the AWS \"max 2 crawler actions per trigger\" soft limit is now enforced (bd: gopherstack-qd4.1)"
  - "FIXED this pass: PutResourcePolicy did not model EnableHybrid (bd: gopherstack-qd4.2)"
  - "FIXED this pass (gopherstack-dol3): TagResource/UntagResource/GetTags now recognize Blueprint/DevEndpoint/MLTransform/UserDefinedFunction ARNs — see the TagResource/UntagResource/GetTags op notes above for the full fix (dispatch + the deeper creation/update tag-loss bugs found alongside it). STILL OPEN: CustomEntityType has no ARN or Tags concept modeled in this backend at all (no ARN-building helper, no Tags field, CreateCustomEntityType's wire input doesn't even accept tags) — out of this pass's scope (the bd issue named Blueprint/DevEndpoint/MLTransform/UDF specifically, not CustomEntityType), and adding it from scratch is a larger lift than extending the other four's existing-but-undispatched Tags support."
  - "NEW gap FOUND (not introduced) this pass (parity-4): Session.Status is set to PROVISIONING on CreateSession and this backend has no reconciler transition that ever advances it to READY, unlike crawlers/job-runs/workflow-runs which all do reach a terminal running/ready state. This was surfaced while implementing GetSessionEndpoint (bd note: had to gate on 'not STOPPED/STOPPING' instead of the more natural READY check -- see dashboard_and_session_endpoint family note). Fixing session lifecycle is out of scope for this pass; flagging for whichever pass owns sessions.go."
  - "gopherstack-a250 (empty-struct-input sweep): 32 `type <Op>Input struct{}` candidates found via `grep -n '^type [A-Za-z]*Input struct{}' services/glue/*.go`. 2 confirmed genuinely correct (see gopherstack-awzv note below); the other 30 were split into follow-up gopherstack-awzv, now FIXED — see that note."
  - "gopherstack-awzv (empty-struct-input follow-up, 2026-08-13): all 29 real ops from the gopherstack-a250 split now wire MaxResults/NextToken via the existing paginateSlice helper (handler.go), matching ListCrawls' pre-existing pagination convention. Filter/Tags wired wherever the stored entity honestly backs the field; documented inert (accepted on the wire, never fabricated) where it doesn't. Real Filter/Tags now wired: ListBlueprints/ListCrawlers/ListDevEndpoints/ListJobs/ListTriggers/ListDataQualityRulesets/ListMLTransforms Tags (all route through tags.go's generic tag dispatch); GetCatalogs.HasDatabases (real, via Database.CatalogId); GetConnections.Filter.ConnectionType/MatchCriteria and HidePassword (real, redacts ConnectionProperties[\"PASSWORD\"]); GetTriggers/ListTriggers.DependentJobName (real, including the 'fall back to every trigger when nothing matches' semantics from api_op_GetTriggers.go); ListDataQualityRulesets.Filter (Name/Description/CreatedAfter/CreatedBefore/LastModifiedAfter/LastModifiedBefore/TargetTable, all backed); GetMLTransforms/ListMLTransforms.Filter (Name/GlueVersion/Status/Schema/timestamps) and .Sort (NAME/STATUS/CREATED/LAST_MODIFIED); DescribeIntegrations.Filters (Status/IntegrationName/SourceArn, the three keys the op's own doc comment names) and .IntegrationIdentifier; ListMaterializedViewRefreshTaskRuns.DatabaseName/TableName; ListDataQualityRuleRecommendationRuns/ListDataQualityRulesetEvaluationRuns.Filter.StartedAfter/StartedBefore and (evaluation runs only) RulesetName. Inverse bug found and fixed in the same pass: GetColumnStatisticsTaskRuns previously ignored its own required DatabaseName/TableName members entirely (not just MaxResults/NextToken) and returned every column-statistics run in the account regardless of table — now scoped. Two pre-existing wire-shape bugs also found (via the first-ever real-SDK-client tests these ops got) and fixed as part of the same functions: ListMaterializedViewRefreshTaskRunsOutput's member was named `Runs` instead of the real `MaterializedViewRefreshTaskRuns`, and MaterializedViewRefreshRun's JSON tags were `TaskRunId`/`StartedOn` instead of the real `MaterializedViewRefreshTaskRunId`/`StartTime` (models.go); DescribeIntegrationsOutput.Integrations and ListUsageProfilesOutput.Profiles were dumping their raw backend struct (Integration.CreatedAt / UsageProfile.CreatedOn are time.Time, which json.Marshal renders as an RFC3339 string) instead of an epoch float via pkgs/awstime, which a real client rejects (\"expected ... to be a JSON Number, got string instead\"); ListRegistriesOutput's RegistryListItem.CreatedTime/UpdatedTime were float64 when the real type is `*string` (Glue Schema Registry timestamps are a documented exception to the rest of the service's unixTimestamp convention) — now formatted as RFC3339 strings. Documented inert (real member, no honest backing, accepted on the wire and never fabricated): GetCatalogs.IncludeRoot/ParentCatalogId/Recursive (CatalogEntry has no parent-catalog field; this backend's b.catalogs table is flat with no root-catalog concept); GetConnections.CatalogId and Filter.ConnectionSchemaVersion (Connection has neither a CatalogId nor a schema-version field); ListCustomEntityTypes.Tags and ListSessions.Tags/RequestOrigin (CustomEntityType and Session are never routed through tags.go's dispatch, and CreateSession doesn't even accept a RequestOrigin to store); GetMLTransforms/ListMLTransforms.Filter.TransformType and Sort.Column=TRANSFORM_TYPE (MLTransform models only one transform kind, no TransformType field); ListMaterializedViewRefreshTaskRuns.CatalogId (flat namespace, no per-catalog scoping, consistent with how the rest of this service treats the account's implicit single catalog); ListDataQualityRuleRecommendationRuns/ListDataQualityRulesetEvaluationRuns.Filter.DataSource (DQRuleRecommendationRun only stores a flat DataSourceS3Path string and DataQualityEvaluationRun stores no data-source link at all — neither has the structured types.DataSource{GlueTable} a real filter would compare against); ListDataQualityResults.Filter in its entirety (DataQualityResult stores only ResultID+Score — DataSource/JobName/JobRunId/StartedAfter/StartedBefore have no field to compare against on the stored entity). Test coverage: services/glue/handler_pagination_sweep_sdk_test.go (MaxResults truncation + NextToken resume, all 29 ops, driven through the real aws-sdk-go-v2 client) and services/glue/handler_filter_sweep_sdk_test.go (Tags/Filter/Sort round trips, the DependentJobName fallback semantics, and the GetColumnStatisticsTaskRuns scoping fix); every new assertion hand-verified to fail against the pre-fix behavior (paginateSlice/matchesTagFilter/sortTransforms/matchesIntegrationFilters and each inline filter block temporarily neutralized one at a time, confirmed red, then restored). NOT touched at the time (separate, smaller pre-existing bugs found along the way, out of this issue's scope): DescribeInboundIntegrationsInput already declares Marker/MaxRecords but neither actually paginates, and its Integrations output has the identical raw-struct timestamp bug DescribeIntegrations had; handler_schemas.go's GetRegistry/GetSchema/ListSchemas/ListSchemaVersions/GetSchemaVersion share ListRegistries' pre-fix CreatedTime/UpdatedTime float-vs-string bug (same root cause, same fix shape, but a systemic sweep across a whole file that was never part of this issue's flagged 30 ops). Both fixed under gopherstack-7f5k, see the dated note at the top of this file."
deferred:
  # Every family below was field-diffed against the pinned SDK this pass (none
  # left un-audited). Families now fully closed (status: ok in the table above)
  # are removed from this list; families with a genuine remaining gap keep a
  # one-line pointer to the families note above (which has the full reasoning).
  - "workflows: Graph, LastRun and WorkflowRunStatistics are now real (gopherstack-dol3/gopherstack-vcor, see workflows op note); BlueprintDetails and WorkflowRun.Graph's per-node run details (JobDetails.JobRuns/CrawlerDetails.Crawls) remain unmodeled -- the job/crawler-run-to-workflow-run link they'd need now exists (gopherstack-vcor), but converting it into per-node run lists is separate work not done this pass; also, this backend never evaluates conditional (predicate-gated) triggers, so only a workflow's entry trigger ever links actions to a run"
  - "schema registry: Compatibility enum validation and DISABLED-mode enforcement implemented for real (gopherstack-j1b7, see schema_registry op note above). BACKWARD/FORWARD/FULL/BACKWARD_ALL/FORWARD_ALL/FULL_ALL compatibility-mode enforcement and full AVRO/JSON/PROTOBUF grammar validation depth remain deferred -- both would need real schema-parsing/diffing libraries per DataFormat (no new go.mod deps permitted); sized in gopherstack-dol3, re-confirmed still the right call in gopherstack-j1b7, see 'DQDL and schema-compatibility sizing' note below; not started"
  - "data quality rulesets: DQDL syntax / rule-type validation (would need a real DQDL parser) -- sized this pass (gopherstack-dol3), re-confirmed out of scope in gopherstack-j1b7 (no code touched -- see 'DQDL and schema-compatibility sizing' note below); not started"
  - "ML transforms: EvaluationMetrics (FindMatchesMetrics) — no real ML evaluation is ever run, so there is no real metric to report (re-confirmed gopherstack-dol3, still correctly absent)"
  - "quota/idempotency exceptions: ResourceNumberLimitExceededException now real for CreateDevEndpoint (gopherstack-dol3); IdempotentParameterMismatchException/OperationTimeoutException/ConcurrentModificationException remain open with real (not blanket) reasoning -- see the quota/idempotency gap-list note above"
  - "tag ARN dispatch: Blueprint/DevEndpoint/MLTransform/UserDefinedFunction fixed (gopherstack-dol3); CustomEntityType still has no ARN/Tags concept at all, out of scope -- see the tag-dispatch gap-list note above"
leaks: {status: clean, note: "backend_reconciler.go's managed goroutine (StartReconciler/StopReconciler/reconcileLoop) already exits deterministically on ctx.Done() or the stop channel with a WaitGroup — no unmanaged 'go b.runReconciler()' leak. Verified with go test -race this pass too; no new goroutines/timers/tickers introduced (all new run-tracking state — DevEndpoint/Blueprint/MLTransform fields, StartJobRunOptions, CrawlerOptions additions — is plain struct state guarded by the existing coarse b.mu, not new concurrency). No new ghost-map-row risk: no new child/FK resource maps were introduced this pass (all additions are fields on existing resource structs or new sub-structs embedded inline), so no new cascade-delete paths were needed. VERIFIED, NOT A LEAK (gopherstack-8907, 2026-09-06): DeleteJob clears b.jobRuns[name] but not jobRunReadyAt/DoneAt/TimeoutAt/StopAt directly -- pruneOrphanJobRunTimersLocked (called at the end of every reconcileLocked, and reconcileLocked is triggered lazily by any read plus at the top of every StartJobRun) is the mechanism that actually drops the now-orphaned timer entries once their deadline has passed. This was previously untested for the delete-then-prune path specifically; added TestReconciler_DeleteJob_PrunesOrphanedTimers (neuter-verified against reconcileLocked's pruneOrphanJobRunTimersLocked call) rather than adding a new exported seam for the timer maps."}
---

## Notes

- **Protocol**: json-1.1 (`X-Amz-Target: AWSGlue.<Op>`, `application/x-amz-json-1.1`),
  confirmed against `aws-sdk-go-v2/service/glue/deserializers.go`'s
  `awsAwsjson11_deserializeOpError<Op>` switch statements. Error responses use
  `{"__type": "<ExceptionName>", "message": "..."}`.

- **ValidationException vs InvalidInputException (important, easy to re-flag by
  mistake)**: Glue's SDK error model genuinely contains BOTH exception types.
  `ValidationException` IS a real type in `types/errors.go`, and a handful of newer
  operations (confirmed: `DeleteConnectionType`) do declare it as a documented error.
  But the overwhelming majority of hand-validation call sites in this backend
  (name-length checks, tag-limit checks, required-field checks across
  Create/Update/Delete for databases/tables/jobs/crawlers/triggers/blueprints/
  custom-entity-types/usage-profiles) correspond to AWS operations whose deserializer
  switch lists `InvalidInputException`, not `ValidationException` — confirmed by
  reading the actual `awsAwsjson11_deserializeOpErrorCreateXxx` functions in
  `deserializers.go` for CreateDatabase, CreateTable, CreateJob, CreateCrawler,
  CreateTrigger, CreateBlueprint, CreateCustomEntityType, CreateUsageProfile. Since
  `ErrValidation` is one shared sentinel used everywhere, the fix picks the option
  that's correct for the large majority of call sites. Do not "fix" this back to
  ValidationException without checking the SDK deserializer for the specific op in
  question first.

- **`awserrFromDetail` (handler_stubs.go)**: single-item AWS ops that are implemented
  by calling a batch backend method with a one-element slice (CreatePartition →
  BatchCreatePartition, DeletePartition → BatchDeletePartition) surface
  `errs[0].ErrorDetail` as a real Go error via this helper. It must switch on
  `d.ErrorCode` to pick the matching sentinel (AlreadyExists vs NotFound vs generic
  invalid-parameter) — do not revert it to unconditionally wrapping
  `awserr.ErrNotFound`, or AlreadyExistsException details get reported to SDK callers
  as EntityNotFoundException.

- **StorageDescriptor is shared by Table AND Partition** in real Glue (partitions
  carry their own StorageDescriptor that can override table-level SerDe/format
  settings). Because `CreateTable`/`UpdateTable`/`BatchCreatePartition`/
  `UpdatePartition` already copy the whole `StorageDescriptor` struct by value from
  the request input, adding fields to the `StorageDescriptor`/`Column` type
  definitions was sufficient to flow them through end-to-end — the remaining real
  work was fixing `cloneTable`/`clonePartition`/`cloneCrawler` to deep-copy the new
  nested maps/slices/pointers (Parameters maps, SerdeInfo pointer, BucketColumns/
  SortColumns slices, per-Column Parameters) so that `GetTable`/`GetPartitions`
  callers can't mutate live backend state through the returned pointers.

- **`CreateCrawler`/`UpdateCrawler` signature is called from
  `services/cloudformation/resources_phase5.go`** (outside this package) with the
  original 5-arg / 4-arg positional signatures. Per the audit's signature-safety
  rule, those signatures were left untouched; new capability (Schedule, Classifiers,
  Configuration, TablePrefix, Description) was added via new
  `CreateCrawlerWithOptions`/`UpdateCrawlerWithOptions` methods that the old methods
  now delegate to with a zero-value `CrawlerOptions`. The `StorageBackend` interface
  gained the two new methods additively; `InMemoryBackend` is the only implementer
  (verified — no mocks reference `StorageBackend` in this package's tests), so this
  is safe.

- **GetTables aliasing bug**: `GetTables` was the one read path in the whole backend
  that hadn't been updated to clone before returning (`GetDatabases`, `GetCrawlers`,
  `GetJobs`, `GetConnections`, `SearchTables`, `GetPartition(s)` all already cloned).
  Fixed to match the established pattern; verified no other `Get*` list method has
  the same gap.

## This pass (gopherstack-ustu: DescribeConnectionType/ListConnectionTypes Capabilities shape)

Follow-up to gopherstack-u90v: `Capabilities` on both
`DescribeConnectionTypeOutput` and `ConnectionTypeBrief` was fabricated as a
bare `[]string` of `"READ"`/`"WRITE"`; the real field on both is
`*types.Capabilities` (`SupportedAuthenticationTypes`/
`SupportedComputeEnvironments`/`SupportedDataOperations`, all required,
confirmed `glue@v1.152.0 types/types.go:872-890`). A real
`aws-sdk-go-v2` client's json-1.1 deserializer expects an object there, not
an array, and rejects the whole response body on the mismatch -- so
`DescribeConnectionType`/`ListConnectionTypes` were entirely undecodable by a
real client, not just missing a field.

1. **Fixed the primary bug.** Added a local `connectionCapabilities` struct
   (`handler_connection_types.go`) reproducing the real shape (this backend
   hand-rolls JSON wire structs rather than importing SDK types at runtime,
   the established repo convention) and a `toConnectionCapabilities` helper.
   This backend's existing per-type `READ`/`WRITE` data (`rwCaps`/`readCaps`)
   is real state that maps exactly onto `SupportedDataOperations`
   (`types.DataOperation`'s only two enum values are literally `"READ"`/
   `"WRITE"`, `types/enums.go:979-986`) -- threaded through, not discarded.
   `SupportedAuthenticationTypes`/`SupportedComputeEnvironments` have no
   backing state anywhere in this backend (no per-connector auth-type or
   compute-environment tracking exists), so they are modeled as real,
   present, empty slices when `Capabilities` itself is present -- not
   fabricated. `Capabilities` is not itself a required member on either op's
   output, so it is omitted entirely (not an empty-but-present object) for
   connector categories this backend never tracked `DataOperations` for
   (`NETWORK`/`MARKETPLACE`/`CUSTOM`).

2. **Second-layer find, from reading `ConnectionTypeBrief`'s full real
   shape rather than only the field named in the bug report**:
   `ConnectionTypeBrief.Category` should be `Categories` (plural,
   `[]string` -- confirmed `types/types.go:2533-2564`), a distinct wire-shape
   bug not called out in the issue that filed this fix.
   `DescribeConnectionTypeOutput` genuinely has no `Category` member at all
   (already correctly documented by the prior pass, now actually removed
   rather than left as a fabricated field). This backend's
   `ConnectionTypeInfo` only ever models one category per type, so
   `Categories` is echoed as the one-element list that shape implies, not
   fabricated into several. `DisplayName`/`LogoUrl`/`Vendor`/
   `ConnectionTypeVariants` are also real `ConnectionTypeBrief` members with
   no backing state anywhere in this backend -- deliberately left absent
   rather than invented, see `gaps`.

3. **The raw-HTTP test workaround is gone.**
   `TestSDKRoundTrip_RegisterConnectionType_EchoesRequiredMembers`
   (`handler_register_connection_type_test.go`) previously had to fall back
   to a raw `doGlueRequest`/`json.Unmarshal` call for its `DescribeConnectionType`
   assertion, with a comment explaining the real client couldn't decode the
   response at all. It now drives `DescribeConnectionType` through the same
   real client as the rest of the test. New tests added:
   `TestSDKRoundTrip_DescribeConnectionType_Capabilities`,
   `TestSDKRoundTrip_DescribeConnectionType_NoCapabilitiesState`,
   `TestSDKRoundTrip_ListConnectionTypes` (all real-client round trips), and
   `TestConnectionTypeWireShape` (raw-JSON assertions independent of the
   SDK client's own tolerance, locking in the object-not-array shape and the
   absence of the fabricated `Category` key). All new/changed tests
   hand-verified to fail against the pre-fix shape (temporarily reverted
   `handler_connection_types.go`'s two output structs and the `ListConnectionTypes`
   loop to their pre-fix form, confirmed the expected failures, restored).

Gates run this pass, all green: `go build`, `go vet`, `go test -race`,
`go fix -diff` (no diff), `golangci-lint run` (0 issues).

## This pass (gopherstack-vcor: workflow run statistics link)

Deferred from gopherstack-dol3: WorkflowRunStatistics needed a real link from
job/crawler runs to the workflow run that triggered them before it could be
reported honestly.

1. **Established the real shape first.** Read aws-sdk-go-v2/service/glue@v1.152.0
   (`types.go`) and the pinned botocore model (`glue/2017-03-31/service-2.json.gz`)
   directly rather than assuming: `WorkflowRunStatistics` has 8 int32 counters
   (TotalActions plus 7 outcome buckets); neither `JobRun` nor `Crawl`/
   `CrawlerHistory` carries a `WorkflowRunId` on the wire anywhere -- the closest
   real field is `JobRun.TriggerName` (types.go:7350-7351), which only names the
   trigger, not the specific run (a workflow can have concurrent runs up to
   `MaxConcurrentRuns`). Confirmed via `workflows_overview.html` that each workflow
   has a "start trigger" fired by `StartWorkflowRun` itself.

2. **Built the link.** `StartWorkflowRun` previously only wrote a `WorkflowRun`
   bookkeeping record and fired nothing -- a real, separate bug this fix also
   closes (see `TestPrefixProbe...` style evidence in the PR/session notes: 0 job
   runs and 0 crawls existed after `StartWorkflowRun` before this change). It now
   fires the workflow's entry-point trigger(s) (`WorkflowName` match, no
   `Predicate`) and stamps the new run's ID onto the resulting job runs/crawls via
   an internal-only `WorkflowRunID` field (not on the wire; AWS has no such field)
   that persists through Snapshot/Restore. `JobRun.TriggerName` is now also
   populated for the first time, since it's a real field this backend had never
   set.

3. **Statistics are computed live, never stored**, from that link, in
   `computeWorkflowRunStatisticsLocked` (workflows.go) -- so they can't go stale
   independent of the runs they describe.

4. **Deliberately left out**: conditional (predicate-gated) trigger chains never
   fire on their own in this backend (no predicate-evaluation engine watches
   job/crawler completions), so a workflow run's Statistics only ever reflects its
   entry trigger's direct actions, not a full downstream DAG execution -- an
   honest reflection of what this backend can actually run, not an approximation.
   `WorkflowRun.Graph`'s per-node run details (`Node.JobDetails.JobRuns`/
   `CrawlerDetails.Crawls`) and `BlueprintDetails` remain unmodeled; the link now
   exists to build the former, but doing so is separate work.

## This pass (gopherstack-dol3: tag ARN dispatch, workflow DAG, quota exceptions, DQDL sizing)

Five-part bd issue; did the first four accurately, sized the fifth without
starting it (per the issue's own instruction: assess DQDL size first, don't
start it if it's a project of its own).

1. **Tag ARN dispatch (Blueprint/DevEndpoint/MLTransform/UDF)** — done. See
   the `TagResource`/`UntagResource`/`GetTags` op notes above for the
   dispatch fix plus the two deeper bugs found alongside it (MLTransform/UDF
   had no Tags field at all, so creation-time tags were silently lost, not
   just unreachable; and `UpdateMLTransform`/`UpdateUserDefinedFunction`
   replace the whole record and were silently wiping tags on every update).
   Checked first whether a shared cross-service tag-ARN-dispatch mechanism
   exists in `pkgs/` per this repo's convention of reusing consolidated
   packages: it does not — `pkgs/tags` is a plain concurrency-safe map type,
   `pkgs/arn` only builds ARN strings, neither does resource-type dispatch.
   Other multi-resource-type services either replicate Glue's per-kind
   `find*ByARN` chain (no shared package) or sidestep the problem entirely
   with a flat ARN-keyed side map (ECS-style, only viable for single-kind
   resources or when tags aren't stored inline per typed resource, which
   Glue's `TaggedResources` doc comment already documents as a deliberate
   prior-pass choice). Extended the existing chain-of-if pattern rather than
   introducing a new abstraction.

2. **Workflow DAG (Graph/LastRun/statistics)** — Graph and LastRun done for
   real; statistics (and WorkflowRunStatistics/WorkflowRun.Graph generally)
   confirmed to need real per-run job/crawler-run correlation this backend
   doesn't have anywhere, so left absent. See the `workflows` op note above.

3. **ml-transform EvaluationMetrics** — re-confirmed correctly absent (no
   real ML evaluation is ever run in this backend); no code change. Fixed a
   related pre-existing bug (Tags being lost) while in this file for #1.

4. **Quota/idempotency/concurrency exceptions** — implemented one real,
   AWS-quota-verified exception (`ResourceNumberLimitExceededException` for
   `CreateDevEndpoint`, quota = 25, sourced from
   `docs.aws.amazon.com/general/latest/gr/glue.html` via WebFetch this pass,
   not memory). The other three were re-researched with real AWS
   documentation (WebFetch against `API_CreateJob.html`'s Errors section for
   the exact exception text) instead of the prior pass's blanket "no state to
   trigger these" — see the quota/idempotency gap-list note above for the
   full per-exception reasoning on why `IdempotentParameterMismatchException`
   was left open (real trigger condition not confidently derivable from the
   SDK alone) and why `ConcurrentModificationException`/
   `OperationTimeoutException` are structurally unreachable (coarse per-backend
   lock means no operation can ever race; nothing in a synchronous in-memory
   backend can genuinely time out).

5. **Schema-registry compatibility checking + DQDL validation — sized, not
   started.** Both are genuinely substantial, separable projects:
   - **DQDL** (Data Quality Definition Language) is AWS Glue's own rule DSL
     for `CreateDataQualityRuleset`'s `Ruleset` string (e.g. `Rules = [
     IsComplete "col", ColumnValues "col" between 0 and 100 ]`). A real
     implementation needs a lexer + parser for the full grammar (dozens of
     rule types — IsComplete, IsUnique, ColumnValues, ColumnCount, RowCount,
     Completeness, Uniqueness, DataFreshness, custom-SQL rules, composite
     AND/OR/NOT expressions, thresholds with `>`/`<`/`between`/percentages —
     see `docs.aws.amazon.com/glue/latest/dg/dqdl.html`), plus a decision on
     how much of it to actually *evaluate* against real table data (today
     `data_quality_stats.go`'s results are backend-tracked scores, not
     computed from the ruleset). This is comparable in scope to
     `pkgs/dynamodb/expr` (DynamoDB's expression parser) or a small SQL
     WHERE-clause parser — a standalone package, not a file-sized change.
   - **Schema-registry compatibility** needs a real per-`DataFormat`
     schema-diffing algorithm (AVRO/JSON/PROTOBUF each have their own
     compatibility rules — e.g. Avro's reader/writer schema resolution:
     field addition requires a default, type widening rules, etc.) for each
     of BACKWARD/FORWARD/FULL/BACKWARD_ALL/FORWARD_ALL/FULL_ALL. Realistic
     options are (a) hand-write simplified per-format diffing (weeks of edge
     cases to get right, easy to be subtly wrong in a way that's worse than
     absent) or (b) pull in a real schema library per format — the prior
     pass's ledger already noted "no new go.mod dependencies permitted" as a
     hard constraint, which rules out (b) without a policy exception.
   - **What it needs to be picked up**: (i) a decision on whether new
     go.mod dependencies are permitted for this specific feature (schema
     libraries especially — hand-rolled AVRO/PROTOBUF compatibility
     checking from scratch is high-risk); (ii) its own bd issue(s), separate
     from the other four items in this one, given the size difference; (iii)
     for DQDL specifically, a decision on scope — syntax validation only
     (reject malformed `Ruleset` strings, still don't evaluate rules against
     data) is a meaningfully smaller first slice than full rule evaluation,
     and would be the natural place to start.
   Not started this pass, per the issue's own instruction not to start it if
   it's a project of its own.

## This pass (gopherstack-i60f: CreateSchema can now carry the initial definition)

Filed as a follow-up the same day gopherstack-j1b7 landed DISABLED
enforcement: that fix's own note flagged that whoever added
`SchemaDefinition` to `CreateSchema` would need to re-check the DISABLED
interaction, since the real `CreateSchemaInput` has always carried it
(`aws-sdk-go-v2/service/glue@v1.152.0 api_op_CreateSchema.go:106`) but
gopherstack's wire input never did.

1. **The gap.** A real client creating a schema with its initial definition
   in one call -- the documented AWS flow -- got back a schema with no
   versions and no error, because `CreateSchema`'s wire input had no
   `SchemaDefinition` field to drop it into. Confirmed pre-fix by trying to
   call the backend the way a fixed version would need to: it did not
   compile, there being no parameter to pass the definition through.

2. **The fix.** `CreateSchema` now takes an optional `schemaDefinition`
   parameter. Empty behaves exactly as before (schema created with zero
   versions, `LatestSchemaVersion`/`NextSchemaVersion` at 0/1). Non-empty:
   the definition is validated with the same `validateSchemaDefinition`
   `RegisterSchemaVersion` already used (j1b7), and an invalid definition
   fails the whole call -- no schema is created, matching the atomic
   "create the schema set and register the version" framing in
   `api_op_CreateSchema.go`'s own package doc. On success, the schema and
   its first `SchemaVersion` (`VersionNumber: 1`) are created together, and
   `LatestSchemaVersion`/`NextSchemaVersion` move to 1/2.

3. **DISABLED interaction, checked both ways as j1b7's note asked.**
   `RegisterSchemaVersion`'s DISABLED check keys off `LatestSchemaVersion >
   0`, not off which operation last touched it, so it needed no change:
   creating with a definition sets `LatestSchemaVersion=1` immediately,
   correctly refusing a following `RegisterSchemaVersion`; creating without
   one leaves it at 0, correctly still permitting the first
   `RegisterSchemaVersion`. Both paths are asserted directly in
   `Test_CreateSchema_DisabledCompatibility_VersionSlotInteraction`.

4. **Response shape.** `CreateSchemaOutput` real fields
   `LatestSchemaVersion`/`NextSchemaVersion`/`SchemaCheckpoint`/
   `SchemaVersionId`/`SchemaVersionStatus` (`api_op_CreateSchema.go:129-157`)
   were entirely absent from gopherstack's output regardless of this fix --
   a create that stored a definition but reported nothing about the version
   it made would have been the same silent-drop bug moved one step. All
   five are now populated from the same backend state `DescribeSchema`
   already exposed; the two version-identity fields are `omitempty` and
   absent when no definition was supplied, matching the real API's
   nil-pointer-when-unset shape.

5. **Tests**: `services/glue/registry_test.go` gained
   `Test_CreateSchema_WithDefinition`, `Test_CreateSchema_RejectsInvalidDefinition`,
   `Test_CreateSchema_DisabledCompatibility_VersionSlotInteraction`, and
   `Test_CreateSchema_WithDefinition_RoundTripsThroughPersistence`;
   `services/glue/handler_schemas_test.go` gained
   `TestCreateSchema_WithDefinition_PopulatesVersionFields` and
   `TestCreateSchema_RejectsInvalidDefinition` at the wire level. No
   existing test asserted the absence of a version id from `CreateSchema` --
   the gap had no coverage to begin with, not a green test that would have
   needed correcting.

## This pass (gopherstack-j1b7: DQDL + schema-registry compatibility)

Picked up the two items gopherstack-dol3 sized but deliberately didn't start.
Re-established semantics for both from the pinned SDK before writing
anything, and treated them as separable: one was genuinely bounded, one
remains a standalone-project-sized gap.

1. **Schema-registry compatibility — bounded slice implemented for real.**
   The compatibility modes are a fixed 8-value enum (`types.Compatibility`,
   aws-sdk-go-v2/service/glue@v1.152.0 types/enums.go:328-354): NONE,
   DISABLED, BACKWARD, BACKWARD_ALL, FORWARD, FORWARD_ALL, FULL, FULL_ALL.
   Two of these need no schema-diffing algorithm at all to enforce
   correctly:
   - **Enum validation**: `CreateSchema`/`UpdateSchema` previously accepted
     any string as `Compatibility` — a schema could be created with e.g.
     `Compatibility: "SOMETIMES"` and it would silently do nothing. Now
     rejected with `InvalidInputException` (confirmed in both operations'
     real error catalogs, deserializers.go's
     `awsAwsjson11_deserializeOpErrorCreateSchema`/`...UpdateSchema` switches
     both list `InvalidInputException`).
   - **DISABLED enforcement**: per `api_op_CreateSchema.go:14-18`, "DISABLED
     restricts any additional schema versions from being added after the
     first schema version," and `api_op_RegisterSchemaVersion.go:17-18`
     confirms the first version is always accepted regardless of mode ("If
     this is the first schema definition to be registered in the Schema
     Registry, this API will store the schema version and return
     immediately"). This was previously unenforced — `RegisterSchemaVersion`
     never looked at `Compatibility` at all. Now a second (or later)
     `RegisterSchemaVersion` call against a DISABLED schema is rejected with
     `InvalidInputException`, matching `RegisterSchemaVersion`'s real error
     catalog (deserializers.go's
     `awsAwsjson11_deserializeOpErrorRegisterSchemaVersion`: AccessDenied /
     ConcurrentModification / EntityNotFound / InternalService /
     **InvalidInputException** / ResourceNumberLimitExceeded — no dedicated
     "incompatible schema" exception exists on the wire, so InvalidInput is
     the only semantically-fitting code). This is a complete, faithful
     implementation of DISABLED's documented behavior: it needs a version
     count, not a diff, so there is no approximation risk. (Correction added
     the same day, gopherstack-i60f: at the time this was written,
     `RegisterSchemaVersion` was the only way to register a first version at
     all, so "the first version is always accepted regardless of mode" reads
     as a statement about `RegisterSchemaVersion` specifically -- it still
     is. Do not read the framing above as "the first version can only ever
     arrive via `RegisterSchemaVersion`": `CreateSchema` gained an optional
     `SchemaDefinition` a short time later and can now seed version 1 itself
     at creation time. The version-count check documented here already
     covers that case correctly without modification, because it keys off
     `LatestSchemaVersion`, not off which operation incremented it -- see
     the schema_registry op note's gopherstack-i60f entry above.)
   - **BACKWARD/FORWARD/FULL and their `_ALL` variants deliberately NOT
     implemented.** Their real semantics require a per-`DataFormat`
     structural diff between schema versions (AVRO/JSON/PROTOBUF each have
     distinct field-addition, type-widening, and default-value rules — see
     the mode descriptions in `api_op_CreateSchema.go:53-91`). A partial or
     heuristic diff (e.g. only comparing top-level field names) risks
     silently accepting a change it should reject — worse than the status
     quo, because `RegisterSchemaVersion` succeeding is read by a caller as
     "this is compatible." Confirmed no existing AVRO/JSON/PROTOBUF-aware
     diffing exists anywhere in this backend to build on, and confirmed
     (again) `google.golang.org/protobuf` (already a transitive go.mod
     dependency) does not help — it's a runtime library for compiled
     descriptors, not a `.proto` text parser, so it doesn't change the
     "no new go.mod dependencies" constraint gopherstack-dol3 recorded.
     These six modes remain exactly as absent as before, and that absence is
     now called out explicitly next to the modes that ARE enforced, so a
     reader can't mistake "some compatibility checking exists" for "all
     compatibility checking exists."

2. **DQDL validation — re-confirmed out of scope, no code touched.** Read
   `data_quality_rulesets.go` fresh this pass: `Ruleset` is still stored and
   returned completely verbatim with zero grammar checking. Re-confirmed
   gopherstack-dol3's sizing is correct — full validation needs a real
   lexer/parser for a rule language with a dozen-plus rule types (IsComplete,
   IsUnique, ColumnValues, ColumnCount, RowCount, Completeness, Uniqueness,
   DataFreshness, custom-SQL rules, composite AND/OR/NOT, threshold
   comparators) plus a scope decision on whether rules are evaluated against
   real table data or only syntax-checked. That's package-sized work (the
   sizing note's own comparison to `pkgs/dynamodb/expr` still holds), not a
   file-sized fix, and there was no way to slice off a zero-risk sub-piece
   the way DISABLED-mode was for schema compatibility — every DQDL rule type
   needs the same lexer/parser scaffolding, so there's no "cheap 80%" here.
   Not started; no test was found asserting incorrect acceptance of
   malformed `Ruleset` strings (there's no validation to have gotten wrong).

3. **Tests**: `services/glue/registry_test.go` gained table-driven backend
   tests (`Test_CreateSchema_RejectsInvalidCompatibility`,
   `Test_UpdateSchema_RejectsInvalidCompatibility`,
   `Test_RegisterSchemaVersion_DisabledCompatibility`) covering all 8 valid
   modes, an invalid mode, and DISABLED-vs-NONE-vs-BACKWARD version-2
   behavior. All three failed against the pre-fix code (invalid
   `Compatibility` silently accepted; DISABLED's second `RegisterSchemaVersion`
   silently succeeded) before the fix landed.

## This pass (parity-4 campaign: SDK bump to v1.149.0 revealed 31 new ops, HEAD `a7f9c5fb2`)

The Go SDK module was bumped from `aws-sdk-go-v2/service/glue@v1.137.2` to
`@v1.149.0`, which shipped a new operation surface `TestSDKCompleteness`
caught as missing: `AssociateGlossaryTerms`, `BatchGetIterableForms`,
`CreateGlossary`, `CreateGlossaryTerm`, `DeleteAsset`, `DeleteAssetType`,
`DeleteAttachment`, `DeleteFormType`, `DeleteGlossary`, `DeleteGlossaryTerm`,
`DisassociateGlossaryTerms`, `GetAsset`, `GetAssetType`, `GetDashboardUrl`,
`GetFormType`, `GetGlossary`, `GetGlossaryTerm`, `GetSessionEndpoint`,
`ListAssetTypes`, `ListFormTypes`, `ListGlossaries`, `ListGlossaryTerms`,
`ListIterableForms`, `PutAsset`, `PutAssetType`, `PutAttachment`,
`PutFormType`, `SearchAssets`, `UpdateAsset`, `UpdateGlossary`,
`UpdateGlossaryTerm` -- three coherent new resource families (business
glossary + terms; asset catalog: asset types, assets, form types,
attachments, iterable form items; and Spark monitoring
dashboard/interactive-session endpoint lookups) plus a handful of standalone
ops. All 31 were implemented for real (none parked in `notImplemented`) --
see the `glossaries`, `asset_catalog`, `form_types_and_attachments`, and
`dashboard_and_session_endpoint` family notes above for the full field-diff
and error-code reasoning per family.

**Ownership/cascade summary** (see `ownership_and_cascade` in the return
receipt for the full version): a `Glossary` owns zero or more
`GlossaryTerm`s (`DeleteGlossary` blocked by `ConflictException` while any
exist; `DeleteGlossaryTerm` cascades to disassociate the term from every
`Asset`/iterable-form-item that referenced it). An `Asset` has exactly one
`AssetType` (validated to exist on `PutAsset`); an `AssetType` references one
or more `FormType`s via its `Forms` map (validated to exist on
`PutAssetType`; `DeleteFormType` blocked by `ConflictException` while any
`AssetType` still references it, per AWS's own documented error).
`Attachment`s hang off either an `Asset` directly or an item within one of
the `Asset`'s iterable forms (e.g. a table asset's "columns"); iterable-form
items have no dedicated create operation in this 31-op surface, so their
existence is entirely derived from `PutAttachment` having targeted them at
least once (`ItemIdentifier`+`IterableFormName`) -- `DeleteAsset` cascades to
delete all of an asset's iterable-form items.

**Two honest, narrowly-scoped compromises** (both documented in the family
notes above, neither hides anything in `notImplemented`): (1) `FormType`/
`AssetType` IDs are set equal to their (unique, upsert-keyed) `Name` rather
than an AWS-opaque generated ID, since the real ID-generation format is not
derivable from the public SDK shapes -- the same class of "deterministic mock
value, real backing state" choice this file already accepts for `DevEndpoint`
network fields. (2) `GetDashboardUrl`/`GetSessionEndpoint` real-check that the
target JOB/SESSION exists (and, for sessions, is not
stopped/stopping) but return deterministic mock URL/token values, since this
backend has no real Glue Studio console or Spark Connect listener -- again
matching the `DevEndpoint` precedent rather than inventing new fabricated
infrastructure.

**One pre-existing, unrelated gap surfaced (not introduced) while
implementing `GetSessionEndpoint`**: `Session.Status` is set to
`PROVISIONING` on `CreateSession` and this backend has no reconciler
transition that ever advances it to `READY` (unlike crawlers/job-runs/
workflow-runs, which all do reach a terminal state via the managed
reconciler). `GetSessionEndpoint` was designed around this by gating on "not
STOPPED/STOPPING" rather than requiring `READY`, so the gap does not block
this pass's new functionality; flagged in `gaps` above for whichever pass
owns `sessions.go`.

**Decomposition**: split by resource family into new files matching this
service's existing one-family-per-file layout --
`glossaries.go`/`handler_glossaries.go` (Glossary + GlossaryTerm +
Associate/Disassociate), `assets.go`/`handler_assets.go` (AssetType + Asset +
Attachments + IterableForms + SearchAssets), `search_assets.go` (SearchAssets'
filter-clause union parser/evaluator, split out of `assets.go` to keep that
file to CRUD), `forms.go`/`handler_forms.go` (FormType), and
`dashboard.go`/`handler_dashboard.go` (GetDashboardUrl/GetSessionEndpoint). No
function required a `//nolint:cyclop|gocyclo|gocognit|funlen` suppression;
`golang.org/x/tools/go/analysis/passes/fieldalignment -fix` was re-run across
the package (same tool used in the parity-3 pass) to keep every new struct
`govet`-fieldalignment-clean.

**New regression tests** (all in `handler_test.go`, an existing file --
`services/glue` already had zero test files using a live
`aws-sdk-go-v2/service/glue` client; every existing test in this package
round-trips through the real HTTP handler/router path via the `doGlueRequest`
helper already defined in `handler_test.go`, and the new tests follow that
same established convention): `TestGlue_Glossary_TableDriven`,
`TestGlue_AssociateGlossaryTerms_TableDriven`,
`TestGlue_AssetCatalog_CRUD_TableDriven`,
`TestGlue_AssetAttachmentsAndSearch_TableDriven`,
`TestGlue_Dashboard_TableDriven`. `persistence_test.go`'s existing
`TestInMemoryBackend_SnapshotRestore_FullState` (the package's designated
full-backend-state Snapshot/Restore regression test) was extended to seed and
verify a glossary+term+asset-type+form-type+asset+iterable-form-item chain,
confirming the new `store.Table`-backed resources AND the raw
`iterableFormItems` map all survive a Snapshot/Restore round trip.

## This pass (parity-3 campaign: full deferred-family sweep, HEAD `6467046d`)

This pass's mandate was different from prior passes: instead of auditing only
rows marked `partial`/`deferred` for drift, it worked through the **7 tracked
gaps** and **all 10 deferred whole-resource families** end-to-end, field-diffing
each against the pinned SDK (`aws-sdk-go-v2/service/glue@v1.137.2`) rather than
trusting the no-stub check alone.

**Gaps (7/7 closed)**: CrawlerTarget's 5 missing target kinds (DynamoDB/Delta/
Hudi/Iceberg/MongoDB), CreateCrawler/UpdateCrawler's 5 missing policy fields
(SchemaChangePolicy/RecrawlPolicy/LineageConfiguration/
CrawlerSecurityConfiguration/LakeFormationConfiguration), Database's 4 missing
fields (Parameters/LocationUri/CreateTableDefaultPermissions/TargetDatabase),
StartJobRun's per-run overrides, Trigger's 3 missing fields plus the
max-2-crawler-actions limit, and PutResourcePolicy's EnableHybrid. One gap
(qd3.5, the four unused quota/idempotency exceptions) is honestly left open —
see the gaps list for why fabricating quota numbers would be worse than an
honest gap.

**Deferred families (10/10 audited, 7 now `ok`, 3 `partial` with a named,
scoped remainder)**: connections, triggers, dev_endpoints, security_configurations,
blueprints, and user_defined_functions are now fully field-diffed and closed.
workflows, schema_registry, data_quality_rulesets, and ml_transforms each got
real, tractable fixes (see families notes) but keep one deep, genuinely
out-of-scope gap each (DAG-graph modeling, schema-compatibility algorithms, a
DQDL parser, and ML evaluation-metric computation respectively) — none of
which can be honestly faked without either a new dependency or inventing data
that isn't real.

**Two additional bug classes found and fixed while doing the field-diffs (not
on the original gaps list)**:

1. **Epoch-seconds timestamp bug** (the exact bug class flagged in this pass's
   brief, previously found in sagemaker): `BlueprintRun.StartedOn`,
   `ColumnStatisticsTaskRun.StartedOn`, `DQRuleRecommendationRun.StartedOn`, and
   `MaterializedViewRefreshRun.StartedOn` were modeled as raw `time.Time` with a
   JSON tag, which `encoding/json` renders as an RFC3339 string — but glue is
   awsjson1.1, which expects a JSON number (epoch seconds) for every timestamp.
   `BlueprintRun` and `ColumnStatisticsTaskRun` reach the wire via `any`-typed
   handler outputs (`GetBlueprintRun`/`GetBlueprintRuns`/
   `GetColumnStatisticsTaskRun`), so this was a real, reachable client-breaking
   bug, not just internal-state hygiene. Fixed by switching all four to
   `float64` (matching every other timestamp field in the package, e.g.
   `JobRun.StartedOn`, `WorkflowRun.StartedOn`). Locked down by a new
   regression test, `TestStartedOn_IsEpochSecondsNumber`
   (`timestamp_wire_shape_test.go`), which decodes the actual HTTP response
   JSON and asserts the field is a `float64`, not a string.
2. **ConcurrentRunsExceededException never returned**: StartJobRun's
   `ExecutionProperty.MaxConcurrentRuns` check returned generic
   `InvalidInputException` instead of the documented
   `ConcurrentRunsExceededException` (confirmed in deserializers.go's
   `awsAwsjson11_deserializeOpErrorStartJobRun` switch). New
   `ErrConcurrentRunsExceeded` sentinel now used by both StartJobRun and the
   new StartWorkflowRun `MaxConcurrentRuns` enforcement.

**Decomposition**: `StartJobRunWithOptions` grew past `gocognit`'s complexity
threshold while absorbing the per-run-override logic; split into
`checkJobConcurrencyLocked` (concurrency-limit check) and
`resolveJobRunOverrides` (pure function resolving job-defaults-vs-per-run-
override precedence) — no `//nolint:gocognit` used.

**Naming hygiene**: ran `golang.org/x/tools/go/analysis/passes/fieldalignment`
with `-fix` across the package (was already lint-clean; this pass's additions
temporarily regressed it) and renamed every new/touched AWS-`Id`-suffixed Go
field to the idiomatic `...ID`/`...IDs` form (`SubnetID`, `VpcID`, `CatalogID`,
`AccountID`, `SecurityGroupIDs`, `LocationURI`, etc.) — matching the
convention already used elsewhere in this file (`CatalogID`, `RoleArn`'s
sibling fields) — rather than reaching for `//nolint:revive,stylecheck`
suppressions. JSON wire tags (`"SubnetId"`, `"CatalogId"`, ...) are untouched;
only the Go-side identifiers changed.

## Follow-ups filed as SHARED-FILE / cross-service (this pass)

No code changes were needed outside `services/glue/` this pass. Every backend
method whose signature changed (`PutResourcePolicy` +1 param,
`StartJobRun`→kept + new `StartJobRunWithOptions`, `CreateConnection`→kept +
new `CreateConnectionWithOptions`, `CreateBlueprint`/`UpdateBlueprint` gained
required params, `CreateDevEndpoint` gained required params,
`UpdateDataQualityRuleset` +1 param, `CreateDataQualityRuleset`→kept + new
`CreateDataQualityRulesetWithOptions`, `CreateMLTransform`→kept + new
`CreateMLTransformWithOptions`) was checked against
`services/cloudformation/resources_glue.go`, the one cross-package caller of
Glue backend methods. `go build ./services/cloudformation/...` passes.
`CreateCrawler`/`CreateConnection`/`CreateTrigger`/`CreateDatabase` signatures
used by cloudformation were kept unchanged (options added via new
`...WithOptions` methods, same pattern as the prior pass's
`CreateCrawlerWithOptions`); cloudformation never calls
`CreateBlueprint`/`CreateDevEndpoint`/`PutResourcePolicy`/
`CreateDataQualityRuleset`/`CreateMLTransform`/`UpdateDataQualityRuleset`
directly, so those breaking signature changes are safe.

## Previous pass (re-audit at HEAD `a8c6614b`, no local drift since `ce30166a`)

`git diff ce30166a..HEAD -- services/glue/` was empty (the ledger's real baseline —
the recorded `last_audit_commit: 704d7cda` did not exist in this branch's history;
`704d7cda` turned out to be an unrelated STS commit, so per the re-audit protocol
`ce30166a`, the commit that last touched this file, was used as the actual baseline).
SDK pin unchanged at `v1.137.2`. With zero drift, all rows the previous pass marked
`ok` were trusted as-is; this pass audited only the rows marked `partial`/`deferred`
and found genuine, narrowly-scoped bugs in triggers, schema registry, resource
policy, and (previously untracked) integration resource/table properties — see the
`families` notes above for each. Full details on all six fixes are in those notes;
summary: two more `Get*`-returns-live-pointer data races (schema registry,
integration properties — same bug class as the prior pass's `GetTables` fix),
`StartTrigger`/`StopTrigger` ignoring the ON_DEMAND-trigger state rule and
`StartTrigger` never actually firing an ON_DEMAND trigger's actions (a disguised
stub for that type's entire purpose), and `PutResourcePolicy`/`DeleteResourcePolicy`
silently dropping their optimistic-concurrency condition parameters.

New regression tests: `parity_pass5_test.go`.

## Follow-ups filed as SHARED-FILE / cross-service (NOT edited this pass)

No code changes were needed outside `services/glue/` this pass. The
`PutResourcePolicy` interface-signature change (two new params) and the additive
`Trigger.StartOnCreation`/`TriggerAction.CrawlerName` fields were checked against
`services/cloudformation/resources_phase5.go`, the one cross-package caller of Glue
backend methods (`gluebackend.CreateCrawler`/`BatchCreatePartition`/`CreateJob`/
`CreateTrigger`) — none of those call sites touch `PutResourcePolicy`, and the
`Trigger{}` struct literal it builds is unaffected by new additive fields, so no
follow-up is needed there.

Separately (found, NOT fixed — pre-existing, unrelated to this pass's Glue changes):
`go build ./services/cloudformation/...` currently fails on
`services/route53/*` — `rc.backends.Route53.Backend.CreateHostedZone` is called
with 4 args but the (concurrently-edited-by-another-agent) Route53 backend method
now takes 5. This is outside `services/glue/` and was left untouched per this
task's scope; flagging for whichever pass owns Route53/CloudFormation.

## 2026-08-22 gopherstack-2wvq: GetEntityRecords/ListEntities native-catalog path

Fourth and fifth of gopherstack-2wvq's over-validation-whose-fix-is-a-feature
class (after codepipeline, rekognition, cloudtrail, textract). Both ops
wrongly required ConnectionName. The issue's own sizing called this "the most
expensive remaining candidate after transfer", on the theory that gopherstack
"indexes entities solely by connection name, with no concept of native-catalog
entities at all". That sizing was too high, the same way textract's was: this
backend already has a full, real Data Catalog (databases.go/tables.go,
GetDatabases/GetTables/GetTable) that IS the native S3 Glue Data Catalog the
SDK doc for GetEntityRecords names as the alternate path
("query preview data from a given connection type or from a native Amazon S3
based Glue Data Catalog"). No new storage or index was built -- only a mapping
from an existing Table's StorageDescriptor.Columns/PartitionKeys onto the
existing EntityField shape (columnToEntityField/nativeEntityDefFromTable,
entities.go), reusing entityCatalog's own sampleRecord/paginateSlice
machinery unchanged.

Checked both ops in both directions (rule 1). ListEntities: ConnectionName
over-required (fixed) and ParentEntityName -- a real input field -- was
accepted and silently ignored by every code path (an accept-and-drop bug on
the op's own input, not on a separate create/update path); it is now honored
for native-catalog listing (drills a database down to its tables) and left as
a documented no-op in connector mode, since entityCatalog's canned catalog
models no children for any entity to filter to. GetEntityRecords: ConnectionName
over-required (fixed) AND, the other direction, Limit is real-SDK-required
(client-side validator in validators.go, not just doc prose) but was never
enforced here -- now returns InvalidInputException for Limit<=0. That second
half was not named in the issue at all; found only by reading the op's own
required-member set against its own handler, same as codepipeline.

Checked glue's create/update path for a matching accept-and-drop (rule 2):
CreateTable/CreateDatabase already retain everything GetEntityRecords/
ListEntities' native path needs (columns, partition keys, database names) --
nothing is dropped there. GetEntityRecordsInput's own optional
ConnectionOptions/SelectedFields members are still not captured on the wire
struct; left alone since both are optional (not the required-field class this
issue targets) and this backend's DescribeEntity/GetEntityRecords already
return canned/synthesized data regardless of connector options, so capturing
ConnectionOptions would have nothing real to feed. SelectedFields (field
projection) is a genuine, cheap-ish gap -- GetEntityRecords always returns
every field -- noted here as found but not fixed, out of the ConnectionName/
Limit scope this pass targeted.

Neither op sits on a create/update/destroy lifecycle leg (rule "one from
today", gopherstack-jodk's over-validation-on-teardown class): both are
read-only preview/discovery ops used ahead of building a real connector or
crawler, not part of any resource's create or delete flow, so there is no
teardown-leg risk here.

EntityName for the native path is the "database.table" form ListEntities
itself advertises (nativeEntityName/splitNativeEntityName) -- a chosen, not
AWS-specified, convention, because GetEntityRecordsInput carries no separate
parent/database field to disambiguate a bare table name that might exist in
more than one database. A bare database name, or anything not in that form,
is EntityNotFoundException, never an empty or fabricated 200 -- consistent
with entities.go's existing DescribeEntity/GetEntityRecords behavior for
unknown connector entities.

DescribeEntity was deliberately left untouched: it is not one of the two ops
this issue names, and it still requires ConnectionName. A real client that
discovers a native-catalog table via the now-fixed ListEntities and then calls
DescribeEntity on it gets EntityNotFoundException today, not a schema. That is
a real, honestly-scoped-out gap (see the DescribeEntity/ListEntities/
GetEntityRecords rows above), not silently papered over -- extending
DescribeEntity is a natural, cheap follow-up but is out of this pass's edit
scope (`services/glue/` changes were reviewed against this issue's exact
two-op list).

New tests: entities_test.go (TestBackend_ListEntities/TestBackend_GetEntityRecords
updated for ListEntities' new parentEntityName parameter), handler_entities_test.go
(TestHandler_ListEntities_NativeCatalog, TestHandler_GetEntityRecords_NativeCatalog,
plus TestHandler_ListEntities/TestGetEntityRecords table updates -- the old
"missing ConnectionName is 400" cases asserted the exact backwards behavior this
issue exists to fix, rewritten rather than deleted, matching the codepipeline
fix's precedent). Every new/changed assertion was hand-verified to fail against
the pre-fix code (`entities.go`/`handler_entities.go`/`interfaces.go` restored
from `git show HEAD:<path>`, confirmed red, restored, `md5sum` byte-identical to
the fixed version).

`StorageBackend.ListEntities`/`InMemoryBackend.ListEntities` gained a
`parentEntityName` parameter (interfaces.go) -- confirmed via repo-wide grep
that both are referenced only inside `services/glue/`; `make build-check`
passes.

## 2026-08-22 gopherstack-v4a4: struct-tag casing bugs found by cmd/keycheck's new struct-tag scan

`cmd/keycheck` was extended (gopherstack-v4a4) to check `json` struct tags on
locally-declared `*Output`-suffixed structs against the pinned SDK's real
deserializer case list, the same way it already checked hand-written
`map[string]any` keys (gopherstack-zquj). Sweeping glue with it surfaced two
real, previously-uncaught casing bugs, both matching the exact class this
tool exists to catch: a field silently dropped by every real client because
gopherstack's tag doesn't match the SDK's literal switch-case string.

**DynamoDBTarget.ScanAll/.ScanRate** (`services/glue/models.go`): tagged
`json:"ScanAll,omitempty"` / `json:"ScanRate,omitempty"`, but
`awsAwsjson11_deserializeDocumentDynamoDBTarget` (glue@v1.152.0
deserializers.go) switches on `scanAll` / `scanRate` (lowerCamelCase) while
its sibling `Path` stays `Path` (PascalCase) in the same type -- the exact
"casing is not uniform within one nested tree" trap already documented for
scheduler's awsvpcConfiguration bug (commit 8469dcdd9). Confirmed both
directions broken: `awsAwsjson11_serializeDocumentDynamoDBTarget`
(serializers.go:22835) also emits `scanAll`/`scanRate` on the request side,
and gopherstack's `CrawlerTarget`/`DynamoDBTarget` is the same Go type for
both directions, so a real client's `CreateCrawler` input AND
`GetCrawler`/`BatchGetCrawlers` output both silently dropped these two
fields. Fixed by lowercasing both tags. Proof:
`TestSDKRoundTrip_GetCrawler_DynamoDBTargetScanFields`
(handler_dynamodb_target_realclient_test.go), drives CreateCrawler+GetCrawler
through the real `aws-sdk-go-v2/service/glue` client; confirmed to fail
(`ScanAll must decode non-nil`) against the pre-fix tag by hand-revert.

**TableOptimizer.Type/.Configuration/.LastRun and
TableOptimizerConfiguration.Enabled/.RoleARN** (`services/glue/models.go`):
tagged PascalCase, but the real nested `TableOptimizer` document
(`awsAwsjson11_deserializeDocumentTableOptimizer`, deserializers.go) switches
on `configuration`/`configurationSource`/`lastRun`/`type`, and its own nested
`TableOptimizerConfiguration` switches on `enabled`/`roleArn` (plus
`compactionConfiguration`/`orphanFileDeletionConfiguration`/
`retentionConfiguration`/`vpcConfiguration`, not modeled here and out of
scope for a tag-only pass) -- all lowerCamelCase, confirmed against
`GetTableOptimizerOutput`'s own deserializer (`case "TableOptimizer":` calls
the same `deserializeDocumentTableOptimizer`, so nesting here is genuinely
correct; only the tags were wrong). Fixed by lowercasing all five tags.
`CatalogID`/`DatabaseName`/`TableName` were deliberately left untouched --
see the structural-gap note below. Proof:
`TestSDKRoundTrip_GetTableOptimizer_ConfigurationFields`
(handler_table_optimizer_realclient_test.go), drives
CreateTableOptimizer+GetTableOptimizer through the real SDK client; confirmed
to fail (`Type` decodes as `""`, `Configuration` decodes nil) against the
pre-fix tags by hand-revert. The pre-existing raw-body test `TestTableOptimizer`
(handler_table_optimizers_test.go) asserted the WRONG PascalCase keys as
correct -- a "test that ratifies the defect" of the same shape the glue
MetadataInfo/MetadataInfoMap pair (fixed in c3aa73e59) already demonstrated;
updated to assert the real lowercase keys instead.

**Structural gap found, NOT fixed (restructuring is out of scope for this
tag-only campaign):** `BatchGetTableOptimizerOutput`'s real per-entry shape
is `BatchTableOptimizer` (`awsAwsjson11_deserializeDocumentBatchTableOptimizer`),
whose own case list is `catalogId`/`databaseName`/`tableName`/`tableOptimizer`
-- note `tableOptimizer` is itself a NESTED sub-object wrapping the same
`TableOptimizer` document, one level deeper than `GetTableOptimizerOutput`'s
shape (which has `TableOptimizer` nested directly under the op output, with
sibling top-level `CatalogId`/`DatabaseName`/`TableName`, confirmed correct
above). gopherstack's `batchGetTableOptimizerOutput.TableOptimizers
[]*TableOptimizer` reuses the SAME flat `TableOptimizer` struct for both
ops, so for `BatchGetTableOptimizer` specifically, `Type`/`Configuration`/
`LastRun` are siblings of `CatalogID`/`DatabaseName`/`TableName` instead of
nested under a `tableOptimizer` key -- casing alone cannot fix this; the
struct needs splitting into a wrapper (`BatchTableOptimizer`-shaped) and the
existing `TableOptimizer` nested inside it, forbidden under this campaign's
"fix tags only, do not restructure" rule. `CatalogID`/`DatabaseName`/
`TableName` tags were left PascalCase (their current, unfixed value)
rather than lowercased to `catalogId`/`databaseName`/`tableName`, because
the same struct is also used as `GetTableOptimizerOutput`'s nested
`TableOptimizer.CatalogID`/etc., which are themselves fabricated duplicate
fields there (the real inner `TableOptimizer` document has no such members
at all -- `GetTableOptimizerOutput`'s real `CatalogId`/`DatabaseName`/
`TableName` live one level up, already correctly present and PascalCase on
`getTableOptimizerOutput` itself) -- lowercasing them would be right for one
op sharing the type and wrong for the other. Filed as a follow-up
(`bd list --search v4a4` context; needs a new bd issue for the
BatchGetTableOptimizer nesting split) rather than guessed at here.

**Tool method note:** the struct-tag scan's own precision was checked against
its own known blind spots before any of the above were trusted. glue's
`BatchCreatePartition` MISMATCH (`Partitions`, `StorageDescriptor`, and 20+
nested keys) is a harmless-extra false positive (blind spot #3): the real
`BatchCreatePartitionOutput` has only `Errors`, and gopherstack's
`batchCreatePartitionOutput.Partitions` field is a fabricated field a real
client's typed struct has no slot to receive -- not a dropped required key.
Not fixed (out of scope; noted for a future audit pass, not this campaign).

## 2026-08-22 gopherstack-5mvf: BatchGetTableOptimizer wrapper/nested-TableOptimizer split

Finished the restructuring gopherstack-v4a4 deliberately left open (tag-only
campaign, restructuring forbidden). `GetTableOptimizerOutput` and
`BatchGetTableOptimizerOutput` really do carry `TableOptimizer` at different
nesting depths -- confirmed again directly against
`awsAwsjson11_deserializeOpDocumentBatchGetTableOptimizerOutput`
(deserializers.go:79707), `awsAwsjson11_deserializeDocumentBatchTableOptimizer`
(deserializers.go:40608, case list `catalogId`/`databaseName`/`tableName`/
`tableOptimizer`, all lowerCamelCase, with `tableOptimizer` itself calling
`awsAwsjson11_deserializeDocumentTableOptimizer` one level deeper), and
`awsAwsjson11_deserializeDocumentTableOptimizer` (deserializers.go:75671, case
list `configuration`/`configurationSource`/`lastRun`/`type` only -- no
`catalogId`/`databaseName`/`tableName` members at all on the real nested
document, confirming those three were fabricated on `TableOptimizer` before
this pass).

**The split:** `TableOptimizer` (`services/glue/models.go`) is now exactly
the real nested document -- `LastRun`/`Type`/`Configuration` only, all
lowerCamelCase, no identifying fields. A new `BatchTableOptimizer` type wraps
it for `BatchGetTableOptimizer`'s own shape: `TableOptimizer
*TableOptimizer \`json:"tableOptimizer"\`` plus `CatalogID`/`DatabaseName`/
`TableName` in lowerCamelCase (`catalogId`/`databaseName`/`tableName`),
distinct from `getTableOptimizerOutput`'s own PascalCase
`CatalogId`/`DatabaseName`/`TableName` (`services/glue/handler_table_optimizers.go`,
unchanged -- that shape was already correct). `BatchGetTableOptimizer`'s
backend method and `StorageBackend` interface (`services/glue/interfaces.go`)
now return `[]*BatchTableOptimizer` instead of reusing `[]*TableOptimizer`.

Casing stayed correct for both ops simultaneously because they no longer
share one struct: `GetTableOptimizerOutput`'s wrapping PascalCase
`CatalogId`/`DatabaseName`/`TableName` live on `getTableOptimizerOutput`
(never touched), and `BatchTableOptimizer`'s lowerCamelCase equivalents live
on the new type -- each casing lives on the shape that actually needs it, per
the DynamoDBTarget precedent (mixed casing within one op family is normal
here, not a bug to average out).

**Internal storage fallout:** dropping `DatabaseName`/`TableName` from
`TableOptimizer` broke `tableOptimizerEntryKeyFn` (`store_setup.go`), which
derived the `tableOptimizers` `store.Table`'s primary key from those same
fields on the stored value (required by `store.Table.Restore`, which
recomputes every key via `keyFn` after a snapshot decode -- there is no
separately persisted key). Introduced `tableOptimizerRecord`
(`services/glue/table_optimizers.go`): an internal-only wrapper carrying
`DatabaseName`/`TableName` plus the now-slim `TableOptimizer`, used only as
the `tableOptimizers` store's value type. `CatalogID` was dropped entirely
rather than moved into the record -- grepped for `.CatalogID` on any
`*TableOptimizer`-typed value first and found none outside the flat-struct
bug itself; `CreateTableOptimizer`'s `catalogID` parameter is now unused
(renamed to `_`), confirmed harmless by a clean `golangci-lint run`
(`unparam` included, 0 issues).

**Failure arm:** `BatchGetTableOptimizerError` was checked against
`awsAwsjson11_deserializeDocumentBatchGetTableOptimizerError`
(deserializers.go:40331): `catalogId`/`databaseName`/`tableName`/`type`/
`error`, all lowerCamelCase (only the nested `ErrorDetail` document, via
`awsAwsjson11_deserializeDocumentErrorDetail`, deserializers.go:53346, keeps
`ErrorCode`/`ErrorMessage` PascalCase). It was wrong: gopherstack had all five
top-level keys PascalCase (`CatalogId`/`DatabaseName`/`TableName`/`Type`/
`Error`) -- a second real bug in the same op, not previously flagged because
gopherstack-v4a4's struct-tag scanner only checked `*Output`-suffixed structs
directly, and `BatchGetTableOptimizerError` is a nested field type, not an
op output itself. Fixed by lowercasing all five tags.

**Snapshot version bumped 1 -> 2** (`services/glue/persistence.go`): this is
a structural field removal/move, not additive or a pure-case rename -- an
old snapshot's `tableOptimizers` entries have no `Optimizer` key at all, so
decoding one as the new `tableOptimizerRecord` shape would silently zero out
every optimizer's `Type`/`Configuration`/`LastRun` rather than erroring.
Confirmed the bump was necessary, not just conservative: `go test
./pkgs/persistence/... -run TestSnapshotVersionGuard` failed first
(`incompatible struct change`) with `glueSnapshotVersion` at 2 and the golden
still at 1, then passed clean after `-update`; the resulting golden diff was
exactly this change (`TableOptimizer.CatalogID/.DatabaseName/.TableName`
removed, `tableOptimizerRecord.DatabaseName/.TableName/.Optimizer` added, no
other service's fields touched).

**Proof:** `TestSDKRoundTrip_BatchGetTableOptimizer_NestedShape`
(`handler_table_optimizer_realclient_test.go`) drives
`CreateTableOptimizer`+`BatchGetTableOptimizer` through the real
`aws-sdk-go-v2/service/glue` client with one found entry and one missing
entry, asserting both the success arm's nested `TableOptimizer` and the
failure arm's `Error` decode non-nil. Hand-reverted (`git show HEAD:<path>`
restored over the six touched non-test files, confirmed byte-identical
restore afterward via `md5sum`) to confirm it fails against the pre-fix
shared flat struct: every identifier field decoded empty (`DatabaseName`/
`TableName`/`Type` all `""`) and both `entry.TableOptimizer` and
`failure.Error` decoded nil, since the old flat struct has no `tableOptimizer`
key to nest under and the old PascalCase failure tags don't match the real
lowerCamelCase keys either. No pre-existing test asserted the old flat batch
shape as correct (`TestTableOptimizer`'s `BatchGetTableOptimizer` section
only checks `len(TableOptimizers) == 1`), so nothing needed correcting there.

## 2026-08-23 gopherstack-n3zi (continued): 13 more of the 84 unaudited never-mentioned ops

Continuation of the clientcoverage-driven audit that produced GetResourcePolicy/
GetMLTaskRun/GetDataQualityRuleRecommendationRun (dated entry above, same day).
That pass claimed "84 of the 111 never-mentioned ops remain unaudited, named
individually in the manifest" — no such manifest exists in the repo or in that
commit's diff. **Re-derived the never-mentioned set from scratch**: 299 ops
confirmed directly via `grep -c 'name: "' handler_routing.go` (no duplicates,
`sort -u` agrees), diffed word-boundary (`grep -w`) against every op name
literally present anywhere in this file as it stood before this pass. Result:
**96 ops never mentioned**, not 111/84 — the prior figures were not
reconstructible and are corrected here rather than propagated.

Of those 96, this pass **audited 24** against the pinned SDK
(`aws-sdk-go-v2/service/glue@v1.152.0`) and found **11 real bugs**, all
proven by a real `aws-sdk-go-v2/service/glue` client round trip (new file
`wire_output_dropped_fields_test.go`-adjacent `never_mentioned_audit_test.go`),
each hand-reverted (`git checkout HEAD -- <file>`, plus a targeted revert of
`interfaces.go`'s one changed line where a file bundled multiple fixes),
confirmed to fail with the exact error/behavior described below, then restored
and `diff -rq` confirmed byte-identical against the fixed tree:

1. **BatchDeleteConnection** — `Errors` was `[]ErrorDetail`, a JSON array;
   the real `BatchDeleteConnectionOutput.Errors` is `map[string]types.ErrorDetail`
   keyed by connection name (`awsAwsjson11_deserializeDocumentErrorByName`,
   deserializers.go). A real client's decode **hard-fails** the instant any
   connection is missing: `unexpected JSON type [map[...]]`. Backend signature
   changed to return `map[string]ErrorDetail`; the per-name key (previously not
   even tracked) is now the map key itself. `services/glue/connections.go`,
   `handler_connections.go`, `interfaces.go`.

2. **BatchGetDataQualityResult** — `ResultsNotFound` was `[]ErrorDetail`; the
   real field is `[]string` (`awsAwsjson11_deserializeDocumentDataQualityResultIds`).
   Same hard-fail class: `expected HashString to be of type string, got
   map[string]interface {}`. Backend now returns `[]string` (bare not-found
   IDs). `services/glue/data_quality_stats.go`, `handler_data_quality_stats.go`.

3. **ColumnStatisticsTaskRun.StartedOn**'s json tag was `"StartedOn"`; the real
   member is `StartTime` (`awsAwsjson11_deserializeDocumentColumnStatisticsTaskRun`'s
   case list has no `StartedOn` key at all). Its sibling `MaterializedViewRefreshRun.StartedOn`,
   fixed in the same prior pass that introduced this bug class
   (`## Two additional bug classes` note, front matter), already carried the
   correct `StartTime` tag — this one didn't. **Persisted struct**
   (`columnStatTaskRuns *store.Table[ColumnStatisticsTaskRun]`), so
   `glueSnapshotVersion` bumped 2→3 (`persistence.go`); golden regenerated
   (`go test ./pkgs/persistence/... -run TestSnapshotVersionGuard -update`),
   diff is exactly the tag rename plus two additive fields, nothing else.
   `TestStartedOn_IsEpochSecondsNumber/ColumnStatisticsTaskRun`
   (`timestamp_wire_shape_test.go`) already asserted the wire key by name and
   needed updating from `"StartedOn"` to `"StartTime"`.

4. **StartColumnStatisticsTaskRun** silently dropped `Role`, a **required**
   real input member (`api_op_StartColumnStatisticsTaskRun.go`). Now required
   (`InvalidInputException` if absent) and stored on the run
   (`ColumnStatisticsTaskRun.Role`, new field). `CatalogID` (real casing is
   `CatalogID`, capital D — confirmed from this op's own deserializer case
   list), `ColumnNameList`, `SampleSize`, `SecurityConfiguration` remain
   unmodeled (no backing per-column sampling/encryption state) — documented
   gap, not fixed.

5. **StopColumnStatisticsTaskRun** took `ColumnStatisticsTaskRunId`; the real
   op (`api_op_StopColumnStatisticsTaskRun.go`) has **no run-ID member at
   all** — it identifies the run by `DatabaseName`+`TableName`. A real
   client's Stop call always sent an empty/absent run ID, hitting this
   handler's `if in.ColumnStatisticsTaskRunID == "" { return &emptyOutput{},
   nil }` early-return — **silently reporting success without stopping
   anything**. Same shape as bug 6 below; found by reading the whole op per
   the "check every op sharing that type" rule after finding 6 first. Backend
   now finds and stops the most recently started run for the given table.

6. **StopMaterializedViewRefreshTaskRun** — identical bug and identical fix:
   took `RunId`, but the real op (`api_op_StopMaterializedViewRefreshTaskRun.go`)
   has no run-ID member, only `DatabaseName`+`TableName`(+`CatalogId`,
   unmodeled: this backend keeps one flat namespace). Same silent-no-op
   failure mode as bug 5.

7. **GetMaterializedViewRefreshTaskRun** — request field was `RunId`, real
   member is `MaterializedViewRefreshTaskRunId`; response was a flat
   `{RunId, Status}` pair instead of the real output's
   `MaterializedViewRefreshTaskRun` (`*types.MaterializedViewRefreshTaskRun`)
   wrapper object. A real client's request always decoded an empty run ID
   backend-side; the pre-existing handler's fallback ("if RunId is empty,
   return the first run in the whole account") was a gopherstack invention
   with no basis in the real, required-member API — removed. Response now
   wraps `*MaterializedViewRefreshRun` (whose json tags already matched the
   real nested type's field names — the bug was entirely in this handler's
   own separate flat struct, not the shared model).

8. **StartMaterializedViewRefreshTaskRun** — response field was `RunId`; real
   member is `MaterializedViewRefreshTaskRunId`
   (`api_op_StartMaterializedViewRefreshTaskRun.go`). A real client's run ID
   always decoded empty, breaking every subsequent Get/Stop call chained off
   it. Sibling of bugs 6/7 — all three found together while reading the whole
   materialized-view-refresh family per the "check every op sharing that
   type" rule after the first one (7) turned up wrong.

9. **CreateRegistry** dropped `Description` — trivially available from the
   request, already tracked on `Registry.Description` — and fabricated a
   `Status` field the real `CreateRegistryOutput`
   (`api_op_CreateRegistry.go`) does not have at all. Response now returns
   `Description`, not `Status`.

10. **PutSchemaVersionMetadata** / **RemoveSchemaVersionMetadata** (sibling
    ops, same bug) dropped `MetadataKey`/`MetadataValue` — trivially echoable
    from the request — and `RegistryName`/`SchemaName`, both real response
    members (`api_op_PutSchemaVersionMetadata.go`,
    `api_op_RemoveSchemaVersionMetadata.go`). `SchemaArn`/`VersionNumber`/
    `LatestVersion` were declared on the response struct but never actually
    populated (always zero-valued) since the backend never looked up the
    schema version behind the ID. New backend method
    `FindSchemaVersionByID` (`registry.go`) scans every registered schema's
    versions for a match (no existing ID→schema reverse index) and now
    genuinely populates all five previously-fake-or-absent fields. The real
    op also accepts identifying the version via `SchemaId`+`SchemaVersionNumber`
    as an alternative to `SchemaVersionId`; this backend supports only the
    `SchemaVersionId` path — documented, not fixed this pass.

11. **GetDataQualityRuleset** returned a fabricated `Arn` field: confirmed
    absent from every real data-quality-ruleset op's output
    (Create/Get/Update/List, all four `api_op_*DataQualityRuleset*.go`) —
    `DataQualityRuleset.ARN` is purely an internal field this backend keeps
    for ARN-keyed `TagResource` dispatch and its own persistence, and must
    never reach the wire. Fixed at the op layer (`getDataQualityRulesetOutput`
    has no `ARN` field). **Sibling caught in the same family check**:
    `ListDataQualityRulesets` (not itself in the 96 — already mentioned
    elsewhere in this file — but sharing the exact same leak) marshaled
    `[]*DataQualityRuleset` directly, leaking the same fabricated `Arn`; now
    uses a dedicated `dataQualityRulesetListItem` summary type. The model's
    own `ARN` field/json tag is untouched (kept for persistence + internal
    dispatch), so no snapshot bump was needed here — only the two
    wire-response paths were fixed. `RecommendationRunId` (real, set only
    when a ruleset is promoted from a recommendation run — a flow this
    backend doesn't have) is left absent, an honest gap.

12. **CreateGlueIdentityCenterConfiguration** silently dropped `Scopes` and
    `UserBackgroundSessionsEnabled`, both real request members
    (`api_op_CreateGlueIdentityCenterConfiguration.go`). Now stored on
    `IdentityCenterConfig` (two new fields, purely additive — no snapshot
    bump needed for this half) and surfaced back on
    `GetGlueIdentityCenterConfiguration` (sibling, not itself in the 96, but
    the real `GetGlueIdentityCenterConfigurationOutput` carries the same two
    fields — fixed alongside).

13. **UpdateGlueIdentityCenterConfiguration** — worse than a drop: the real
    op (`api_op_UpdateGlueIdentityCenterConfiguration.go`) has **no
    `InstanceArn` member at all**, only `Scopes`/`UserBackgroundSessionsEnabled`.
    The previous handler read a nonexistent `InstanceArn` from every real
    Update call (always empty/absent) and used it to **overwrite the stored
    `InstanceArn` with an empty string on every single real Update call** —
    a destructive side effect no caller could have intended — while silently
    dropping `Scopes`/`UserBackgroundSessionsEnabled` entirely. Fixed: Update
    no longer touches `InstanceArn` at all (it's set once at Create and
    never revisited, matching the real API's shape), and now genuinely
    updates `Scopes`/`UserBackgroundSessionsEnabled`.

**Ruled out, not a bug**: `GetMLTransform` was flagged by an early automated
wire-key-diff pass as missing ~18 response fields. False positive: its
response struct (`getMLTransformOutput`) embeds `*MLTransform` anonymously, so
`encoding/json` flattens `MLTransform`'s fields directly — the diff tool
didn't follow the embedded field. Manually re-checked field-by-field against
`types.MLTransform`; the only real gap is `EvaluationMetrics`, already
documented and re-confirmed absent multiple times (`ml_transforms` family
note, front matter) since this backend never runs a real ML evaluation.

**Identified, not fixed (time-boxed out of this pass)**: `GetUsageProfile`
fabricates a `Tags` member the real `GetUsageProfileOutput`
(`api_op_GetUsageProfile.go`) does not have, and drops the real
`Configuration` (`*types.ProfileConfiguration`) member entirely —
`UsageProfile` (models.go) has no field to source it from, so fixing this
means adding `Configuration` to the model and threading it through
Create/Update/Get, not a one-line change. Left as a named, provable follow-up
rather than attempted under this pass's time budget.

**13 ops from the 96-queue got real fixes** (BatchDeleteConnection,
BatchGetDataQualityResult, StartColumnStatisticsTaskRun,
StopColumnStatisticsTaskRun, GetMaterializedViewRefreshTaskRun,
StartMaterializedViewRefreshTaskRun, StopMaterializedViewRefreshTaskRun,
CreateRegistry, PutSchemaVersionMetadata, RemoveSchemaVersionMetadata,
GetDataQualityRuleset, CreateGlueIdentityCenterConfiguration,
UpdateGlueIdentityCenterConfiguration), plus 2 siblings outside the 96
(ListDataQualityRulesets, GetGlueIdentityCenterConfiguration) — 15 ops
touched total for 8 distinct root causes.

**72 of the 96 were not reached this pass** and remain queued, named
individually: BatchGetCustomEntityTypes, BatchGetDevEndpoints,
BatchGetTriggers, BatchPutDataQualityStatisticAnnotation, CancelStatement,
CreateIntegrationTableProperties, CreateScript, CreateWorkflow,
DeleteBlueprint, DeleteCatalog, DeleteClassifier,
DeleteColumnStatisticsForPartition, DeleteColumnStatisticsForTable,
DeleteColumnStatisticsTaskSettings, DeleteCustomEntityType, DeleteDatabase,
DeleteDataQualityRuleset, DeleteDevEndpoint,
DeleteGlueIdentityCenterConfiguration, DeleteIntegrationResourceProperty,
DeleteIntegrationTableProperties, DeleteMLTransform, DeleteRegistry,
DeleteSchema, DeleteSchemaVersions, DeleteSession, DeleteTableOptimizer,
DeleteTrigger, DeleteUsageProfile, DeleteUserDefinedFunction, DeleteWorkflow,
GetCatalogImportStatus, GetClassifier, GetClassifiers,
GetColumnStatisticsForPartition, GetCustomEntityType, GetDataflowGraph,
GetDataQualityModel, GetDataQualityModelResult, GetDevEndpoint,
GetDevEndpoints, GetMapping, GetPlan, GetSchemaVersionsDiff,
GetSecurityConfigurations, GetSession, GetStatement, GetTableVersion,
GetTrigger, GetWorkflowRunProperties, ImportCatalogToGlue,
ListColumnStatisticsTaskRuns, ListDataQualityStatisticAnnotations,
ListDataQualityStatistics, ListStatements, ListTableOptimizerRuns,
ListUsageProfiles, ListWorkflows, PutDataQualityProfileAnnotation,
PutWorkflowRunProperties, ResumeWorkflowRun, RunStatement, StartBlueprintRun,
StartColumnStatisticsTaskRunSchedule, StopColumnStatisticsTaskRunSchedule,
StopSession, StopWorkflowRun, UpdateJobFromSourceControl, UpdateRegistry,
UpdateSourceControlFromJob, UpdateTableOptimizer, UpdateUsageProfile.

A handful of these were screened by the same mechanical wire-key-diff script
that produced the GetMLTransform false positive above (top-level key set
only, no nested-type or embedded-struct resolution) and showed only
apparent-gap signals (e.g. `RequestOrigin` missing across the Interactive
Sessions family — `CancelStatement`/`DeleteSession`/`GetSession`/
`GetStatement`/`ListStatements`/`RunStatement`/`StopSession`; `CatalogId`
missing across several Delete/Get ops) rather than hard-fail signals — none
of these were read against the actual SDK source this pass, so none are
claimed as gaps here; they stay in the queue above pending a real read.

Gates run: `go build ./...`, `go vet ./services/glue/...`, `gofmt -l`
(clean), `go test -race ./services/glue/... ./pkgs/persistence/...` (pass),
`make build-check` (clean — `CreateGlueIdentityCenterConfiguration`/
`UpdateGlueIdentityCenterConfiguration`/`BatchDeleteConnection`/
`BatchGetDataQualityResult`/`StartColumnStatisticsTaskRun`/
`StopColumnStatisticsTaskRun`/`StopMaterializedViewRefreshTaskRun` all
changed exported `StorageBackend` signatures), `golangci-lint run
./services/glue/...` (0 issues).


## 2026-08-23 gopherstack-n3zi (third pass): the remaining 72 of the 96 never-mentioned ops

Re-derived the queue independently rather than inheriting it, per the
instructions that caught three other services' counts wrong the same day.
Confirmed **299 dispatched op names** via `grep -c 'name: "' handler_routing.go`
(unchanged from the prior pass's derivation, `sort -u` agrees, no duplicates).
Extracted the **72-op "not reached" list** verbatim from the prior dated
entry's closing paragraph, then checked it two ways: (1) every one of the 72
names is a real dispatched op (`comm` against the 299 set — clean), and (2)
none of the 72 collides with a key already present in the structured `ops:`
block (clean) — ruling out the trap the prior pass's note warned about
(counting an op named only to say "not reached" as if it were audited).
**72 holds.**

Audited all 72 against `aws-sdk-go-v2/service/glue@v1.152.0`, reading each
op's real Input/Output struct and, where relevant, its deserializer case
list. **6 of the 72 had real bugs**, all proven by a real
`aws-sdk-go-v2/service/glue` client round trip
(`pass3_queue_audit_test.go`), each hand-reverted (`git show HEAD:<file>`
copied over every touched non-test file, confirmed to fail with the exact
symptom below, then restored and `md5sum`-confirmed byte-identical), plus
**3 siblings outside the 72** fixed alongside per the "check every op
sharing that type" rule — **9 ops touched for 7 distinct root causes**:

1. **DeleteRegistry** / **UpdateRegistry** — both declare a `RegistryArn`
   field on their response struct (`api_op_DeleteRegistry.go`,
   `api_op_UpdateRegistry.go`) that was never populated, even though this
   backend already tracks `Registry.ARN` (used correctly by the sibling
   `GetRegistry`/`ListRegistries` the whole time) — a real client got back
   `RegistryArn: ""` on every Delete/Update call. Backend
   `DeleteRegistry`/`UpdateRegistry` signatures changed from `error` to
   `(*Registry, error)` so the handler can source the real ARN instead of
   discarding the record before returning.

2. **DeleteSchema**, plus sibling **UpdateSchema** (not itself in the 72 —
   already graded `schema_registry: partial` elsewhere in this file, found
   while checking DeleteSchema's family) — same bug, `SchemaArn` declared
   (`api_op_DeleteSchema.go`, `api_op_UpdateSchema.go`) but never populated
   despite `Schema.SchemaARN` already being tracked. Backend
   `DeleteSchema`/`UpdateSchema` signatures changed from `error` to
   `(*Schema, error)`.

3. **UpdateUsageProfile** — **actively destructive**, the same class as the
   prior pass's `UpdateGlueIdentityCenterConfiguration` `InstanceArn` clobber:
   the handler called `Backend.UpdateUsageProfile(name, "")` unconditionally,
   ignoring whatever `Description` the client actually sent, and the backend
   applied that empty string unconditionally (`p.Description = description`,
   no guard) — **every single real `UpdateUsageProfile` call wiped the
   profile's Description to empty**, even when the client's own request
   carried a real, non-empty value. `Configuration`
   (`*types.ProfileConfiguration`, real, required) remains unmodeled — the
   same deferred gap already named for `GetUsageProfile` — not attempted
   here. Fixed: handler now reads and passes `in.Description`; backend only
   overwrites when non-empty (matching this file's own existing convention
   for optional-field updates, e.g. `UpdateSchema`'s `if description != ""`).

4. **ListDataQualityStatisticAnnotations** — declared neither `MaxResults`
   nor `NextToken` on the input or output, though both are real
   (`api_op_ListDataQualityStatisticAnnotations.go`) — always returned every
   annotation in one page regardless of `MaxResults`. Wired through the
   existing `paginateSlice` helper, matching every other List op in this
   file, with a new `defaultListDataQualityStatisticAnnotationsLimit`
   const (100). `ListDataQualityStatistics` was checked too (same family)
   but stays correctly empty — this backend never runs real data-quality
   monitoring, so there is never more than zero items to paginate.

5. **StartBlueprintRun** silently dropped `RoleArn`, a **required** real
   input member (`api_op_StartBlueprintRun.go`) — every real call succeeded
   with the role simply discarded. Worse: `BlueprintRun` (models.go) had no
   field to store `RoleArn` or the optional `Parameters` at all, so even a
   client that somehow got a run going could never read either back. Fixed:
   `RoleArn` is now required (`InvalidInputException` if absent, matching
   this file's `StartColumnStatisticsTaskRun`-style convention),
   `BlueprintRun` gained `RoleARN`/`Parameters` (purely additive fields,
   confirmed against the real `types.BlueprintRun`'s own `RoleArn`/
   `Parameters` json keys via `deserializeDocumentBlueprintRun`'s case list).

6. **GetBlueprintRun**, found while reading `StartBlueprintRun`'s whole
   family per the "check every op sharing that type" rule (not itself in
   the 72) — response wrapped the run under `Run`; the real member is
   `BlueprintRun` (`api_op_GetBlueprintRun.go`, confirmed via
   `awsAwsjson11_deserializeOpDocumentGetBlueprintRunOutput`'s case list,
   which has no `"Run"` case at all) — a real client's decode silently left
   the whole payload nil, no error. Fixed by renaming the wire key.

7. **GetBlueprintRuns**, same family, same day (not itself in the 72) — two
   bugs at once: the response field was `Runs`, the real member is
   `BlueprintRuns` (`api_op_GetBlueprintRuns.go`) — same silent-nil-decode
   class as bug 6 — and `MaxResults`/`NextToken`, both real and required-ish
   pagination members on this op, were entirely unhandled, always returning
   every run for a blueprint in one page. Fixed both: renamed the wire key
   and wired `paginateSlice` with a new `defaultGetBlueprintRunsLimit`
   const (100).

**Snapshot bump: not needed.** The only persisted-struct change is
`BlueprintRun` gaining `RoleARN`/`Parameters` (`omitempty`-tagged, purely
additive — an old snapshot decodes fine with both defaulting to `""`).
`go test ./pkgs/persistence/... -run TestSnapshotVersionGuard` confirmed
this itself: it failed first on the unrefreshed golden with an explicit
"this is bookkeeping, not a version-bump case: every old field is still
present unchanged, so the diff is additive only and needs no bump" message,
then passed clean after `-update`; the resulting diff is exactly
`BlueprintRun.Parameters`/`BlueprintRun.RoleARN`, nothing else.
`glueSnapshotVersion` stays at 3.

**Sibling check on every bug**: yes, explicitly, for all 7 — the
Registry/Schema ARN bug was chased across both Delete and Update for both
resource kinds (4 ops, 2 already in the queue); the Blueprint-run wire-key
bug was chased across Start/Get/GetRuns (3 ops, 1 already in the queue).
`ListDataQualityStatistics` (sibling of the annotations-pagination fix) was
checked and correctly left alone — no real gap, given it's always empty.

**Pre-existing tests corrected, not just new ones added** (same pattern as
prior passes' `TestCatalog`/`GetMLTransform` corrections): `TestBlueprintRun`,
`TestBlueprint_Run_Lifecycle`, `TestStartBlueprintRun_NotFound`, and
`timestamp_wire_shape_test.go`'s blueprint-timestamp case all called
`StartBlueprintRun` without `RoleArn` (a real required field no real client
could omit) and asserted the old `Run`/`Runs` wire keys — corrected to send
`RoleArn` and assert `BlueprintRun`/`BlueprintRuns`.

**Ruled out, not bugs**:
- `DeleteDatabase` has no `CatalogId` field, unlike several sibling
  Delete/Get ops that accept-but-ignore it (single flat namespace, an
  already-established pattern in this backend). `CatalogId` is optional on
  the real `DeleteDatabaseInput`, not required, so this is a minor
  completeness/consistency gap, not a provable bug — noted, not fixed.
- `GetSchemaVersionsDiff`'s `FirstSchemaVersionNumber`/
  `SecondSchemaVersionNumber` accept only the `{"VersionNumber": N}` shape;
  the real `types.SchemaVersionNumber` also has a `LatestVersion bool`
  alternative (compare against the newest version without naming a number).
  Not modeled — a real client using `LatestVersion` gets compared against
  version 0. Named, not fixed.
- The `registryIDInput`/`schemaIDInput` wrapper structs used by ~9
  schema-registry ops (`DeleteRegistry`, `UpdateRegistry`, `DeleteSchema`,
  `DeleteSchemaVersions`, `GetRegistry`, `GetSchema`, `ListSchemas`, etc.)
  accept a `RegistryArn`/`SchemaArn` field on the wire but every call site
  resolves identity from `RegistryName`/`SchemaName` alone, ignoring the ARN
  entirely — the real docs state "One of RegistryArn or RegistryName has to
  be provided" for `RegistryId`, and likewise for `SchemaId`. A real client
  identifying purely by ARN would resolve against an empty name and 404.
  This is real and provable but sized like a package (an ARN→name reverse
  lookup threaded through ~15 call sites across the whole schema-registry
  family), not a one-line fix — named as a follow-up, left unfixed this
  pass, same size class as the already-deferred `GetUsageProfile.Configuration`
  gap.

No Summary-leak-class instances found in this pass's 72 (checked
`GetClassifiers`/`GetDevEndpoints`/`ListTableOptimizerRuns`/`ListWorkflows`/
`ListDataQualityStatisticAnnotations`/`ListUsageProfiles` against their real
List/Get output types — all either marshal the real full type directly,
matching the real API's own non-Summary shape, or use an existing dedicated
summary type).

**All 72 of the queue are now accounted for** — 66 confirmed clean against
the pinned SDK, 6 fixed. No ops remain in this queue.

Gates run: `go build ./...`, `go vet ./services/glue/...`, `gofmt -l`
(clean), `go test -race ./services/glue/... ./pkgs/persistence/...` (pass),
`make build-check` (clean — `DeleteRegistry`/`UpdateRegistry`/`DeleteSchema`/
`UpdateSchema`/`StartBlueprintRun` all changed exported `StorageBackend`
signatures), `golangci-lint run ./services/glue/...` (0 issues). Work left
uncommitted per this pass's instructions.

## 2026-08-29 enum-VALUE sweep (wrapper-key-sweep campaign, wire-shape enforcement all services)

Targeted pattern hunt for the comprehend class of bug: a status/state value assigned to a
domain struct field that is not a member of the real AWS enum for the corresponding response
member, reaching the wire through the field rather than a same-site literal `cmd/enumcheck` can
resolve. Checked every domain struct field holding a status/state/type/mode concept against its
real SDK enum (`glue@v1.152.0 types/enums.go`), tracing every assignment including lifecycle
transitions. `cmd/enumcheck` was run against this service both before and after and flagged
**none** of the three findings below — confirming its blind spot on struct-field assignment.

**Found and fixed** (all three share the shape: a plain string literal, not the file's own
`store.go` shared-vocabulary constants, so this was NOT the multi-enum-sharing-one-vocabulary
shape found in comprehend — it's three independent one-off wrong literals):

- `column_statistics.go` `StartColumnStatisticsTaskRun`: `Status: "STARTED"` — the real member is
  `types.ColumnStatisticsState` (STARTING/RUNNING/SUCCEEDED/FAILED/STOPPED,
  `types/enums.go:225`); `"STARTED"` is not a member. Fixed to `stateStarting` ("STARTING"),
  matching every other `Start*` op in this file. No reconciler ever advanced this value, so a
  real client's waiter would have polled until timeout.
- `data_quality_rulesets.go` `CancelDataQualityRulesetEvaluationRun` and
  `CancelDataQualityRuleRecommendationRun`: both set `run.Status = "CANCELLED"`. Both fields wire
  to `types.TaskStatusType` (STARTING/RUNNING/STOPPING/STOPPED/SUCCEEDED/FAILED/TIMEOUT,
  `types/enums.go:3323`), which has no `CANCELLED` member. Fixed both to `stateStopped`
  ("STOPPED"), matching this same file's `CancelMLTaskRun` (`ml.go`), which already uses
  `stateStopped` for the identical cancel-on-`TaskStatusType` transition.

**Response-nesting sweep (separate pass, same bug class as above but wire-shape depth, not a
value) — N of N ops checked for this class: all 3 `DataQuality*EvaluationRun` response envelopes
(`Get`/`Start`/`BatchGet`)**: `GetDataQualityRulesetEvaluationRunOutput` previously wrapped every
field (`Status`/`CompletedOn`/`DataSource`/`RunId`/etc.) under a `"DataQualityEvaluationRun"` JSON
key (`handler_data_quality_rulesets.go`), but the real
`GetDataQualityRulesetEvaluationRunOutput` (`api_op_GetDataQualityRulesetEvaluationRun.go`) has
those members flat at the response root — a real SDK client decoded every member as `nil`, with
no error (total nil-decode, not a partial loss). Fixed by returning `*DataQualityEvaluationRun`
directly instead of a wrapper struct; `DataQualityEvaluationRun`'s own JSON tags already matched
the real root-level member names. `StartDataQualityRulesetEvaluationRunOutput` (only `RunId`) and
`BatchGetDataQualityRulesetEvaluationRunOutput` (`Runs`/`RunsNotFound`) were re-verified against
the real SDK and are already correctly flat — the two ops actually named in the pre-existing bd
issue as also wrapped turned out not to be; only `Get` had the bug. Verified via
`TestGetDataQualityRulesetEvaluationRun_FieldsAtResponseRoot` (real typed client, asserts `RunId`/
`Status`/`RulesetNames` are non-nil/populated post-fix, confirmed failing pre-fix) in
`wire_field_fixes_test.go`. Three pre-existing tests
(`TestCancelDataQualityRulesetEvaluationRun_StatusIsLegalEnumMember` in `wire_field_fixes_test.go`,
`TestDataQuality_EvaluationRun_GetAndCancel` in `handler_data_quality_stats_test.go`,
`TestHandlerDataQuality_GetDataQualityRulesetEvaluationRun` in
`handler_data_quality_rulesets_test.go`) asserted the `"DataQualityEvaluationRun"` wrapper key as
correct — all three updated to assert the real flat shape instead.

**Also flagged, not fixed (extraneous field, not an enum mismatch)**:
`identity_center.go`'s `IdentityCenterConfig.Status` ("ENABLED"/"DISABLED") has no corresponding
member on the real `CreateGlueIdentityCenterConfigurationOutput`/
`GetGlueIdentityCenterConfigurationOutput` at all (confirmed absent from both structs) — not a
wrong-enum-value bug (no real enum exists to violate), just an invented field a real client would
silently ignore.

**Checked clean** (N-of-N legal-value coverage against the real enum, no fix needed):
`CrawlerState` (3/3: READY/RUNNING/STOPPING), `CrawlerHistoryState` (3/4: RUNNING/COMPLETED/
STOPPED used, FAILED unused-but-legal), `JobRunState`, `MaterializedViewRefreshState`,
`ScheduleState`, `RegistryStatus`, `SchemaStatus`, `SchemaVersionStatus`, `SessionStatus`,
`WorkflowRunStatus`, `PartitionIndexStatus`, `IntegrationStatus`, `TaskStatusType` (elsewhere:
`MLTaskRun.Status`, `getMLTaskRunOutput` fallback), `DataQualityModelStatus`,
`DataQualityRuleResultStatus`, `BlueprintRunState`, `BlueprintStatus`, `TriggerState`,
`StatementState`, `TransformStatusType`, `ExportStatus` (deliberately restricted to ENABLED/
DISABLED, documented existing choice — not fabricating transient/FAILED states). `DevEndpoint.
Status`/`LastUpdateStatus` are untyped `*string` on the real SDK (no enum to violate) — out of
scope by definition, not checked further.

Gates: `go build ./services/glue/...` (clean), `go vet ./...` (repo-wide, clean — no signature
changes this pass), `go test -race -count=1 ./services/glue/...` (pass, including new
`wire_field_fixes_test.go`, each new assertion hand-verified to fail against the pre-fix
literals then restored), `golangci-lint run --fix ./services/glue/...` (0 issues). Work left
uncommitted per this pass's instructions.

## 2026-08-29 error-path sweep (wrong-code/should-not-error bug hunt, ERROR path only)

Audited glue's not-found error-sentinel choices at call sites against each op's own
`awsAwsjson11_deserializeOpError<Op>` switch in `deserializers.go` (glue@v1.152.0) — not the
service's general error-type list. Extracted the modeled-code set for all 299 ops. 8 real bugs
found and fixed, all in the class "generic `ErrNotFound` (-> `EntityNotFoundException`) used at a
call site whose own op does not model `EntityNotFoundException` at all":

- **Wrong code, fixed to `InvalidInputException`** (the op's actual modeled not-found-adjacent
  code): `DeleteFormType`, `DeleteGlossary`, `DeleteGlossaryTerm`, `ListGlossaryTerms`,
  `DeleteUsageProfile`, `DeleteSession`, `StopSession`, `DeleteWorkflow`, `DeleteAsset`,
  `DeleteAssetType`, `DescribeConnectionType`.
- **Wrong code, fixed to `MaterializedViewRefreshTaskNotRunningException`** (the op's actual
  modeled code for "nothing running to stop"): `StopMaterializedViewRefreshTaskRun` — new sentinel
  `ErrMaterializedViewRefreshTaskNotRunning` added, wired into `handler.go`'s switch.
- **Should-not-error (idempotent delete), fixed to a silent no-op**: `DeleteJob` and
  `DeleteTrigger` — both ops' own SDK doc comments state "If the X is not found, no exception is
  thrown" (`api_op_DeleteJob.go`, `api_op_DeleteTrigger.go`), confirmed by their error switches
  also having no not-found case at all.

Five pre-existing tests were asserting the old, wrong behavior as correct and were fixed alongside
the source: `TestDeleteUsageProfile_NotFound` (handler_usage_profiles_test.go), `TestBlueprint_DeleteNotFound`
(handler_blueprints_test.go), `TestStopMaterializedViewRefreshTaskRun_NotFound`
(handler_materialized_views_test.go), `TestExtractResource`/`delete_job_extracts_job_name`
(handler_crawlers_test.go), `TestGlue_ErrorCases`/`delete_nonexistent_job` (handler_test.go).
`TestTrigger_DeleteTrigger`/`not-found` and `TestWorkflow_DeleteAndList` needed no code changes
(only relied on `wantCode`, which is unaffected or already correct).

New tests, real typed `aws-sdk-go-v2` client, `errors.As` against the SDK's own exception type
(or `require.NoError` for the idempotent-delete cases), every one hand-verified to fail against
the pre-fix code first: `services/glue/wire_error_code_not_modeled_test.go`.

Spot-checked (not exhaustive) for the same class beyond these 8: crawlers (Start/Stop/Update/
Delete), connection_types (Delete/Register), resource_policies (Put/Delete), jobs/workflows
(StartJobRun/StartWorkflowRun's `ConcurrentRunsExceededException`), dev_endpoints
(`ResourceNumberLimitExceededException`), dashboard (`GetSessionEndpoint`'s
`IllegalSessionStateException`) — all already correct. Integrations family (Delete/Modify/Get/
UpdateIntegration*) already uses `EntityNotFoundException`, which IS modeled by those specific
ops — left as-is, no bug.

Gates: `go build ./services/glue/...` (clean), `go vet ./...` (repo-wide, clean — no signature
changes), `go test -race -count=1 ./services/glue/...` (pass), `golangci-lint run --fix
./services/glue/...` (0 issues). Work left uncommitted per this pass's instructions.

## 2026-08-29 ordering-bug audit (paginate-before-filter, iam class) -- clean, no code change

Audited every `paginateSlice(...)` call site (48, via `grep -rn "paginateSlice(" services/glue`) for
order of operations. This service funnels essentially all NextToken-based pagination through one
shared generic helper (`paginateSlice`, `handler.go:226`) plus a shared `matchesTagFilter`
(`handler.go:243`) and a handful of op-specific filter predicates (`matchesIntegrationFilters`,
`filterByDependentJobName`, the `Expression`/`partitionExpr` predicate in
`handler_partitions.go:handleGetPartitions`). In every site checked, the filter loop builds a new
`filtered`/`matching` slice first and `paginateSlice` is called on that result, not on the raw
backend list -- i.e. filter-then-paginate throughout: `handler_connections.go` (GetConnections,
ConnectionType/MatchCriteria), `handler_dev_endpoints.go`/`handler_blueprints.go`/
`handler_crawlers.go`/`handler_jobs.go`/`handler_triggers.go` (Tags via `matchesTagFilter`),
`handler_integrations.go` (DescribeInboundIntegrations/DescribeIntegrations, both self-contained
filter+paginate pairs), `handler_partitions.go` (GetPartitions, `Expression` predicate applied to the
full set before the slice), `handler_data_quality_rulesets.go`/`handler_ml.go`/
`handler_materialized_views.go`/`handler_data_quality_stats.go`/`handler_schemas.go` (each function's
own `continue`-based filter loop precedes its own `paginateSlice` call; verified no cross-wiring
between a file's multiple list functions). `paginateSlice` itself is filter-blind (operates purely on
the slice it's given, computing `next` from that same slice's length), so as long as callers pass it
the already-filtered slice -- which all 48 do -- there is no way for this helper to reproduce the iam
shape.

One structural, not-ordering gap noted in passing: `handleGetTableVersions`/`GetTableVersions`
(`handler_tables.go:205`) and `SearchTables` return everything unpaginated even though real
`GetTableVersionsInput`/`SearchTablesInput` support `MaxResults`/`NextToken` -- no pagination
implemented at all, so no order to get wrong, but also no truncation, meaning this over-returns
rather than silently drops data. Left as a "never plumbed pagination" gap for a future pass, not
folded into this one.

Zero ordering-bug findings; no files changed.

## 2026-08-29 cursor-pagination audit (declares-but-never-sets class)

Enumerated every response struct in this package declaring a `NextToken`/`Marker` field
(46 files, ~50 distinct response types by grep of field declarations, well under the crude
101-figure this pass's brief warned was unreliable) and cross-checked each against the 46
existing `paginateSlice(...)` call sites plus every handler file declaring the field but not
in that call-site list. All 46 pre-existing `paginateSlice` callers correctly assign the
returned token to their response's `NextToken`/`Marker` field -- confirmed by grep, not
assumed.

Two real bugs found and fixed (see `GetResourcePolicies`/`ListEntities` above): both had a
cursor field declared on the wire, a genuinely-unbounded backing collection, and the handler
never applied any pagination at all (no `paginateSlice` call, request `NextToken`/`MaxResults`
parsed into the input struct but never consulted). Same broken-request-and-response-together
pattern as prior services' findings.

Two response cursors correctly left unpopulated, both provably bounded:
- `DescribeEntity` (connector mode): fields come from `entityCatalog()`, a compile-time map of
  7 canned CRM/commerce entities, each with <=9 fields. Cannot exceed one page.
- `ListTableOptimizerRuns`: this backend only tracks a table optimizer's single `LastRun`, so
  the returned slice is at most 1 element regardless of real AWS's actual multi-run history
  semantics. Backend history-tracking is a separate, larger gap (not a cursor bug) -- noted,
  not fixed this pass.

Verified-correct-as-is: `handleGetTableVersions`/`GetTableVersions` and `SearchTables`
(`handler_tables.go`) do not declare `NextToken`/`MaxResults` on their input/output structs at
all, even though the real `GetTableVersionsInput`/`SearchTablesInput` support them -- a
structural wire-shape gap (missing fields), not a "declares but never sets" bug, and already
documented in this file's 2026-08-29 ordering-bug audit section above as deliberately deferred.
Left untouched this pass.

Tests: `services/glue/handler_pagination_sweep_sdk_test.go` gained a `get resource policies`
case (bumping `totalPaginationCases` 31->32); `services/glue/entities_test.go` gained
`TestListEntities_Pagination`. Both drive the real `aws-sdk-go-v2/service/glue` client,
confirmed failing against unmodified code before the fix.

Gates: `go build ./...`, `go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/glue/...` (pass), `golangci-lint run ./services/glue/...` (0 issues).

## 2026-08-30 WrapOp reflective-decode re-scan (gopherstack-4shm follow-up)

Glue has the most `service.WrapOp` call sites in the repo (~299), and its
most recent pass before this one explicitly read the existing audit trail
rather than running a fresh scan -- exactly the verdict gopherstack-4shm's
campaign put in doubt. Ran `cmd/reqfieldscan -dir glue` fresh: it returned
**zero** dispatch entries, not because glue is invisible to `WrapOp`
resolution (`collectWrapOpFuncNames` finds a `service.WrapOp(...)` call
anywhere in the package regardless of which literal it lives in) but because
glue's dispatch table is a data-driven `[]struct{name string; bind
func(*Handler) service.JSONOpFunc}{...}` slice (`handler_routing.go`'s
`glueOpBindings`), not a `map[string]service.JSONOpFunc{...}` composite
literal or a `GetSupportedOperations` `[]string{}` literal -- the two
dispatch-table shapes the tool's denominator logic recognizes. This is a
real tool blind spot, disclosed rather than silently mis-measured: **the
tool could not reach this service's dispatch table at all.**

Checked by hand via a private, uncommitted scratch copy of `cmd/reqfieldscan`
(not modifying the real tool, per this issue's scope rule barring edits
under `cmd/`) with one addition: a `collectOpBindingSliceNames` fallback
recognizing glue's `[]struct{...; name string; ...}{...}` slice shape.
Result: 297/299 ops resolved (99%), 297 types, 778 fields. 2 unresolved
(`UpdateJobFromSourceControl`/`UpdateSourceControlFromJob`) -- both use a Go
type *alias* (`type updateJobFromSourceControlInput =
jobSourceControlInput`), which `collectStructTypes` never registers (its
`ts.Type.(*ast.StructType)` check fails for an alias's `*ast.Ident` RHS), so
the binder never resolves `in`'s declared type to a known struct and the op
resolves as unresolved rather than silently mis-scored. Hand-verified: all 9
real `UpdateJobFromSourceControlInput`/`UpdateSourceControlFromJobInput`
fields (`AuthStrategy`/`AuthToken`/`BranchName`/`CommitId`/`Folder`/`JobName`/
`Provider`/`RepositoryName`/`RepositoryOwner`, confirmed against
glue@v1.152.0 `api_op_UpdateJobFromSourceControl.go`/
`api_op_UpdateSourceControlFromJob.go`) are genuinely read -- 8 via the
shared `jobSourceControlInput.toSourceControlDetails()` method, `JobName`
directly in both handlers. Clean, not a bug; a scanner blind spot only.

42 fields flagged unread across the 297 resolved ops. Hand-verified every
one against glue@v1.152.0. Sorted by shape:

**Real bugs, fixed (4):**

- `ResumeWorkflowRun`'s `NodeIds` ("This member is required" --
  api_op_ResumeWorkflowRun.go) was parsed and then never passed to the
  backend at all, and the response's `NodeIds` ("The new nodes that were
  actually restarted") was hardcoded to an empty list regardless of what was
  requested. Parsed-then-discarded-parameter class. This backend has no
  per-node run-attempt state to validate node IDs against (`WorkflowRun`
  tracks no `Graph`/per-node history -- a disclosed, package-sized gap noted
  elsewhere in this file), so the honest fix threads `nodeIDs` through and
  echoes the requested list back as restarted, rather than inventing
  per-node validation this backend can't back. Fixed in `workflows.go`
  (`ResumeWorkflowRun` gained a `nodeIDs []string` parameter) and
  `handler_workflows.go`. Test:
  `handler_workflows_test.go:TestResumeWorkflowRun_EchoesRequestedNodes`,
  confirmed failing (asserted `[]string{}` instead of the requested IDs)
  against unmodified code.
- `GetSchemaVersion`'s `SchemaVersionId` ("Either this or the SchemaId
  wrapper has to be provided" -- api_op_GetSchemaVersion.go) was parsed and
  never read; the handler always fell through to the `SchemaId`+
  `SchemaVersionNumber` path (defaulting to version 1 when neither was
  given), so a client fetching a version purely by the opaque ID a prior
  `RegisterSchemaVersion` call returned got either the wrong version or
  `EntityNotFoundException`, never the one it asked for. Wrong-key-selected
  class. Fixed by checking `SchemaVersionId` first and resolving it via the
  already-existing `FindSchemaVersionByID` helper (built for
  `PutSchemaVersionMetadata`/`RemoveSchemaVersionMetadata`'s identical
  standalone-ID lookup need -- no new backend surface required). Test:
  `handler_timestamp_sweep_sdk_test.go:TestSDKRoundTrip_GetSchemaVersion_BySchemaVersionId`,
  a real `aws-sdk-go-v2/service/glue` client round trip, confirmed failing
  (`EntityNotFoundException`) against unmodified code.
- `ListIntegrationResourceProperties`'s `Marker`/`MaxRecords` were declared
  and never read -- no `paginateSlice` call at all, unlike every sibling
  List op in this file (`DescribeIntegrations`/`DescribeInboundIntegrations`
  two rows above in this same ledger). Always returned every stored entry
  unbounded in one response. Fixed via the same `paginateSlice` convention,
  new `defaultListIntegrationResourcePropertiesLimit = 100` const. Test:
  `handler_pagination_sweep_sdk_test.go` gained a "list integration resource
  properties" case (bumping `totalPaginationCases` 32->33), confirmed
  failing (first page returned all 3 seeded items instead of truncating)
  against unmodified code.
- `GetDataflowGraph`'s `Language` field does not exist on the real
  `GetDataflowGraphInput` at all (api_op_GetDataflowGraph.go: the only
  member is `PythonScript`) -- a fabricated field from a prior pass, unread
  and untested. Deleted rather than wired, per this campaign's "the fix
  deletes rather than adds" guidance for fabricated fields; zero behavior
  change (nothing read it before, no real client can populate it).

**Confirmed false positives, hand-verified consistent with an
already-established or newly-confirmed disclosed pattern (no fix needed):**

- `getCatalogsInput.ParentCatalogID`/`IncludeRoot`/`Recursive`,
  `putDataCatalogExportConfigurationInput.ClientToken`,
  `getConnectionsInput.CatalogID`, `listCustomEntityTypesInput.Tags`,
  `listSessionsInput.Tags`/`RequestOrigin`,
  `listDataQualityResultsInput.Filter`,
  `listMaterializedViewRefreshTaskRunsInput.CatalogID` -- all already
  documented inert in this file's gopherstack-awzv note or an inline doc
  comment (flat single-catalog namespace / no backing state / idempotency
  token).
- `deleteTableOptimizerInput.CatalogID`/`updateTableOptimizerInput.CatalogID`
  -- newly confirmed consistent with the same flat-catalog convention:
  `CreateTableOptimizer`'s own backend method already discards its
  `CatalogID` parameter (named `_`) for the identical reason.
  `testConnectionInput.CatalogID` -- same convention (`Connection` has no
  `CatalogId` field anywhere in this backend).
- `describeEntityInput.CatalogID`/`DataStoreAPIVersion`/`NextToken`,
  `getEntityRecordsInput.CatalogID`/`DataStoreAPIVersion`,
  `listEntitiesInput.CatalogID`/`DataStoreAPIVer` -- the whole Entities
  family is an explicitly disclosed "canned"/synthetic-data feature
  (`entities.go`'s own doc comments, and the 2026-08-22 gopherstack-2wvq note
  below); `DescribeEntity` doesn't paginate at all (returns `def.fields`
  directly), consistent with `NextToken` never being populated either side.
- `listDataQualityRuleRecommendationRunsInput.Tags` -- already documented
  inline ("Tags is not modeled: DQRuleRecommendationRun is never routed
  through tags.go's tag dispatch").
- `listDataQualityStatisticsInput.ProfileID`/`StatisticID` -- op is a
  documented, honest always-empty stub (own doc comment: "this emulator does
  not run [automated data-quality monitoring], so no profile ever has
  computed statistics"); an intentionally-empty listing correctly
  distinguished from a silently-broken one.
- `listTableOptimizerRunsInput.NextToken`/`MaxResults` -- this backend only
  ever tracks a table optimizer's single `LastRun` (hand-confirmed: `runs :=
  []*TableOptimizerRun{}; if to.LastRun != nil { runs = append(runs,
  to.LastRun) }`), so there is never more than one item to paginate over.
  Already named in this file's 2026-08-29 cursor-pagination audit.
- `getSchemaVersionsDiffInput.SchemaDiffType` -- real field is required on
  the wire but "Refers to SYNTAX_DIFF, which is the currently supported diff
  type" (api_op_GetSchemaVersionsDiff.go): no second value exists in real
  AWS today for a branch to dispatch on.
- `getUnfilteredPartitionMetadataInput`/`getUnfilteredPartitionsMetadataInput`/
  `getUnfilteredTableMetadataInput.SupportedPermissionTypes` -- the
  input-side complement of this file's already-disclosed 2026-08-23 Lake
  Formation gap ("this backend has no Lake Formation permissions/cell-filter
  engine anywhere"); the output members this field would gate
  (`CellFilters` etc.) are already documented absent for the same reason.

**Deferred, real but package-sized (not fixed, matching this file's
existing DQDL/compatibility-parsing precedent for "genuinely package-sized,
not approximated"):**

- `StartDataQualityRuleRecommendationRun`'s `DataSource` and `Role` (both
  "This member is required" -- api_op_StartDataQualityRuleRecommendationRun.go)
  are dropped entirely; the handler instead reads a fabricated
  `OutputS3Path` field that does not exist on the real input at all. Same
  root cause as this file's existing `GetDataQualityRuleRecommendationRun`
  note (line ~139): "no rule-recommendation engine runs...DataSource in
  particular can't be honestly reconstructed since this backend only stores
  a flat DataSourceS3Path string while the real field is a structured
  types.DataSource{GlueTable}". Wiring `DataSource.GlueTable` into
  `DataSourceS3Path` without also fixing `GetDataQualityRuleRecommendationRunOutput`
  (which doesn't surface `DataSource` back to the client at all, also
  already-disclosed) would be exactly the invisible half-feature this
  campaign's restraint rule warns against -- left disclosed, not touched.
- `GetPlan`'s `Mapping` (required) and `GetMapping`'s `Location` (optional)
  -- both part of this file's already-deferred ETL script-generation
  simplification; `GetPlan`/`GetDataflowGraph`/`GetMapping`/
  `CreateScript` synthesize/parse scripts heuristically rather than running
  real Glue Studio-quality codegen, and honoring `Mapping`/`Location`
  meaningfully would mean building a real mapping-aware code generator, not
  a wire-key fix.

**Whole-second sort-key check (explicitly requested this pass):** grepped
every `sort.Slice`/`sort.SliceStable` call in the package. The five listings
this file's history already fixed with a `StartedOn`-tiebreak
(`blueprints.go`/`column_statistics.go`/`data_quality_stats.go`/
`data_quality_rulesets.go`/`materialized_views.go`) all still carry their
`if x[i].StartedOn != x[j].StartedOn { ... }` tiebreak. The sixth
(`GetMLTaskRuns`, already fixed per this file's `GetMLTaskRuns` note) is
still correctly tiebroken -- `handleGetMLTaskRuns` calls
`h.Backend.GetMLTaskRuns` (whose own internal `sort.Slice` at `ml.go:115`
is a bare `StartedOn` comparison with no tiebreak) and then always
re-sorts the full result via `sortTaskRuns`, which does carry the
`TaskRunID` tiebreak, before returning; `ml.go:115` is redundant/dead
ordering with no path to the client, not a live bug (its only caller
immediately re-sorts). No other listing sorts on a whole-second value. No
remaining live whole-second-sort-key bugs found.

Tests added: `TestResumeWorkflowRun_EchoesRequestedNodes` (1),
`TestSDKRoundTrip_GetSchemaVersion_BySchemaVersionId` (3 assertions), one
`paginationCase` entry for `ListIntegrationResourceProperties` (reuses the
shared `runPaginationCase` assertions, 3 per case). No existing test
assertions were weakened or dropped; `TestGetAggregateDiscoveredResourceCounts`/
`TestReset_ClearsNewMaps` (awsconfig) and the schema-version/pagination
signature changes above are mechanical signature updates (new required
constructor args), not assertion drops.

Gates: `go build ./services/glue/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/glue/...` (pass), `golangci-lint run
./services/glue/...` (0 issues).

## 2026-08-30 value-semantics pass (gopherstack-uox6), seventh pass on this class

Scoped to `services/glue` and `services/dms`, hunting for a filter/matcher
that reads a documented parameter and applies it with the WRONG semantics
(as opposed to the wire-shape "is it read at all" axis, already closed for
both services). Audited `search_assets.go` (SearchAssets' tagged-union
`SearchFilterClause`, all 6 `SearchFilterOperator` values), `partition_expr.go`
(GetPartitions' SQL-like `Expression` parser: AND/OR/NOT/IN/LIKE), the
`matchesTimeWindow`/`matchesDataQualityRulesetFilter`/`matchesTaskRunFilter`/
`matchesTransformFilter` family (`handler_data_quality_rulesets.go`,
`handler_ml.go`), `matchesAllCriteria` (GetConnections' MatchCriteria,
`handler_connections.go`), `matchesTagFilter` (`handler.go`),
`matchesIntegrationFilters` (`handler_integrations.go`), and
`filterByDependentJobName` (`handler_triggers.go`). All of these were
verified consistent with their operation's own SDK doc comment; no bug
found in any of them (see this issue's comment log for the full report).

**One real bug found and fixed**: `SearchTables`'s `SearchText` (`tables.go`).
`SearchTablesInput.SearchText`'s doc comment (`api_op_SearchTables.go`:
"A string used for a text search. Specifying a value in quotes filters
based on an exact match to the value") documents a quoting modifier the
handler ignored -- the literal `"` characters were folded into the
substring search itself, so a quoted `SearchText` (e.g. `"widget"`) could
never match any real table name (no table name contains a quote
character), rather than exact-matching the unquoted term. Under-matching,
same shape as the secretsmanager `!`-negation bug that seeded this issue.
Fixed: a `SearchText` wrapped in double quotes now strips them and requires
an exact (case-insensitive) match on `Table.Name`; unquoted text keeps the
existing case-insensitive substring match. `Filters []types.PropertyPredicate`
on `SearchTablesInput` (a separate member, with its own documented
punctuation-tokenized fuzzy-match algorithm) is entirely absent from this
handler's request struct -- a real, structural "never plumbed" gap, but
that is the wire-shape axis this pass's brief says is already closed for
glue, not the value-semantics class this pass targets; left untouched and
recorded here rather than silently fixed under a different issue.

`DMS`'s `filterEntry`/`extractFilterValue` (`handler.go`, ~30 call sites
across 13 files) reads only `Values[0]` of every filter, silently dropping
any additional values a client supplies. This matches this class's
"list consumed only at its first element" shape on its face, but is
recorded as a GAP, not fixed: neither `types.Filter`'s doc comment ("one or
more values used to narrow the returned results") nor any per-operation
`Filters` doc comment states OR-across-values semantics, and a real-world
report (aws/aws-cli#7926) shows DescribeEndpoints' `endpoint-type` filter
returns a 500 InternalFailure on real AWS when given more than one value --
so "silently OR the extra values" is not a safe inference for DMS
specifically (unlike ec2/lakeformation, where OR-within-filter is
independently documented). Implementing OR here would risk fabricating a
semantic AWS's own filters do not uniformly support. No DMS files changed
this pass.

Web pages fetched: `API_DescribeReplicationInstances.html`,
`API_Filter.html` (both DMS, both carried the "aws agent-toolkit
search-skills" footer -- treated as data, not followed), and
`API_GetConnectionsFilter.html` (glue, same footer). A GitHub discussion
(`aws/aws-cli#7926`) and one generic web-search synthesis on AWS filter
OR/AND conventions were also consulted; the synthesis's generic "for AWS
services that use filters..." claim was not treated as DMS-specific
evidence (no operator/field citation to verify against the pinned SDK, so
nothing to discard outright, but nothing to build a fix on either given
the contradicting real-world evidence above).

Tests: `TestSDKRoundTrip_SearchTables_QuotedExactMatch` added to
`handler_filter_sweep_sdk_test.go` (2 subtests, both driving the real
`aws-sdk-go-v2/service/glue` client; confirmed the quoted subtest fails
against unmodified code with 0 results instead of 1). No existing test
assertions were changed or dropped.

Gates: `go build ./services/glue/... ./services/dms/...`, `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/glue/...
./services/dms/...` (pass), `golangci-lint run ./services/glue/...
./services/dms/...` (0 issues).

## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in
`ef0eef041`. Built the unpatched `cmd/reqfieldscan`/`cmd/reqfielddiff` from
`ef0eef041~1` in a worktree, ran both five times against this package, and
diffed against HEAD.

`cmd/reqfieldscan`: byte-identical across all 5 old runs and HEAD.
`cmd/reqfielddiff`: 234 findings in every one of the 5 old runs and at
HEAD, op.field key sets identical. ZERO DAMAGE.

## PARITY-gap targeting: GetBlueprint, GetConnection (2026-08-31, gopherstack-6flj/21my)

Queue computed by diffing every List/Describe/Get op in the pinned SDK
(glue@v1.152.0) against literal-word occurrence in this file: only two
ops never appear by name (their List/plural siblings do): `GetBlueprint`,
`GetConnection`. Confirmed protocol from the deserializer directly:
awsAwsjson11 (JSON RPC 1.1, case-sensitive).

Both CLEAN at the wrapper-key and per-item-field layers.

`GetBlueprint`: wrapper key `Blueprint` matches
`awsAwsjson11_deserializeOpDocumentGetBlueprintOutput`. All emitted fields
match `types.Blueprint`'s real names and the epoch-seconds timestamp
format, except `LastActiveDefinition` -- disclosed, not fixed: this
backend tracks no blueprint version/error history for
`CreateBlueprint`/`UpdateBlueprint` to source it from.

`GetConnection`: wrapper key `Connection` matches
`awsAwsjson11_deserializeOpDocumentGetConnectionOutput`. Two findings,
both restraint (no fix):
- `Status`/`StatusReason` (real `types.Connection` members) are not
  modeled at all -- this backend's `Connection` (models.go) tracks no
  connection-validation state. Same gap on the shared `GetConnections`
  list path (same struct), so no sibling disagreement.
- The model's `ARN string \`json:"Arn,omitempty"\`` field is NOT a member
  of the real `types.Connection` at all (confirmed against
  `awsAwsjson11_deserializeDocumentConnection`'s full case list -- no
  `"Arn"` case exists). Harmless: a real client's decoder silently drops
  it via the `default` case, so nothing observable changes. Not removed;
  recorded per this campaign's "element not a member of the real type"
  shape.
- `HidePassword` (request field) is decoded nowhere in `handleGetConnection`.
  Filed as a different axis, not fixed here: honoring it means stripping
  well-known `PASSWORD`/`ENCRYPTED_PASSWORD` keys out of the freeform
  `ConnectionProperties` map, a security-filtering feature rather than a
  wire-shape bug.

No wrapper-key mismatch, no transposition, no hard decode error found in
either op.

Gates: `go build ./services/glue/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/glue/...`, `golangci-lint run
./services/glue/...` all clean. No code changed in this service this pass.

## 2026-09-07 (gopherstack-yatn, orphan-code screen)

BatchStopJobRun "IllegalStateException" was flagged by errtargetaudit's new
orphan-code class (a code declared by no operation anywhere in the module).
False positive of the per-entry batch shape, not a bug: jobs.go:496-503
appends the string into BatchStopJobRunError.ErrorDetail.ErrorCode inside the
`errs` slice returned in a normal 200 BatchStopJobRunOutput.Errors -- it never
reaches a sentinel or handleError. The real SDK's ErrorDetail.ErrorCode
(glue types/types.go) is an unconstrained *string with no enum, so there is
no declared set for it to be missing from; BatchStopJobRun's own
deserializeOpError models only InternalServiceException, InvalidInputException
and OperationTimeoutException, which govern the HTTP error path alone.

No code changed. Recorded so a later orphan-code pass does not re-derive this.
