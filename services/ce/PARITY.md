---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: ce
sdk_module: aws-sdk-go-v2/service/costexplorer@v1.67.4   # version actually pinned in go.mod; corrected stale v1.63.8 reference
last_audit_commit: 021efa0d5 # HEAD as of the 2026-08-30 pagination/filter retrofit pass; this pass's own changes are uncommitted on top of it
last_audit_date: 2026-08-30
overall: A            # 2026-08-30 pagination/filter retrofit pass (gopherstack, following gopherstack-43o8's deferred 68-field backlog): regenerated the reqfieldscan count independently (68 fields across 24 ops, confirmed identical to the carried-forward figure) and closed all but 8, each of the remaining 8 a hand-verified honest gap (documented below in gaps), not a defect. Wired real NextPageToken/MaxResults/PageSize pagination via the existing paginateList[T] helper (plus a new paginateOrdered[T] sibling for ops with an independent SortBy/display order paginateList's own re-sort would have discarded) across GetCostAndUsage/GetCostAndUsageComparisons/GetCostAndUsageWithResources(shape-only)/GetCostComparisonDrivers(shape-only)/GetDimensionValues/GetTags/GetCostCategories/GetReservationCoverage/GetReservationUtilization/GetReservationPurchaseRecommendation/GetRightsizingRecommendation/GetSavingsPlansCoverage/GetSavingsPlansPurchaseRecommendation/GetSavingsPlansUtilizationDetails/ListSavingsPlansPurchaseRecommendationGeneration/ListCommitmentPurchaseAnalyses/ListCostAllocationTagBackfillHistory/ListCostAllocationTags/ListCostCategoryResourceAssociations. Implemented Filter/GroupBy/SortBy/SearchString/Context/AccountScope/DataType/RecommendationIds/AnalysisStatus/EffectiveOn with real backing state per op (never fabricated); found and fixed 5 real bugs along the way (see the dated Notes section below) including a cursor-pagination off-by-one in the new paginateOrdered helper itself, caught by this pass's own completeness tests before being carried forward. 68→8 unread-field count verified via `go run ./cmd/reqfieldscan -dir ce`.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateAnomalyMonitor: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: MonitorType is now enforced required (was previously only format-validated when present), matching validateAnomalyMonitor. FIXED 2026-08-29 (write-only-state, commit 16c7cbeba finished this pass) -- AnomalyMonitor.MonitorSpecification (*types.Expression, required for CUSTOM or TAG/COST_CATEGORY-dimensioned DIMENSIONAL monitors per types.go's AnomalyMonitor doc comment) was entirely absent: accepted by nothing, stored nowhere, omitted from every GetAnomalyMonitors response regardless of what was sent on Create. Now threaded through CreateAnomalyMonitor's backend signature and echoed on Get. See TestCreateAnomalyMonitor_MonitorSpecification_RealClient."}
  DeleteAnomalyMonitor: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: was ResourceNotFoundException, real AWS is UnknownMonitorException"}
  UpdateAnomalyMonitor: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: handler wrongly required MonitorName (real AWS's UpdateAnomalyMonitorInput only requires MonitorArn -- 'Specify the fields you want to update, omitted fields are unchanged'); this rejected valid real-client requests. Backend now leaves MonitorName unchanged when omitted instead of blanking it."}
  GetAnomalyMonitors: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: an unknown ARN in MonitorArnList silently returned an empty page instead of UnknownMonitorException. FIXED 2026-08-29 -- now echoes MonitorSpecification (see CreateAnomalyMonitor); sweeping AnomalyMonitor's remaining sibling fields found DimensionalValueCount (types.AnomalyMonitor, 'the value for evaluated dimensions') also entirely absent, with real non-fabricated backing state for the SERVICE/LINKED_ACCOUNT dimensions (distinct-value count in the synthetic cost ledger, the same data GetDimensionValues reads) -- now computed for those two dimensions; TAG/COST_CATEGORY dimensions and LastEvaluatedDate stay unset/undocumented, no real backing state exists for either (see gaps). See TestGetAnomalyMonitors_DimensionalValueCount_RealClient."}
  CreateAnomalySubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: MonitorArnList/Subscribers/Frequency now enforced required, matching validateAnomalySubscription (previously only SubscriptionName was required). FIXED 2026-08-29 -- AnomalySubscription.ThresholdExpression (*types.Expression, the non-deprecated replacement for Threshold) was entirely absent, same shape of bug as MonitorSpecification above; now threaded through and echoed on Get. See TestAnomalySubscription_ThresholdExpression_RealClient."}
  DeleteAnomalySubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: was ResourceNotFoundException, real AWS is UnknownSubscriptionException"}
  UpdateAnomalySubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: not-found was ResourceNotFoundException (now UnknownSubscriptionException); MonitorArnList entries were never checked against existing monitors (now UnknownMonitorException). FIXED 2026-08-29 -- also accepted no ThresholdExpression argument (see CreateAnomalySubscription); now threaded through and applied when non-nil (omitted-field-unchanged semantics, matching UpdateAnomalyMonitor's precedent). See TestAnomalySubscription_ThresholdExpression_RealClient."}
  GetAnomalySubscriptions: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: an unknown ARN in SubscriptionArnList silently returned an empty page instead of UnknownSubscriptionException; MonitorArn filter deliberately left non-validating (see Notes). FIXED 2026-08-29 -- now echoes ThresholdExpression (see CreateAnomalySubscription)."}
  GetAnomalies: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: DateInterval.StartDate now enforced required, matching validateAnomalyDateInterval. FIXED 2026-08-30 (gopherstack-43o8 reqfieldscan validation pass) -- GetAnomaliesInput.TotalImpact (real types.TotalImpactFilter{NumericOperator,StartValue,EndValue}, costexplorer@v1.67.4) was typed as a bare map[string]any and never read anywhere in handleGetAnomalies: parsed off the wire, then silently discarded, so a GREATER_THAN/BETWEEN dollar-impact filter never narrowed results. Now a typed totalImpactFilterInput threaded through backend.GetAnomalies (same pre-pagination filter-then-paginate shape as MonitorArn/Feedback/date-interval). See TestGetAnomalies_TotalImpactFilter_RealClient."}
  ProvideAnomalyFeedback: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCostCategoryDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: ServiceQuotaExceededException on duplicate name was HTTP 409, real AWS is HTTP 400; fixed this pass: RuleVersion/Rules now enforced required, matching validateOpCreateCostCategoryDefinitionInput. FIXED 2026-08-30 (gopherstack-43o8 reqfieldscan validation pass) -- SplitChargeRules and EffectiveStart (both real CreateCostCategoryDefinitionInput fields) were parsed off the wire and completely discarded: UpdateCostCategoryDefinition already threaded SplitChargeRules correctly, Create did not, so a real client's split-charge configuration silently vanished on create; a caller-supplied EffectiveStart was always overridden with now() instead of honored (real AWS only defaults to 'first day of current month' when the field is omitted). costCategorySummary (DescribeCostCategoryDefinition's response type) was also missing the SplitChargeRules field entirely, so even a correctly-stored value had nowhere to be echoed back. See TestCreateCostCategoryDefinition_SplitChargeRulesAndEffectiveStart_RealClient. NOTE: EffectiveStart is accepted and honored verbatim, not validated against real AWS's 'first day of the month, not before the previous twelve months, not in the future' constraints -- out of scope for this fix, consistent with this service's existing permissive-parse convention elsewhere."}
  DeleteCostCategoryDefinition: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeCostCategoryDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: ResourceNotFoundException was HTTP 404, real AWS is HTTP 400. FIXED 2026-08-30 (pagination retrofit pass) -- EffectiveOn (selects which historical version of the category was effective on that date) was parsed and never read. This backend has no version history, only the current rule set's own EffectiveStart, so real AWS's full historical lookup cannot be honored; the one non-fabricated use is treating a date before EffectiveStart as not-found (the category did not exist yet). Proven in TestCostCategoryEffectiveOn_RealClient."}
  ListCostCategoryDefinitions: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-30 (pagination retrofit pass) -- same EffectiveOn-ignored bug and fix shape as DescribeCostCategoryDefinition, applied as a pre-pagination filter in the backend (categories whose EffectiveStart is after EffectiveOn are excluded). Proven in TestCostCategoryEffectiveOn_RealClient."}
  UpdateCostCategoryDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: RuleVersion/Rules now enforced required, matching validateOpUpdateCostCategoryDefinitionInput"}
  GetCostAndUsage: {wire: ok, errors: ok, state: n/a, note: "deterministic mock over a synthetic cost ledger -- acceptable per parity rules, no real billing data exists to emulate. Earlier pass fixed the missing GroupDefinitions response field (echoes back the request's GroupBy, per GetCostAndUsageOutput). fixed this pass: TimePeriod and Metrics are now enforced required, matching GetCostAndUsageInput ('This member is required' on both, confirmed via api_op_GetCostAndUsage.go; TimePeriod.Start/.End are each independently required per types.DateInterval). A prior revision silently defaulted a missing/partial TimePeriod to defaultStartDate/defaultEndDate and never checked Metrics at all, so a request missing either real-required member got a permissive, silently-defaulted 200 instead of the ValidationError real AWS returns. Metrics enum-value validation (AmortizedCost/BlendedCost/NetAmortizedCost/NetUnblendedCost/NormalizedUsageAmount/UnblendedCost/UsageQuantity) is intentionally not added: existing coverage (TestGetCostAndUsage_AlternateMetrics's unknown_metric case) deliberately exercises an unrecognized metric name falling back to BlendedCost via getMetricValue, and Metrics is a plain []string on the wire (not an enum-constrained type), so this fix is a presence check only. FIXED 2026-08-30 (pagination retrofit pass) -- Filter (SERVICE dimension, same GetReservationCoverageFiltered pattern) and NextPageToken were both parsed and never read; GetCostAndUsage's own dropped Filter and missing pagination were both real bugs, not documented gaps. Added filterEntriesByService to the backend and paginateList over ResultsByTime (bucket TimePeriod.Start is unique, sorting is a no-op since buildTimeBuckets already emits ascending order). Proven in TestGetCostAndUsage_Pagination_RealClient (130-day DAILY range forces >1 page, full union asserted) and TestGetCostAndUsage_FilterNarrowsResults_RealClient."}
  GetCostForecast: {wire: ok, errors: ok, state: n/a, note: "FIXED 2026-08-30 (pagination retrofit pass) -- GetForecastByTime always used BlendedCost regardless of the request's Metric (a real dropped-field bug: types.Metric's SCREAMING_SNAKE_CASE enum values like USAGE_QUANTITY never matched this file's CamelCase getMetricValue/metricUnit switch at all, so even reading in.Metric would not have worked -- see normalizeMetricName). Filter's SERVICE dimension was also dropped. Both now threaded through. Separately found and fixed: Total was wire-shaped as a ForecastResult (MeanValue/PredictionIntervalLowerBound/PredictionIntervalUpperBound) but real GetCostForecastOutput.Total is *types.MetricValue (Amount/Unit) -- a real client's Total.Amount was always empty. Proven in TestGetCostForecast_Metric_RealClient. TimePeriod/Metric still lack required-field validation (see gaps)."}
  GetUsageForecast: {wire: ok, errors: ok, state: n/a, note: "same Metric-ignored/Filter-dropped/Total-wire-shape bugs and fixes as GetCostForecast (shared GetForecastByTime/metricUnit backend)."}
  GetDimensionValues: {wire: ok, errors: ok, state: ok, note: "FIXED 2026-08-12 (gopherstack-a8y0) -- real GetDimensionValuesInput carries Filter *types.Expression and SortBy []types.SortDefinition, both entirely absent; a client's filter/sort was silently dropped and the call returned success with unfiltered, unsorted results. Filter.Dimensions now constrains which ledger entries are considered before the target dimension's unique values are collected (new backend.GetDimensionValuesFiltered); SortBy orders the returned values by their total cost metric in the ledger (new backend.DimensionValueCost). Proven to genuinely narrow a multi-item result (12 seeded services down to 1) and reorder by cost, not just parse, in TestGetDimensionValuesFilterAndSortNarrow. FIXED 2026-08-30 (pagination retrofit pass) -- TimePeriod required-field presence check added (deterministic like GetCostAndUsage's); Context validated against its 3 real enum values (this emulator's ledger has one flat dimension space, so Context is checked but doesn't change resolution); NextPageToken/MaxResults now paginate via the new paginateOrdered helper, not paginateList -- vals may already be in SortBy's cost order, which paginateList's own re-sort by value would have discarded. Proven in TestGetDimensionValues_Pagination_RealClient (also the regression test for the paginateOrdered cursor off-by-one this pass found -- see the dated Notes section)."}
  GetTags: {wire: ok, errors: ok, state: ok, note: "FIXED 2026-08-12 (gopherstack-a8y0) -- same Filter/SortBy-absent bug as GetDimensionValues. Filter.Tags and SortBy are now real, wired code paths (backend.GetTagKeysFiltered/GetTagValuesFiltered/TagValueCost), but this emulator's synthetic cost ledger (seedCostLedger) never populates CostEntry.Tags -- no CE operation anywhere writes per-transaction tags -- so there is currently no tagged state for the filter to narrow. Documented rather than fabricated; see TestGetTagsFilterAndSortAccepted. FIXED 2026-08-30 (pagination retrofit pass) -- TimePeriod required-field presence check added; NextPageToken/MaxResults now paginate via paginateOrdered (same "must not undo SortBy's cost order" reasoning as GetDimensionValues)."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: ResourceTags now enforced required, matching validateOpTagResourceInput"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: ResourceTagKeys now enforced required, matching validateOpUntagResourceInput"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  GetCostAndUsageWithResources: {wire: ok, errors: ok, state: n/a, note: "fixed this pass: was missing GroupDefinitions and Filter/Granularity required-field validation; ResultsByTime is legitimately always empty -- real AWS resource-level cost data is keyed by individual resource ARN, and this emulator's synthetic ledger (seedCostLedger) only models service+date granularity, not per-resource entries, so there is no state to derive a non-empty result from. FIXED 2026-08-30 (pagination retrofit pass) -- TimePeriod/Metrics required-field validation added (same real-required members as GetCostAndUsage, api_op_GetCostAndUsageWithResources.go); validation-only, ResultsByTime stays empty by design. Real input also carries NextPageToken but it was deliberately NOT added to the wire struct: ResultsByTime is permanently empty (structural, no per-resource ledger), so declaring-and-never-reading it would just be a new unread field with no honest use, unlike Filter above which is at least parsed for the required-field check."}
  GetCostAndUsageComparisons: {wire: ok, errors: ok, state: n/a, note: "fixed this pass (3 wire-shape bugs): request fields BaseTimePeriod/Metrics were invented (real: BaselineTimePeriod/MetricForComparison, the latter a required singular string not an array); response field CostAndUsages was invented (real: CostAndUsageComparisons) and TotalCostAndUsage was wire-typed as an array instead of a map keyed by metric name. Now derives real baseline/comparison totals from the cost ledger via the same DAILY-bucketed aggregation GetCostAndUsage uses, instead of always returning an empty envelope. FIXED 2026-08-30 (pagination retrofit pass) -- Filter's SERVICE dimension now narrows both the baseline and comparison ledger totals (previously silently dropped). GroupBy was previously parsed off the wire and completely unused -- CostAndUsageComparisons always collapsed to one aggregate entry regardless of GroupBy. Now grouped by the request's single dimension (SERVICE/REGION/USAGE_TYPE/LINKED_ACCOUNT) into one entry per group value, each carrying a real CostAndUsageSelector Expression identifying the group (real types.CostAndUsageComparison field, previously absent from the wire struct entirely). NextPageToken/MaxResults now paginate the (possibly grouped) comparisons list. Proven in TestGetCostAndUsageComparisons_GroupBy_RealClient (>1 entry, each with a unique selector) and TestGetCostAndUsageComparisons_MetricForComparison_RealClient."}
  GetCostComparisonDrivers: {wire: ok, errors: ok, state: n/a, note: "field-diffed against GetCostComparisonDriversOutput this pass -- CostComparisonDrivers/NextPageToken already matched, no bug found. FIXED 2026-08-12 (gopherstack-a8y0) -- real input also carries Filter *types.Expression, absent from the request struct; now accepted for wire-shape parity, but deliberately left inert and documented as such: this emulator never computes comparison drivers at all (CostComparisonDrivers is always []), so there is no state anywhere for a filter to narrow. FIXED 2026-08-30 (pagination retrofit pass) -- the request's metric member was wire-declared \"Metric\", which matches no real GetCostComparisonDriversInput field at all (real: the required singular MetricForComparison string, same shape as GetCostAndUsageComparisons); a real aws-sdk-go-v2 client's MetricForComparison was silently dropped and the required-field check below it never fired for a request omitting the wrong name. Renamed and now enforced required, along with BaselineTimePeriod/ComparisonTimePeriod. NextPageToken now threaded through paginateList over the (still always-empty) CostComparisonDrivers list -- real, not fabricated, since it is the correct terminal-page shape for zero items. GroupBy/MaxResults were deliberately NOT added to the wire struct: with CostComparisonDrivers permanently empty, declaring them would just be new unread fields (see gaps for Filter, already in this category)."}
  GetCostCategories: {wire: ok, errors: ok, state: ok, note: "FIXED 2026-08-12 (gopherstack-a8y0) -- real GetCostCategoriesInput carries Filter *types.Expression and SortBy []types.SortDefinition, both entirely absent (verified representative for the whole cluster, services/ce/handler_cost_categories.go:244-250 pre-fix). Filter.CostCategories now intersects the returned CostCategoryValues with the requested allow-list (this emulator derives CostCategoryValues from cost-category Rule definitions, not tagged billing transactions the way real AWS does, so a Dimensions/Tags-based Filter has no backing state -- only the CostCategories clause has a real, non-fabricated effect here); SortBy honors SortOrder over the values (already alphabetical; no per-value cost metric exists to sort by numerically, so only ASCENDING/DESCENDING is applied, not fabricated per-value costs). Proven to genuinely narrow (3 values to 2) and reverse-order in TestGetCostCategoriesFilterAndSortNarrow. FIXED 2026-08-30 (pagination retrofit pass) -- TimePeriod required-field presence check added; SearchString was parsed off the wire and never applied (now a case-insensitive substring match over names or values, matching GetDimensionValues/GetTags' SearchString handling and real AWS's documented dual meaning); NextPageToken/MaxResults now paginate via paginateOrdered (preserving SearchString/SortBy's order). Proven in TestGetCostCategories_SearchStringAndPagination_RealClient."}
  GetReservationCoverage: {wire: ok, errors: ok, state: ok, note: "FIXED 2026-08-12 (gopherstack-a8y0) -- real GetReservationCoverageInput carries Filter *types.Expression and SortBy *types.SortDefinition (note: singular pointer, not a slice, unlike GetCostCategories/GetDimensionValues/GetTags -- don't 'fix' it to a slice), both entirely absent. Filter.Dimensions{Key:SERVICE} now constrains the cost ledger entries summed into each time bucket (new backend.GetReservationCoverageFiltered); other documented Filter dimensions (AZ/PLATFORM/TENANCY/...) have no per-entry breakdown in this ledger and are not applied. SortBy honors the documented 'Time' key to reorder the CoveragesByTime buckets; the several numeric SortBy keys real AWS also documents (OnDemandCost, CoverageHoursPercentage, ...) are accepted but left in chronological order rather than fabricating a metric-based ordering. Proven real (not just parsed) in TestGetReservationCoverageServiceFilterZeroesCost (filtering to a nonexistent service zeroes the computed cost) and TestGetReservationCoverageSortByTimeReorders (multi-bucket reordering). FIXED 2026-08-30 (pagination retrofit pass) -- NextPageToken was parsed and never read; now paginates via the new paginateOrdered helper (not paginateList) since coverages may already be in SortBy=Time DESCENDING order, which paginateList's own re-sort by TimePeriod.Start ascending would have silently flipped back to ascending across the page boundary. GroupBy stays accepted-but-unread (see gaps): Groups is always [], no per-group RI coverage breakdown exists to disguise a fabricated one from. Proven in TestGetReservationCoverage_Pagination_PreservesSortOrder_RealClient (also the completeness regression test for the paginateOrdered off-by-one -- see Notes)."}
  GetReservationPurchaseRecommendation: {wire: ok, errors: ok, state: ok, note: "FIXED 2026-08-12 (gopherstack-a8y0) -- real input carries Filter *types.Expression, absent. Real AWS documents Filter for this op as LINKED_ACCOUNT-only; this emulator is single-account (every recommendation is for the request's own account, no multi-account state exists), so the filter's only honest effect is exclude/include: an account that doesn't match the filter genuinely gets no recommendation, rather than the filter being silently accepted and ignored. Proven in TestGetReservationPurchaseRecommendationAccountFilterNarrows. FIXED 2026-08-30 (pagination retrofit pass) -- AccountScope (PAYER/LINKED) was parsed and never validated; this emulator has only one account's state either way so it is validated (rejecting an unrecognized value, matching real AWS) rather than acted on. NextPageToken/PageSize now paginate the 0-or-1-item Recommendations list via paginateList."}
  GetReservationUtilization: {wire: ok, errors: ok, state: ok, note: "same Filter/SortBy-absent bug and fix shape as GetReservationCoverage (new backend.GetReservationUtilizationFiltered); proven in TestGetReservationUtilizationSortByTimeReorders. FIXED 2026-08-30 (pagination retrofit pass) -- same NextPageToken-dropped bug, paginateOrdered fix, and accepted-but-unread GroupBy shape as GetReservationCoverage; both handlers now share a generic buildTimeSeriesResponse[T,A] helper (dupl-linter-driven decomposition, not a behavior change)."}
  GetSavingsPlansCoverage: {wire: ok, errors: ok, state: ok, note: "FIXED 2026-08-12 (gopherstack-a8y0) -- real input carries Filter *types.Expression and SortBy *types.SortDefinition, both absent. This op always computes exactly one synthetic coverage entry (no per-REGION/SERVICE/INSTANCE_FAMILY breakdown exists in this emulator), so SortBy on a single-item list is documented as inert rather than implemented; Filter.Dimensions{Key:REGION} is given a real effect since the one entry's Region is always the request's own region -- a REGION filter that excludes it correctly narrows the result to zero items. Proven in TestGetSavingsPlansCoverageRegionFilterNarrows. FIXED 2026-08-30 (pagination retrofit pass) -- Granularity was parsed and never applied: the op always collapsed to exactly one entry regardless of DAILY/MONTHLY, when real AWS documents (and GetReservationCoverage/GetSavingsPlansUtilization's ByTime both already model) one entry per time bucket. Now bucketed via buildTimeBuckets, matching that sibling pattern; NextToken/MaxResults now paginate the resulting bucket list via paginateList. SortBy (no documented \"Time\" key for this op, unlike GetReservationCoverage) and GroupBy/Metrics (no per-group breakdown; Metrics' only valid value doesn't change the Coverage struct's shape) stay accepted-but-unread -- see gaps. Proven in TestGetSavingsPlansCoverage_Pagination_RealClient."}
  GetSavingsPlansPurchaseRecommendation: {wire: ok, errors: ok, state: ok, note: "FIXED 2026-08-12 (gopherstack-a8y0) -- real input carries Filter *types.Expression (no SortBy field exists on this op's real input -- don't add one). Same single-account LINKED_ACCOUNT exclude/include fix shape as GetReservationPurchaseRecommendation. Proven in TestGetSavingsPlansPurchaseRecommendationAccountFilterNarrows. FIXED 2026-08-30 (pagination retrofit pass) -- same AccountScope-unvalidated bug/fix and NextPageToken/PageSize-dropped pagination as GetReservationPurchaseRecommendation, applied to the 0-or-1-item RecommendationDetails list."}
  GetApproximateUsageRecords: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed this pass: Services/TotalRecords were wire-typed as strings, real AWS types them as JSON numbers (map[string]int64/int64 -- NonNegativeLong); ApproximationDimension/Granularity now enforced required. Now derives per-service counts from the cost ledger's UsageQuantity over a trailing 30-day LookbackPeriod instead of always returning zero."}
  ListCostCategoryResourceAssociations: {wire: ok, errors: ok, state: n/a, note: "fixed this pass: response fields CostCategoryReference/ResourceTagsCount were invented; real AWS field is CostCategoryResourceAssociations ([]CostCategoryResourceAssociation{CostCategoryArn,CostCategoryName,ResourceArn}). Always returns zero associations: real AWS resource associations tie a cost category to actual AWS resources via resource tags, and this emulator has no such resource-tag inventory to associate against -- there is no state to disguise a no-op here. FIXED 2026-08-30 (pagination retrofit pass) -- the request struct also had a fabricated \"ResourceTagFilter\" field matching no real ListCostCategoryResourceAssociationsInput member (real: CostCategoryArn/MaxResults/NextToken only), and MaxResults was entirely absent. Removed the fabricated field, added MaxResults, and threaded NextToken/MaxResults through paginateList over the (still always-empty) association list. CostCategoryArn stays accepted-but-unread: real AWS's own validators.go has no required-field check for this op and there is no confirmed evidence of what a nonexistent ARN does here, so inventing a not-found error was deliberately NOT done (see gaps) -- an earlier draft of this fix added exactly that speculative validation and broke TestListCostCategoryResourceAssociations, which was the correct signal to remove it."}
  GetSavingsPlanPurchaseRecommendationDetails: {wire: ok, errors: ok, state: n/a, note: "fixed this pass: response field RecommendationDetail was invented; real AWS field is RecommendationDetailData (a RecommendationDetailData struct, not `any`). RecommendationDetailId now enforced required. Now derives synthetic-but-real values from the SP utilization ledger instead of returning an empty envelope."}
  StartSavingsPlansPurchaseRecommendationGeneration: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: response field GenerationId was invented; real AWS field is RecommendationId. Was a pure stub (empty envelope, no state at all) -- now creates and persists a SavingsPlansGeneration record (new store.Table), mirroring the CommitmentAnalysis start/persist/list pattern."}
  ListSavingsPlansPurchaseRecommendationGeneration: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: GenerationSummaryList entries used the invented GenerationId field; real AWS field is RecommendationId (GenerationSummary type). Was always an empty list regardless of state -- now reads back real generation jobs created by StartSavingsPlansPurchaseRecommendationGeneration, with real GenerationStatus filtering. FIXED 2026-08-30 (pagination retrofit pass) -- RecommendationIds was parsed and never applied (now an allow-list filter, same shape as GenerationStatus); NextPageToken/PageSize now paginate via paginateOrdered, preserving the existing most-recently-started-first order. Also fixed a latent ordering bug the new pagination cursor exposed: ListSavingsPlansGenerations sorted by GenerationStartedTime (second precision) over a Table.All() map walk (unspecified order) with a plain (unstable) sort.Slice -- two jobs started in the same second could tie and reorder nondeterministically across calls, which is silently correct without pagination but drops/duplicates records once a cursor depends on a fixed order. Added a RecommendationID tiebreak. Proven in TestListSavingsPlansPurchaseRecommendationGeneration_RecommendationIDs_RealClient."}
families:
  AnomalyMonitor: {status: ok, note: "CRUD + Get(list) verified against backend.go; 3 error-shape bugs fixed last pass, 1 required-field gap + 1 over-validation bug fixed an earlier pass; MonitorSpecification write-only-state bug and DimensionalValueCount silent-drop fixed 2026-08-29 (see ops above)"}
  AnomalySubscription: {status: ok, note: "CRUD + Get(list) verified against backend.go; 3 error-shape/referential-integrity bugs fixed last pass, 1 required-field gap fixed an earlier pass; ThresholdExpression write-only-state bug (Create+Update+Get) fixed 2026-08-29 (see ops above)"}
  GetAnomalies: {status: ok, note: "date-interval overlap filter, monitor/feedback filter, pagination all verified real (not a stub); AnomalyScore/Impact struct shapes match API_Anomaly.html; StartDate required-field gap fixed this pass"}
  CostCategory: {status: ok, note: "Create/Describe/Update/Delete/List all real state, ARN-keyed store.Table, deep-copies on read/write; 2 HTTP-status bugs fixed last pass, RuleVersion/Rules required-field gap fixed this pass (Create+Update)"}
  Tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource operate across costCategories/anomalyMonitors/anomalySubscriptions maps, real mutation, HTTP-status fix inherited from the shared ErrNotFound mapping; ResourceTags/ResourceTagKeys required-field gap fixed this pass"}
  CostAndUsageQueries: {status: ok, note: "GetCostAndUsage/GetCostForecast/GetUsageForecast/GetDimensionValues/GetTags/GetCostCategories -- deterministic mock over a 90-day synthetic cost ledger, per parity rules this is acceptable (no real billing data to emulate); DateInterval wire shape (yyyy-MM-dd strings, not epoch) verified correct. GetCostAndUsage's missing GroupDefinitions field fixed in an earlier pass; GetCostAndUsage's required-field validation gap (TimePeriod/Metrics) closed this pass -- see the GetCostAndUsage op note and the gaps list below for GetCostForecast/GetUsageForecast/GetDimensionValues/GetTags/GetCostCategories, which still lack it. GetDimensionValues/GetTags/GetCostCategories' Filter/SortBy-absent bug fixed 2026-08-12 (gopherstack-a8y0) -- see their op notes above."}
  CostAndUsageComparisonAndResourceQueries: {status: ok, note: "GetCostAndUsageComparisons/GetCostAndUsageWithResources/GetCostComparisonDrivers -- field-diffed this pass (were previously grouped under the deferred/unverified CostAndUsageQueries note). GetCostAndUsageComparisons had 3 invented/wrong-typed fields, now fixed and deriving real ledger totals. GetCostAndUsageWithResources was missing GroupDefinitions + required-field validation, now fixed; ResultsByTime legitimately stays empty (no per-resource ledger state exists to derive from). GetCostComparisonDrivers already matched the real shape."}
  ReservationsAndSavingsPlans: {status: ok, note: "GetReservationCoverage/GetReservationUtilization/GetReservationPurchaseRecommendation/GetRightsizingRecommendation/GetSavingsPlans* -- all deterministic synthetic-ratio mocks derived from the cost ledger, acceptable (no state to mutate, matches AWS response shapes); not deep-audited for numeric-formula fidelity this pass (see deferred). GetSavingsPlanPurchaseRecommendationDetails's invented field fixed this pass; Start/ListSavingsPlansPurchaseRecommendationGeneration converted from pure stubs to real persisted state this pass. GetReservationCoverage/GetReservationUtilization/GetReservationPurchaseRecommendation/GetSavingsPlansCoverage/GetSavingsPlansPurchaseRecommendation's Filter/SortBy-absent bug fixed 2026-08-12 (gopherstack-a8y0) -- see their op notes above. FIXED 2026-08-30 (pagination retrofit pass) -- GetRightsizingRecommendation.Filter was dropped (now the same LINKED_ACCOUNT exclude/include shape as GetReservationPurchaseRecommendation) and NextPageToken/PageSize were unwired (now real via paginateList over the 0-or-1-item RightsizingRecommendations list). GetSavingsPlansUtilization.Filter (REGION/LINKED_ACCOUNT exclude/include on the whole per-bucket list) and SortBy (real, numeric TotalCommitment/UsedCommitment/UnusedCommitment/NetSavings keys genuinely vary per DAILY/MONTHLY bucket -- UtilizationPercentage is a fixed synthetic constant so sorting by it ties every entry, included for completeness not fabricated significance) were both dropped and are now wired; proven in TestGetSavingsPlansUtilization_SortBy_RealClient. GetSavingsPlansUtilizationDetails's Fields field matched no real member (real: DataType []types.SavingsPlansDataType) -- renamed, and now genuinely selects which of Attributes/Utilization/Savings/AmortizedCommitment populate per item (SavingsPlansUtilizationDetail's three sub-struct fields are now pointers so 'omitted' is representable); Filter (REGION/SAVINGS_PLAN_ARN exclude/include) and NextToken/MaxResults pagination were also dropped and are now wired; SortBy stays accepted-but-unread (single synthetic item, ordering is trivially a no-op). Proven in TestGetSavingsPlansUtilizationDetails_DataType_RealClient. See per-op notes above for GetReservationCoverage/Utilization/PurchaseRecommendation and GetSavingsPlansCoverage/PurchaseRecommendation."}
  CostAllocationTags: {status: ok, note: "ListCostAllocationTags/UpdateCostAllocationTagsStatus/StartCostAllocationTagBackfill/ListCostAllocationTagBackfillHistory -- real store.Table-backed state, verified. FIXED 2026-08-30 (pagination retrofit pass) -- both List ops had NextToken/MaxResults parsed and never read. ListCostAllocationTags already sorted ascending by the unique TagKey, so paginateList's own re-sort is a no-op there -- direct reuse of the established pattern. ListCostAllocationTagBackfillHistory's BackfillJob had no unique field at all (a plain append-only slice, sorted descending by RequestedAt with second precision); added an internal-only BackfillID (uuid, never on the wire -- real CostAllocationTagBackfillRequest has no such field either, NextToken is fully opaque) as a sort tiebreak/pagination cursor key, then paginated via paginateOrdered to preserve the most-recently-requested-first order. Proven in TestListCostAllocationTagBackfillHistory_Pagination_RealClient."}
  CommitmentPurchaseAnalysis: {status: ok, note: "StartCommitmentPurchaseAnalysis/GetCommitmentPurchaseAnalysis/ListCommitmentPurchaseAnalyses -- real store.Table-backed state, verified. FIXED 2026-08-30 (pagination retrofit pass) -- ListCommitmentPurchaseAnalyses had AnalysisStatus/NextPageToken/PageSize all parsed and never read. AnalysisStatus is now a real equality filter (this backend's analyses never leave PROCESSING, so filtering to SUCCEEDED/FAILED correctly returns empty -- proven in TestListCommitmentPurchaseAnalyses_StatusFilterAndPagination_RealClient); NextPageToken/PageSize paginate via paginateOrdered. Same latent same-second-tie ordering bug as ListSavingsPlansPurchaseRecommendationGeneration (ListCommitmentAnalyses sorted by AnalysisStartedTime over an unordered Table.All() with a plain sort.Slice) was found and fixed with an AnalysisID tiebreak."}
  GetApproximateUsageRecords: {status: ok, note: "fixed this pass: wrong wire types (string instead of JSON number) and a disguised no-op (always-zero regardless of input); now derives real per-service counts from the cost ledger"}
  ListCostCategoryResourceAssociations: {status: ok, note: "fixed this pass: 2 invented field names; correctly and legitimately returns zero associations (no resource-tag inventory modeled in this emulator)"}
  RouteMatcher: {status: ok, note: "X-Amz-Target prefix \"AWSInsightsIndexService.\" verified byte-for-byte against every httpBindingEncoder.SetHeader(\"X-Amz-Target\") call in aws-sdk-go-v2/service/costexplorer@v1.63.8/serializers.go"}
gaps:
  - "2026-08-30 pagination/filter retrofit pass: reqfieldscan regenerated independently (68 fields across 24 ops, matching gopherstack-43o8's carried-forward figure exactly) and closed to 8, every one hand-verified as an honest gap, not a defect deferred for time. Remaining: (1) GetCostComparisonDriversInput.Filter -- CostComparisonDrivers is always [] (no per-line-item cost-change attribution state exists), so there is nothing for a filter to narrow; accepted for wire parity only. (2) GetReservationCoverageInput.GroupBy and (3) GetReservationUtilizationInput.GroupBy -- both ops' CoveragesByTime/UtilizationsByTime entries never populate a per-group Groups breakdown (always []), no per-SERVICE/AZ/... RI state exists to derive one from. (4) GetSavingsPlansCoverageInput.SortBy -- this op documents no 'Time' sort key (unlike GetReservationCoverage), and the numeric keys it does document have no per-bucket-varying value to sort by honestly. (5) GetSavingsPlansCoverageInput.GroupBy -- same no-per-group-breakdown shape as Reservation Coverage/Utilization. (6) GetSavingsPlansCoverageInput.Metrics -- the only real valid value (SpendCoveredBySavingsPlans) doesn't change the Coverage struct's fixed shape, so there is no differing output to select between. (7) GetSavingsPlansUtilizationDetailsInput.SortBy -- this op always returns exactly one synthetic detail item, so any ordering is trivially a no-op (same shape as GetSavingsPlansCoverage's SortBy before this pass added bucketing). (8) ListCostCategoryResourceAssociationsInput.CostCategoryArn -- real AWS's own validators.go has no required-field check for this op and there is no confirmed evidence (doc page or SDK source) of what a nonexistent ARN does; an earlier draft of this fix guessed 'return not-found' and broke an existing test, which is exactly the fabricated-validation-behavior class this campaign warns against, so it was reverted. All 8 are declared on their wire structs (not silently dropped from the struct entirely) and documented at their op/family notes above. Every ADDRESSED item from gopherstack-43o8's list (pagination on GetCostAndUsage/GetCostAndUsageComparisons/GetCostAndUsageWithResources/GetReservationCoverage/GetReservationPurchaseRecommendation/GetReservationUtilization/GetRightsizingRecommendation/GetSavingsPlansCoverage/GetSavingsPlansPurchaseRecommendation/GetSavingsPlansUtilization/GetSavingsPlansUtilizationDetails/ListCostCategoryResourceAssociations/ListSavingsPlansPurchaseRecommendationGeneration/ListCostAllocationTags/ListCostAllocationTagBackfillHistory, plus AccountScope/ResourceTagFilter-bug/RecommendationIDs/AnalysisStatus/EffectiveOn) is now real, wired, and tested -- see per-op notes above."
  - "GetCostForecast/GetUsageForecast still lack required-field validation that the real aws-sdk-go-v2 client-side validators enforce (TimePeriod and Metric are both 'This member is required' on GetCostForecastInput/GetUsageForecastInput). GetCostAndUsage's TimePeriod/Metrics gap was closed in an earlier pass, and this 2026-08-30 pass closed the same gap for GetDimensionValues/GetTags/GetCostCategories (see their op notes) -- GetCostForecast/GetUsageForecast are the two ops still open from the original five-op list, deliberately left alone since Metric's absence changed this pass's forecast-metric fix (GetForecastByTime now genuinely uses the requested Metric) rather than its presence validation, and touching required-field validation here risks the same larger set of existing lenient test call sites the earlier pass flagged. Candidate for a dedicated follow-up pass. (bd: needs issue)"
  - "AnomalyMonitor.LastEvaluatedDate (types.AnomalyMonitor, 'the date the monitor last evaluated for anomalies') is never set. Unlike DimensionalValueCount (fixed 2026-08-29), there is no real backing state to derive this from: this backend has no anomaly-detection evaluation engine anywhere (StartJanitor's evictExpiredAnomalies only expires already-existing Anomaly records, it does not generate them from cost data or 'evaluate' a monitor), so any timestamp here would be fabricated rather than read from real state. AnomalyMonitor.DimensionalValueCount for the TAG/COST_CATEGORY dimensions has the same gap (only SERVICE/LINKED_ACCOUNT have a real per-entry field in the cost ledger to count distinct values of)."
  - "gopherstack-s2i4 (audited 2026-09-08, CLOSED as not-a-defect): DataUnavailableException (types.DataUnavailableException, costexplorer@v1.67.4 types/errors.go:118, doc comment verbatim 'The requested data is unavailable.') is declared in the wire model on 22 of ce's 47 operations (confirmed identical 22-op sets independently in both botocore's ce/2017-10-25/service-2.json.gz per-op `errors` lists and this SDK's deserializers.go case sites -- the original issue title's 'every op' overstates this) but errors.go's ErrDataUnavailable is never referenced anywhere outside its own declaration -- no backend path returns it and handler.go's handleError has no case for it. Two independent documentation research passes (gopherstack-1kse, closed 2026-09-06, reverified this pass) found no citable AWS trigger, threshold, or condition tying this exception to any documented behavior: CE's 14/38-month data-retention window is documented but never linked to this exception, and the one adjacent documented case (forecast horizon limits) is documented as producing a validation error instead. gopherstack's own data-less-query behavior is deliberately never 'unavailable': GetCostAndUsage's GroupBy path fabricates a full synthetic non-empty result for any period with no ledger entries (syntheticGroupsFallback, cost_usage.go:47, '...giving callers a valid non-empty response shape for any time period'), and its non-GroupBy path returns a real zero-valued total, never an error. Since this emulator's synthetic ledger never holds real billing data, every query is 'unavailable' by the naive doc-string reading -- emitting the exception on that basis would fail every GetCostAndUsage-family call, strictly worse than the status quo. Verdict: correctly not modelled; the declared-but-unreturned code is honest, not a defect."
deferred:
  - "Reservation/SavingsPlans numeric-formula fidelity (the specific ratios in backend.go's syntheticServiceCatalog / spCommitmentRatio / riPurchasedCostRatio etc.) -- these produce plausible, internally-consistent numbers but were not cross-checked against any real AWS CE billing behavior; by definition there is no real data to match against, so this is a modeling-quality concern for a future pass, not a correctness bug."
  - "GetCostAndUsageWithResources.ResultsByTime and ListCostCategoryResourceAssociations.CostCategoryResourceAssociations are always empty by design (see per-op notes above) -- both would need a per-resource / resource-tag inventory this emulator doesn't model anywhere else in the service. Not a disguised no-op (input-driven required-field validation now happens, and the wire shape is correct), just genuinely no backing state to report. A future pass could seed a small synthetic per-resource inventory if resource-level fidelity becomes a priority."
leaks: {status: clean, note: "StartJanitor's anomaly-eviction goroutine (evictExpiredAnomalies) is a single ticker loop stopped via ctx.Done, no per-request goroutines. This pass added one new store.Table (savingsPlansGenerations, registered via the same registry.ResetAll/SnapshotAll/RestoreAll lifecycle as every other table -- see store_setup.go) and zero new goroutines or unbounded maps. 2026-08-30 pagination retrofit pass: added one new struct field (BackfillJob.BackfillID, a uuid string) and zero new goroutines/tables/maps; backfillJobs stays a plain append-only slice."}
---

## Notes

Protocol: AWS JSON 1.1 (`application/x-amz-json-1.1`), single POST endpoint, dispatch via
`X-Amz-Target: AWSInsightsIndexService.<Op>` header (verified against every
`serializers.go` `SetHeader("X-Amz-Target")` call in the vendored SDK). `RouteMatcher`
correctly checks the full prefix including the internal Coral service name
`AWSInsightsIndexService` — this is NOT the public "Cost Explorer" name, and it's easy to
mistype/second-guess when unfamiliar with the API; it's confirmed correct.

`DateInterval`/`TimePeriod` fields are always `yyyy-MM-dd` strings (never epoch numbers),
confirmed against `API_AnomalyDateInterval.html` and the `Start`/`End` map wire shape
used throughout `getCostAndUsageInput`/`getCostForecastInput`/etc.

### Anomaly write-only-state pass (2026-08-29)

Resumed a session cut off mid-write by a rate limit (commit `16c7cbeba`), which had
already threaded `AnomalyMonitor.MonitorSpecification` and
`AnomalySubscription`/`UpdateAnomalySubscriptionInput.ThresholdExpression` through the
backend and added `wire_field_fixes_test.go`, but left no fail-before evidence for
anything finished after its last confirmation and never updated this file. Verified both
fixes directly against `costexplorer@v1.67.4 types/types.go`
(`AnomalyMonitor.MonitorSpecification *Expression`,
`AnomalySubscription.ThresholdExpression *Expression`) and confirmed `go build`/`go
vet`/`go test -race -count=1`/`golangci-lint run` all pass on the committed state.

Per this campaign's "sweep every sibling field in the same struct" rule, swept
`AnomalyMonitor`'s two other real members the fix hadn't touched:
`DimensionalValueCount` and `LastEvaluatedDate`. `DimensionalValueCount` ("the value for
evaluated dimensions") was completely absent from the wire and always the zero value —
but for a `DIMENSIONAL` monitor on the `SERVICE` or `LINKED_ACCOUNT` dimension this
backend has real, non-fabricated state to derive it from: the distinct-value count for
that dimension in the synthetic cost ledger, the same data `GetDimensionValues` already
reads. Fixed (`handler_anomalies.go`'s new `dimensionalValueCount` helper), proven via
`TestGetAnomalyMonitors_DimensionalValueCount_RealClient` (asserts the real SDK client
sees `12`, matching `syntheticServiceCatalog`'s 12 seeded services), confirmed to fail
against the unmodified code first. `LastEvaluatedDate` and `DimensionalValueCount` for
the `TAG`/`COST_CATEGORY` dimensions stay unset — no anomaly-detection evaluation engine
exists anywhere in this backend to derive a real value from (see `gaps`); fabricating one
would be exactly the fabrication class this campaign has repeatedly found and reverted.

Also performed a full write-only-state and per-op wire-shape sweep of
`services/outposts` (43 ops) at the same time, since its own audit trail (a dated,
detailed, A-graded `PARITY.md` with no `wire_field_fixes*_test.go`) matches the
higher-risk pattern this campaign has previously found a real bug hiding under
(`servicediscovery`). Field-diffed every Get/List/Describe response and every
Create/Update request against the pinned `outposts@v1.66.1` SDK, traced six
domain-object write paths (Order, Quote, Site, CapacityTask, Connection,
Outpost) end-to-end from their Create/Update handlers to their read paths, and
cross-checked every enum constant. No bug found — a genuinely clean pass, not a
skipped one; see `services/outposts/PARITY.md` for the full record.

### Bugs fixed this pass (earlier: 2026-07-29)

All 7 fixes are in the same family: **wrong or missing error-code/HTTP-status mapping**,
none are disguised no-ops (every op in the AnomalyMonitor/AnomalySubscription/CostCategory
families already did real `store.Table`-backed state mutation before this pass — the gap
was exclusively in how failures were reported on the wire).

1. **HTTP status codes were wrong for every modeled CE exception** (handler.go
   `handleError`). `ErrNotFound` → `ResourceNotFoundException` was returned as HTTP 404,
   and `ErrAlreadyExists` → `ServiceQuotaExceededException` was returned as HTTP 409.
   Checked 6 separate AWS API reference pages
   (`API_DescribeCostCategoryDefinition`, `API_CreateCostCategoryDefinition`,
   `API_DeleteAnomalyMonitor`, `API_DeleteAnomalySubscription`, `API_UpdateAnomalyMonitor`,
   `API_GetAnomalyMonitors`) — every single documented CE client-fault exception is HTTP
   400, with no exceptions. Both mappings now return 400.

2. **AnomalyMonitor "not found" used the generic `ResourceNotFoundException`** instead of
   real AWS's `UnknownMonitorException` (backend.go `DeleteAnomalyMonitor`,
   `UpdateAnomalyMonitor`). Confirmed via `API_DeleteAnomalyMonitor`/
   `API_UpdateAnomalyMonitor` error lists and cross-checked that
   `aws-sdk-go-v2/service/costexplorer/types/errors.go` models
   `UnknownMonitorException` as a distinct typed error a real caller could
   `errors.As` against — the generic mapping meant such callers would get the wrong Go
   type. Added the `ErrUnknownMonitor` sentinel (wraps `awserr.ErrNotFound` like the
   other not-found sentinels, so `errors.Is(err, awserr.ErrNotFound)`-style generic
   checks still work, but `errors.Is(err, ce.ErrNotFound)` no longer does — see the trap
   below).

3. **AnomalySubscription "not found" used the generic `ResourceNotFoundException`**
   instead of real AWS's `UnknownSubscriptionException` (backend.go
   `DeleteAnomalySubscription`, `UpdateAnomalySubscription`). Same fix pattern as #2,
   confirmed via `API_DeleteAnomalySubscription`/`API_UpdateAnomalySubscription`.

4. **`GetAnomalyMonitors`/`GetAnomalySubscriptions` silently dropped unknown ARNs**
   instead of erroring (backend.go). Passing a `MonitorArnList`/`SubscriptionArnList`
   containing an ARN that doesn't exist returned a shorter-than-expected page instead of
   `UnknownMonitorException`/`UnknownSubscriptionException`
   (`API_GetAnomalyMonitors`/`API_GetAnomalySubscriptions` both document this). Both
   backend methods gained an `error` return value; the whole call now fails fast on the
   first unknown ARN rather than silently filtering, matching real AWS's all-or-nothing
   behavior. `GetAnomalySubscriptions`'s separate `MonitorArn` *filter* parameter
   (distinct from `SubscriptionArnList`) was deliberately left non-validating — AWS
   documents no `UnknownMonitorException` for that parameter on this op, it's a genuine
   filter that returns zero matches for an unknown monitor.

5. **`CreateAnomalySubscription`/`UpdateAnomalySubscription` accepted a `MonitorArnList`
   referencing nonexistent monitors**, silently persisting referentially-invalid state
   (backend.go). Real AWS returns `UnknownMonitorException`
   (`API_CreateAnomalySubscription`/`API_UpdateAnomalySubscription`). This is the one fix
   in this pass closest to the "disguised no-op" class from the parity principles: the
   create/update themselves were real, but they'd happily create a subscription
   permanently pointing at nothing, which a real client can never do. `Update` validates
   before mutating (holds the lock, checks first) so a rejected update leaves the
   existing subscription's `MonitorArnList` untouched.

### Traps for the next auditor

- `ce.ErrUnknownMonitor` and `ce.ErrUnknownSubscription` both wrap the shared
  `awserr.ErrNotFound` sentinel (same as `ce.ErrNotFound`), but they are **distinct
  pointer values** from `ce.ErrNotFound` itself. `errors.Is(err, ce.ErrNotFound)` will be
  `false` for an `ErrUnknownMonitor`/`ErrUnknownSubscription` error — this is
  intentional (see `handler.go` `handleError`'s three separate `case` arms) and lets each
  map to its own `__type` on the wire. If you add a new not-found error to this package,
  decide explicitly whether it's generic (`ce.ErrNotFound`) or resource-specific, and add
  a `handleError` case for anything resource-specific — don't assume the existing
  `ErrNotFound` case will catch it.
- `GetAnomalySubscriptions`'s `MonitorArn` request field is a **filter**, not a foreign
  key that must resolve — don't "fix" it to return `UnknownMonitorException` on a
  nonexistent monitor; that's the one FK-shaped parameter on these ops that AWS
  deliberately does *not* validate (see `TestHandler_GetAnomalySubscriptions_MonitorArnFilter`
  and `TestInMemoryBackend_GetAnomalySubscriptions_MonitorArnFilterIgnoresUnknown`).
- The cost/usage/forecast/reservation/savings-plans query families
  (`GetCostAndUsage`, `GetCostForecast`, `GetReservationUtilization`, etc.) are
  intentionally deterministic mocks derived from a 90-day synthetic cost ledger seeded in
  `seedCostLedger`. Per this project's parity rules, this is acceptable — there is no real
  billing data for an emulator to reproduce — so don't flag the synthetic ratios
  (`spCommitmentRatio`, `riPurchasedCostRatio`, etc.) as stubs; they're a deliberate
  modeling choice, not a disguised no-op. What *would* be a bug is if one of these ops
  stopped reading the ledger and returned a hardcoded literal instead — they don't.
- `GetAnomalies` filters on a `[startDate, endDate]` overlap against each anomaly's own
  `AnomalyStartDate`/`AnomalyEndDate` — this looks like it should require both bounds but
  deliberately doesn't (an anomaly with an unset `AnomalyEndDate`/`AnomalyStartDate`, e.g.
  ones inserted via the `AddAnomaly` test helper, never gets filtered out by either bound
  since the guard is `a.AnomalyEndDate != "" && ...`). This is why
  `TestHandler_SnapshotRestoreWithAnomalies` and other tests can call `GetAnomalies` with
  an empty body and still see seeded anomalies.

## 2026-07-24 pass

This pass closed both items the prior pass had explicitly deferred to a "next pass"
(the required-field-validation gap, and the unconfirmed `ErrValidation` wire type), then
field-diffed the remaining ops the prior pass had only wire-shape-"spot-checked"
(`GetCostAndUsageComparisons`/`GetCostAndUsageWithResources`/`GetCostComparisonDrivers`/
`ListCostCategoryResourceAssociations`/`ListSavingsPlansPurchaseRecommendationGeneration`/
`GetSavingsPlanPurchaseRecommendationDetails`/`StartSavingsPlansPurchaseRecommendationGeneration`/
`GetApproximateUsageRecords`) against the real generated Go SDK source (types, serializers.go,
deserializers.go, validators.go), not just doc pages. That surfaced several real,
previously-undetected bugs:

1. **`ErrValidation`'s wire `__type` was the invented `"InvalidParameterException"`.**
   Confirmed via `types/errors.go`'s full exception list (no `InvalidParameterException`
   or `ValidationException` modeled for any CE op) and the CE API reference's
   `CommonErrors.html`, which documents `ValidationError` (HTTP 400) as the shared
   client-fault type for malformed/missing-required-member requests. Changed to
   `"ValidationError"`. Also swept every ad-hoc `errInvalidRequest`-based required-field
   check in the handler layer (which rendered as a bare `{"message": "..."}` body with no
   `__type` field at all — itself a wire-shape bug) over to `ErrValidation`, so every
   required-field violation across the package now gets a consistent, correct
   `__type: "ValidationError"` / HTTP 400 response.

2. **Seven ops were missing required-field validation** that real AWS's
   `validators.go` enforces: `CreateAnomalyMonitor.AnomalyMonitor.MonitorType`,
   `CreateAnomalySubscription.AnomalySubscription.{MonitorArnList,Subscribers,Frequency}`,
   `CreateCostCategoryDefinition.{RuleVersion,Rules}`,
   `UpdateCostCategoryDefinition.{RuleVersion,Rules}`, `TagResource.ResourceTags`,
   `UntagResource.ResourceTagKeys`, `GetAnomalies.DateInterval.StartDate`. All seven are
   now enforced, matching `validateAnomalyMonitor`/`validateAnomalySubscription`/
   `validateOpCreateCostCategoryDefinitionInput`/`validateOpUpdateCostCategoryDefinitionInput`/
   `validateOpTagResourceInput`/`validateOpUntagResourceInput`/`validateAnomalyDateInterval`
   exactly. Required ~15 existing test call sites across `handler_anomalies_test.go`,
   `handler_anomaly_detection_test.go`, `handler_tags_test.go`, and `handler_test.go` to
   gain the now-required fields (mostly `MonitorArnList: []`/a `Subscribers` entry on
   `CreateAnomalySubscription`, and `DateInterval.StartDate` on `GetAnomalies`) — none of
   these were behavior regressions, just previously-lenient test fixtures.

3. **`UpdateAnomalyMonitor` over-validated: it wrongly required `MonitorName`.** Real
   AWS's `UpdateAnomalyMonitorInput` only requires `MonitorArn` — `MonitorName` is
   optional ("Specify the fields that you want to update. Omitted fields are
   unchanged."), confirmed directly from the generated `UpdateAnomalyMonitorInput`
   struct comment. The handler check was deleted, and the backend now only overwrites
   `MonitorName` when non-empty, matching the same "omitted means unchanged" pattern
   `UpdateAnomalySubscription` already used. This was rejecting *valid* real-client
   requests with a 400 — the opposite failure mode from the missing-validation bugs
   above, so worth calling out distinctly.

4. **Three invented/wrong-typed response or request fields**, found by diffing against
   the vendored SDK's actual Go struct fields (not just doc pages, which don't show
   field-name typos):
   - `ListCostCategoryResourceAssociations` returned `CostCategoryReference`/
     `ResourceTagsCount` — neither exists on the real
     `ListCostCategoryResourceAssociationsOutput`. Real field is
     `CostCategoryResourceAssociations []CostCategoryResourceAssociation`
     (`CostCategoryArn`/`CostCategoryName`/`ResourceArn`).
   - `StartSavingsPlansPurchaseRecommendationGeneration` returned `GenerationId` — real
     field is `RecommendationId` (confirmed in both
     `StartSavingsPlansPurchaseRecommendationGenerationOutput` and the `GenerationSummary`
     type `ListSavingsPlansPurchaseRecommendationGeneration` returns).
   - `GetCostAndUsageComparisons` had three separate bugs at once: request fields
     `BaseTimePeriod`/`Metrics` (real: `BaselineTimePeriod`/`MetricForComparison`, the
     latter a *required singular string*, not an array — this op compares exactly one
     metric per call); response field `CostAndUsages` (real:
     `CostAndUsageComparisons`); and `TotalCostAndUsage` wire-typed as an array (real: a
     `map[string]ComparisonMetricValue` keyed by metric name).
   - `GetSavingsPlanPurchaseRecommendationDetails` returned `RecommendationDetail` (an
     untyped `any`) — real field is `RecommendationDetailData`, a
     `RecommendationDetailData` struct.

5. **`GetApproximateUsageRecordsOutput.Services`/`.TotalRecords` were wire-typed as
   strings** (`map[string]string` / `string`); real AWS types both as JSON numbers
   (`map[string]int64` / `int64`, the `NonNegativeLong` shape) — confirmed in
   `deserializers.go`'s `case "TotalRecords":` branch, which parses a `json.Number`, not
   a string. This is the epoch-vs-string timestamp bug class from the parity playbook,
   just for a counter instead of a timestamp: a real client's JSON unmarshal would fail
   outright on a quoted string where it expects a bare number.

6. **Two ops were pure "always returns the same static empty/zero envelope regardless of
   input" stubs** — not just synthetic-but-input-driven mocks like their sibling query
   ops, but genuinely disconnected from any state or request field:
   `GetApproximateUsageRecords` and `StartSavingsPlansPurchaseRecommendationGeneration`
   (+ its `ListSavingsPlansPurchaseRecommendationGeneration` counterpart, which always
   returned an empty list). Both are now real: `GetApproximateUsageRecords` derives
   per-service counts from the cost ledger's `UsageQuantity` over a trailing 30-day
   `LookbackPeriod`; `Start.../List...RecommendationGeneration` now follows the same
   start/persist/list pattern already established by `CommitmentAnalysis` (`AnalysisID`
   → `RecommendationID`), backed by a new `savingsPlansGenerations` `store.Table`
   registered through the existing `registry.ResetAll`/`SnapshotAll`/`RestoreAll`
   lifecycle (no bespoke persistence code needed, no `ceSnapshotVersion` bump required —
   `RestoreAll` already resets any table missing from an older snapshot to empty rather
   than erroring).

7. **`GetCostAndUsage` (and `GetCostAndUsageWithResources`) were missing the
   `GroupDefinitions` response field entirely** — the groups specified by the request's
   `GroupBy`, echoed back on every response per `GetCostAndUsageOutput`/
   `GetCostAndUsageWithResourcesOutput`. This is the exact field this campaign's task
   brief calls out by name as part of the wire-shape parity bar
   (`GroupDefinitions/ResultsByTime/Groups/Metrics/TimePeriod`), and it was silently
   absent from the currently-`ok`-marked primary `GetCostAndUsage` op — a reminder that
   "ok" from a prior pass only means "verified as of that pass's scope," not
   "exhaustively field-diffed forever."

### New traps for the next auditor

- `GetCostAndUsageComparisons`'s `MetricForComparison` is **singular** (one metric per
  call) — don't "fix" it back to a `Metrics []string` array; that's the invented shape
  this pass removed, not the real one.
- `GetCostAndUsageWithResources.ResultsByTime` is *correctly* always empty — this is not
  a stub regression to "fix" by wiring it to the cost ledger the way `GetCostAndUsage`
  is. Real AWS resource-level cost data is keyed by individual resource ARN (e.g. one
  specific EC2 instance), and `seedCostLedger` only models service+date granularity.
  Wiring it to the service-level ledger would produce data that looks resource-level but
  isn't, which is arguably worse than an honestly-empty result. If resource-level
  fidelity is ever prioritized, it needs its own per-resource ledger, not a reuse of the
  existing one.
- `savingsPlansGenerations` jobs never transition out of `"PROCESSING"` (no time-based
  state machine, matching `CommitmentAnalysis`'s existing `AnalysisStatus` behavior in
  this codebase) — don't be surprised a freshly-`Start`ed generation never shows up under
  a `GenerationStatus: "SUCCEEDED"` filter; that's intentional and covered by
  `TestListSavingsPlansPurchaseRecommendationGeneration_FiltersByStatus`.
- The `GetCostAndUsage`/`GetCostForecast`/`GetUsageForecast`/`GetDimensionValues`/
  `GetTags`/`GetCostCategories` family still does **not** enforce `TimePeriod`/`Metrics`
  as required, even though real AWS's validators do (see `gaps` above) — this was
  deliberately left alone this pass since it's a distinct, larger test-fixture-touching
  gap from the 7 ops closed this pass, not an oversight.

## 2026-08-12 pass: Filter/SortBy-absent sweep (gopherstack-a8y0)

The gopherstack-7rq1 wire-field audit flagged 9 candidate ops missing the real optional
`Filter *types.Expression`/`SortBy` request members. All 9 were individually read
against `aws-sdk-go-v2/service/costexplorer@v1.67.4` (not assumed from the
`GetCostCategories` representative): every op genuinely carries `Filter`, but the
`SortBy` half of the claim only holds for 6 of the 9, and of those 6 only 3
(`GetCostCategories`/`GetDimensionValues`/`GetTags`) use the `[]types.SortDefinition`
slice shape the audit assumed — `GetSavingsPlansCoverage`/`GetReservationCoverage`/
`GetReservationUtilization` use a **singular** `*types.SortDefinition` pointer, and
`GetSavingsPlansPurchaseRecommendation`/`GetReservationPurchaseRecommendation`/
`GetCostComparisonDrivers` have **no `SortBy` field at all**. This is exactly the kind
of same-looking-sibling divergence `gopherstack-sdk-shape` warns about — see each op's
note above for its verified shape.

For every op, the point of the fix is that a client's filter/sort was silently dropped
and the call returned success with unfiltered/unsorted results. Three shapes of fix:

1. **Real, testable narrowing/reordering** (`GetCostCategories`, `GetDimensionValues`,
   `GetTags`, `GetReservationCoverage`, `GetReservationUtilization`) — the backend has
   real multi-item state (cost-category rule values, per-dimension ledger values,
   time-bucketed coverage/utilization) for the filter or sort to act on. Each has a test
   proving the effect, not just that the field parses.
2. **Real exclude/include on a single-tenant backend** (`GetReservationPurchaseRecommendation`,
   `GetSavingsPlansPurchaseRecommendation`, `GetSavingsPlansCoverage`) — this emulator is
   single-account/single-region, so LINKED_ACCOUNT/REGION filters get a genuine (if binary)
   effect: the one synthetic item is kept or dropped based on whether it matches.
3. **Accepted for wire parity, documented as inert** (`GetTags`'s Filter — no CE operation
   anywhere in this emulator ever populates `CostEntry.Tags`, so there is no tagged state to
   narrow, even though the filtering code itself is real; `GetCostComparisonDrivers`'s
   Filter — this op always returns an empty `CostComparisonDrivers` regardless of any
   input, so there is no per-driver state at all).

No op needed a bare "accept and ignore" fix — every Filter/SortBy field added either has
a real, non-fabricated effect today, or is explicitly documented as inert with the reason
why, per the parity principle against disguised stubs.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 3 confirmed bugs

- `GetSavingsPlanPurchaseRecommendationDetails`/
  `GetSavingsPlansPurchaseRecommendation`: {wire: fixed} -- the recommendation
  detail and summary maps used `handlerCurrencyCode`'s own VALUE ("USD") as
  the map KEY, producing `{"USD": "USD"}`; real member (costexplorer@v1.67.4
  deserializers.go) is "CurrencyCode". Introduced/reused the existing
  `mapKeyCurrencyCode` constant.
- `GetReservationPurchaseRecommendation`: {wire: fixed} -- its Metadata map
  carried both the same USD-as-key bug AND a fabricated
  "RecommendationTotalCount" key; types.ReservationPurchaseRecommendationMetadata
  has neither (only AdditionalMetadata/GenerationTimestamp/RecommendationId,
  none tracked by this backend) -- Metadata now correctly omitted entirely.
- `GetSavingsPlansPurchaseRecommendation`'s own Metadata map had the identical
  fabricated "RecommendationTotalCount" key (types.
  SavingsPlansPurchaseRecommendationMetadata has the same three real members,
  no total-count field) -- removed, GenerationTimestamp/AdditionalMetadata
  kept.

All proven via real `aws-sdk-go-v2/service/costexplorer` client round trips
(wire_field_fixes_test.go), hand-reverted/confirmed-failing/restored/
`md5sum`-verified byte-identical.

## 2026-08-30 pagination/filter retrofit pass

Picked up the backlog gopherstack-43o8 deliberately deferred: 68 request
fields flagged unread by `cmd/reqfieldscan` across 24 ops, dominated by
pagination cursors and result-shaping params (Filter/GroupBy/SortBy/
SearchString/Context/AccountScope/DataType/RecommendationIds/AnalysisStatus/
EffectiveOn) parsed off the wire and never applied. Regenerated the count
independently before touching any code (`go run ./cmd/reqfieldscan -dir ce`)
and got the identical 68/24, confirming the carried-forward figure was
correct this time. Closed 60 of the 68; the remaining 8 are hand-verified
honest gaps, listed in `gaps` above with the specific no-backing-state reason
for each.

Followed the established `paginateList[T]` pattern (sort by a unique key,
opaque cursor, default 100-item page) for every op with no independent
display order. For ops with an independent `SortBy` or an already-established
non-alphabetical order (most-recently-started-first job lists), added a
sibling `paginateOrdered[T]` in `store.go` that pages through the list
*without* re-sorting it — `paginateList`'s own re-sort by the cursor key
would have silently discarded that order.

### Real bugs found beyond the retrofit

1. **`paginateOrdered`'s own cursor was off by one on every resumed page.**
   `next` is documented (and computed) as the key of the *first item of the
   next page* (`keyFn(list[end])`, where `list[end]` has not yet been
   included in the current page). The resume logic wrongly treated a match as
   "resume *after* this item" (`start = i + 1`) instead of "resume *at* this
   item" (`start = i`), so every page after the first silently dropped
   exactly one record — never duplicated one, which is why a naive
   duplicate-only check would have missed it. Found by this pass's own
   completeness tests (`TestGetCostCategories_SearchStringAndPagination_RealClient`,
   `TestListCostAllocationTagBackfillHistory_Pagination_RealClient`,
   `TestListCommitmentPurchaseAnalyses_StatusFilterAndPagination_RealClient`)
   asserting the full-union-with-nothing-dropped invariant the campaign brief
   requires — confirmed failing against the buggy helper before the one-line
   fix (`start = i` instead of `start = i + 1`, `services/ce/store.go`).
   `TestGetReservationCoverage_Pagination_PreservesSortOrder_RealClient` was
   strengthened after the fact to also assert completeness (it originally
   only checked for duplicates and order, which cannot catch a dropped
   record) — a reminder that "no duplicates" and "nothing dropped" are two
   separate assertions, not one.
2. **`GetForecastByTime` always used `BlendedCost`, ignoring the request's
   `Metric` entirely** (`GetCostForecastInput`/`GetUsageForecastInput.Metric`,
   `costexplorer@v1.67.4`). A `USAGE_QUANTITY` forecast and a `BLENDED_COST`
   forecast were numerically identical. Compounding this: `Metric` is a real
   Smithy enum in `SCREAMING_SNAKE_CASE` (`"USAGE_QUANTITY"`), while
   `GetCostAndUsage`'s `Metrics []string` uses plain CamelCase
   (`"UsageQuantity"`) — even reading `in.Metric` naively into the existing
   `getMetricValue`/`metricUnit` switch would not have matched. Added
   `normalizeMetricName` (strip underscores before matching) so both
   conventions resolve to the same switch.
3. **`GetCostForecast`/`GetUsageForecast`'s `Total` field used the wrong wire
   shape.** Real `GetCostForecastOutput.Total` is `*types.MetricValue`
   (`Amount`/`Unit`); this handler built it as a `ForecastResult`
   (`MeanValue`/`PredictionIntervalLowerBound`/`PredictionIntervalUpperBound`)
   instead — that shape belongs to each entry of `ForecastResultsByTime`, not
   to `Total`. A real client's typed `Total.Amount`/`.Unit` were always nil.
   Found by `TestGetCostForecast_Metric_RealClient` (real-SDK-client
   `strconv.ParseFloat` on an empty string), not by the reqfieldscan sweep —
   `Total` was already being written to, just under the wrong field names.
4. **`GetCostComparisonDriversInput`'s metric field was wire-declared
   `"Metric"`**, matching no real member at all (real:
   `MetricForComparison`, required, same shape as
   `GetCostAndUsageComparisonsInput`). A real `aws-sdk-go-v2` client's
   `MetricForComparison` was silently dropped.
   `getSavingsPlansUtilizationDetailsInput.Fields` had the identical shape of
   bug — real member is `DataType []types.SavingsPlansDataType`.
   `listCostCategoryResourceAssociationsInput.ResourceTagFilter` was a third
   instance: a fabricated field matching no real
   `ListCostCategoryResourceAssociationsInput` member (real:
   `CostCategoryArn`/`MaxResults`/`NextToken` only) — removed outright rather
   than renamed, since nothing in the real API corresponds to it.
5. **Two latent same-second ordering ties**, both exposed (not caused) by
   adding pagination on top of them:
   `ListSavingsPlansGenerations`/`ListCommitmentAnalyses` sort by a
   second-precision timestamp (`GenerationStartedTime`/`AnalysisStartedTime`)
   over `Table.All()` (an *unspecified-order* map walk) using a plain
   (unstable) `sort.Slice`. Two jobs started in the same second could tie and
   land in a different relative order on different calls, which is silently
   harmless without pagination but drops/duplicates records once a cursor
   depends on a fixed total order. Added a unique-ID tiebreak
   (`RecommendationID`/`AnalysisID`) to both. `ListBackfillHistory` had the
   same shape of risk but no unique ID to tie-break on at all — see
   `BackfillJob.BackfillID` below.

### Ordering decisions

- `resultByTimeKey`/`ReservationCoverageByTime.TimePeriod.Start`/etc. used as
  `paginateList` keys are genuinely unique (one bucket per `buildTimeBuckets`
  boundary, never duplicated) and the buckets already arrive in ascending
  chronological order, so `paginateList`'s own re-sort by that key is a
  provable no-op there — no `paginateOrdered` needed for `GetCostAndUsage`
  itself (only for ops with an independent `SortBy`, like
  `GetReservationCoverage`'s `Time` key).
- `ListCostAllocationTags` sorts ascending by the unique `TagKey` already —
  `paginateList` reusing that exact key is the direct, unmodified established
  pattern, not a new mechanism.
- `BackfillJob` (`ListCostAllocationTagBackfillHistory`) had no unique field
  at all — a plain append-only slice, `RequestedAt` at second precision. Real
  `types.CostAllocationTagBackfillRequest` also has no unique-ID field
  (`NextToken` is fully opaque per the docs), so adding an internal-only
  `BackfillID` (uuid, never serialized on the wire) for the sort
  tiebreak/pagination cursor is not a fabricated wire field — it never
  reaches a real client.

### Traps for the next auditor

- `paginateOrdered` and `paginateList` are **not interchangeable**:
  `paginateList` re-sorts by its `keyFn` (correct when that key also defines
  the whole display order — ARN, Name, an already-ascending bucket start);
  `paginateOrdered` trusts the caller's existing order and must be used
  whenever a `SortBy` or a non-alphabetical established order (most-recent-
  first job lists) is in play. Using the wrong one either silently discards a
  requested sort order or (as this pass found) drops a record per page if the
  cursor logic is wrong — re-derive from first principles before copying
  either helper to a new op, don't assume they're equivalent.
- `normalizeMetricName` (strips underscores) is required whenever a value
  from a *singular* `Metric`/`MetricForComparison` field
  (`types.Metric`/`types.SavingsPlansDataType`-style enums, always
  `SCREAMING_SNAKE_CASE`) is fed into `getMetricValue`/`metricUnit`, which
  were written for the *plural* `Metrics []string` convention
  (`GetCostAndUsage`, plain CamelCase, not a real enum type). Don't assume
  every "metric name" string in this file uses the same casing.
- `SavingsPlansUtilizationDetail.Utilization`/`.Savings`/`.AmortizedCommitment`
  are now pointers (`*SavingsPlansUtilizationAgg`/`*SavingsPlansSavings`/
  `*SavingsPlansAmortized`), not values — changed so `DataType` can genuinely
  omit a section. Any new code constructing one of these (only
  `savings_plans.go`'s `GetSavingsPlansUtilizationDetails` does today) must
  take the address, not assign a bare struct literal.

### 2026-08-30 value-semantics pass (gopherstack-uox6, bug class: field read/applied but wrong)

Scope: `services/ce` only, as part of a 3-service pass (guardduty, resourcegroups, ce)
hunting parameters that are read and applied but implement the wrong algorithm --
invisible to field-shape/enum-value sweeps. `guardduty` and `resourcegroups` came back
clean (see their own PARITY.md files); two real bugs found and fixed here:

1. **`GetAnomalies`' `DateInterval` filtered on the wrong field for its upper bound.**
   `GetAnomaliesInput.DateInterval`'s own doc comment (`api_op_GetAnomalies.go`,
   costexplorer@v1.67.4): "The returned anomaly object will have an `AnomalyEndDate` in
   the specified time range." The filter is defined purely against `AnomalyEndDate` --
   `AnomalyStartDate` plays no part. `anomalies.go`'s `GetAnomalies` instead implemented
   an interval-*overlap* test, excluding only when `AnomalyStartDate > endDate`. Net
   effect: an anomaly that started inside the requested window but whose
   `AnomalyEndDate` fell after `endDate` was wrongly included (over-matching) -- e.g. a
   window of `[2024-05-01, 2024-07-01]` wrongly returned an anomaly spanning
   `2024-04-01..2024-08-01`. Fixed to compare `AnomalyEndDate` against both bounds only.
   Upper bound is inclusive (`AnomalyEndDate == EndDate` matches), matching the doc's
   plain "in the specified time range" (no exclusive-end language, unlike the unrelated
   `DateInterval` type `GetCostAndUsage` etc. use). See
   `TestGetAnomalies_DateIntervalMatchesOnAnomalyEndDateOnly`.

2. **`ListCostCategoryDefinitions` treated an omitted `EffectiveOn` as "no filter"
   instead of "today".** `ListCostCategoryDefinitionsInput.EffectiveOn`'s doc comment:
   "If there is no `EffectiveOn` specified, you'll see cost categories that are
   effective on the current date." `cost_categories.go`'s `ListCostCategoryDefinitions`
   only applied the `EffectiveStart` filter when `effectiveOn != ""`, so an unfiltered
   call returned every category ever created, including ones not yet effective. Real
   `CreateCostCategoryDefinitionInput.EffectiveStart` can never be in the future ("Dates
   can't be ... in the future"), so a real client can't usually trigger this, but this
   backend does not itself enforce that constraint on `CreateCostCategoryDefinition`
   (a separate, disclosed gap -- see the required-field/validation sweep, not this
   pass), so the bug is independently observable through this emulator's own API. Fixed
   by defaulting `effectiveOn` to `time.Now().UTC()` (RFC3339, matching
   `EffectiveStart`'s own `YYYY-MM-DDTHH:MM:SSZ` format so the string comparison stays
   valid) when the caller omits it. See
   `TestListCostCategoryDefinitions_OmittedEffectiveOnDefaultsToCurrentDate`.

**Also checked and confirmed correct (not touched):** `TotalImpactFilter`'s six
`NumericOperator` cases (`EQUAL`/`GREATER_THAN`/`GREATER_THAN_OR_EQUAL`/`LESS_THAN`/
`LESS_THAN_OR_EQUAL`/`BETWEEN`, all inclusive/exclusive per
`types.NumericOperator`'s own enum, no doc text needed beyond the operator names
themselves); `costLedgerInBucket`'s `Start`-inclusive/`End`-exclusive bucket boundary
(matches `types.DateInterval`'s doc comment verbatim, reused consistently by
`GetCostAndUsage`, forecasts, reservation coverage/utilization, and
`GetCostAndUsageComparisons`' baseline/comparison periods); `normalizeMetricName`'s
case-fold across the plural/singular metric-name conventions; `filter.go`'s documented
single-clause `Dimensions`/`Tags`/`CostCategories` simplification (real
`And`/`Or`/`Not` composition not modeled -- pre-existing, disclosed, not attempted this
pass, out of scope for a single-service value-semantics slice).

### gopherstack-s2i4 audit (2026-09-08): DataUnavailableException verdict

Full writeup in `gaps` above. Summary: closed as not-a-defect. `DataUnavailableException`
is declared on 22/47 ops (not "every op" as the issue title claimed), never returned by
this emulator, and no citable AWS documentation ties it to any client-callable condition
gopherstack could reproduce (durable verdict, first established by gopherstack-1kse and
independently reverified here against both the pinned SDK and the botocore wire model).
gopherstack's data-less-query behavior (synthetic non-empty fallback for GroupBy queries,
real zero totals otherwise) is the honest alternative, not a masked bug.

Two secondary observations from the same pass, out of scope for s2i4, not acted on:

- `handleGetCostAndUsage` (`handler_cost_usage.go:45`) accepts any non-empty
  `Granularity` string; real `GetCostAndUsageInput.Granularity` is a true Smithy enum
  (`DAILY`/`MONTHLY`/`HOURLY`, botocore `Granularity` shape) and `buildTimeBuckets`
  (`cost_usage.go:143`) silently treats anything unrecognized as `DAILY` rather than
  rejecting it. Consistent with this op's existing documented choice to also skip
  `Metrics` enum validation (see the `GetCostAndUsage` op note above), but not
  previously written down for `Granularity` specifically.
- `buildTimeBuckets` (`cost_usage.go:136-141`) silently returns zero buckets (an empty,
  200-OK `ResultsByTime`) when `TimePeriod.Start`/`.End` fail to parse as `2006-01-02`,
  rather than rejecting a value that doesn't match the wire model's `YearMonthDay`
  pattern (`(\d{4}-\d{2}-\d{2})(T\d{2}:\d{2}:\d{2}Z)?`, botocore `YearMonthDay` shape).
  Not investigated further this pass.

### gopherstack-5mxi (2026-09-08): Granularity enum + unparseable TimePeriod, both fixed

Both of s2i4's secondary observations above were real defects; fixed this pass.

**Granularity.** Confirmed via botocore's `Granularity` shape (`{"type": "string", "enum":
["DAILY", "MONTHLY", "HOURLY"]}`) it is a real enum, and via `GetCostAndUsageRequest`'s
`required` list that it's mandatory for this op (the shape's doc string -- "If Granularity
isn't set, the response object doesn't include..." -- is boilerplate shared with ops where
it's optional; `GetCostAndUsage` itself has no undocumented default). Of the three enum
values, only `MONTHLY` (`cost_usage.go`'s explicit case) and `DAILY` (the `default` case,
which happened to bucket correctly since it *is* the fallback) were correctly bucketed
before this fix. `HOURLY` was not: `buildTimeBuckets`'s switch had no case for it, so a
well-formed `Granularity=HOURLY` request silently fell into the `DAILY` branch and got
day-sized buckets -- a valid enum value producing wrong data, the worse failure mode the
issue called out. Fixed by (1) rejecting any `Granularity` outside the three enum values
in `handleGetCostAndUsage` (`ErrValidation`), and (2) giving `buildTimeBuckets` a real
`HOURLY` case that steps by `time.Hour` and formats bucket boundaries as
`2006-01-02T15:04:05Z` (needed for uniqueness -- `resultByTimeKey`'s pagination cursor
requires distinct bucket starts, which same-day hour buckets can't have as plain dates).
Note: `TimePeriod.Start`/`.End` are still parsed as plain dates (`2006-01-02`) only, so
`HOURLY` queries are only usable with day-boundary time periods; sub-day `Start`/`.End`
(the `THH:MM:SSZ` suffix `YearMonthDay` also permits) is not supported. The ledger has no
per-hour data (`seedCostLedger` is day-granularity only), so hourly buckets legitimately
carry zero-valued totals or, for `GroupBy` queries, the pre-existing `syntheticGroupsFallback`
non-empty synthetic result (unchanged, still deliberate).

**Error code.** Read `GetCostAndUsage`'s declared error set straight from
`deserializers.go`'s `awsAwsjson11_deserializeOpErrorGetCostAndUsage` (costexplorer@v1.67.4,
starts line 1458): `BillExpirationException`, `BillingViewHealthStatusException`,
`DataUnavailableException`, `InvalidNextTokenException`, `LimitExceededException`,
`RequestChangedException`, `ResourceNotFoundException` -- confirmed identical to botocore's
op-level `errors` list. `ErrValidation` maps to wire code `ValidationError`
(`errors.go:20`), which is in neither this list nor, as it turns out, anywhere in the whole
costexplorer module: `grep -rl "ValidationException\|ValidationError"` over the extracted
SDK zip returns nothing, and botocore's `ce/2017-10-25` shape list has no `ValidationException`
either. So this isn't a `GetCostAndUsage`-specific gap; no CE operation has a typed
validation-exception struct at all, matching `errors.go`'s existing doc comment that
`ValidationError` is CE's documented CommonErrors type, not a per-op modeled exception.
Concretely this means a real client's `errors.As(err, &types.SomeException{})` could never
match a validation failure on *any* CE op regardless of which code is chosen -- the best a
real caller can do is the generic `smithy.APIError` interface (`ErrorCode()`), which is
exactly the precedent `TestGetAnomalyMonitors_MalformedBodySurfacesValidationError`
(`handler_error_type_test.go`) already established and asserts against. Swapping to one of
GetCostAndUsage's 7 declared-but-unrelated exceptions (e.g. `DataUnavailableException`)
would be strictly worse: semantically wrong (claims data is unavailable for a malformed
request) and inconsistent with every other required-field check already in this same
handler using `ErrValidation`. Kept `ErrValidation`/`ValidationError`.

**TimePeriod.** `buildTimeBuckets` returning zero buckets on an unparseable
`Start`/`End` (rather than rejecting) is now caught in the handler: both are validated
with `time.Parse("2006-01-02", ...)` before reaching the backend, rejecting with
`ErrValidation` on failure. `buildTimeBuckets` itself is unchanged for this case (still
returns zero buckets if ever called directly with something unparseable, e.g. from
`GetCostForecast`/`GetUsageForecast`, which don't validate `TimePeriod` format -- out of
scope for this issue, which is `GetCostAndUsage`-specific).

Regression tests: `cost_usage_granularity_test.go`
(`TestGetCostAndUsage_InvalidGranularityRejected`,
`TestGetCostAndUsage_HourlyGranularityBucketsHourly`) and
`cost_usage_timeperiod_test.go` (`TestGetCostAndUsage_UnparseableTimePeriodRejected`), both
confirmed failing against unmodified code before the fix.
