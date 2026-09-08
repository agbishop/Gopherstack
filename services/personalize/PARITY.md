---
# This pass (2026-08-28, wrapper-key/write-only-state sweep) treated the existing
# wire_field_fixes_test.go (gopherstack-sm02's wrapper-key fixes) as a PARTIAL pass per the
# campaign's own rule, not proof of completeness, and found three real write-only-state bugs the
# extensive prior List-op-leak sweep had not caught: (1) UpdateSolutionInput.SolutionUpdateConfig
# (AutoTrainingConfig/EventsConfig, present on the pinned v1.50.4 SDK) was accepted by the real
# client but never read anywhere in this package -- this file's own UpdateSolution note incorrectly
# claimed the real input "only carries performAutoTraining and performIncrementalUpdate", which was
# true against an older SDK but not this pinned one; (2) CreateSolutionVersionInput.Name was never
# read by the handler at all (input["name"] was never looked up), so it was silently dropped; (3)
# the real, always-present Recommender.ModelMetrics member (a plain map[string]float64) was
# completely absent and not documented anywhere in this file -- an audit miss, not a scoped-down
# decision, now a deterministic ARN-hash mock following the same convention already established for
# SolutionVersion metrics. All three are covered by new round-trip tests in
# wire_field_fixes_test.go (real aws-sdk-go-v2 client, fail-before/pass-after verified). enumcheck/
# zeroguard report no findings for this service. `go build`, `go vet`, `go test -race -count=1`,
# and `golangci-lint run`, all scoped to ./services/personalize/..., pass clean.
service: personalize
sdk_module: aws-sdk-go-v2/service/personalize@v1.50.4  # go.mod pins v1.50.4; prior audit passes cited v1.47.11 in this file -- this pass verified every field/citation below against the actually-pinned v1.50.4 module in the Go module cache
sibling_sdk_modules: [aws-sdk-go-v2/service/personalizeruntime@v1.36.2]  # GetRecommendations/GetPersonalizedRanking; see the Runtime family below
last_audit_commit: 12cf224d  # this pass (2026-08-13, gopherstack-sm02) fixed all 16 List-op Get-field leaks; commit hash not yet known at edit time
last_audit_date: 2026-08-13
overall: A
ops:
  CreateDatasetGroup: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'added domain enum validation (ECOMMERCE/VIDEO_ON_DEMAND, or empty for a Custom group) -- an unrecognized value previously succeeded silently'}
  DescribeDatasetGroup: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-7z3p (2026-09-06): datasetGroupToMap omitted failureReason unconditionally even though the SDK deserializer reads it and the sibling ListDatasetGroups summary already emitted it conditionally -- now matches (see the 2026-09-06 entry below)'}
  DeleteDatasetGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDatasetGroups: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.DatasetGroupSummary (datasetGroupArn/name/domain/status/creationDateTime/lastUpdatedDateTime/failureReason) via a dedicated datasetGroupSummaryToMap instead of reusing datasetGroupToMap unscoped -- dropped kmsKeyArn/roleArn, and added failureReason (a real Summary member the backend model already carried but no converter emitted)'}
  CreateDataset: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'added FK validation on datasetGroupArn/schemaArn (ResourceNotFoundException for a dangling reference) and datasetType enum validation (case-insensitive INTERACTIONS/ITEMS/USERS/ACTIONS/ACTION_INTERACTIONS) -- both previously unvalidated'}
  DescribeDataset: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDataset: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDataset: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDatasets: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.DatasetSummary via datasetSummaryToMap instead of the unscoped Describe converter -- dropped datasetGroupArn/schemaArn'}
  CreateSchema: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'added domain enum validation (ECOMMERCE/VIDEO_ON_DEMAND, or empty), same as CreateDatasetGroup'}
  DescribeSchema: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteSchema: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSchemas: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.DatasetSchemaSummary via schemaSummaryToMap instead of the unscoped Describe converter -- dropped schema (the full Avro body, Get-only)'}
  CreateSolution: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'this pass: added FK validation on datasetGroupArn (always required) and recipeArn (required only when performAutoML is false); added eventType (a plain CreateSolutionInput member that was completely unread) and solutionConfig (opaque round-trip) and autoMLResult (populated with a deterministic bestRecipeArn when performAutoML is true). Prior pass: added performAutoTraining (default true)/performIncrementalUpdate, previously silently dropped'}
  DescribeSolution: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'now populates latestSolutionVersion (types.SolutionVersionSummary, a cross-table lookup over solutionVersions picking the max CreationDateTime for this solutionArn) -- previously absent entirely. Not added to ListSolutions: types.SolutionSummary has no latestSolutionVersion member'}
  UpdateSolution: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'wrapper-key/write-only-state sweep pass: UpdateSolutionInput.SolutionUpdateConfig (AutoTrainingConfig/EventsConfig, added to the pinned v1.50.4 SDK -- this file previously and incorrectly claimed UpdateSolutionInput "only carries performAutoTraining and performIncrementalUpdate") was accepted by the real client but silently dropped entirely; now merged into the solution\'s SolutionConfig and readable back via DescribeSolution, and recorded on latestSolutionUpdate.solutionUpdateConfig. Prior pass: now populates latestSolutionUpdate (types.SolutionUpdateSummary-shaped) on every successful call, absent until the first update, matching the real API. Earlier pass: was reading performAutoML/performHPO, fields that do not exist on the real UpdateSolutionInput -- real SDK calls were a silent no-op. Now reads performAutoTraining/performIncrementalUpdate (*bool, nil = unchanged)'}
  DeleteSolution: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSolutions: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: worst leak in the service -- was emitting solutionToMap(sol, nil), a 12+-field Describe shape, for a types.SolutionSummary that only declares 6 (solutionArn/name/recipeArn/status/creationDateTime/lastUpdatedDateTime). Dropped datasetGroupArn/eventType/performAutoML/performHPO/performAutoTraining/performIncrementalUpdate/solutionConfig/autoMLResult/latestSolutionUpdate (9 leaked members) via a new solutionSummaryToMap. The old comment here claimed correctness but only addressed the latestSolutionVersion sub-field -- corrected'}
  CreateSolutionVersion: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'wrapper-key/write-only-state sweep pass: the real, optional CreateSolutionVersionInput.Name member was accepted by the real client but never read by the handler at all (input["name"] was never looked up) -- now stored and echoed via DescribeSolutionVersion (types.SolutionVersionSummary has no Name member, so ListSolutionVersions stays unchanged). Earlier pass: datasetGroupArn/eventType/performAutoML/performHPO/performIncrementalUpdate/recipeArn are now also copied from the parent Solution at creation time (types.SolutionVersion, types.go:2074), snapshotted as plain field copies so a later UpdateSolution cannot retroactively change an already-created version. Earliest pass: added FK validation on solutionArn; solutionConfig is inherited from the parent solution onto the version, matching the real SolutionVersion.solutionConfig field'}
  DescribeSolutionVersion: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'now echoes name (see CreateSolutionVersion)'}
  ListSolutionVersions: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: second-worst leak -- was calling solutionVersionToMap (the full Describe shape, 12 fields) instead of the already-existing solutionVersionSummaryToMap (7 fields, previously only used for Solution.latestSolutionVersion). Dropped solutionArn/datasetGroupArn/recipeArn/eventType/performAutoML/performHPO/performIncrementalUpdate/trainingHours/solutionConfig (9 leaked members) by swapping which converter the handler calls -- no new converter needed here'}
  StopSolutionVersionCreation: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'was setting status to "STOPPED", not a valid SolutionVersion.Status enum member; fixed to "CREATE STOPPED"'}
  GetSolutionMetrics: {wire: ok, errors: ok, state: ok, persist: n/a}
  CreateCampaign: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'added FK validation on solutionVersionArn and campaignConfig (enableMetadataWithRecommendations/itemExplorationConfig/etc., opaque round-trip) support -- both previously missing'}
  DescribeCampaign: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCampaign: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'added FK validation on solutionVersionArn (when supplied), campaignConfig support, and latestCampaignUpdate (types.CampaignUpdateSummary-shaped) population on every successful call -- previously the real UpdateCampaignInput.campaignConfig member was silently dropped and no update history was tracked'}
  DeleteCampaign: {wire: ok, errors: ok, state: ok, persist: ok}
  ListCampaigns: {wire: fixed, errors: ok, state: ok, persist: ok, filter: fixed, note: 'gopherstack-sm02: now emits types.CampaignSummary via campaignSummaryToMap -- dropped solutionVersionArn/minProvisionedTPS/campaignConfig/latestCampaignUpdate (4 leaked members). failureReason is a real CampaignSummary member but the backend Campaign model has no source for it (campaigns never fail asynchronously here), so it stays absent rather than fabricated. Filter-not-honoured sweep (2026-08-29): the SolutionArn filter compared it for exact equality against Campaign.SolutionVersionArn, which is always SolutionArn + "/" + versionID (solutions.go:208) -- that equality is never true, so the filter silently excluded every campaign instead of narrowing to one solution''s. Fixed to a prefix match.'}
  CreateEventTracker: {wire: fixed, errors: ok, state: fixed, persist: ok, note: added FK validation on datasetGroupArn}
  DescribeEventTracker: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEventTracker: {wire: ok, errors: ok, state: ok, persist: ok}
  ListEventTrackers: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.EventTrackerSummary via eventTrackerSummaryToMap -- dropped datasetGroupArn/trackingId (2 leaked members)'}
  CreateFilter: {wire: fixed, errors: ok, state: fixed, persist: ok, note: added FK validation on datasetGroupArn}
  DescribeFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFilters: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.FilterSummary via filterSummaryToMap -- dropped filterExpression (1 leaked member). failureReason is a real FilterSummary member but the backend Filter model has no source for it, so it stays absent rather than fabricated'}
  CreateRecommender: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'added FK validation on datasetGroupArn and recipeArn (against the built-in recipe catalog); recommenderConfig now round-trips in full (previously only minRecommendationRequestsPerSecond was extracted from the sub-object -- enableMetadataWithRecommendations/itemExplorationConfig/etc. were silently dropped, a disguised-partial-implementation bug)'}
  DescribeRecommender: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'wrapper-key/write-only-state sweep pass: the real, always-present Recommender.ModelMetrics member (types.go:1697, a plain map[string]float64 with no fixed key set) was completely absent -- not documented anywhere in this file either, an audit miss rather than a scoped-down decision. Now a deterministic ARN-hash mock (no real training pipeline exists here), following the same convention already established for SolutionVersion metrics (solutions.go svMetric/GetSolutionMetrics)'}
  UpdateRecommender: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'recommenderConfig is a required member on the real UpdateRecommenderInput and is now enforced (was silently optional); now round-trips in full (see CreateRecommender) and populates latestRecommenderUpdate on every successful call, absent until the first update'}
  DeleteRecommender: {wire: ok, errors: ok, state: ok, persist: ok}
  ListRecommenders: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.RecommenderSummary via recommenderSummaryToMap -- dropped latestRecommenderUpdate (1 leaked member). Unlike its List siblings, RecommenderSummary does declare recommenderConfig, so that field is kept (not dropped) -- verified individually rather than assumed by analogy'}
  StartRecommender: {wire: ok, errors: ok, state: ok, persist: ok}
  StopRecommender: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateMetricAttribution: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'this pass: added FK validation on datasetGroupArn. Prior pass: metrics is a required field on the real API and was silently ignored; now required + stored'}
  DescribeMetricAttribution: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateMetricAttribution: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'real request uses addMetrics/removeMetrics, not a metrics replacement; was silently dropped'}
  DeleteMetricAttribution: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMetricAttributions: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.MetricAttributionSummary via metricAttributionSummaryToMap -- dropped datasetGroupArn/metricsOutputConfig (2 leaked members)'}
  ListMetricAttributionMetrics: {wire: fixed, errors: ok, state: fixed, persist: ok, note: 'was a hardcoded fabricated 2-entry list ignoring the actual attribution; now returns the attribution''s real, paginated Metrics'}
  CreateDatasetImportJob: {wire: fixed, errors: ok, state: fixed, persist: ok, note: added FK validation on datasetArn}
  DescribeDatasetImportJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDatasetImportJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.DatasetImportJobSummary via datasetImportJobSummaryToMap -- dropped datasetArn/roleArn/dataSource (3 leaked members)'}
  CreateDatasetExportJob: {wire: fixed, errors: ok, state: fixed, persist: ok, note: added FK validation on datasetArn}
  DescribeDatasetExportJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDatasetExportJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.DatasetExportJobSummary via datasetExportJobSummaryToMap -- dropped datasetArn/roleArn/jobOutput (3 leaked members)'}
  CreateBatchInferenceJob: {wire: fixed, errors: ok, state: fixed, persist: ok, note: added FK validation on solutionVersionArn}
  DescribeBatchInferenceJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListBatchInferenceJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.BatchInferenceJobSummary via batchInferenceJobSummaryToMap -- dropped roleArn/jobInput/jobOutput (3 leaked members). batchInferenceJobMode and failureReason are real Summary members but the backend model has no source for either, so both stay absent rather than fabricated'}
  CreateBatchSegmentJob: {wire: fixed, errors: ok, state: fixed, persist: ok, note: added FK validation on solutionVersionArn}
  DescribeBatchSegmentJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListBatchSegmentJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.BatchSegmentJobSummary via batchSegmentJobSummaryToMap -- dropped roleArn/jobInput/jobOutput (3 leaked members). failureReason is a real Summary member but the backend model has no source for it, so it stays absent rather than fabricated'}
  CreateDataDeletionJob: {wire: fixed, errors: ok, state: fixed, persist: ok, note: added FK validation on datasetGroupArn}
  DescribeDataDeletionJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDataDeletionJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: 'gopherstack-sm02: now emits types.DataDeletionJobSummary via dataDeletionJobSummaryToMap -- dropped roleArn/dataSource/numDeleted (3 leaked members). failureReason is a real Summary member but the backend model has no source for it, so it stays absent rather than fabricated'}
  DescribeRecipe: {wire: ok, errors: ok, state: n/a, persist: n/a}
  ListRecipes: {wire: fixed, errors: ok, state: n/a, persist: n/a, filter: n/a, note: 'gopherstack-sm02: now emits types.RecipeSummary via recipeSummaryToMap -- dropped recipeType (1 leaked member, Describe-only). domain/creationDateTime/lastUpdatedDateTime are real RecipeSummary members but the built-in static recipe catalog has no source for any of them, so all three stay absent rather than fabricated. CHECKED 2026-08-30 (wire-key-read sweep): request-side ListRecipesInput.domain/recipeProvider are also declared and unread -- deliberately, not a bug: recipeProvider (types.RecipeProvider) has exactly one legal value (SERVICE) and this catalog is 100% SERVICE-provided, so the filter can never exclude anything; domain would need domain-specific recipe data this catalog does not model (same missing-data reason as the response-side domain field noted above), so filtering by it is left unimplemented rather than fabricated.'}
  DescribeFeatureTransformation: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAlgorithm: {wire: ok, errors: ok, state: n/a, persist: n/a}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRecommendations: {wire: ok, errors: ok, state: ok, persist: n/a, note: "real personalizeruntime.Client op (confirmed by name against aws-sdk-go-v2/service/personalizeruntime), not personalizesdk.Client -- pkgs/sdkcheck's reverse check flagged this as 'phantom' only because it compared against the control-plane client; sdk_completeness_test.go now checks it against personalizeruntimesdk.Client (2026-07-31, gopherstack-vhw2)"}
  GetPersonalizedRanking: {wire: ok, errors: ok, state: ok, persist: n/a, note: "same as GetRecommendations -- real personalizeruntime.Client op, now checked against the correct sibling client"}
families:
  DatasetGroup/Dataset/Schema: {status: fixed, note: 'ARNs, timestamps (awstime.Epoch), field shapes verified against types.DatasetGroup/Dataset/DatasetSchema deserializers; Schema correctly has no status field (matches real API). This pass (gopherstack-sm02): ListDatasetGroups/ListDatasets/ListSchemas now use dedicated types.DatasetGroupSummary/DatasetSummary/DatasetSchemaSummary converters instead of the unscoped Describe converter (see ops). Prior pass: domain enum validation on DatasetGroup/Schema, datasetType enum validation + datasetGroupArn/schemaArn FK validation on Dataset (see ops)'}
  Solution/SolutionVersion: {status: fixed, note: 'This pass: SolutionVersion now models datasetGroupArn/eventType/performAutoML/performHPO/performIncrementalUpdate/recipeArn/failureReason, snapshotted from the parent Solution at CreateSolutionVersion time (not a live lookup); Solution.latestSolutionVersion (types.SolutionVersionSummary) now populated on DescribeSolution via a solutionVersions cross-table lookup; SolutionConfig deep-typed (see Campaign/Recommender family note) -- verified field-by-field against types.Solution/types.SolutionVersion/types.SolutionVersionSummary. Prior pass: datasetGroupArn/recipeArn/solutionArn FK validation, eventType/solutionConfig/autoMLResult/latestSolutionUpdate wire fields added; CreateSolution/UpdateSolution wire bug fixed; StopSolutionVersionCreation status-string bug fixed'}
  Campaign/EventTracker/Filter/Recommender: {status: fixed, note: 'Create/Describe/Update/Delete field shapes verified against types.CampaignSummary/EventTrackerSummary/FilterSummary/RecommenderSummary. This pass (gopherstack-sm02): List* for all four now use dedicated Summary-scoped converters instead of the unscoped Describe converter -- the "extra fields are harmless because real deserializers ignore them" reasoning that used to justify skipping this was WRONG (see the corrected note below) and had let a real 1-4-member leak per op go unflagged across several prior audit passes; see ops for the per-op diff. Prior pass: CampaignConfig/RecommenderConfig (and SolutionConfig, above) are now deep-typed real Go structs (types.CampaignConfig/RecommenderConfig/SolutionConfig and their nested sub-objects, types.go) instead of opaque map[string]any passthrough -- a caller-supplied field with no counterpart in the real API is now dropped rather than echoed back; Recommender''s duplicated minRecommendationRequestsPerSecond bookkeeping now stays in sync with recommenderConfig''s own typed field instead of being hand-merged in the response builder; datasetGroupArn FK validation on EventTracker/Filter, datasetGroupArn+solutionVersionArn+recipeArn FK validation on Campaign/Recommender, campaignConfig/recommenderConfig full round-trip + latestCampaignUpdate/latestRecommenderUpdate (see ops)'}
  MetricAttribution: {status: fixed, note: 'This pass (gopherstack-sm02): ListMetricAttributions now uses a dedicated types.MetricAttributionSummary converter instead of the unscoped Describe converter (see ops). Prior pass: metrics/addMetrics/removeMetrics/ListMetricAttributionMetrics fixed; datasetGroupArn FK validation added (see ops)'}
  Async jobs (DatasetImportJob/DatasetExportJob/BatchInferenceJob/BatchSegmentJob/DataDeletionJob): {status: fixed, note: 'no Delete/Update ops in the real API either -- gopherstack correctly omits them. This pass (gopherstack-sm02): all five List* ops now use dedicated Summary-scoped converters instead of the unscoped Describe converter, dropping 3 leaked members per op (see ops). Prior pass: datasetArn/solutionVersionArn/datasetGroupArn FK validation added to every Create* op (see ops)'}
  Recipe/Algorithm/FeatureTransformation: {status: fixed, note: 'built-in read-only catalogs, ARNs/status/timestamps verified. This pass (gopherstack-sm02): ListRecipes now uses a dedicated types.RecipeSummary converter instead of returning the full DescribeRecipe entry, dropping recipeType. CHECKED 2026-08-30 (wire-key-read sweep): ListRecipesInput.domain/recipeProvider are declared and unread by listRecipes (recipes.go) -- deliberately left unread, not a bug. recipeProvider: types.RecipeProvider (personalize@v1.50.4 types/enums.go) has exactly one legal value, SERVICE -- this service has no CreateRecipe/custom-recipe path, so every recipe in getBuiltinRecipes() is implicitly SERVICE-provided; the filter can never exclude anything a real client could legally send, so reading it would be a no-op with no observable effect. domain: types.Domain has real values (ECOMMERCE, VIDEO_ON_DEMAND) for AWS-provided domain-specific recipe catalogs, but getBuiltinRecipes() models only the general-purpose (domain-less) recipes -- this backend holds no domain-specific recipe data at all, so filtering by domain would either fabricate matches or (more likely, if implemented "honestly") wrongly return empty for a domain real AWS does serve recipes for. Missing backend data, not a misread key -- left absent rather than guessed.'}
  Tags: {status: ok, note: 'tagKey/tagValue round-trip verified; arnExists() FK check spans all 16 resource tables correctly'}
  Runtime (GetRecommendations/GetPersonalizedRanking): {status: ok, note: 'ValidateCampaign/ValidateCampaignOrRecommender FK checks present and correct -- this pass extended the same validate-parent-existence discipline to every control-plane Create* op, closing the inconsistency previously noted here. UPDATE (2026-07-31, reverse sdkcheck sweep, gopherstack-vhw2): both are real aws-sdk-go-v2/service/personalizeruntime ops, not personalize ops -- added the module to go.mod and pointed sdk_completeness_test.go at it directly. That client also has a third op, GetActionRecommendations, which this Handler does not implement (listed as notImplemented in the completeness check; not otherwise audited this sweep).'}
gaps: []
deferred: []
leaks: {status: clean, note: no goroutines/janitors in this backend; all state is synchronous map/table mutation under lockmetrics.RWMutex. This pass added no new goroutines, tickers, or persistence-relevant fields requiring cleanup.}
---

## Notes

- **Protocol**: awsjson1.1, single POST endpoint, `X-Amz-Target:
  AmazonPersonalize.<Op>` (control plane) or
  `AmazonPersonalizeRuntime.<Op>` (GetRecommendations/GetPersonalizedRanking).
  `Handler.RouteMatcher` accepts both prefixes; `ExtractOperation` strips
  whichever matched. Confirmed both prefixes are exercised by
  handler_test.go / handler_runtime_test.go.

- **Invented op removed this pass**: `DeleteSolutionVersion` was registered
  and routed (`handler.go` `buildOps`) but has no equivalent in the real
  `aws-sdk-go-v2/service/personalize` v1.47.11 `Client` (verified: no
  `api_op_DeleteSolutionVersion.go`, no `Client.DeleteSolutionVersion`
  method -- the real API can only delete a whole solution and all its
  versions together, via `DeleteSolution`). A raw/boto3 caller hitting
  `X-Amz-Target: AmazonPersonalize.DeleteSolutionVersion` got `200` from
  gopherstack vs `UnknownOperationException` from real AWS. Per
  parity-principles (delete gopherstack-invented ops not in the real SDK),
  the route, handler function, and backend method were all removed;
  `TestPersonalize_DeleteSolutionVersion_NotARealOperation` locks that the
  operation now falls through to the standard "operation not implemented"
  `InvalidInputException` path like any other unrecognized op name, and that
  the targeted solution version is left untouched.

- **Systemic FK-validation gap closed this pass**: every `Create*` op that
  references a parent resource ARN (`datasetGroupArn`, `solutionArn`,
  `solutionVersionArn`, `datasetArn`, `schemaArn`, `recipeArn`) now validates
  that the parent actually exists, returning `ResourceNotFoundException` for
  a dangling reference the same way real AWS does. This touched 12 Create
  ops (the 11 documented in the prior audit pass plus `CreateSolution`'s
  `datasetGroupArn`/`recipeArn`, which the prior audit's gap list omitted,
  and `CreateMetricAttribution`'s `datasetGroupArn`, found field-diffing this
  pass) and required rewriting every test fixture that relied on the lenient
  behavior -- `handler_test.go`'s `personalizeCreateCampaign` helper (and the
  new `personalizeCreateDatasetGroup`/`personalizeCreateSchema`/
  `personalizeCreateDataset`/`personalizeCreateSolution`/
  `personalizeCreateSolutionVersion` helpers it now composes) build a real
  parent chain instead of a made-up ARN. `handler_fk_validation_test.go` is a
  new table-driven test locking all 16 dangling-parent-ARN rejection paths
  (some ops have two independently-validated FK fields, e.g.
  `CreateDataset`'s `datasetGroupArn`+`schemaArn` and `CreateRecommender`'s
  `datasetGroupArn`+`recipeArn`). `recipeArn` validation is checked against
  the built-in recipe catalog (`recipeExists`, `recipes.go`) rather than a
  mutable resource table. `UpdateCampaign`'s optional `solutionVersionArn` is
  validated the same way when supplied (`TestPersonalize_
  UpdateCampaign_ValidatesSolutionVersionArn`). The Personalize Runtime ops
  (`GetRecommendations`/`GetPersonalizedRanking`) already validated via
  `ValidateCampaign`/`ValidateCampaignOrRecommender` before this pass -- the
  control plane now applies the same discipline consistently.

- **CampaignConfig/RecommenderConfig/SolutionConfig modeled this pass**: all
  three were previously completely unmodeled (Recommender's handler quietly
  extracted only `minRecommendationRequestsPerSecond` from
  `recommenderConfig` and dropped everything else -- a disguised
  partial-implementation bug). They now round-trip opaquely (whatever the
  caller sends on Create/Update comes back unmodified on Describe/List),
  matching the `DataSource`/`JobOutput`/`MetricsOutputConfig` pattern already
  used elsewhere in this backend for deeply nested optional AWS structures.
  Additionally: `latestCampaignUpdate`/`latestRecommenderUpdate`/
  `latestSolutionUpdate` (types.CampaignUpdateSummary/
  RecommenderUpdateSummary/SolutionUpdateSummary) are now populated on every
  successful Update call and correctly *absent* (not a fabricated empty
  object) until the first update, matching each type's real doc comment;
  `autoMLResult` (types.AutoMLResult.bestRecipeArn) is now populated when
  `performAutoML` is true; `eventType` (a plain `CreateSolutionInput` member
  that was completely unread) now round-trips; `SolutionVersion.solutionConfig`
  is now inherited from the parent `Solution` at training time. One real bug
  caught by the new tests during this pass: `AutoMLResult` was initially
  keyed `recipeArn` (copy-pasted from an unrelated constant) instead of the
  real wire field name `bestRecipeArn` -- caught by
  `TestPersonalize_Solution_ConfigEventTypeAndAutoMLResult` before landing.
  `UpdateRecommenderInput.recommenderConfig` is a *required* member on the
  real API (unlike the optional field on Create) and is now enforced.

- **SolutionVersion parent-field copy, Solution.latestSolutionVersion, and
  CampaignConfig/RecommenderConfig/SolutionConfig deep-typing (this pass,
  gopherstack-b5mw)**: the three gaps recorded above are now fixed.
  `CreateSolutionVersion` copies `datasetGroupArn`/`eventType`/
  `performAutoML`/`performHPO`/`performIncrementalUpdate`/`recipeArn` from
  the parent `Solution` (`types.SolutionVersion`, `types.go:2074`) as plain
  field copies at creation time -- verified snapshot (not live-lookup)
  semantics with `TestPersonalize_SolutionVersion_SnapshotNotLiveLookup`,
  which updates the parent solution after the version exists and asserts the
  version keeps its original value while the parent reflects the update.
  `failureReason` is modeled but never populated (this backend has no
  asynchronous training failure path) and is correctly omitted, not a
  fabricated empty string. `DescribeSolution` now populates
  `latestSolutionVersion` (`types.SolutionVersionSummary`, `types.go:2164`,
  confirmed against the "latestSolutionVersion" case in
  `deserializers.go:15334`) via a `solutionVersions` cross-table scan for the
  max `CreationDateTime`; `trainingType` is hardcoded `"MANUAL"` since this
  backend has no autotraining scheduler (every version is created via an
  explicit `CreateSolutionVersion` call). `ListSolutions` deliberately does
  *not* get this field: `types.SolutionSummary` has no `latestSolutionVersion`
  member. `SolutionConfig`/`CampaignConfig`/`RecommenderConfig` and their
  sub-objects (`AutoMLConfig`, `AutoTrainingConfig`, `EventsConfig`,
  `EventParameters`, `HPOConfig`, `HPOObjective`, `HPOResourceConfig`,
  `HyperParameterRanges` and its three range types, `OptimizationObjective`,
  `TrainingDataConfig`) are now real Go structs in `configs.go`, each cited
  against its `types.go` line, replacing the `map[string]any` passthrough
  the prior pass's note above described. `ItemExplorationConfig` and
  `RankingInfluence` stay `map[string]string`/`map[string]float64` --
  genuinely free-form key/value maps in the real API, not modeled
  sub-structs. A generic `decodeConfig[T]` helper (`handler.go`) round-trips
  the already-`map[string]any`-decoded request body through
  `json.Marshal`/`Unmarshal` into the typed struct; a caller-supplied field
  with no counterpart in the real type is now silently dropped instead of
  echoed back verbatim (locked by `TestPersonalize_Config_DeepTyping`, which
  sends a bogus field inside each of the three config types and asserts it
  does not appear on Describe). `Recommender`'s
  `MinRecommendationRequestsPerSecond` bookkeeping field is kept in sync with
  `RecommenderConfig.MinRecommendationRequestsPerSecond` via a `withMinRPS`
  helper instead of the previous response-time merge in `recommenderToMap`.

- **DatasetType/Domain enum validation added this pass**: `CreateDataset`
  now rejects a `datasetType` outside the documented
  `INTERACTIONS`/`ITEMS`/`USERS`/`ACTIONS`/`ACTION_INTERACTIONS` set
  (case-insensitive, matching the real API's documented acceptance, even
  though the SDK models the field as a plain `*string` rather than a typed
  smithy enum -- there is no `types.DatasetType` to diff against, only the
  API documentation). `CreateDatasetGroup`/`CreateSchema` now reject a
  `domain` outside the real `types.Domain` enum (`ECOMMERCE`/
  `VIDEO_ON_DEMAND`); an empty/omitted domain remains valid since it
  produces a Custom, not Domain, dataset group/schema.

- **Status-string trap (real bug class)**: this backend short-circuits every
  resource straight to its terminal state on Create (`ACTIVE` immediately,
  skipping `CREATE PENDING`/`CREATE IN_PROGRESS`). That skip-the-
  intermediate-states pattern is a *deliberate, codebase-wide
  simplification* and is NOT a bug -- real state is mutated, ARNs are real,
  Describe/List reflect it correctly, and it is the correct call for a
  synchronous emulator. Do NOT re-flag it.
  However, **landing on an invalid enum string** is a real bug, and one
  existed: `StopSolutionVersionCreation` set `SolutionVersion.Status` to the
  bare string `"STOPPED"`, which is not a member of the real
  `SolutionVersion.Status` wire enum (only `CREATE PENDING`, `CREATE
  IN_PROGRESS`, `ACTIVE`, `CREATE FAILED`, `CREATE STOPPING`, `CREATE
  STOPPED` are valid, per the `types.SolutionVersion.Status` doc comment in
  aws-sdk-go-v2). Fixed to `"CREATE STOPPED"`. When auditing status
  transitions elsewhere, check the *value* against the type's own doc-listed
  enum, not just "does a transition happen."

- **UpdateSolution wire-shape bug (fixed prior pass)**: the real
  `UpdateSolutionInput` only carries `performAutoTraining` and
  `performIncrementalUpdate` (both optional `*bool`, nil = leave unchanged).
  `performAutoML`/`performHPO` are creation-only, immutable fields that do
  not exist on `UpdateSolutionInput` at all. gopherstack's `UpdateSolution`
  was reading `performAutoML`/`performHPO` from the request and mutating
  those fields -- meaning a real aws-sdk-go-v2 client calling
  `client.UpdateSolution(ctx, &UpdateSolutionInput{PerformAutoTraining:
  aws.Bool(false)})` would silently no-op (the field it actually sent was
  never read) while `performAutoML`/`performHPO` got silently reset to
  `false` (since those JSON keys were absent from the real payload). This is
  the "disguised no-op via wrong field names" bug class -- always check the
  Update variant's *own* Input struct rather than assuming it mirrors
  Create's.

- **CreateMetricAttribution/UpdateMetricAttribution/ListMetricAttributionMetrics
  (fixed prior pass)**: `metrics` is a *required* field on
  `CreateMetricAttributionInput` (list of `{eventType, expression,
  metricName}`) and was completely unread by gopherstack -- Create succeeded
  without it. `UpdateMetricAttributionInput` mutates the metric list via
  `addMetrics`/`removeMetrics` (by `metricName`), not a `metrics` replacement
  field. `ListMetricAttributionMetricsOutput.Metrics` must reflect what was
  actually configured; gopherstack was returning a hardcoded 2-entry
  fabricated list (`"click"`/`"purchase"`) regardless of what the caller
  created -- a textbook disguised no-op / fabricated-data bug. All three now
  round-trip through a real `MetricAttribution.Metrics []MetricAttribute`
  field on the backend struct.

- **CORRECTED (gopherstack-sm02): "extra fields on List summaries are
  harmless" was wrong and let this bug sit unflagged across several prior
  audit passes.** The previous version of this note claimed all sixteen
  `List*` ops reusing their `Get*` op's `*ToMap` function unscoped was fine
  because real aws-sdk-go-v2 deserializers silently discard unrecognised
  keys. That premise is true (`default: _, _ = key, value` in
  `deserializers.go`) but the conclusion drawn from it was not: an
  SDK-mediated client cannot observe the leak, but gopherstack is a wire
  emulator, not just an SDK-client target -- raw HTTP/boto3/other-language
  callers, and any parity tooling that inspects the actual JSON body, see
  every leaked field. "Ignored by one particular client library" is not the
  same as "matches the real API shape." All sixteen `List*` ops were in fact
  emitting their sibling `Get*` op's full converter output completely
  unscoped, leaking between 1 and 9 Describe-only members per op (worst:
  `ListSolutions` leaked 9 of what should be `types.SolutionSummary`'s 6
  fields; `ListSolutionVersions` leaked 9 of `types.SolutionVersionSummary`'s
  7). Every op now has its own `*SummaryToMap` converter built by reading
  that op's own `types.*Summary` struct individually (not derived from a
  sibling by analogy -- `RecommenderSummary` keeps `recommenderConfig` where
  every other List sibling drops its equivalent nested-config field, and
  `DatasetGroupSummary` is the one type in the set where a real `failureReason`
  member turned out to have an honest backend source and was added rather
  than left absent). See the per-op `wire: fixed` notes above for the full
  per-resource diff. This mirrors the pattern already correct in ssm,
  medialive, and glue: a dedicated summary-scoped converter alongside the
  full one, not a shared function. Regression coverage:
  `handler_list_summary_test.go`'s `TestPersonalize_ListOps_SummaryShape`
  asserts on the raw JSON response body (not through an SDK client, which
  cannot see this class of bug) for all sixteen ops.

- Persistence: `Handler.Snapshot`/`Restore` correctly delegate to
  `InMemoryBackend.Snapshot`/`Restore` (`persistence.go`), which round-trips
  all 16 `store.Table`-registered collections plus the plain `tags` map via
  `store.Registry.SnapshotAll`/`RestoreAll`. Verified via
  `persistence_test.go`'s table-driven per-resource-type coverage (updated
  this pass to seed real parent chains for the FK-validated Create ops
  instead of dangling made-up ARNs). No gaps found here.

## Error-discard sweep (2026-08-29): verified clean, no bugs found

Audited every discarded-error/discarded-return-value assignment
(`x, _ := ...`, bare `_ = ...`) in non-test `.go` files -- 141 sites --
looking for the sesv2 `SendBulkEmail` class of bug: a call whose failure had
a designated place to be reported and wasn't.

Every site is a JSON-body type assertion (`input["field"].(string)` etc.)
extracting a request field where a missing/wrong-typed value legitimately
becomes the zero value. This service has no `Batch*`/`Bulk*` operation that
returns a per-item status list: `CreateBatchInferenceJob` and
`CreateBatchSegmentJob` (batch_jobs.go) each create one job and return one
ARN, not a batch of items with individual outcomes -- the "Batch" in the
name refers to offline/bulk inference mode, not a multi-item request.

No test changes; no source changes. Recorded as genuinely clean for this bug
class.

## Filter/pagination-not-honoured sweep (2026-08-29)

Measured all 17 List ops (List* only -- no Get/Describe op returns a
collection in this service). Every op's constraining parameters beyond
NextToken are: one scoping-ARN filter (SolutionVersionArn/SolutionArn/
DatasetGroupArn/DatasetArn/MetricAttributionArn on 12 ops) and MaxResults;
`ListRecipes` additionally declares `Domain`/`RecipeProvider`.

Found and fixed one bug: `ListCampaigns`' `SolutionArn` filter (see the
`ListCampaigns` `ops:` entry above) -- compared `Campaign.SolutionVersionArn`
for exact equality against the bare `SolutionArn` a real client sends, which
is never true since `SolutionVersionArn` always has `/<versionID>`
appended. The filter silently excluded every campaign rather than
narrowing to one solution's. New test `TestListCampaigns_SolutionArnFilter`
(`list_filter_params_test.go`) drives the real SDK client through the full
DatasetGroup -> two Solutions -> two SolutionVersions -> two Campaigns
chain and fails against the pre-fix comparison.

The 11 other scoping-ARN filters (`ListBatchInferenceJobs`/
`ListBatchSegmentJobs`.`SolutionVersionArn`, `ListDataDeletionJobs`/
`ListDatasetExportJobs`/`ListDatasetImportJobs`/`ListDatasets`/
`ListEventTrackers`/`ListFilters`/`ListRecommenders`/`ListSolutions`.
`DatasetGroupArn`/`DatasetArn`, `ListSolutionVersions`.`SolutionArn`,
`ListMetricAttributionMetrics`.`MetricAttributionArn`) were checked: each
compares the filter parameter against a field on the stored resource that
is the *same* ARN type (e.g. `SolutionVersion.SolutionArn` really does
store the parent solution's bare ARN, unlike `Campaign.SolutionVersionArn`)
-- all correct as written, no change needed.

`ListRecipes.Domain`/`.RecipeProvider` are parsed by neither the handler
nor the backend (`recipes.go`'s built-in catalog has no `domain` field on
any entry at all -- structural gap for `Domain`). Left unfixed:
`RecipeProvider`'s only defined enum value in the pinned SDK
(`aws-sdk-go-v2/service/personalize@v1.50.4/types/enums.go:125-139`) is
`SERVICE` -- there is no second value the real API documents yet, so a
`RecipeProvider` filter can never observably narrow this backend's
all-`SERVICE` catalog. Implementing it would mean inventing behavior for
an enum value the pinned SDK doesn't define, which the campaign's own
restraint rule rules out.

## Equality-matched-cursor restart sweep (2026-08-30)

Every `paginateItems`/`paginate`-backed listing in this service (16 `List*` ops via
`store.go`'s two generic pagination helpers, plus `listRecipes`) resumed a `NextToken`
by scanning for the item whose key equalled the token and left `start` at 0 on no
match -- a deleted resource (or, for the built-in recipe catalog, a forged token)
restarted pagination at page one instead of truncating.

Fixed both generic helpers (`paginateItems[T]`, keyed by each table's own primary-key
function; `paginate[T]`, used only by `ListMetricAttributionMetrics`) to use a
threshold search: resume at the first item whose key is strictly greater than the
token. This is valid everywhere it's used -- every `paginateItems` caller's `keyOf` is
exactly the backing `store.Table`'s own (unique) `keyFn`, so `items` is always sorted by
the cursor's own field (confirmed by reading every one of the 16 `store.Register(...,
store.New(xKeyFn))` calls in `store_setup.go` against every `paginateItems(...,
xKeyFn, ...)` call site), and `ListMetricAttributionMetrics`'s `paginate` caller sorts
its synthetic key list ascending immediately before calling in.

`listRecipes` (recipes.go) is different: `getBuiltinRecipes()` is a fixed, hand-curated
list in an order that does not match `RecipeArn` (its cursor field), so a threshold
search there would be wrong. Fixed by defaulting an unresolved token to the end of the
collection instead. The built-in catalog has no delete operation, so the hostile test
forges an unresolvable token rather than deleting an entry.

New tests (`handler_pagination_restart_test.go`, both confirmed failing pre-fix):
`TestPersonalize_ListCampaigns_Pagination_DeletedMidPage` (real
`store.Table`-backed deletion, representative of all 16 `paginateItems` callers) and
`TestPersonalize_ListRecipes_Pagination_StaleTokenDoesNotRestart`. No prior test in this
service ever deleted an item or forged a token between pages.

Confirmed no other pagination bug class present: every `store.Table.Snapshot()` (the
source for every `paginateItems` caller) is already key-sorted, so there is no
never-sorted-walk bug, and no negative-offset numeric token is decoded anywhere in this
service (every cursor is identifier-based, not a numeric offset).

**Gates**: `go build ./services/personalize/...`, `go vet ./services/personalize/...`,
`go test -race -count=1 ./services/personalize/...` all pass; `golangci-lint run
./services/personalize/...` reports 0 issues.

## 2026-08-30 wire-key-read sweep, continued (remaining Describe/List operations)

Completed the wire-key-read sweep across all 36 Describe/List operations (derived from
`handler.go`'s dispatch-table registrations). The prior pass on this branch covered 18; this pass
audited the remaining 18 (all List ops except ListRecipes, already covered) and found no bugs.

Every `Describe*` op's real Input struct has exactly one field, a single scoping ARN
(AlgorithmArn/BatchInferenceJobArn/.../SolutionVersionArn) -- all 18 handlers read it under the
correct camelCase JSON key (confirmed against `awsAwsjson11_serializeOpDocumentDescribe*Input` for
a sample, e.g. `describeDatasetGroup` reads `datasetGroupArn`, matching the wire key emitted by
`awsAwsjson11_serializeOpDocumentDescribeDatasetGroupInput`).

Every `List*` op's real Input struct is MaxResults/NextToken plus at most one scoping ARN
(DatasetGroupArn/SolutionArn/SolutionVersionArn/DatasetArn/MetricAttributionArn) -- all handlers
read and forward that scoping arg to the backend, and every backend `List*` method filters on it
before pagination (`Snapshot()`-sorted input, filter-then-paginate via the shared `paginateItems`
helper), field-diffed one at a time: `listBatchInferenceJobs`/`listBatchSegmentJobs`
(solutionVersionArn), `listCampaigns` (solutionArn, with the SolutionVersionArn-prefix-match this
file's ListCampaigns already documents), `listDataDeletionJobs`/`listDatasetImportJobs`/
`listDatasetExportJobs`/`listDatasets`/`listEventTrackers`/`listFilters`/`listMetricAttributions`/
`listRecommenders`/`listSolutions` (datasetGroupArn or datasetArn), `listSolutionVersions`
(solutionArn), `listMetricAttributionMetrics` (metricAttributionArn). `listSchemas`/
`listDatasetGroups` have no scoping field in the real API (account-wide lists) -- confirmed against
their Input structs, correctly unscoped. No dropped filter, no wrong key, no wrong cardinality found
across any of these 18 -- a genuine zero-bug result, not an unaudited gap.

Gates: `go build ./services/personalize/...` (no changes made, nothing to build-verify beyond
confirming the tree is unchanged). Work left uncommitted per this pass's instructions.

## 2026-08-30 value-semantics sweep (gopherstack-uox6) -- clean, no code change

Re-audited this service for the class gopherstack-uox6 describes (a parameter that IS read and
applied but with the wrong algorithm, invisible to a field-shape or enum scanner). 36 Describe/List
ops counted directly from `api_op_Describe*.go`/`api_op_List*.go` filenames (18 + 18), matching the
brief's count exactly.

This axis was already almost entirely closed by the "Filter/pagination-not-honoured sweep
(2026-08-29)" and "2026-08-30 wire-key-read sweep" entries above, both of which used this same
discipline (read the doc comment, check the comparison/combining logic, not just whether the field
is read) predating this bd issue's filing. Independently re-verified rather than trusted:

- `ListCampaigns`' `SolutionArn` fix (compared against `Campaign.SolutionVersionArn`, which always
  carries a `/<versionID>` suffix a bare `SolutionArn` can never equal) re-read against the current
  source (`campaigns.go`) -- still correctly comparing `SolutionVersionArn`'s parent-solution field
  match, not a coincidental fix that regressed.
- The 11 other scoping-ARN filters: each compares against the same-typed ARN field on its own
  resource (not a sibling's), matching each op's own Input struct rather than a nearby type.
- `ListBatchInferenceJobs`/`ListBatchSegmentJobs` document "The default value is 100" for
  `MaxResults` -- the only two of 17 List ops with an explicit numeric default documented (the
  other 15 just say "the maximum number to return", no default stated). `store.go`'s
  `defaultPageSize = 100` (shared by both generic pagination helpers, `paginateItems`/`paginate`,
  used by all 16 non-`ListRecipes` List ops) matches. Newly confirmed this pass -- not previously
  checked against the doc's explicit "100" wording.
- `ListRecipes.Domain`/`.RecipeProvider`: re-confirmed as the already-recorded, deliberate
  restraint (RecipeProvider's only legal enum value is SERVICE and the whole catalog is implicitly
  SERVICE-provided, so the filter is provably inert; Domain would need domain-specific recipe data
  this backend's static catalog does not model). Not re-opened, per the brief's explicit
  instruction not to re-open a provably-inert single-enum-value filter.
- `filters.go`'s `ListFilters` (`DatasetGroupArn` equality) and `runtime.go` (campaign/recommender
  existence validation only, no `FilterExpression` DSL evaluation lives in this service --
  `GetRecommendations`'s filter-expression evaluator is in `services/personalizeruntime`, out of
  this pass's scope) checked; both correct/inapplicable as written.

No new bug found; no source or test changes this pass.

Gates: `go build ./services/personalize/...`, `go vet ./services/personalize/...` (no changes,
nothing to verify beyond confirming the tree is unchanged). Work left uncommitted per this pass's
instructions.

## 2026-09-06: status-constant reachability audit (gopherstack-h3th) + DatasetGroup.FailureReason Describe-shape fix (gopherstack-7z3p)

Both issues were filed title-only with no description; both re-derived from the pinned SDK
(`aws-sdk-go-v2/service/personalize@v1.50.4`) and this codebase.

**gopherstack-h3th** ("seven declared status constants are never assigned"): confirmed exactly
seven of `store.go`'s status constants were declared but never assigned anywhere in
`services/personalize/` (`statusActive` and `statusSolutionVersionStopped` are the two that ARE
assigned). Reachability table, verdict per constant, checked against every Status-bearing type's
doc comment in the pinned SDK's `types/types.go`:

| constant | value | real Personalize wire value? | verdict |
|---|---|---|---|
| `statusCreatePending` | `CREATE PENDING` | yes (every resource's Status doc) | unreachable-by-construction: every `Create*` op here is synchronous, so there is no provisioning phase for a resource to be pending in |
| `statusCreateProgress` | `CREATE IN_PROGRESS` | yes | unreachable-by-construction, same reason |
| `statusDeletePending` | `DELETE PENDING` | yes (`Dataset`/`Campaign`/`EventTracker`/etc.) | unreachable-by-construction: every `Delete*` op removes the resource from its `store.Table` synchronously and atomically (e.g. `DeleteDatasetGroup`, `dataset_groups.go`) -- no window where the object still exists in a pending-delete state |
| `statusCreateFailed` | `CREATE FAILED` | yes | unreachable-by-construction: no op in this backend has an async provisioning phase that can fail independently of the synchronous `Create*` call itself; a genuine validation failure (bad domain, dangling FK, etc.) is rejected synchronously with an error and no resource is created at all, matching real AWS's synchronous input validation -- there is no honest slot for a persisted object to land in `CREATE FAILED`. Checked every `Create*` op in the service for a fabricatable failure trigger (batch job `jobInput`/`jobOutput` shape, KMS/role combinations, etc.); none exists without inventing behavior this backend does not otherwise model, which the no-stub/no-fabrication rule forbids |
| `statusStopPending` | `STOP PENDING` | yes, but Recommender-only (`types.Recommender`'s doc comment: `STOP PENDING > STOP IN_PROGRESS > INACTIVE > START PENDING > START IN_PROGRESS > ACTIVE`) | unreachable-by-construction, and already consistent with an existing precedent: `StopRecommender`/`StartRecommender` (`recommenders.go`) already jump directly between `ACTIVE` and `INACTIVE`, skipping `STOP PENDING`/`STOP IN_PROGRESS`/`START PENDING`/`START IN_PROGRESS`, the same way `StopSolutionVersionCreation` jumps straight to `CREATE STOPPED` (this file's existing "Status-string trap" note, above) |
| `statusUpdatePending` | `UPDATE PENDING` | **no** | not a real Personalize wire value for any resource -- grepped every `Status *string` doc comment across all ~34 Status-bearing types in `types/types.go` (`DatasetGroup`, `Dataset`, `Campaign`, `CampaignUpdateSummary`, `Recommender`, `RecommenderUpdateSummary`, `SolutionVersion`, `SolutionUpdateSummary`, `DatasetUpdateSummary`, `Filter`, `EventTracker`, `Solution`); none documents `UPDATE PENDING`/`UPDATE IN_PROGRESS` anywhere. **Removed** rather than left as "unreachable" -- an unused, wire-invalid literal sitting in the same file that already contains one prior "landed on an invalid enum string" bug (`StopSolutionVersionCreation`'s old `"STOPPED"` vs `"CREATE STOPPED"`, see above) is exactly the trap that class of bug came from; keeping it around risks a future op assigning it and reintroducing that bug class |
| `statusUpdateProgress` | `UPDATE IN_PROGRESS` | **no** | same as `statusUpdatePending` -- removed |

Per this file's own pre-existing "Status-string trap" note (added a prior pass): skipping straight
to `ACTIVE` on every synchronous `Create*` op is a **deliberate, codebase-wide simplification, not
a bug** -- this pass re-confirms that verdict rather than re-opening it, and extends the same
reasoning to `CREATE FAILED`/`DELETE PENDING`/`STOP PENDING`: a synchronous emulator with no
provisioning-worker/job-queue model has no honest slot for any intermediate or async-failure
state. Building a fake `CREATE PENDING -> CREATE IN_PROGRESS -> ACTIVE` ticker was considered and
rejected -- it would fabricate AWS behavior this backend does not have and make tests slow/flaky
(the repo's own ban on `time.Sleep` in tests exists for exactly this reason). Five constants stay
declared, now with a `store.go` comment recording the reachability verdict and citing this table;
two (`statusUpdatePending`/`statusUpdateProgress`) were deleted as fabricated values with no wire
basis. No source or test change follows from this half of the pass beyond the `store.go` comment
and constant removal -- there is no runtime behavior for a regression test to lock, since nothing
was ever wired to the deleted constants.

**gopherstack-7z3p** ("DatasetGroup.FailureReason is omitted from the Describe shape though the
SDK deserializes it"): confirmed. `awsAwsjson11_deserializeDocumentDatasetGroup`
(`deserializers.go:10968`, `case "failureReason":` at `deserializers.go:11024`) reads a
`failureReason` key into `types.DatasetGroup.FailureReason` for `DescribeDatasetGroupOutput`. This
service's `datasetGroupToMap` (`handler_dataset_groups.go`, the `DescribeDatasetGroup` response
builder) omitted `failureReason` unconditionally, even though `DatasetGroup.FailureReason` was
already modeled on the backend struct (`models.go`) and its sibling `datasetGroupSummaryToMap`
(`ListDatasetGroups`) already emitted it conditionally when non-empty. Fixed `datasetGroupToMap` to
match: `if dg.FailureReason != "" { m["failureReason"] = dg.FailureReason }`, the same pattern
already used by `datasetGroupSummaryToMap` and by `solutionVersionToMap`/
`solutionVersionSummaryToMap` (`handler_solutions.go`) for `SolutionVersion.FailureReason`.

Other personalize types carrying a real `FailureReason` member (grepped `types/types.go`):
`BatchInferenceJob(Summary)`, `BatchSegmentJob(Summary)`, `Campaign(Summary)`,
`CampaignUpdateSummary`, `DataDeletionJob(Summary)`, `DatasetExportJob(Summary)`,
`DatasetGroup(Summary)`, `DatasetImportJob(Summary)`, `DatasetUpdateSummary`, `Filter(Summary)`,
`MetricAttribution(Summary)`, `RecommenderUpdateSummary`, `SolutionUpdateSummary`,
`SolutionVersion(Summary)`. Of these, only `DatasetGroup` and `SolutionVersion` have a
`FailureReason` field on their backend model (`models.go`) at all -- the rest have no backend
source for the field, which is already correctly recorded per-op above ("`failureReason` is a real
Summary member but the backend model has no source for it, so it stays absent rather than
fabricated"), not a gap this pass reopens. `SolutionVersion.FailureReason` was already wired
symmetrically into both its Describe and Summary shapes (a prior pass); `DatasetGroup` was the one
asymmetric case, now fixed.

Given the h3th verdict above (`CREATE FAILED` is unreachable-by-construction, no live path sets
`DatasetGroup.FailureReason`), this fix does not make `failureReason` observable through the live
API today -- it makes the Describe response shape *correct* for the case where the field is set
(directly on the backend struct, e.g. by a future feature or a test), matching the same
modeled-but-never-populated status already accepted for `SolutionVersion.FailureReason` (this
file's 2026-08-13 entry: "modeled but never populated ... correctly omitted, not a fabricated empty
string"). This is the intersection the two issues predicted: h3th explains why 7z3p's field is
unobservable, and neither issue's honest fix requires inventing an observation path.

**Files changed**: `store.go` (removed two fabricated status constants, added reachability
comment), `handler_dataset_groups.go` (`datasetGroupToMap` now includes `failureReason`
conditionally), `whitebox_test.go` (new, in-package -- `TestDatasetGroupToMap_FailureReason`, table
cases `present`/`absent`; there is no live HTTP path that sets `DatasetGroup.FailureReason`, so this
constructs the struct directly rather than driving it through the SDK client, per the exact
`whitebox_test.go` in-package-testpackage-exemption convention already used elsewhere in this repo,
e.g. `services/medialive/whitebox_test.go`).

**Regression proof**: reverted `datasetGroupToMap` to the pre-fix unconditional-omit shape, kept
the new test, and reran -- `TestDatasetGroupToMap_FailureReason/present` fails
(`expected: string("boom"), actual: <nil>(<nil>)`); `/absent` still passes. Restored the fix; both
subtests pass. No regression test accompanies the h3th constant removal -- there is no observable
behavior change to lock (the two removed constants were never assigned to anything).

**Gates**: `go build ./services/personalize/...`, `go vet ./services/personalize/...`,
`go test -race ./services/personalize/...` all pass; `golangci-lint run
./services/personalize/...` reports `0 issues.` (note: an early draft of the `store.go` doc comment
put a dedicated inline comment directly above `statusStopPending`'s own line, which caused
`unused` (bundled in `golangci-lint`) to newly flag it despite `statusSolutionVersionStopped`
immediately above being assigned -- apparently contiguous, comment-free `ValueSpec` runs in the
same `const (...)` block are what keeps an unused sibling from being flagged next to a used one.
Folding that comment into the single block-level doc comment above `const (` instead restored the
clean `0 issues.` result without changing which constants are declared).
