---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: xray
sdk_module: aws-sdk-go-v2/service/xray@v1.39.4   # version audited against (go.mod pin; was stale at v1.36.20)
last_audit_commit: 4ad94a2e4                       # HEAD when this manifest was last rewritten
last_audit_date: 2026-08-29
overall: A            # A = genuine fixes found; B = already-accurate, proven op-by-op
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  PutTraceSegments: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-yatn orphan-code triage FALSE POSITIVE: \"InvalidSegment\" (handler_trace_segments.go:43) is not an HTTP error code -- it's the per-item ErrorCode on UnprocessedTraceSegments, a batch/per-entry field inside a 200 response body (types.UnprocessedTraceSegment.ErrorCode is a free-form *string, not a typed/enum exception; real AWS's own PutTraceSegments emits free-text values like this here, e.g. \"Segment size exceeded maximum size\"). deserializeOpErrorPutTraceSegments (deserializers.go) only ever dispatches InvalidRequestException/ThrottledException as HTTP-level errors -- this field is never routed through that switch, so its absence from the op's declared HTTP error set is not a bug. No change made."}
  PutTelemetryRecords: {wire: ok, errors: ok, state: ok, persist: deferred, note: "ring buffer, intentionally ephemeral"}
  GetTraceSummaries: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): EntryPoint was a plain string, real wire shape is a ServiceId object {Name,Type} -- a real client's deserializer errors on a string here; per-item StartTime was entirely missing (a required real-API field); per-item ApproximateTime was a gopherstack-INVENTED field (DELETED) -- the real ApproximateTime is an envelope-level field on GetTraceSummariesOutput (now added there instead). FIXED (6flj sweep, 2026-08-15, flagship Go-kind bug): per-item Annotations was emitted as a flat map[string]<scalar>; the real shape (confirmed against xray@v1.39.4 deserializers.go's awsRestjson1_deserializeDocumentAnnotations) is map[string][]ValueWithServiceIds{AnnotationValue,ServiceIds} -- a JSON ARRAY of tagged-union objects per key. A real client's deserializer calls value.([]interface{}) on each map value and hard-errors ('unexpected JSON type') on anything else, so every real GetTraceSummaries call against a trace with at least one annotation failed outright, not just silently emptied -- this survived the 2026-08-10 pass because that pass diffed member names/nesting but not the Go KIND of a collection value. Fixed by tracking each annotation value's reporting service(s) per distinct value (AnnotationOccurrence, traces.go's accumulateAnnotations) and emitting the tagged union (StringValue/NumberValue/BooleanValue, handler_traces.go's toAnnotationValueView) per the real type. Also disclosed (not fixed): GetTraceSummariesInput's optional Sampling/SamplingStrategy request members are parsed (Sampling) or not modeled at all (SamplingStrategy) and have no effect -- gopherstack has no sampling engine on the trace-summary read path, so every call returns the full unsampled result set, a safe superset rather than a truncation. FIXED (2026-08-29 pass, write-only-state/REVERSE direction): TraceSummary.AvailabilityZones ([]AvailabilityZoneDetail{Name}) and .InstanceIds ([]InstanceIdDetail{Id}) (confirmed against deserializers.go's awsRestjson1_deserializeDocumentTraceSummary case \"AvailabilityZones\"/\"InstanceIds\") were entirely absent from the response, even though the data to compute them (segment aws.ec2.{availability_zone,instance_id}, per docs.aws.amazon.com/xray/latest/devguide/xray-api-segmentdocuments.html) was already parsed and stored on every segment (models.go's Segment.AWS, populated by PutTraceSegments) -- Segment.AWS had no read path anywhere in the package. Now accumulated per trace (traces.go's accumulateAWSResourceInfo, de-duplicated) and surfaced (handler_traces.go). Disclosed, not fixed (structural, cross-service/cross-segment analysis gopherstack's per-segment model doesn't perform): ErrorRootCauses/FaultRootCauses/ResponseTimeRootCauses (require root-cause correlation across a trace's segments, same class as Insight's RootCauseServiceId gap below) and MatchedEventTime (X-Ray's separate 'defined events' feature, not modeled at all -- TimeRangeType=Event is accepted but has no distinct behavior)."}
  BatchGetTraces: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): added missing LimitExceeded field (always false; gopherstack does not enforce/track the trace-document size limit, matching the not-exceeded case)"}
  GetServiceGraph: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): Edge objects now carry SummaryStatistics/StartTime/EndTime, aggregated from the downstream segment on each edge (buildEdgeStats reuses accumulateNodeStats). Also FIXED a direction bug: edgeKey{From,To} was built as {callee,caller} (buildEdgeSet), so nodeToView attached each edge to the DOWNSTREAM node pointing back at its caller -- backwards from the real Edge doc ('Connections to downstream services', types/types.go:1192 on Service.Edges), and meant every real client's rendered service map had arrows running the wrong way, and the upstream node's own Edges list was always empty. Now From=caller/To=callee, matching real semantics. EdgeType intentionally left unset: it is only populated for async 'link' edges (types/types.go:114-115), and gopherstack does not model segment links, so omitting it is the honest case, not a gap. See handler_service_graph_test.go:TestGetServiceGraph_EdgeStatisticsAndDirection. FIXED (2026-08-29 pass, discarded-filter bug, sibling to GetInsightSummaries' 6flj fix): GetServiceGraphInput's optional GroupName/GroupARN (api_op_GetServiceGraph.go: 'The name of a group based on which you want to generate a graph') were parsed by the handler but never passed to the backend at all -- every group, including a nonexistent one, returned the same unfiltered graph. Now resolved to the group's FilterExpression and applied per-trace via the existing evaluateFilter (handler_service_graph.go's resolveGroupFilterExpression); an unresolvable group yields an empty graph (not an error: this op declares no ResourceNotFoundException, only InvalidRequestException/ThrottledException, confirmed in deserializers.go's error switch). See TestGetServiceGraph_GroupFilterExpression_RealClient."}
  GetTraceGraph: {wire: ok, errors: ok, state: ok, persist: ok, note: "same edge-statistics/direction fix as GetServiceGraph (shared buildServiceGraph). GetTraceGraphInput has no GroupName/GroupARN member (scoped directly by TraceIds), so the sibling group-filter bug does not apply here -- confirmed against api_op_GetTraceGraph.go."}
  GetTimeSeriesServiceStatistics: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-29 pass): same discarded GroupName/GroupARN bug and fix as GetServiceGraph (handler_service_graph.go's resolveGroupFilterExpression, applied per-trace before segments are bucketed). See TestGetTimeSeriesServiceStatistics_GroupFilterExpression_RealClient. STATE IS 'partial' because of two real, disclosed-not-fixed gaps: EntitySelectorExpression ('a filter expression defining entities that will be aggregated...supports ID, service, and edge functions') and ForecastStatistics (forecasted high/low fault counts, requires an EntitySelectorExpression ID) are both real optional request members (api_op_GetTimeSeriesServiceStatistics.go) that are accepted but have zero effect -- gopherstack has neither an entity-selector query engine nor a fault-count forecasting model, and per this campaign's standing rule against fabricating a plausible-looking number (see SamplingRateBoost's BoostRate below), no invented forecast is produced. Safe superset (always returns edge-level statistics, per the doc's own 'if no selector expression is specified, edge statistics are returned' default), never a truncation."}
  CreateGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-30 (gopherstack-101r, fabricated-error-code sweep): duplicate-name rejection emitted 'GroupAlreadyExistsException', which names no type anywhere in xray@v1.39.4 (absent from types/errors.go and from every awsRestjson1_deserializeOpError* switch). CreateGroup's own deserializer models only InvalidRequestException and ThrottledException, so InvalidRequestException is the correct code. TestCreateGroup_AlreadyExists_RealClient (error_code_fixes_test.go) confirmed failing pre-fix against the real typed SDK client."}
  GetGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  GetGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): InsightsConfiguration was parsed from the request body and silently discarded -- UpdateGroup could never actually change insights/notifications settings. Also FIXED: FilterExpression was unconditionally overwritten (including with empty string) even when the caller only wanted to change InsightsConfiguration; both fields are now independently optional (pointer/patch semantics), matching real UpdateGroupInput"}
  DeleteGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateSamplingRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): added missing SamplingRateBoost field (config passthrough only, see gaps) and missing RuleLimitExceededException cap enforcement (2000 rules/account, AWS default quota). FIXED 2026-08-30 (gopherstack-101r, fabricated-error-code sweep): duplicate-name rejection emitted 'RuleAlreadyExistsException' and field-validation failures (RuleName/ServiceName/Priority/FixedRate/ReservoirSize) emitted 'InvalidSamplingRuleException' -- neither type exists anywhere in this SDK. CreateSamplingRule's own deserializer models only InvalidRequestException, RuleLimitExceededException, and ThrottledException; InvalidRequestException is the correct code for both conditions. TestCreateSamplingRule_AlreadyExists_RealClient and TestCreateSamplingRule_InvalidPriority_RealClient (error_code_fixes_test.go) confirmed failing pre-fix against the real typed SDK client."}
  GetSamplingRules: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): SamplingRateBoost now included in samplingRuleView"}
  UpdateSamplingRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): added RuleARN-based lookup (previously RuleName-only; real SamplingRuleUpdate allows specifying either); added SamplingRateBoost update support. FIXED 2026-08-07 (gopherstack-6iwu): the real SamplingRuleUpdate type (types.go, confirmed against aws-sdk-go-v2/service/xray) has an Attributes map[string]string field that samplingRuleUpdateInput had no field for at all, so a real client's UpdateSamplingRule Attributes value was silently dropped by json.Unmarshal even though Attributes round-tripped correctly on CreateSamplingRule -- added Attributes to samplingRuleUpdateInput/SamplingRuleUpdate, threaded it into UpdateSamplingRuleWithPointers (maps.Clone on provided, nil leaves unchanged, matching every other optional-pointer field's semantics), and reverted the xray dashboard's read-only-Attributes workaround now that the backend accepts it. Verified with TestHandler_UpdateSamplingRule_Attributes."}
  DeleteSamplingRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): added RuleARN-based lookup, and fixed the Default-rule-undeletable check to run against the resolved rule's name (previously checked the raw ruleName parameter, which combined with an ARN-lookup path would have let a caller delete Default by ARN)"}
  GetSamplingStatisticSummaries: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass): REST path was /GetSamplingStatisticSummaries, real SDK sends /SamplingStatisticSummaries"}
  GetSamplingTargets: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass): REST path was /GetSamplingTargets, real SDK sends /SamplingTargets. FIXED (this pass), STUB CLASS BUG: SamplingBoostStatisticsDocuments was completely absent from the request struct (silently dropped by json.Unmarshal for any real client using SamplingRateBoost) and UnprocessedBoostStatistics was absent from the response envelope. Now parsed/echoed: a boost document for a known rule is accepted (not reported unprocessed), a boost document for an unknown rule is reported in UnprocessedBoostStatistics, same as SamplingStatisticsDocuments. SamplingTargetDocument.SamplingBoost is deliberately left unset in every case -- an earlier version of this fix computed a fabricated BoostRate by linearly interpolating FixedRate/MaxRate against the observed anomaly ratio, but AWS does not publish the real boost-trigger algorithm (API_SamplingBoostStatisticsDocument.html describes the inputs only qualitatively) and a plausible-looking invented rate is a worse gap than an absent one: a client reads and acts on a boost rate, so a wrong-but-plausible number does more damage than a missing field a client would notice and investigate. This is now an honest 'accepted, no engine behind it' gap, same shape as GetInsightImpactGraph's Services: []."}
  GetEncryptionConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "real SDK always POST /EncryptionConfig; handler also accepts GET, harmless superset"}
  PutEncryptionConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  CancelTraceRetrieval: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): previously a silent idempotent no-op on an unknown RetrievalToken -- PARITY.md previously (incorrectly) asserted this 'matches AWS' without checking the modeled error set. CancelTraceRetrieval declares ResourceNotFoundException (confirmed in deserializers.go's awsRestjson1_deserializeOpErrorCancelTraceRetrieval switch); an unknown token now returns 400 ResourceNotFoundException, and cancelling the same token twice now correctly fails on the second call"}
  StartTraceRetrieval: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-29 pass), disabled-validation bug: StartTraceRetrievalInput.StartTime/.EndTime are both real, required fields (api_op_StartTraceRetrieval.go: 'the time range to retrieve traces', required alongside TraceIds) that the handler parsed but never enforced as required and never passed to the backend -- a retrieval token always returned every requested trace ID regardless of the requested time range. Now enforced as required and applied: InMemoryBackend.StartTraceRetrieval only includes a trace whose StartTime falls within [StartTime,EndTime] (inclusive, per the field doc comments). See TestStartTraceRetrieval_TimeRangeFiltering_RealClient (backend signature change: traceIDs []string -> traceIDs []string, rangeStart, rangeEnd time.Time; only in-package callers, repo-wide `go build ./...` reconfirmed clean)."}
  ListRetrievedTraces: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): each RetrievedTrace's document-list field was wire key \"Segments\"; the real field is \"Spans\" (types.Span{Document,Id}) -- awsRestjson1_deserializeDocumentRetrievedTrace only recognizes \"Spans\" and silently drops unknown keys, so every real SDK client received an EMPTY Spans list for every retrieved trace despite a 200 response. Also FIXED: unknown RetrievalToken now returns ResourceNotFoundException (see CancelTraceRetrieval) instead of a fabricated COMPLETE/empty response. Also added the previously-missing TraceFormat field (always \"XRAY\": gopherstack never stores OTEL-format spans)"}
  GetRetrievedTracesGraph: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (this pass): same unknown-token ResourceNotFoundException fix as CancelTraceRetrieval/ListRetrievedTraces. FIXED (2026-08-30, request-field axis sweep): the prior state:ok was itself wrong -- the backend never consulted b.retrievedTraces, so Services/NextToken were unconditionally empty regardless of what StartTraceRetrieval had actually matched. Now builds a real service graph from the retrieved traces' segments (same buildServiceGraph GetTraceGraph uses) and paginates via pkgs/page; see Notes."}
  DeleteResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): PolicyRevisionId was parsed by the handler but never passed to/enforced by the backend -- the atomic/guarded delete this parameter exists for was a complete no-op. Now validated against the stored policy's current revision, returning InvalidPolicyRevisionIdException on mismatch"}
  ListResourcePolicies: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): resourcePolicyView now includes LastUpdatedTime (see PutResourcePolicy)"}
  PutResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass): (1) ResourcePolicy.LastUpdatedTime was completely absent from the model and wire view (a real, documented field: 'When the policy was last updated, in Unix time seconds') -- added and set on every Put; (2) the max-5-policies violation used the wrong exception -- was InvalidRequestException, now correctly PolicyCountLimitExceededException (PutResourcePolicy's modeled error set does not even include InvalidRequestException as a fallback, per deserializers.go); (3) added PolicySizeLimitExceededException enforcement, previously entirely unenforced (AWS docs: policy document 'can be up to 5kb in size'). Revision-ID conflict + JSON validation remain correctly enforced. RE-CHECKED (this pass): BypassPolicyLockoutCheck/LockoutPreventionException confirmed still genuinely blocked, not merely under-implemented -- see gaps for why (this is NOT the same as the other 'IAM simulation' claims that turned out reachable this campaign; the blocker here is architectural, not effort)"}
  GetIndexingRules: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): each IndexingRule now includes the Rule.Probabilistic.{DesiredSamplingPercentage,ActualSamplingPercentage} object (see UpdateIndexingRule)"}
  UpdateIndexingRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass), STUB CLASS BUG: the request's Rule field (Rule.Probabilistic.DesiredSamplingPercentage -- the entire point of this operation, a probabilistic-sampling-percentage update) was not modeled anywhere: IndexingRule had no Rule field, the handler ignored any Rule in the request body, and UpdateIndexingRule only ever bumped ModifiedAt. This is exactly the 'real-looking op that is a disguised stub' pattern (parity-principles.md #4) -- it always returned 200 but never changed anything a caller asked it to change. Now implemented: ProbabilisticRuleValue{DesiredSamplingPercentage,ActualSamplingPercentage} added to the model, wired through the request (tagged-union {\"Probabilistic\":{...}} per IndexingRuleValueUpdate) and response. Also FIXED a second, independent bug in the same handler: the not-found response was hand-built as json.Marshal(map[string]any{\"ModifiedAt\": rule.ModifiedAt}) -- marshaling a raw time.Time produces an RFC3339 string, not the required epoch-seconds number (the exact epoch-seconds bug class this audit was briefed to hunt); GetIndexingRules already did this correctly, only UpdateIndexingRule's response had the bug. Also FIXED wrong error code: not-found was InvalidRequestException, real modeled error is ResourceNotFoundException"}
  GetInsight: {wire: ok, errors: ok, state: ok, persist: ok, note: "REST path /Insight (fixed prior pass). FIXED (this pass), STUB CLASS BUG: insightView already declared a Categories field but toInsightView never set it -- a real-looking field that was silently always empty. Categories is now always [\"FAULT\"] on every gopherstack-detected insight, which is not a guess: InsightCategory (types/enums.go) has exactly one defined value, FAULT, and gopherstack's detector (detectInsights) is exclusively a fault-rate-threshold detector, so this is the only value that could ever be correct. Also FIXED: ClientRequestImpactStatistics (real field: 'the number of requests to the client service and whether the requests were faults or okay' -- i.e. literally {OkCount,FaultCount,TotalCount}) is now populated straight from the same w.Total/w.FaultCount window counters detectInsights already computes to decide whether to open the insight in the first place -- this is not new analysis, just surfacing data the detector already had. RootCauseServiceId/RootCauseServiceRequestImpactStatistics/TopAnomalousServices remain unset (see gaps): those claim a cross-service root-cause determination gopherstack's single-service threshold detector does not perform, unlike Categories/ClientRequestImpactStatistics which are direct reflections of already-tracked counts."}
  GetInsightEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "REST path /InsightEvents (fixed prior pass)"}
  GetInsightImpactGraph: {wire: ok, errors: ok, state: ok, persist: deferred, note: "REST path /InsightImpactGraph (fixed prior pass); Services always [] (out of scope, see gaps -- unchanged this pass)"}
  GetInsightSummaries: {wire: ok, errors: ok, state: partial, persist: ok, note: "REST path /InsightSummaries (fixed prior pass). FIXED (prior pass): InsightSummary.LastUpdateTime was completely absent. FIXED (this pass): same Categories/ClientRequestImpactStatistics fix as GetInsight -- InsightSummary (distinct type from GetInsight's Insight, but declares the identical Categories/ClientRequestImpactStatistics/RootCauseServiceId/RootCauseServiceRequestImpactStatistics/TopAnomalousServices field set) now surfaces both via insightSummaryView. FIXED (6flj sweep, 2026-08-15, discarded-filter bug): GroupARN/GroupName (one required) and StartTime/EndTime (both required) were parsed by the handler but never passed to the backend at all -- every group and every time window returned the exact same unfiltered set of insights. Now enforced as required (matching this op's declared InvalidRequestException) and applied: results are filtered to insights whose active window [StartTime,EndTime) overlaps the request's, and whose GroupName matches the resolved group. STATE IS 'partial', NOT 'ok', because of a real remaining structural gap: this backend's insight detector (detectInsights, insights.go) has no per-group filter-expression evaluation at all -- every detected insight is unconditionally labelled with the fixed group name \"default\" regardless of how many real Group records exist or their FilterExpression. The fix above correctly returns empty for any group other than \"default\" (previously every group incorrectly got the same \"default\" insights back), but a request scoped to \"default\" is not itself evaluating that group's real FilterExpression against traffic -- it is simply \"all insights\", same as before this fix, for the one group name the detector ever produces. Implementing true per-group filter-expression-scoped detection is a detector redesign, out of scope for a wire-shape fix."}
  GetTraceSegmentDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "traceSegmentDest snapshot/Reset fixed prior pass"}
  UpdateTraceSegmentDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): (1) resourceTags is now included in backendSnapshot/Restore, closing the previously-deferred persistence gap; (2) added ResourceARN existence validation -- previously any ARN, including ones that were never a real group or sampling rule, silently returned an empty tag list. Real AWS declares ResourceNotFoundException for TagResource/UntagResource/ListTagsForResource (confirmed in deserializers.go); now enforced against groupsByARN/samplingRulesByARN"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): same ResourceARN existence check as ListTagsForResource, plus added TooManyTagsException enforcement (50 tags/resource cap, AWS docs 'Maximum number of user-applied tags per resource: 50') -- previously unenforced, an unbounded number of tags could be applied"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass): same ResourceARN existence check as ListTagsForResource"}
families:
  route_matcher: {status: ok, note: "unchanged this pass; prior pass audited all 34 dispatch-table paths against serializers.go opPath literals and fixed 6 mismatches (GetInsight/GetInsightEvents/GetInsightImpactGraph/GetInsightSummaries/GetSamplingStatisticSummaries/GetSamplingTargets)"}
  persistence: {status: ok, note: "FIXED (this pass): resourceTags was a plain map (not store.Table-backed) that (a) was never included in backendSnapshot -- tags were lost across every gopherstack restart -- and (b) was never cleared by InMemoryBackend.Reset(), the exact same bug class the prior pass fixed for traceSegmentDest but missed here. Both fixed: resourceTags now round-trips through Snapshot/Restore and is reset to an empty map in Reset()"}
  error_codes: {status: ok, note: "FIXED (this pass): independently field-diffed every operation's modeled error set against aws-sdk-go-v2/service/xray@v1.36.20's deserializers.go per-op error switch (awsRestjson1_deserializeOpError<Op>), not just handleError's own type switch. Found and fixed: UpdateIndexingRule not-found was InvalidRequestException (real: ResourceNotFoundException); PutResourcePolicy's policy-count-limit violation was InvalidRequestException (real: PolicyCountLimitExceededException, and InvalidRequestException isn't even in that op's modeled error set); TagResource/UntagResource/ListTagsForResource/CancelTraceRetrieval/ListRetrievedTraces/GetRetrievedTracesGraph never returned ResourceNotFoundException at all despite it being modeled for all six. Added ErrResourceNotFound/ErrTraceRetrievalNotFound/ErrPolicySizeLimitExceeded/ErrRuleLimitExceeded/ErrTooManyTags sentinels and corresponding handleError overrides. Confirmed unchanged/correct: GetGroup/DeleteGroup/UpdateGroup/GetSamplingRules/CreateSamplingRule/UpdateSamplingRule/DeleteSamplingRule/GetInsight*/DeleteResourcePolicy all declare ONLY InvalidRequestException (+ThrottledException, +RuleLimitExceededException for CreateSamplingRule) for not-found -- X-Ray's Smithy model does NOT give these ops ResourceNotFoundException, so gopherstack's existing InvalidRequestException mapping for Group/SamplingRule/Insight/ResourcePolicy not-found was already correct and is unchanged"}
gaps:
  - "GetInsightSummaries' GroupARN/GroupName filter is honored at the wire/query layer (6flj sweep, 2026-08-15) but the insight DETECTOR itself (detectInsights, insights.go) has no per-group filter-expression evaluation -- every detected insight is unconditionally labelled GroupName=\"default\" regardless of how many real Group records a caller has created or what their FilterExpression says. A request scoped to \"default\" gets every detected insight (correct behavior only by coincidence of there being one implicit group); a request scoped to any other real group correctly gets an empty result now, but not because that group's filter was evaluated -- because no insight is ever labelled with it. True per-group detection would require evaluating each group's FilterExpression against live segment traffic, a detector redesign out of scope for a wire-shape fix."
  - "GetTraceSummariesInput's optional Sampling (bool) and SamplingStrategy (Name/Value) request members have no effect: gopherstack has no sampling engine on the trace-summary read path (Sampling is parsed and discarded; SamplingStrategy is not modeled at all). Every call returns the full unsampled TraceSummaries set regardless of what a client requests, which is a safe superset (never a truncation a client wouldn't expect), not a correctness bug -- but flagged here as a real, never-modelled request member per 6flj's checklist."
  - PutTelemetryRecords ring buffer (100 entries) not persisted across restart; low-risk, AWS telemetry data itself is operational/ephemeral by nature (unchanged this pass)
  - "Insight.RootCauseServiceId, RootCauseServiceRequestImpactStatistics, TopAnomalousServices, and GetInsightImpactGraph's Services remain always empty/unset -- these claim a cross-service root-cause/topology determination that gopherstack's insight detector (detectInsights in insights.go, a single-service fault-rate-threshold heuristic with no service-graph awareness) does not perform. NARROWED this pass: Categories and ClientRequestImpactStatistics were moved OUT of this gap and implemented (see GetInsight/GetInsightSummaries ops) once it was established they need no anomaly-detection algorithm at all -- Categories has exactly one possible enum value (FAULT) given gopherstack's detector, and ClientRequestImpactStatistics is a direct surfacing of the w.Total/w.FaultCount counters the detector already computes to decide whether to open the insight. The remaining fields genuinely require cross-service causality analysis this detector was never designed to do; judged out of scope, same as before."
  - "SamplingRateBoost's runtime boost-trigger VALUE (the actual BoostRate number X-Ray would compute) is NOT implemented and never will be guessed: AWS does not publish the algorithm (API_SamplingBoostStatisticsDocument.html describes the inputs, AnomalyCount/SampledAnomalyCount/TotalCount, only qualitatively), so SamplingTargetDocument.SamplingBoost is always left unset. An earlier draft of this pass computed a fabricated rate (linear interpolation between FixedRate and MaxRate by anomaly ratio) and was reverted on review: a fabricated quota/price/rate is worse than an absent one, because a client reads and acts on it without rechecking a plausible-looking number. NARROWED this pass: the WIRE gap (SamplingBoostStatisticsDocuments/UnprocessedBoostStatistics were previously silently absent regardless of the algorithm question) IS fixed -- documents for known rules are now accepted, documents for unknown rules are now reported in UnprocessedBoostStatistics. The net effect for a client: submitting a boost document for a rule with SamplingRateBoost configured is accepted and produces no error, but also produces no observable SamplingBoost on the returned target -- an honest 'accepted, no engine behind it' gap."
  - "PutResourcePolicy's BypassPolicyLockoutCheck field is parsed but LockoutPreventionException is never raised. RE-VERIFIED this pass (WebFetch against docs.aws.amazon.com/xray/latest/api/API_PutResourcePolicy.html): the real check is 'the policy would prevent THE CALLER OF THIS REQUEST from calling PutResourcePolicy in the future' -- i.e. it evaluates the submitted policy document against the calling IAM principal's identity, not against any abstract/generic principal. gopherstack's xray package never resolves or threads a calling principal into request handling at all (grep confirms zero use of pkgs/awsmeta, which only carries Account/Region/Partition/RequestID, not a principal ARN) -- there is no 'the caller' value in scope to evaluate against. This is a genuine architectural gap distinct from the six other 'blocked' claims resolved this campaign: those were blocked by unimplemented-but-available logic, this one is blocked by an identity concept the request pipeline does not carry at all. Implementing a real per-principal check would require adding caller-identity plumbing to the whole service (or repo-wide), which is out of scope for a resource-policy op. The parameter is still accepted (matches wire shape) but has no effect, which is safe (never falsely rejects a real client's request) even though it under-enforces relative to real AWS."
  - "ThrottledException is declared in the modeled error set for every X-Ray operation but is never emitted anywhere in gopherstack (no rate limiting is modeled). This is consistent with the rest of gopherstack's emulation approach (no service throttles by default) and is not treated as a gap specific to X-Ray."
  - "GetTraceSummaries' TraceSummary.ErrorRootCauses/FaultRootCauses/ResponseTimeRootCauses and MatchedEventTime remain always empty/unset (2026-08-29 pass): the root-cause fields require cross-segment causality analysis gopherstack's per-segment model doesn't perform (same class as Insight's RootCauseServiceId gap above); MatchedEventTime belongs to X-Ray's separate 'defined events' feature, not modeled at all."
  - "GetTimeSeriesServiceStatisticsInput's EntitySelectorExpression (entity-selector query language) and ForecastStatistics (fault-count forecasting) are real optional request members (2026-08-29 pass) that are accepted but have no effect -- gopherstack has neither engine, and per this file's standing rule against fabricating a plausible-looking number (see SamplingRateBoost below), no invented forecast is produced. Always returns the documented default (edge-level statistics), a safe superset."
deferred:
  - none; all routed ops covered by ops/families above
leaks: {status: clean, note: "Janitor.Run uses pkgs/worker.Group with Ticker + Stop() on ctx.Done(); sweepExpiredTraces holds b.mu.Lock only around map mutation, releases before telemetry/logging calls. Re-verified this pass: no new goroutines/tickers introduced; all new lock paths (resourceExists, resolveSamplingRule, DeleteResourcePolicy's revision check) execute entirely within their caller's existing Lock/RLock and use defer Unlock/RUnlock."}
---

## Notes

- **Route-matcher bug class** (prior pass, unchanged): 6 of 34 routed X-Ray operations used
  their operation-name-shaped path (e.g. `/GetInsight`) instead of the actual REST path the
  real `aws-sdk-go-v2/service/xray` client serializes (e.g. `/Insight`). See git history for
  the full list; not re-audited this pass since the route table is unchanged.
- **Paths confirmed correct** (prior pass, unchanged): `GetServiceGraph`→`/ServiceGraph`,
  `GetTraceGraph`→`/TraceGraph`, `GetGroup`→`/GetGroup`, `GetGroups`→`/Groups`,
  `GetSamplingRules`→`/GetSamplingRules`, `GetIndexingRules`→`/GetIndexingRules`,
  `GetRetrievedTracesGraph`→`/GetRetrievedTracesGraph`,
  `GetTraceSegmentDestination`→`/GetTraceSegmentDestination`,
  `GetTimeSeriesServiceStatistics`→`/TimeSeriesServiceStatistics`.
- `/EncryptionConfig` real SDK client only ever sends POST for `GetEncryptionConfig`;
  gopherstack's `RouteMatcher` also accepts GET on that path -- harmless superset, left as-is.
- **New this pass -- client-breaking wire-shape bugs found by field-diffing responses
  against `deserializers.go`, not just request routing**: the route-matcher audit in the
  prior pass proved 6 ops were *unreachable*; this pass found operations that *were*
  reachable and returned HTTP 200, but whose response bodies a real SDK client would
  silently mis-parse or partially drop:
  - `GetTraceSummaries`: `EntryPoint` was serialized as a JSON string; the real
    `TraceSummary.EntryPoint` field is a `ServiceId` object. A real client's
    `awsRestjson1_deserializeDocumentServiceId` call fails outright on a string value
    (`"unexpected JSON type"`), meaning any real SDK caller reading `EntryPoint` off of
    `GetTraceSummaries` would have hit a deserialization error on every single trace
    summary with a root segment.
  - `ListRetrievedTraces`: each retrieved trace's span-document list was sent under the
    key `"Segments"`; the real key is `"Spans"`. Because `awsRestjson1_deserializeDocumentRetrievedTrace`
    silently ignores unrecognized keys (the `default: _, _ = key, value` case), this
    doesn't error -- it just silently produces an **empty** `Spans` slice on every
    `RetrievedTrace`, for every caller, forever. A 200 response with quietly-dropped
    data is worse than an error: nothing signals the client that its request was
    misunderstood.
  - `GetTraceSummaries`'s per-item `ApproximateTime` field was invented (not in the real
    `TraceSummary` type at all) while the *real* `ApproximateTime` field -- which
    genuinely exists, just one level up, on `GetTraceSummariesOutput` itself ("the start
    time of this page of results") -- was completely absent from the response envelope.
    This is a case where fixing "no such field" and "missing field" were the same
    one-line move: delete the wrong one, add the right one at the right nesting level.
  - `UpdateIndexingRule`'s not-found response path hand-built its own JSON with
    `json.Marshal(map[string]any{"ModifiedAt": rule.ModifiedAt})`, marshaling a raw
    `time.Time` (RFC3339 string) instead of the epoch-seconds number every other
    timestamp field in this service correctly uses via `float64(t.Unix())`. This is the
    exact "epoch-seconds timestamp bug class" this audit was briefed to hunt for --
    it had evidently been missed in earlier passes because it only fires on the
    (previously essentially untested) success path of one specific handler, not the
    general JSON-marshal path most other ops share via a shared `toXView` helper.
- **Stub-class bug**: `UpdateIndexingRule` is the clearest example found this pass of
  parity-principles.md's warning #4 ("a 'real-looking' op may be a disguised stub"). It
  always returned HTTP 200 with a plausible-looking `IndexingRule` body, and every
  existing unit test for it passed -- but the one thing a caller uses this operation
  for (changing the indexing sampling percentage) had no code path at all: the request's
  `Rule` field was never read, and `IndexingRule` itself had no field to hold a sampling
  percentage in the first place. Green tests did not catch this because the tests only
  asserted "the call succeeds and `ModifiedAt` changes," never "the sampling percentage
  I asked for is reflected back."
- `SamplingRule.Version` in the wire view (`samplingRuleView.Version`) is hardcoded to
  `1` -- matches real AWS behavior (X-Ray sampling rules do not expose a mutable version
  counter via the API), NOT a bug. Unchanged this pass.
- `evaluateFilter` implements a deliberately small subset of the X-Ray filter-expression
  grammar (fault/error/throttle/http.status/responsetime/annotation.KEY); judged
  acceptable emulator scope, unchanged this pass.
- `maxSamplingRules`: RE-VERIFIED this pass against **two independent sources**, both
  keyed to the same AWS Service Quotas quota code `L-8C0C998A`: (1)
  docs.aws.amazon.com/general/latest/gr/xray.html's published "Service quotas" table
  ("Custom sampling rules per region", Default 25, Adjustable: Yes); (2) a third-party
  snapshot of AWS's own `list-aws-default-service-quotas` API output
  (github.com/fanovilla/aws-default-service-quotas, quotas/xray_quotas.json --
  `"QuotaCode": "L-8C0C998A", "QuotaName": "Custom sampling rules per region", "Value":
  25.0, "Adjustable": true`), independently confirming the same value from the API
  gopherstack's own number was supposed to reflect, not just a second doc page scrape
  of the same source. FIXED: constant corrected from 2000 to 25.
  The 2000 value under-enforced relative to real AWS by two orders of magnitude: a
  real client relying on RuleLimitExceededException firing at the documented quota
  (e.g. to test its own quota-handling logic) would have seen gopherstack silently
  accept up to 2000 rules instead. Proven with a probe test that failed against the old
  constant (creating a 26th rule returned 200 instead of RuleLimitExceededException);
  existing `TestCreateSamplingRule_RuleLimitExceeded` (whitebox_test.go) parameterizes off
  the constant itself, so it required no changes and still passes at the corrected value.
  NOTE on "Adjustable: Yes": unlike the ELB precedent (CreateLoadBalancerPolicy's
  TooManyPolicies, gopherstack-6851 -- left unenforced because AWS's official quota
  table has NO row at all for that limit, so no default number exists to enforce), X-Ray's
  25-rule quota IS an AWS-published DEFAULT, just one a real account can raise via a
  Service Quotas increase request. Enforcing the published default is not fabricating a
  hard cap: it is the same "enforce the documented default, since gopherstack cannot know
  any individual real account's raised limit" pattern this file already uses elsewhere
  without controversy (TooManyTagsException at 50 tags/resource, maxResourcePolicies at 5
  -- both also technically increasable in real AWS). A real, unmodified account hits
  RuleLimitExceededException at 25, same as gopherstack now does.
- `defaultIndexingRuleSamplingPct = 1.0` (the built-in "Default" indexing rule's initial
  `DesiredSamplingPercentage`/`ActualSamplingPercentage`): RE-VERIFIED this pass (WebSearch
  against CloudWatch Transaction Search docs) -- AWS's actual default indexing rate is
  1%, confirming this constant was already correct. No change needed.

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors

`Handler()`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.
xray is restjson1 (confirmed from `xray@v1.39.4` deserializers.go's
`awsRestjson1_deserializeOpError*` prefix); plain text doesn't decode
through `restjson.GetErrorInfo`, so a real client got `*json.SyntaxError`,
not even `UnknownError`.

Fixed by routing the ReadBody error through this handler's own
`handleError(c, path, err)` (the unused second parameter is the request
path, threaded here since `op` isn't extracted yet at this point in
`Handler()`): none of its typed `case`s (`awserr.ErrNotFound`,
`awserr.ErrConflict`, `awserr.ErrInvalidParameter`, `errInvalidRequest`,
`errUnknownPath`, syntax/type errors) match a `*http.MaxBytesError`/read
error, so it falls through to the pre-existing default (`__type:
"InternalServiceError"`, 500).

NOTE (pre-existing, NOT fixed by this pass): `"InternalServiceError"` does
not appear in `xray@v1.39.4` `types/errors.go`'s modeled list
(`InvalidPolicyRevisionIdException`, `InvalidRequestException`,
`LockoutPreventionException`, `MalformedPolicyDocumentException`,
`PolicyCountLimitExceededException`, `PolicySizeLimitExceededException`,
`ResourceNotFoundException`, `RuleLimitExceededException`,
`ThrottledException`, `TooManyTagsException`) -- it falls through to the
client's generic `smithy.GenericAPIError` branch rather than a modeled
struct. Still surfaces the correct `ErrorCode()` (proof standard met), but
a possible pre-existing wire-code mismatch in the genuine per-operation
default, out of this ticket's ReadBody-only scope.

Proven with a real `aws-sdk-go-v2/service/xray` client's `CreateGroup`,
whose `FilterExpression` field alone exceeds
`httputils.MaxRequestBodyBytes` (16 MiB).
`TestHandler_OversizedBodySurfacesInternalServiceError`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalServiceError"`; confirmed it fails pre-fix with
`*json.SyntaxError` (hand-reverted, byte-identical restore after).

## gopherstack-o7gx follow-up (2026-08-22): default error path fixed to InternalFailure

The NOTE above flagged `"InternalServiceError"` as unmatched in
`xray@v1.39.4`'s `types/errors.go`. Confirmed: `xray@v1.39.4` models zero
5xx/internal-fault exceptions at all (its 10 modeled types --
`InvalidPolicyRevisionIdException`, `InvalidRequestException`,
`LockoutPreventionException`, `MalformedPolicyDocumentException`,
`PolicyCountLimitExceededException`, `PolicySizeLimitExceededException`,
`ResourceNotFoundException`, `RuleLimitExceededException`,
`ThrottledException`, `TooManyTagsException` -- are all 4xx client faults).
No replacement code maps to a modeled type here either way, so per the
mediapackage/sagemaker precedent (prefer a generic AWS-wide code over
reusing another service's specific modeled exception name), fixed
`handler.go`'s `handleError` default to `errType = "InternalFailure"` --
the same generic fallback already used by 7+ other gopherstack services
with no modeled internal fault. `"InternalServiceError"` was a real AWS
code, just not xray's (it's `secretsmanager`/`transfer`'s modeled type).

`TestHandler_OversizedBodySurfacesInternalFailure` (renamed from
`...InternalServiceError`) now asserts `apiErr.ErrorCode() ==
"InternalFailure"`; confirmed it fails pre-fix with the old
`"InternalServiceError"` code (hand-reverted, byte-identical restore
after).

## gopherstack-wlo1 (2026-08-22): Handler()'s !xrayPaths[path] branch was untyped -- and structurally unreachable via routing

`Handler()`'s own `if !xrayPaths[path] { return c.String(http.StatusNotFound,
"not found") }` guard (handler.go) wrote a bare text/plain 404. xray is
restjson1 (`xray@v1.39.4` `awsRestjson1_` prefix; error decode via
`restjson.GetErrorInfo`), so a real client would have seen
`smithy.GenericAPIError{Code:"UnknownError"}`.

UNLIKE every other dispatch-miss instance this issue found (securityhub,
scheduler, cleanrooms, databrew, apigateway), this one is provably
unreachable via any request a real client can construct AND unreachable via
gopherstack's own routing pipeline: `RouteMatcher` (handler.go) checks the
*identical* `xrayPaths[path]` condition before `Handler()` is ever invoked,
so no request that reaches `Handler()` through
`service.NewServiceRouter`/`RouteMatcher` can ever fail this check -- there
is no daylight between the coarse router check and the fine one, unlike the
prefix-vs-classifier gap that made the other services' analogous branches
provable. `TestHandler_UnknownPath` (handler_test.go) is the one place this
branch is exercised, and it does so by calling `h.Handler()(c)` directly,
bypassing `RouteMatcher`/`RouteHandler` entirely -- a white-box-only path,
not a real-client one.

Fixed defensively for consistency with the class, reusing the existing
`errUnknownPath` sentinel and `handleError` (already routes it to
`errInvalidRequestException` at 400, the same generic 400 `dispatch()`'s
own `errUnknownPath` site already produces two lines below) -- not proven
by a real SDK client, since none can reach it. `TestHandler_UnknownPath`
updated to assert the new typed 400 body instead of the old bare 404.

## 2026-08-29 pass: write-only-state sweep (gopherstack-6flj/21my), forward and reverse

Re-audited despite six prior campaign passes (per this campaign's standing
"a prior pass proves nothing" rule). Verified the premise first: `git log`
showed no drift since the 2026-08-15 6flj sweep; `sdk_version` (xray@v1.39.4)
matched the checked-out module exactly (no SDK bump to re-audit); bd issue
gopherstack-yjn2 ("xray FOLLOW-UP") was read and treated as a claim to
verify, not a map -- its SamplingRateBoost/LockoutPreventionException/edge-
statistics/insight-anomaly-field/quota items were all already resolved or
correctly disclosed-as-gap by the 2026-08-15 pass (see notes above); the one
new angle it raised (Edge SummaryStatistics/StartTime/EndTime) was already
fixed, and the "verify maxSamplingRules/defaultIndexingPct" item was already
independently re-verified against two sources. yjn2's own suggestion was not
where this pass's bugs turned out to be -- both real bugs found this pass
(AvailabilityZones/InstanceIds, GroupName/GroupARN filtering on
GetServiceGraph/GetTimeSeriesServiceStatistics) came from the briefed
REVERSE method applied fresh to GetTraceSummaries/GetServiceGraph, not from
yjn2's list.

Applied the write-only-state method in both directions against every op:

- REVERSE (response-computable-from-stored-state): grepped every field
  declared on `Segment` (models.go) for a read site outside its own
  declaration. `Segment.AWS` (the segment document's `aws` block, parsed by
  `PutTraceSegments`/`trace_segments.go` on every ingested segment) had
  zero read sites anywhere in the package -- confirmed via
  `docs.aws.amazon.com/xray/latest/devguide/xray-api-segmentdocuments.html`
  that `aws.ec2.{instance_id,availability_zone}` are real, documented
  fields, and confirmed via `deserializers.go`'s
  `awsRestjson1_deserializeDocumentTraceSummary` that `AvailabilityZones`/
  `InstanceIds` are real `TraceSummary` response members gopherstack never
  populated. Fixed (see GetTraceSummaries above). Considered but did NOT
  fix the sibling `ResourceARNs`/`ErrorRootCauses`/`FaultRootCauses`/
  `ResponseTimeRootCauses`/`MatchedEventTime` fields: `ResourceARNs` has no
  single unambiguous source field in the segment document schema (several
  candidates -- `ecs.container_arn`, `cloudwatch_logs[].arn` -- none
  canonically "the" resource), and the RootCause/MatchedEventTime fields
  require cross-segment causality analysis or a distinct "defined events"
  feature gopherstack does not implement; disclosed as gaps rather than
  guessed at, per this file's standing rule against fabricating a
  plausible-looking value.
- FORWARD (accepted-request-field with no read path / disabled validation):
  wrote a script diffing every `*Input` struct field against its usage
  sites in the same file. Two real hits, both fixed: `GetServiceGraphInput`/
  `GetTimeSeriesServiceStatisticsInput`'s optional `GroupName`/`GroupARN`
  (parsed, never passed to the backend -- every group returned the
  identical unfiltered graph/stats) and `StartTraceRetrievalInput`'s
  required `StartTime`/`EndTime` (parsed, never enforced as required and
  never passed to the backend -- every retrieval token returned every
  requested trace ID regardless of the requested time range, a disabled
  validation in the same class as emr's SessionEnabled/fsx's
  SourceSnapshotARN/appconfig's LatestDeploymentNumber). All other script
  hits were false positives from nested-struct field access the crude regex
  didn't follow (e.g. `in.SamplingRule.ResourceARN`), manually verified used.

All three fixes proven with a real `aws-sdk-go-v2/service/xray` client
round-tripping through the real `pkgs/service` router
(`wire_field_fixes_test.go`): each test was written and confirmed to fail
against the pre-fix code before the fix was applied, including a
hand-revert-and-reconfirm of the `StartTraceRetrieval` fix specifically
(temporarily forced `filterExpr = ""` at the
`GetTimeSeriesServiceStatistics` call site, reconfirmed the sibling test
failed, restored byte-for-byte via `cp` from a scratchpad copy).

Ops NOT specifically re-audited this pass beyond the two directions above
(unchanged since 2026-08-15, no SDK drift): `PutTraceSegments`,
`PutTelemetryRecords`, `BatchGetTraces`, `CreateGroup`/`GetGroup`/
`GetGroups`/`UpdateGroup`/`DeleteGroup`, all `SamplingRule`/
`SamplingStatistic`/`SamplingTarget` ops, `GetEncryptionConfig`/
`PutEncryptionConfig`, `CancelTraceRetrieval`/`ListRetrievedTraces`/
`GetRetrievedTracesGraph` (beyond confirming they don't share
`StartTraceRetrieval`'s bug -- they take a token, not a time range),
`DeleteResourcePolicy`/`ListResourcePolicies`/`PutResourcePolicy`,
`GetIndexingRules`/`UpdateIndexingRule`, `GetInsight`/`GetInsightEvents`/
`GetInsightImpactGraph`/`GetInsightSummaries` (beyond re-confirming the
6flj group-filter fix's own scope), `GetTraceSegmentDestination`/
`UpdateTraceSegmentDestination`, and all three tag ops.

## 2026-08-30: enumcheck struct-field-hop fix (gopherstack-3dzb), 0 confirmed bugs
`cmd/enumcheck` gained struct-field-hop resolution (a value assigned to a
local struct field, then read back into a `map[string]any` wire-key
position, is now resolved the same way a direct literal/SDK-selector value
already was). Re-run across the whole repo produced the SAME 71 findings as
before the fix (0 confident either way) -- the fix closed a real blind spot
but found nothing new here.

xray's own single hit, `service_graph.go:164`'s `"State": "active"` under
the ambiguous `State` key, was manually verified against
`xray@v1.39.4/types/types.go:1213`: `Service.State` is a plain `*string`
("The service's state.", no enum), not `types.InsightState` -- the exact
Polymorphic collision already documented in `cmd/enumcheck/wirekeys.go`'s
own package doc comment. FALSE POSITIVE, not fixed (nothing to fix: this
field has no SDK-declared legal-value set to check "active" against).

## 2026-08-30: request-field axis sweep (gopherstack-4shm's class), reqfieldscan

Ran `cmd/reqfieldscan -dir xray`: dispatch table 38 ops, 36/38 resolved
(95%, all via the literal-decode path -- xray never uses
`service.JSONOpFunc`/`service.WrapOp`, so the tool's coverage guard is
silent by construction here, confirmed by reading its own
`packageMentionsJSONOpFunc` gate rather than inferring from silence). The 2
unresolved ops, `GetEncryptionConfig`/`GetTraceSegmentDestination`, take no
request body at all (`handleGetEncryptionConfigBody`/
`handleGetTraceSegmentDestination` both `func(_ context.Context, _ []byte)`)
-- correctly unresolved, not a blind spot. 6 fields flagged.

**1 real bug found and fixed:** `getRetrievedTracesGraphInput.NextToken` led
to discovering `GetRetrievedTracesGraph` (backend, `trace_retrieval.go`)
never consulted `b.retrievedTraces` at all -- the exact store
`ListRetrievedTraces` (same file, same retrieval token) reads for its own
response. The handler always emitted `Services: []`/`NextToken: ""`
regardless of what a real `StartTraceRetrieval` had actually matched: **a
listing that never consults its store**, gopherstack-4shm's own named
shape. Fixed: `GetRetrievedTracesGraph`'s signature changed from
`(string, []*Trace, error)` to `(string, []map[string]any, error)`
(`interfaces.go`, `trace_retrieval.go`) -- it now looks up each retrieved
trace's segments via `b.traceSegments` (the same index `GetTraceGraph`
already uses) and calls the existing `buildServiceGraph`, mirroring
`GetTraceGraph`'s pattern exactly rather than inventing a new one. The
handler (`handler_trace_retrieval.go`) now passes the real result through
`pkgs/page.New` for `NextToken`, the same pagination helper
`GetServiceGraph`/`GetTraceGraph` already use, instead of hardcoding both
`Services` and `NextToken` to empty. New test
`TestHandler_GetRetrievedTracesGraph_ReflectsRetrievedTraces`
(`handler_trace_retrieval_test.go`) seeds a real segment, starts a
retrieval that matches it, and asserts `Services` is non-empty with the
right service name; confirmed failing (`Services: []`) against unmodified
code before the fix. No existing test assertion was weakened -- the
pre-existing `TestHandler_GetRetrievedTracesGraph`'s "returns status for a
real retrieval token" subtest still correctly asserts empty `Services`
(its `startTestRetrieval` helper retrieves a trace ID with no segment data
seeded, so empty is the honest answer there too; left unchanged). Repo-wide
`go build ./...`/`go vet ./...` reconfirmed clean -- the only two call
sites of the changed signature were this package's own handler and two
tests already discarding the second return value.

**Confirmed already-documented honest gaps (no new work):**
`getTraceSummariesInput.Sampling` (see `gaps`, GetTraceSummaries note above
-- no sampling engine on the read path); `getTimeSeriesServiceStatisticsInput
.EntitySelectorExpression`/`.ForecastStatistics` (GetTimeSeriesServiceStatistics
`ops:` note, `partial` state -- no entity-selector query engine or
fault-forecast model); `putResourcePolicyInput.BypassPolicyLockoutCheck`
(`gaps` above -- architectural: no caller-identity plumbing anywhere in the
request pipeline to evaluate the lockout check against).

**Newly clarified (folded into an existing gap, not a new one):**
`getInsightImpactGraphInput.NextToken` has nothing to paginate because
`GetInsightImpactGraph`'s `Services` is unconditionally `[]` by the
already-disclosed, deliberate design gap above ("Services always [] (out
of scope, see gaps)") -- unlike `GetRetrievedTracesGraph`, this handler
never discards a real backend return value; there is no backend call to
compute per-insight service impact at all. Confirmed via code read, not
assumed from the existing gap note.

Gates: `go build ./services/xray/...`, `go vet ./services/xray/...`,
`go test -race -count=1 ./services/xray/...` all clean;
`golangci-lint run ./services/xray/...` 0 issues (see below).

**2026-08-31 error-target audit (`cmd/errtargetaudit`, gopherstack-6flj/uox6):**
2 class A findings, both `PutResourcePolicy`, both at the shared
`errInvalidRequest` sentinel (which 26 other operations in this service use
legitimately -- every one re-confirmed to declare `InvalidRequestException`
in its own `awsRestjson1_deserializeOpError<Op>` switch; see
`error_code_fixes_test.go` for the per-op enumeration). `PutResourcePolicy`'s
own deserializer declares `InvalidPolicyRevisionIdException`,
`LockoutPreventionException`, `MalformedPolicyDocumentException`,
`PolicyCountLimitExceededException`, `PolicySizeLimitExceededException`,
`ThrottledException` -- no `InvalidRequestException` at all.

- **PolicyDocument == "" (handler_resource_policies.go:91-93, FIXED):** the
  handler-level check short-circuited before the backend's own
  `PutResourcePolicy` (`resource_policies.go:26-28`), which already validates
  the document as JSON and returns `ErrMalformedPolicyDocument` -- a
  correctly declared type -- for exactly this case. Removed the redundant,
  wrongly-classified pre-check; the backend's existing validation now
  handles it correctly. `validateOpPutResourcePolicyInput` (pinned SDK
  `validators.go`) only checks `PolicyDocument != nil`, not non-empty, so
  `aws.String("")` passes client-side validation and this path is reachable
  by a real client. New test `TestPutResourcePolicy_EmptyDocument_RealClient`
  asserts `errors.As` against `*types.MalformedPolicyDocumentException`;
  confirmed to fail against the unfixed handler (got untyped
  `InvalidRequestException` instead).
- **PolicyName == "" (handler_resource_policies.go:87-89, NOT fixed --
  refusal):** no modeled type in `PutResourcePolicy`'s declared set fits "the
  name field is empty/missing" (its list has none of ValidationException,
  InvalidRequestException, or an analogue of MalformedPolicyDocumentException
  for names). Left as-is rather than invent a code the model doesn't
  declare for this condition -- the operation's own model declares no type
  for it.

Zero other findings for xray. Gates: `go build`/`go vet`/`go test -race
-count=1 ./services/xray/...` clean; `golangci-lint run ./services/xray/...`
0 issues.
