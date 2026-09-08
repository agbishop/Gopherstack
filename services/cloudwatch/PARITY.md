---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: cloudwatch
sdk_module: aws-sdk-go-v2/service/cloudwatch@v1.71.0
last_audit_commit: 9e2b1b8e8   # HEAD when this audit pass started (gopherstack-r80d batch 33)
last_audit_date: 2026-08-21
overall: A            # 2026-08-07 pass (bd gopherstack-lrmf): metric streams now actually deliver
                      # matched PutMetricData records to their configured Firehose delivery stream
                      # when OutputFormat=json, via a new FirehosePutter interface (SetFirehosePutter,
                      # mirroring the existing SNSPublisher/LambdaInvoker alarm-dispatch pattern) --
                      # previously PutMetricStream/matchingRunningStreamNames only tracked config and
                      # advanced LastUpdateDate, never delivering anywhere (see the metric-stream gap
                      # below, and services/firehose's PARITY.md 2026-08-07 pass for the matching
                      # cross-service finding on the Redshift side of that same file). Wiring
                      # SetFirehosePutter to the local firehose backend in cli.go is out of this pass's
                      # scope (cli.go forbidden), so delivery is a documented no-op until wired -- same
                      # deferred-wiring shape as firehose's new RedshiftDataExecutor. Insight-rule
                      # RuleDefinition validation deepened from "well-formed JSON object" to the real
                      # Contributor Insights Rule Syntax's structural rules (Schema.Name/Version,
                      # LogFormat, LogGroupNames, Contribution.Keys, AggregateOn's Count/Sum enum with
                      # the required ValueOf when summing) -- see PutInsightRule row and Notes.
                      # RESTORED from A- (2026-07-26 follow-up pass, bd gopherstack-yvb7). The A- pass
                      # implemented the 7 operations added by the aws-sdk-go-v2 module bump from
                      # v1.55.1 to v1.65.0: PutLogAlarm (a real THIRD alarm type, LogAlarm, confirmed
                      # via types.LogAlarm/AlarmType enum and wired into DescribeAlarms/DeleteAlarms/
                      # DescribeAlarmHistory/SetAlarmState/EnableAlarmActions/DisableAlarmActions -- see
                      # the PutLogAlarm row and "Notes" below), GetDataset/AssociateDatasetKmsKey/
                      # DisassociateDatasetKmsKey (dataset-level customer-managed-KMS-key association
                      # for the implicit "default" dataset, with a fully-qualified-ARN-only KMS
                      # validator distinct from this repo's more permissive alias-accepting pattern,
                      # because AssociateDatasetKmsKey's own doc comment documents that stricter shape),
                      # and GetOTelEnrichment/StartOTelEnrichment/StopOTelEnrichment (account-level
                      # vended-metric OTel/PromQL enrichment status, modeled as a real Running/Stopped
                      # state machine, not fabricated enrichment data). That pass was downgraded from A
                      # to A- because implementing PutLogAlarm's DescribeAlarms integration surfaced a
                      # genuine, previously-undetected bug in the *existing* MetricAlarm/CompositeAlarm
                      # DescribeAlarms default-type-inclusion behavior, left unfixed at the time (out of
                      # that pass's declared scope). THIS PASS fixes it: DescribeAlarms.includeComposite
                      # now defaults to false (only true when AlarmTypes explicitly includes
                      # "CompositeAlarm"), matching LogAlarm's already-correct default and the
                      # documented AlarmTypes contract exactly ("If you omit this parameter, only metric
                      # alarms are returned, even if composite alarms or log alarms exist in the
                      # account" -- confirmed directly against
                      # aws-sdk-go-v2/service/cloudwatch@v1.65.0/api_op_DescribeAlarms.go). See
                      # "DescribeAlarms AlarmTypes default-inclusion bug" in Notes for the full
                      # before/after writeup. Nothing else found holding the grade down this pass.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  PutMetricData: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-07-11 pass — write-time Timestamp acceptance window (2 weeks past / 2 hours future) now enforced, closing bd gopherstack-pyv. Prior pass's fixes (fabricated UnprocessedMetricData field removed, all-or-nothing semantics, Values/Counts array support, NaN/Inf/range validation) remain correct, unchanged. NEW 2026-08-07 (bd gopherstack-lrmf) — after storing the batch, records matching a running metric stream's IncludeFilters/ExcludeFilters are now actually serialized (CloudWatch Metric Streams JSON output format) and delivered via deliverMetricStreams to the wired FirehosePutter, not just used to bump LastUpdateDate. Firehose wiring itself is deferred (cli.go), so delivery is a real, tested, but currently-unwired code path — see families.metric-streams-delivery. CBOR error code FIXED 2026-08-29 — see error-codes family note."}
  GetMetricStatistics: {wire: ok, errors: ok, state: ok, persist: ok, note: "proven correct: period-aligned buckets, Average/Sum/Min/Max/SampleCount, extended-statistic percentiles via collectRawBuckets, anomaly band annotation"}
  GetMetricData: {wire: ok, errors: ok, state: ok, persist: ok, note: "proven correct: metric-math expressions (topo-sorted), ScanBy asc/desc, MaxDatapoints pagination with resumable cursor, PartialData/ArithmeticError messages, cross-account AccountId returns empty not error"}
  ListMetrics: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED this pass — RecentlyActive=PT3H filter was parsed nowhere (silently ignored); now validated and enforced. CBOR error code FIXED 2026-08-29 — see error-codes family note."}
  PutMetricAlarm: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "CBOR error code FIXED 2026-08-29 — see error-codes family note. Metrics (metric-math alarms) FIXED 2026-08-30 (gopherstack-p1ph) — cborPutMetricAlarm never read the 'Metrics' member; the dead legacy XML handlePutMetricAlarm parsed it via parseMetricDataQueriesFromForm but no real client reaches that path. Now parsed via parseMetricDataQueries(input, \"Metrics\") (generalized from the existing GetMetricData.MetricDataQueries parser — both share the _MetricDataQueries wire shape per schemas.go) and echoed back on DescribeAlarms/DescribeAlarmsForMetric via new buildMetricDataQueriesCBOR. Proven with a real aws-sdk-go-v2 write-then-read round trip (metric_math_alarm_p1ph_test.go). MetricStat.Unit remains unmodeled (repo's MetricStat struct has no Unit field, matching the legacy XML parser's pre-existing gap) — not fixed, noted in Notes."}
  PutCompositeAlarm: {wire: ok, errors: ok, state: ok, persist: ok, note: "AlarmRule AND/OR/NOT parsing with cycle + depth-limit detection proven correct"}
  PutLogAlarm: {wire: ok, errors: fixed, state: ok, persist: ok, note: "NEW this pass (v1.65.0 op). Third alarm type (types.LogAlarm, AlarmType enum has CompositeAlarm/MetricAlarm/LogAlarm) — not a MetricAlarm/CompositeAlarm variant. Field-diffed against types.LogAlarm + types.ScheduledQueryConfiguration/ScheduleConfiguration. ComparisonOperator restricted to the 4 real values (no anomaly-detection band operators — log alarms compare one aggregated query result to a scalar Threshold). Required-field/range validation (QueryResultsToAlarm<=QueryResultsToEvaluate in [1,100], ActionLogLineCount in [0,50] with RoleArn required when >0, ScheduledQueryConfiguration.{QueryString,AggregationExpression,ScheduledQueryRoleARN,ScheduleConfiguration.ScheduleExpression} required) mirrors this file's existing PutMetricAlarm/PutCompositeAlarm validation style. No CloudWatch Logs Insights query engine exists here, so EvaluationState/automatic state transitions are never fabricated — state only changes via explicit SetAlarmState, same manual-only model composite alarms use between PutCompositeAlarm re-evaluations. create-or-update semantics (re-PUTting an existing AlarmName replaces it in place) match the SDK doc comment. CBOR error code FIXED 2026-08-29 — see error-codes family note."}
  DescribeAlarms: {wire: ok, errors: ok, state: ok, persist: ok, note: "returns three lists (types.DescribeAlarmsOutput has CompositeAlarms/LogAlarms/MetricAlarms), single combined MaxRecords/NextToken pagination window extended across all three. FIXED THIS PASS (bd gopherstack-yvb7): includeComposite previously defaulted to true when AlarmTypes was omitted, contradicting DescribeAlarmsInput.AlarmTypes's own doc comment (\"If you omit this parameter, only metric alarms are returned, even if composite alarms or log alarms exist in the account\", confirmed against aws-sdk-go-v2/service/cloudwatch@v1.65.0/api_op_DescribeAlarms.go). Now includeComposite := typeSet[\"CompositeAlarm\"] -- composite alarms, like log alarms, are excluded by default and returned only when AlarmTypes explicitly requests them. wire restored to ok; see \"DescribeAlarms AlarmTypes default-inclusion bug\" in Notes for the before/after and the list of tests updated to assert the corrected default."}
  DescribeAlarmsForMetric: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAlarmHistory: {wire: ok, errors: ok, state: ok, persist: ok, note: "UPDATED this pass — log alarm Put/state-change/action history entries are correctly tagged AlarmType=LogAlarm (threaded through appendHistory the same way the prior pass fixed composite-alarm mistagging); AlarmType=LogAlarm filtering proven correct and proven to exclude other alarm types' history. Prior pass's fix (Action-history entries for composite alarms were hardcoded AlarmType=MetricAlarm) remains correct, unchanged. FIXED (2026-08-30 wrapper-key-sweep pagination-reproducibility pass) — pagination was not reproducible across calls: b.alarmHistory is a map[string][]AlarmHistoryItem walked by DescribeAlarmHistory in unspecified (Go map) order, and the result was sorted only by Timestamp, which is not unique (two history items recorded in the same instant, e.g. for different alarms, tie). sort.Slice is unstable, so paging in small windows dropped or duplicated a tied record at the page boundary between two otherwise-identical calls. Fixed by adding a monotonic per-item seq (AlarmHistoryItem.seq, unexported/unpersisted) assigned in appendHistory and used as the sort tiebreak; Restore reindexes seq deterministically (sorted alarm name, then stored per-alarm order) since the field isn't part of the JSON snapshot. See TestDescribeAlarmHistory_PaginationStableAcrossTiedTimestamps (alarm_history_pagination_internal_test.go), hand-reverted to confirm it fails against the unfixed sort (drops/duplicates on the first of 30 iterations), then restored."}
  DeleteAlarms: {wire: ok, errors: ok, state: ok, persist: ok, note: "UPDATED this pass — now also deletes log alarms (b.logAlarms.Delete) and cleans up their history/tags via GetAlarmARNs, which now includes log alarm ARNs too. A log alarm PutLogAlarm creates that no op could later delete would have been an orphan/parity bug; verified it isn't."}
  SetAlarmState: {wire: ok, errors: ok, state: ok, persist: ok, note: "UPDATED this pass — now also accepts log alarm names (setAlarmStateLocked checks b.alarms/b.compositeAlarms/b.logAlarms in that order), decomposed into per-type applyMetricAlarmStateLocked/applyCompositeAlarmStateLocked/applyLogAlarmStateLocked helpers to keep the 3-way branch's complexity down. fires actions only on real transition, correct action-list selection per new state, composite re-evaluation cascades (unchanged, still correct)."}
  EnableAlarmActions: {wire: ok, errors: ok, state: ok, persist: ok, note: "UPDATED this pass — now also toggles log alarms' ActionsEnabled."}
  DisableAlarmActions: {wire: ok, errors: ok, state: ok, persist: ok, note: "UPDATED this pass — now also toggles log alarms' ActionsEnabled."}
  GetDataset: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass (v1.65.0 op). Only the default dataset is supported (real doc comment: implicit, exists without being created) — DatasetIdentifier accepts \"default\" or the full dataset ARN, anything else is ResourceNotFoundException. Field-diffed against types (Arn/DatasetId always present, KmsKeyArn omitted entirely when no key associated, matching the real 'response omits the KmsKeyArn field' doc language, not an empty string)."}
  AssociateDatasetKmsKey: {wire: ok, errors: fixed, state: ok, persist: ok, note: "NEW this pass (v1.65.0 op). KmsKeyArn validated against a fully-qualified-key-ARN-only regex (rejects bare key IDs and alias/alias-ARN forms) per AssociateDatasetKmsKeyInput's own doc comment (\"Key IDs, aliases, and alias ARNs are not accepted\") — deliberately NOT this repo's more permissive validateKmsKeyID pattern (services/comprehend/store.go), which accepts aliases for fields that documented aliases as valid; this field does not. Create-or-replace semantics (re-associating overwrites the prior key) match the doc comment. CBOR error code FIXED 2026-08-29 — see error-codes family note."}
  DisassociateDatasetKmsKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass (v1.65.0 op). Fails with ResourceNotFoundException when the dataset has no KMS key currently associated, matching the doc comment exactly (not InvalidParameterValue or a silent no-op)."}
  GetOTelEnrichment: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass (v1.65.0 op). Status modeled as a real two-state machine (Running/Stopped, types.OTelEnrichmentStatus's only two values), defaulting to Stopped before StartOTelEnrichment is ever called — no enrichment output data (resource ARN/tag labels, PromQL query results) is fabricated anywhere, since gopherstack has no telemetry-enrichment pipeline to actually produce it; this op only tracks whether the account-level setting is on."}
  StartOTelEnrichment: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass (v1.65.0 op). Sets status to Running; both Input/Output are empty structs in the real SDK, matched exactly (no fields on the wire either direction)."}
  StopOTelEnrichment: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass (v1.65.0 op). Sets status to Stopped; both Input/Output are empty structs in the real SDK, matched exactly."}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "generic ARN-keyed tag store — log alarm ARNs (arn.Build with alarm:<name>, same scheme as metric/composite alarms) work here with no changes needed."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  PutDashboard: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass (bd gopherstack-3ro) — DashboardBody is now validated against the documented dashboard JSON/widget schema (dashboard_validation.go): malformed JSON, non-object root, non-array widgets, non-object widget entries, missing widget type, and non-numeric layout fields are errors (DashboardInvalidInputError, HTTP 400, body not persisted, DashboardValidationMessages embedded in the error per the SDK's DashboardInvalidInputError exception shape); unrecognized widget type, missing properties, and metric-widget-missing-metrics are warnings (dashboard still persisted, DashboardValidationMessages returned informationally on the 200 response). Both XML and CBOR wire paths covered independently."}
  GetDashboard: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDashboards: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDashboards: {wire: ok, errors: ok, state: ok, persist: ok}
  PutAlarmMuteRule: {wire: ok, errors: fixed, state: ok, persist: ok, note: "create-or-update semantics confirmed against the real op (no separate Update op exists); re-PUTting an existing MuteName updates in place. CBOR error code FIXED 2026-08-29 — see error-codes family note."}
  GetAlarmMuteRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (gopherstack-r80d batch 33) — MuteTargets was gated on len(rule.AlarmNames)>0, so a real client that legally set MuteTargets with an explicit empty AlarmNames array (validateMuteTargets only null-checks it) got the entire wrapper omitted, indistinguishable from a rule with no MuteTargets set at all. Now gated on rule.AlarmNames != nil. See Notes."}
  DeleteAlarmMuteRule: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAlarmMuteRules: {wire: ok, errors: ok, state: ok, persist: ok}
  PutAnomalyDetector: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteAnomalyDetector: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAnomalyDetectors: {wire: partial, errors: ok, state: ok, persist: ok, note: "GAP found 2026-09-08 (gopherstack-cqzt), NOT fixed this pass — AnomalyDetectorIds and AnomalyDetectorTypes are real modeled input filters (confirmed against botocore's DescribeAnomalyDetectorsInput) that both handler_anomaly_detectors.go and rpcv2cbor_anomaly_detectors.go silently accept-and-ignore: every detector matching Namespace/MetricName is returned regardless. AnomalyDetectorIds is straightforward to implement (AnomalyDetector.ID already exists); AnomalyDetectorTypes is closer to structural since this backend only ever creates SINGLE_METRIC detectors (no MetricMathAnomalyDetector concept exists here). Not fixed this pass because DescribeAnomalyDetectors's backend signature is positional (namespace, metricName, nextToken, maxResults) with ~10 call sites across tests plus two real-SDK-client tests (mutating_create_ids_test.go, wire_field_fixes_test.go); a signature change to add a filter touches all of them for a P3, read-op, lowest-priority-bucket fix — flagged for a dedicated pass instead of rushed under this pass's time budget."}
  PutInsightRule: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-07 pass — RuleDefinition is now validated as well-formed JSON (must decode to a JSON object); previously any non-JSON string was accepted and stored verbatim (insight_rule_validation.go). DEEPENED 2026-08-07 (bd gopherstack-lrmf) — RuleDefinition now also enforces the real Contributor Insights Rule Syntax's structural rules: Schema.Name (CloudWatchLogRule/CloudWatchLogRule2)/Version (1), LogFormat (JSON/CLF), LogGroupNames (non-empty string array), Contribution.Keys (1-4 string entries), and AggregateOn's Count/Sum enum with the required Contribution.ValueOf when summing — verified against AWS's published Contributor Insights Rule Syntax reference (not a generated SDK type; RuleDefinition is opaque there too, see Notes). Deliberately NOT enforced: whether AggregateOn is restricted to a specific Schema.Name (a pre-existing integration test exercises AggregateOn=Count against the base CloudWatchLogRule schema successfully, so this is not cross-checked), Contribution.Filters' per-match-type field shape, and CLF's Fields position-mapping requirement, to avoid diverging from real AWS on rules this pass could not verify against a generated type. PutManagedInsightRules deliberately bypasses this validation (it stores a plain TemplateName string, not JSON, in Definition — verified this is the correct real-AWS shape distinction, not an oversight). create-or-update semantics confirmed (no separate Update op); re-PUTting an existing RuleName re-validates and updates in place. CBOR error code FIXED 2026-08-29 — see error-codes family note."}
  DeleteInsightRules: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-09-08 (gopherstack-cqzt) — handleDeleteInsightRules/cborDeleteInsightRules never called deleteResourceTags, unlike DeleteAlarms/DeleteDashboards/DeleteAnomalyDetector/DeleteMetricStream, which all clean up tags on delete. Insight rules ARE a taggable resource (TagResource's own doc: 'alarms, dashboards, metric streams and Contributor Insights rules'). Tags set via TagResource on an insight rule's ARN survived DeleteInsightRules as a ghost row. Fixed with a new insightRuleARNs helper (resolves ARNs before delete, since the record is gone after) called from both the XML and CBOR paths. See TestHandler_DeleteInsightRules_CleansUpTags."}
  DescribeInsightRules: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableInsightRules: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableInsightRules: {wire: ok, errors: ok, state: ok, persist: ok}
  GetInsightRuleReport: {wire: partial, errors: ok, state: ok, persist: ok, note: "DISCLOSED 2026-08-21 (gopherstack-r80d batch 33) — types.InsightRuleContributor.Datapoints is required but was never emitted at all (no key), matching the existing top-level MetricDatapoints limitation (this backend has no per-timestamp breakdown, only a range-wide sum). Now emitted as an honest empty list. NOT provable via a real aws-sdk-go-v2 client round trip: the rpc-v2-cbor deserializer collapses a present-but-zero-length list to nil identically to an absent key (confirmed for this field and for MuteTargets.AlarmNames), so fixed for wire correctness but not counted as a proven bug. See Notes."}
  ListManagedInsightRules: {wire: ok, errors: ok, state: ok, persist: ok}
  PutManagedInsightRules: {wire: ok, errors: ok, state: ok, persist: ok}
  PutMetricStream: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-07 pass — FirehoseArn/RoleArn/OutputFormat are all 'This member is required' in PutMetricStreamInput (true on every call, not just create, since Put is a full-replace not a patch) but were previously unenforced; OutputFormat now validated against the 3 real enum values (json/opentelemetry0.7/opentelemetry1.0); IncludeFilters+ExcludeFilters-together now rejected per the documented mutual exclusion (metric_stream_validation.go). create-or-update semantics confirmed (no separate Update op); re-PUTting an existing Name updates in place. DELIVERY IMPLEMENTED 2026-08-07 (bd gopherstack-lrmf) — see PutMetricData row and families note below. FIXED 2026-08-21 (gopherstack-r80d batch 33) — StatisticsConfigurations now parsed and stored; see GetMetricStream row and Notes. CBOR error code FIXED 2026-08-29 — see error-codes family note."}
  GetMetricStream: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (gopherstack-r80d batch 33) — StatisticsConfigurations (whose members AdditionalStatistics/IncludeMetrics are both required, cloudwatch@v1.66.3 types/types.go:3270) was structurally absent from gopherstack's MetricStream model entirely: never parsed on PutMetricStream, never stored, never emitted here. A real client configuring additional statistics had that configuration silently discarded. Now threaded through end to end. See Notes."}
  ListMetricStreams: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMetricStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-09-08 (gopherstack-cqzt) — the XML handler's tag cleanup on delete used a hardcoded 'arn:aws:cloudwatch::metric-stream/'+name (empty region/account), which never matched the real ARN PutMetricStream actually hands back to callers (arn.Build(\"cloudwatch\", region, accountID, ...) — non-empty region/account per config.DefaultRegion/DefaultAccountID), so deleteResourceTags always deleted under the wrong key and real tags survived delete as a ghost row. The pre-existing TestHandler_DeleteMetricStream_CleansUpTags test never called TagResource/ListTagsForResource at all despite its name, so it never caught this. The CBOR path (cborDeleteMetricStream) didn't attempt tag cleanup at all. Both fixed to fetch the stream (for its real .Arn) before deleting. See the strengthened TestHandler_DeleteMetricStream_CleansUpTags."}
  StartMetricStreams: {wire: ok, errors: ok, state: ok, persist: ok}
  StopMetricStreams: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAlarmContributors:
    wire: fixed
    errors: ok
    state: fixed
    persist: ok
    note: >
      FIXED 2026-08-13 (bd gopherstack-kb66): both the CBOR (rpc-v2, the only
      protocol this SDK version speaks) and legacy XML paths wrote
      "Contributors"; the real wire key is "AlarmContributors"
      (cloudwatch@v1.66.3 schemas/schemas.go:4033, types.AlarmContributor).
      Deeper bug found while fixing the key: the backend's AlarmContributor
      Go type used a Keys/Sum shape that belongs to the unrelated
      GetInsightRuleReport contributor concept (types.InsightRuleContributor)
      -- the two ops shared one Go struct despite having no relationship in
      the real API, the same shared-type blind spot gopherstack-bv5d records
      for cleanrooms. Split into two types: InsightRuleContributor (Keys/Sum,
      GetInsightRuleReport's contributor calc, behavior unchanged) and a new
      AlarmContributor (ContributorId/ContributorAttributes/StateReason/
      StateTransitionedTimestamp, matching the real type). Composite-alarm
      contributors now report the real child alarm's own StateReason/
      StateTransitionedTimestamp rather than a fabricated Keys/Sum=1 tuple.
      Proven with a real aws-sdk-go-v2 client round trip
      (TestDescribeAlarmContributors_SDKRoundTrip).
  GetMetricWidgetImage: {wire: ok, errors: ok, state: n/a, persist: n/a, note: "rendering-only op; PNG output not byte-compared against real AWS"}
# Families audited as a group (when per-op is impractical):
families:
  metric-streams-delivery: {status: ok, note: "NEW 2026-08-07 (bd gopherstack-lrmf). deliverMetricStreams (metric_stream_delivery.go) filters PutMetricData's batch per running stream's IncludeFilters/ExcludeFilters (reusing the exact filterExcludesMetric/filterIncludesMetric logic already used to decide whether ANY delivery was owed), serializes matches into the public CloudWatch Metric Streams JSON record format (metric_stream_name/account_id/region/namespace/metric_name/dimensions/timestamp/value{max,min,sum,count}/unit -- not a generated SDK type, this is Firehose's payload contract, documented at AWS's CloudWatch-Metric-Streams-formats-json reference), and delivers via a new FirehosePutter interface (SetFirehosePutter, mirroring SNSPublisher/LambdaInvoker). Only OutputFormat=json is serialized; opentelemetry0.7/opentelemetry1.0 are real OTLP protobuf formats this backend has no encoder for (documented gap, not fabricated). SetFirehosePutter is not wired to the local firehose backend in cli.go (forbidden in this pass's scope) -- delivery is a real, unit-tested code path (mockFirehosePutter) that is a documented no-op until wired, not a silent failure disguised as success."}
  alarm-evaluation-state-machine: {status: ok, note: "FIXED this pass — breachesThreshold was missing the LessThanLowerThreshold comparison operator entirely (fell through to default:false, so alarms configured with it never fired). All 4 TreatMissingData modes (missing/notBreaching/breaching/ignore) proven correct in countBreachingPeriods/evaluateMetricAlarmState, including ignore's 'maintain current state when no data' rule and M-of-N DatapointsToAlarm."}
  alarm-action-dispatch: {status: ok, note: "FIXED this pass — composite-alarm action history mistagged AlarmType=MetricAlarm (see DescribeAlarmHistory). SNS/Lambda/EC2-automate/AutoScaling-policy ARN routing, best-effort delivery (failures logged, other actions still run), EC2 InstanceId dimension extraction all proven correct. Actual SNS/Lambda/EC2/ASG client wiring lives in cli.go (out of scope per task boundary) — only the in-package dispatch/selection logic was audited/fixed."}
  alarm-state-change-subscription: {status: ok, note: "NEW 2026-09-06 (gopherstack-x842/gopherstack-9939, driven by an FIS stop-condition gap, not a cloudwatch bug). SubscribeAlarmStateChange(alarmArn, cb) (alarm_subscriptions.go) is a generic 'notify me when alarm X changes state' hook, distinct from AlarmActions/OKActions/InsufficientDataActions (which only fire ARNs the alarm's own owner configured) -- any service can attach to an alarm it does not own without cloudwatch importing it. Fired from setAlarmStateLocked's single state-transition choke point only on an actual state change, independent of ActionsEnabled/muting. Callbacks are collected under b.mu and invoked only after SetAlarmState releases it (same pattern as the existing AlarmActions dispatch above), so a subscriber may safely call back into another backend without a cross-backend lock-order deadlock -- see TestSubscribeAlarmStateChange_CallbackRunsAfterLockReleased. Zero subscribers (nothing wired) is a no-op; SetAlarmState behaves identically to before this feature."}
  error-codes: {status: fixed, note: "ResourceNotFoundException/InvalidParameterValue/InvalidParameterCombination/LimitExceeded all HTTP 400 (correct for CloudWatch's query/XML protocol, which never uses 404); InternalFailure is 500. Spot-checked across alarms/dashboards/mute-rules/anomaly-detectors/insight-rules/metric-streams. New PutMetricStream/PutDashboard/PutInsightRule validation errors this pass correctly route through errors.Is(err, ErrValidation) to InvalidParameterValue/DashboardInvalidInputError rather than falling through to InternalFailure. FIXED 2026-08-29 (error-code protocol sweep) — the bare codes above are the AWSQueryError compatibility aliases cloudwatch's schemas.go embeds on each exception (e.g. InvalidParameterValueException's alias is InvalidParameterValue), resolved only when a client negotiates query-compat mode. gopherstack's XML path (handler_*.go, h.xmlError) correctly uses these bare aliases. But rpcv2cbor_*.go (h.cborError) was ALSO using the bare aliases as the CBOR __type body field, and the real aws-sdk-go-v2 client (which speaks rpc-v2-cbor exclusively, non-query-compatible) resolves __type by exact shape name via smithy-go's TypeRegistry, not the alias — so errors.As(&types.InvalidParameterValueException{}) never matched even though gopherstack-7fyf (below) had already fixed __type's transport. Corrected to the Exception/Fault-suffixed shape names on all reachable CBOR call sites: PutMetricData (InvalidParameterCombinationException/LimitExceededFault/InvalidParameterValueException, via new putMetricDataCBORErrorCode), ListMetrics, PutMetricAlarm, PutAlarmMuteRule, PutInsightRule, PutMetricStream, PutLogAlarm, and the three dataset ops (via new datasetCBORErrorStatus). Proven with real aws-sdk-go-v2 client round trips in error_path_sweep_test.go. NOT fixed: ~21 'X is required' cborError call sites across the same files still emit the bare alias — confirmed unreachable (the corresponding Input field is 'This member is required' and validators.go rejects client-side before any request is sent, e.g. sdk_alarm_mute_rule_test.go:17's comment for PutAlarmMuteRule's Name/Rule.Schedule), so left alone per this sweep's own restraint rule rather than guessed-and-unverified. Also NOT fixed: PutMetricData's InvalidParameterCombinationException path (Value+StatisticValues both set) is separately unreachable via the CBOR client for an unrelated reason — cborDecodeDatum (rpcv2cbor_metrics.go) short-circuits on the first shape it decodes (Values, then StatisticValues, then Value) and never records that another shape was also present in the CBOR map, so datumShapeCount can never observe more than one shape through this path; the code fix was still made (correct once that separate decode-order bug is fixed) but has no passing regression test for that specific combination."}
  persistence: {status: ok, note: "backendSnapshot/persistence.go covers metrics, alarms, composite alarms, alarm history, dashboards, anomaly detectors, insight rules, metric streams, alarm mute rules; field names unchanged by this pass. The metricFilters table (and its persistence_test.go round-trip coverage) was REMOVED this pass along with the rest of the invented PutMetricFilter family -- see Notes; it was never wired into backendSnapshot's real persistence anyway (only a test-only round-trip existed), so no live snapshot format is affected."}
gaps:                      # known divergences NOT fixed — link bd issue ids
  # "DescribeAlarms AlarmTypes default-inclusion bug" (bd gopherstack-yvb7) FIXED 2026-07-26 --
  # see the DescribeAlarms ops row above and the Notes writeup below. No longer a gap.
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - widget.go / widget_draw.go / widget_font.go (GetMetricWidgetImage PNG rendering internals — not a wire-shape or state-correctness concern, only visual fidelity)
  - "IMPLEMENTED 2026-08-07 (bd gopherstack-lrmf): metric-stream Firehose delivery -- see families.metric-streams-delivery. Remaining: opentelemetry0.7/opentelemetry1.0 OutputFormat byte-level OTLP protobuf shape not encoded (json only); SetFirehosePutter cli.go wiring itself is deferred (forbidden in this pass's scope), so delivery does not fire in a real running gopherstack server yet, only under test with a wired mock/real backend."
  - "DEEPENED 2026-08-07 (bd gopherstack-lrmf): insight-rule Definition schema validation -- see PutInsightRule row. Remaining: Contribution.Filters per-match-type field shape (Match/In/NotIn/StartsWith/EqualTo/NotEqualTo) and CLF's Fields position-mapping requirement are not enforced, deliberately, since neither is part of the generated SDK model and this pass could not verify their exact shape against a typed struct."
  - MetricAlarm/LogAlarm fields added to the real SDK model alongside this pass's SDK bump but not part of the 7 named new operations: types.MetricAlarm now also has StateUpdatedTimestamp, EvaluationCriteria, EvaluationInterval, EvaluationWindow, EvaluateLowSampleCountPercentile, and Unit, none of which gopherstack's MetricAlarm struct carries (StateUpdatedTimestamp WAS added to the new LogAlarm type this pass, since that type was authored fresh — but retrofitting it and the other new fields onto the pre-existing MetricAlarm struct is a larger, separate change against a type used across ~15 files, out of scope here). Discovered while field-diffing LogAlarm against MetricAlarm for comparison; worth a dedicated pass.
  - inline Tags on PutLogAlarm's request (PutLogAlarmInput.Tags []types.Tag) is parsed nowhere, matching the exact same pre-existing gap on PutMetricAlarmInput.Tags/PutInsightRuleInput.Tags (neither is parsed either) — deliberately NOT fixed to single out PutLogAlarm, since that would make it inconsistent with its two Put* siblings; tagging still works via the separate TagResource op for all three.
leaks: {status: clean, note: "Janitor (janitor.go) owns the single alarm-eval + metric-sweep goroutine, ctx-cancel-aware, StartWorker only spawns it for *InMemoryBackend. storeDatum/filterAlivePoints reslice (not just filter) to release oversized backing arrays (#60 total-metrics counter avoids O(namespaces) walks). No new goroutines/tickers introduced this pass. New tables (logAlarms, datasets, otelEnrichment, registered in store_setup.go) are plain store.Table[T] with no background workers; log alarms have no automatic evaluation loop (no CloudWatch Logs query engine exists here) so nothing was added to janitor.go's sweep."}
---

## Notes

CloudWatch here speaks **AWS Query (XML) protocol** for the classic SDK path (`Action=` form
POST, `<Foo Response>` root, `ResponseMetadata>RequestId`) and **rpc-v2-cbor** for
`aws-sdk-go-v2/service/cloudwatch@v1.55+` (routed via `X-Amz-Target`/CBOR path,
`/service/GraniteServiceVersion20100801/operation/<Op>`). Every op has two independent encoders
(`handler.go` XML, `rpcv2cbor.go` CBOR) that must be checked separately — a fix in one does not
imply the other is correct.

**Confirmed again this pass at v1.65.0**: `grep -c "type awsAwsquery_serializeOp" serializers.go`
is **0** — every single operation in the currently-pinned SDK module, old and new alike, is
rpc-v2-cbor only; there is no generated query-protocol serializer for anything, not even
`PutMetricAlarm`. This isn't a new development (v1.55.1 was already 0/43 the same way), just
re-verified so the next auditor doesn't waste time thinking the 7 new ops are somehow the first
CBOR-only ones. gopherstack's XML path remains a deliberate compatibility shim for
older-SDK/raw-HTTP callers, not something derived from the pinned module — both new-op wire paths
(`handler_log_alarms.go`/`handler_datasets.go`/`handler_otel_enrichment.go` for XML,
`rpcv2cbor_log_alarms.go`/`rpcv2cbor_datasets.go`/`rpcv2cbor_otel_enrichment.go` for CBOR) were
built by hand to match this package's existing dual-protocol convention, field-diffed against the
generated Go *types* (`types.LogAlarm`, `types.ScheduledQueryConfiguration`,
`types.ScheduleConfiguration`, `GetDatasetOutput`, `GetOTelEnrichmentOutput`) since there's no XML
serializer to diff the XML shape against directly.

### rpc-v2-cbor errors were missing `__type` in the body (bd gopherstack-7fyf — FIXED 2026-07-26)

`cborError` (in `rpcv2cbor.go`) set the exception name **only** in the `X-Amzn-Errortype` HTTP
header. Confirmed against `aws-sdk-go-v2/service/cloudwatch@v1.65.0/deserializers.go`:
`getProtocolErrorInfo(payload []byte)` takes only the decoded CBOR payload and resolves the
exception name from `mv["__type"]` — no header is consulted for this protocol. A caller doing
`errors.As(&types.ResourceNotFoundException{})` against a CloudWatch error received over CBOR
therefore degraded to an untyped/`UnknownError` on the client, even though the HTTP status code
was correct (which is why this went unnoticed — status-code-only test assertions all still
passed).

**Fix:** the CBOR error body now includes `"__type": <code>` alongside `"message"`, matching the
shape AppStream's `rpcv2cbor.go` already used. The `X-Amzn-Errortype` header is still set too
(harmless, matches this codebase's convention), but the body is what the SDK actually reads.

**Shared helper extraction:** since this made CloudWatch's and AppStream's `cborError`/`writeCBOR`/
`isCBORRequest`/`extractCBOROperation` byte-for-byte identical (modulo each service's own path
prefix constant), they were pulled into `pkgs/service/rpcv2cbor.go`
(`WriteRPCv2CBORError`/`WriteRPCv2CBORResponse`/`IsRPCv2CBORRequest`/`ExtractRPCv2CBOROperation`);
both services now delegate to it. This is deliberately scoped to just those four small
protocol-plumbing functions — CloudWatch's per-field CBOR extraction helpers (`cborStr`,
`cborFloat`, `cborTime`, etc., reading directly off a decoded `cbor.Map`) and AppStream's
generic JSON-bridge helpers (`cborToGo`/`goToCBOR`, which route CBOR through the existing
JSON-body op handlers) operate at a fundamentally different level of abstraction from each other
and were left alone rather than forcing them into a shared shape.

### PutLogAlarm / dataset KMS association / OTel enrichment (v1.65.0 SDK bump, 2026-07-25 pass)

The `aws-sdk-go-v2/service/cloudwatch` module bump from v1.55.1 to v1.65.0 added 7 operations:
`PutLogAlarm`, `GetDataset`, `AssociateDatasetKmsKey`, `DisassociateDatasetKmsKey`,
`GetOTelEnrichment`, `StartOTelEnrichment`, `StopOTelEnrichment`. All 7 are now implemented for
real (routed, backed by real state, persisted) — see their `ops:` rows above for field-diff and
error-code detail. Three points worth calling out beyond the per-op notes:

- **LogAlarm is a genuine third alarm type, not a MetricAlarm/CompositeAlarm variant.**
  `types.AlarmType` has exactly three values (`CompositeAlarm`, `MetricAlarm`, `LogAlarm`), and
  `types.LogAlarm` is its own struct (not embedding or aliasing either existing alarm type). An
  alarm that only `PutLogAlarm` could create but no other op could see or remove would have been
  a parity bug in its own right — a client that calls `PutLogAlarm` then reasonably expects
  `DescribeAlarms(AlarmTypes=[LogAlarm])`, `DeleteAlarms`, `DescribeAlarmHistory(AlarmTypes=
  [LogAlarm])`, `SetAlarmState`, `EnableAlarmActions`/`DisableAlarmActions`, and
  `ListTagsForResource`/`TagResource` (via the alarm's ARN) to all work, the same as they do for
  metric and composite alarms. Checked each one specifically (see the updated `ops:` rows) and
  wired log alarms into all of them; `ListTagsForResource`/`TagResource`/`UntagResource` needed no
  code changes since they're already generic over any resource ARN, and log alarm ARNs use the
  same `arn:aws:cloudwatch:<region>:<account>:alarm:<name>` scheme as the other two alarm types.
  `SetAlarmState`/`DeleteAlarms`/`EnableAlarmActions`/`DisableAlarmActions` all now check
  `b.alarms` → `b.compositeAlarms` → `b.logAlarms` in that fixed order; `setAlarmStateLocked` was
  decomposed into per-type `applyMetricAlarmStateLocked`/`applyCompositeAlarmStateLocked`/
  `applyLogAlarmStateLocked` helpers specifically to keep the now-3-way branch under this repo's
  cyclomatic-complexity budget without a `//nolint`.
- **No CloudWatch Logs Insights query engine exists here**, so a log alarm's
  `ScheduledQueryConfiguration` (the query string, log groups, schedule, aggregation expression)
  is stored and returned verbatim but never actually executed — `EvaluationState` is never
  auto-populated and `StateValue` never transitions on its own. This mirrors how composite alarms
  already work between explicit `PutCompositeAlarm` re-evaluations: state changes only happen
  through code paths that are themselves triggered by an explicit call (`SetAlarmState` for log
  alarms; `PutCompositeAlarm`/child-alarm-transition re-evaluation for composite alarms), never a
  background timer that "runs" the alarm's logic. Fabricating query results or auto-transitions
  here would be worse than not implementing them — it would silently misrepresent alarm behavior
  no real gopherstack deployment could reproduce.
- **KMS key ARN validation for datasets is deliberately stricter than this repo's existing
  pattern.** `services/comprehend/store.go`'s `validateKmsKeyID` accepts a bare key ID, a key ARN,
  *or* an alias ARN, because the Comprehend fields it validates document all three shapes as
  valid. `AssociateDatasetKmsKeyInput.KmsKeyArn`'s own doc comment is explicit
  that this field is narrower: "It must be specified as a fully qualified key ARN. Key IDs,
  aliases, and alias ARNs are not accepted." Reusing the permissive Comprehend-style validator
  here would have silently accepted inputs real AWS rejects — `datasets.go`'s
  `kmsKeyArnOnlyRe`/`validateDatasetKmsKeyArn` implements the narrower shape instead, proven by
  `TestCloudWatchBackend_AssociateDatasetKmsKey`'s `alias_arn_rejected`/`bare_key_id_rejected`
  cases.

### DescribeAlarms AlarmTypes default-inclusion bug (bd gopherstack-yvb7 — FIXED 2026-07-26)

While implementing `LogAlarm`'s `DescribeAlarms` integration in the prior pass,
`DescribeAlarmsInput.AlarmTypes`'s own doc comment was read closely for the first time in this
file's history: *"If you omit this parameter, only metric alarms are returned, even if composite
alarms or log alarms exist in the account."* Re-confirmed directly against
`aws-sdk-go-v2/service/cloudwatch@v1.65.0/api_op_DescribeAlarms.go` this pass before fixing.
gopherstack's `DescribeAlarms` did not honor that: `alarms.go` set
`includeMetric := len(typeSet) == 0 || typeSet["MetricAlarm"]` **and**
`includeComposite := len(typeSet) == 0 || typeSet["CompositeAlarm"]` — both defaulted to `true`
when `AlarmTypes` was omitted, so composite alarms were always returned by default alongside
metric alarms, contradicting the documented behavior. `LogAlarm` was implemented against the
*documented-correct* default in the prior pass (excluded unless `AlarmTypes` explicitly includes
`"LogAlarm"`) specifically so it wouldn't inherit this bug, which is exactly what exposed the
inconsistency and earned that pass's A → A- downgrade.

**Fix (this pass):** `includeComposite` is now simply `typeSet["CompositeAlarm"]` — composite
alarms, like log alarms, are excluded unless explicitly requested. This is a genuine default-output
change: callers of unfiltered `DescribeAlarms(..., nil, ...)` (or CBOR requests omitting
`AlarmTypes`) that were relying on composite alarms coming back unasked-for will now get an empty
`CompositeAlarms` list instead, matching real AWS.

Every test that encoded the old wrong assumption was found and updated to either assert the
corrected default (composite alarms absent when `AlarmTypes` is omitted) or to pass
`AlarmTypes=["CompositeAlarm"]` explicitly to keep testing what it was actually about (composite
alarm CRUD/state/re-evaluation, not default-listing behavior) — mirroring the
`log_alarm_excluded_by_default`/`log_alarm_included_when_requested` precedent `LogAlarm` already
set. Updated: `composite_alarms_test.go` (16 `DescribeAlarms` call sites given an explicit
`AlarmTypes=["CompositeAlarm"]`), `alarm_state_test.go` (2 call sites, same), `alarms_test.go`
(`TestCloudWatchBackend_DescribeAlarms_WithComposite` rewritten to assert the omitted-AlarmTypes
case returns no composite alarms AND an explicit-AlarmTypes case still does; two new table cases
`composite_alarm_excluded_by_default`/`composite_alarm_included_when_requested` added to
`TestCloudWatchBackend_DescribeAlarms` alongside the existing `log_alarm_*` cases),
`handler_composite_alarms_test.go` (1 call site), `handler_test.go` (the `DescribeAlarms/with_composite`
case split into a default-excludes case and an explicit-both-types case), `rpcv2cbor_test.go` (1
CBOR call site), `persistence_test.go` (1 call site in the full-state snapshot/restore round-trip
check, given both types explicitly since it verifies both alarm kinds survive persistence), and
`test/integration/cloudwatch_test.go`'s `TestIntegration_CloudWatch_CompositeAlarms` (real
aws-sdk-go-v2 client integration test against a live gopherstack server — same wrong assumption,
now fixed with an explicit-types call plus a new default-omitted assertion). 24 test call sites
across 8 files total encoded the old assumption.

### The big one: PutMetricData has NO partial-success shape

`aws-sdk-go-v2/service/cloudwatch/api_op_PutMetricData.go`'s `PutMetricDataOutput` struct has
**zero fields** besides `ResultMetadata`. The real query-protocol response body is:

```xml
<PutMetricDataResponse xmlns="...">
  <ResponseMetadata><RequestId>...</RequestId></ResponseMetadata>
</PutMetricDataResponse>
```

There is no `UnprocessedMetricData` — that concept exists for other AWS batch APIs (e.g. SQS
`SendMessageBatch`) but **not** for `PutMetricData`. Before this pass, gopherstack's handler
accepted the whole batch, stored every valid datum, and returned HTTP 200 with a fabricated
`<UnprocessedMetricData>` list describing which entries were "unprocessed" (bad StatisticSet
combos, bad StorageResolution, namespace-cap overflow). A real SDK client would never produce or
parse that field — it isn't in the generated deserializer. This is a **wire-shape bug that also
changes API semantics**: real CloudWatch validates the whole request and either accepts all of it
or rejects all of it with a single API error (a datum-level problem anywhere in the batch fails
the entire call, HTTP 400, nothing gets stored). Fixed by splitting `PutMetricData` into a
validate-the-whole-batch pass (`validatePutMetricDataBatch`, no mutation) and a commit pass, only
reached if validation passes.

**Trap for the next auditor**: don't assume every CloudWatch write op lacks a partial-result
field just because PutMetricData does — `PutDashboard`'s `DashboardValidationMessages` and
`DeleteInsightRules`/`DisableInsightRules`/`EnableInsightRules`/`DeleteMetricFilter`'s per-name
`Failures` list are real, generated-from-model fields. Always check the actual SDK struct
(`grep -n "type <Op>Output struct" -A 20` in the unzipped SDK module) before assuming a
partial-failure shape is fabricated OR before assuming one doesn't exist.

### Values/Counts array (new feature this pass)

`MetricDatum.Values`/`.Counts` (parallel arrays, up to 150 unique values, each with an occurrence
count, default count 1 when `Counts` is omitted) was not parsed at all in either wire path —
neither the form parser nor the CBOR decoder read the `Values`/`Counts` member, so data submitted
this way was silently dropped with no error and no stored datapoints. This is CloudWatch's
mechanism for "publish a distribution, get real percentiles back" without pre-aggregating into a
StatisticSet. Implemented: parsing (form + CBOR), validation (mutual exclusion with Value/
StatisticSet via new `Has{Value,StatisticSet,ValuesArray}` presence flags — presence, not a
"non-zero" check, which is what AWS actually validates), `aggregateValuesCounts` for
Sum/SampleCount/Min/Max (computed once, in `storeDatum`, so every caller — not just the two wire
parsers — gets consistent aggregation), and `expandValuesCounts` for exact percentile
reconstruction (proportionally capped at `maxStatSetExpand` samples, unlike the StatisticSet path
which has to *synthesize* a distribution from only Sum/Min/Max/Count).

### LessThanLowerThreshold silently never fired

`ComparisonOperator` has 7 real values (`aws-sdk-go-v2/service/cloudwatch/types/enums.go`), not 6:
`GreaterThan{,OrEqualTo}Threshold`, `LessThan{,OrEqualTo}Threshold`,
`LessThanLowerOrGreaterThanUpperThreshold`, **`LessThanLowerThreshold`**, and
`GreaterThanUpperThreshold`. `breachesThreshold`'s `switch` had no case for
`LessThanLowerThreshold` and fell through to `default: return false` — any alarm configured with
that anomaly-detection operator would never breach, ever, silently. Distinct from
`LessThanLowerOrGreaterThanUpperThreshold` (which fires on either bound); `LessThanLowerThreshold`
only fires on the lower-bound breach.

### Composite-alarm action history mistagging

`executeActions` hardcoded `alarmTypeName="MetricAlarm"` on every Action-history entry it wrote,
even when firing actions for a **composite** alarm (via `SetAlarmState` on a composite alarm
directly, or via `fireCompositeTransitions` when a child alarm's state change cascades). Since
`DescribeAlarmHistory` filters by `AlarmType`, querying a composite alarm's history with
`AlarmType=CompositeAlarm` would miss its own fired-action entries (they were mistagged as
MetricAlarm), while `AlarmType=MetricAlarm` would incorrectly include them. Fixed by threading the
real alarm-type string through both `executeActions` call sites.

### ListMetrics RecentlyActive

`RecentlyActive` only accepts the literal string `"PT3H"` (AWS's only documented value); anything
else is `InvalidParameterValue`. It restricts results to metrics with at least one datapoint in
the last 3 hours (from *now*, not from the request's implicit time context — there is no
StartTime/EndTime on ListMetrics). Was not parsed in either wire path before this pass, so the
filter was silently a no-op (every metric always returned regardless of the param).

### Already-correct traps (do not re-flag)

- `populateBuckets`/`collectRawBuckets` bucket index is `timestamp.Unix() / period` (aligned to
  Unix-epoch boundaries), which is intentional and matches how CloudWatch aligns period
  boundaries for GetMetricStatistics — it is **not** a bug that buckets aren't aligned to the
  request's StartTime.
- `GetMetricData` cross-account queries (`AccountId` set to a different account) return an empty
  result for that query ID rather than erroring — this is intentional (documented behavior: local
  emulator has no cross-account metrics, but AWS itself just returns nothing rather than failing
  the whole request for an inaccessible-but-valid account).
- `expandDatumValues`'s StatisticSet synthetic expansion (one sample at Min, one at Max, remainder
  at residual mean) looks like it's fabricating data, but it's a deliberate, documented
  approximation for extended-statistic (percentile) queries over StatisticSet-only data, which
  AWS itself can only do exactly when `SampleCount==1` or `Min==Max` (see the PutMetricData SDK
  doc comment) — this is explicitly a best-effort approximation.
- `cwMaxMetricNamesPerNamespace`/`cwMaxTotalMetricRecords` are **emulator-only** memory-safety
  caps, not modeled AWS service quotas — CloudWatch's real per-account metric quotas are not
  synchronously enforced by PutMetricData. Do not "fix" the LimitExceeded error code/behavior
  without first checking whether AWS actually enforces a limit at write time (it does not, for
  the metrics-count quota).

### PutMetricData Timestamp acceptance window (2026-07-11 pass)

`validateMetricDatum` (`backend_accuracy.go`) now rejects any `MetricDatum.Timestamp` more than
`cwMetricTimestampPastWindow` (14 days) in the past or `cwMetricTimestampFutureWindow` (2 hours) in
the future, relative to a `now` snapshotted once per `validatePutMetricDataBatch` call (matches
AWS's documented PutMetricData timestamp acceptance window; closes bd `gopherstack-pyv`, previously
deferred by the prior pass "to keep [that pass's] PutMetricData fix reviewable as one change").
`validateMetricDatum`'s signature gained a `now time.Time` parameter — unexported, only two call
sites (`backend.go`'s `validatePutMetricDataBatch` and the `export_test.go` test wrapper), does
**not** touch `PutMetricData`'s exported signature that `cli.go` calls (`cli.go`'s two call sites
already set `Timestamp: time.Now()` explicitly, so they were never at risk).

**Test-suite blast radius**: ~15 existing tests seeded `MetricDatum`/form-encoded `Timestamp`
values from a hardcoded `2024-01-01` (or `2020-01-01`) calendar-date literal, which is now well
outside the enforced window and was silently rejected. Fixed by:
- `export_test.go` adds `RecentTestAnchor()` (now-1h, minute-truncated — safely inside the window,
  and truncation keeps period-bucket-alignment tests behaving like they did with a fixed anchor)
  and `ShiftTestTimestampForTest(anchor, legacyLiteral)` (remaps a `2024-01-01`-relative RFC3339
  literal onto one relative to a real anchor, preserving the offset) for tests that read better
  keeping their literal-date-with-offset structure.
- `export_test.go` also adds `StoreDatumForTest`, a raw bypass of `PutMetricData`'s validation
  used **only** by `SweepExpiredMetrics` eviction tests (`parity_test.go`, `backend_test.go`,
  `batch1_accuracy_test.go`) that need to seed a datapoint already aged past
  `cwMetricRetentionDays` (15 days) — such a point is now impossible to create via the public
  `PutMetricData` API in real AWS too (the write-time window caps at 14 days), so these tests
  model "data that was valid when written and has since aged past retention," which is the only
  way real data ever gets old enough to sweep.
- `rpcv2cbor_test.go`'s `fixedTS` changed from a compile-time `const` (2024-06-01) to a
  test-run-time `var` computed from `time.Now()` — every one of its ~10 call sites (`fixedTS`,
  `fixedTS - 3600`, `fixedTS + 60`, etc.) kept working unchanged.
- A few tests only asserted HTTP status codes / envelope shape and would have silently kept
  passing with zero stored data after this fix (e.g. `TestCloudWatchHandler_GetMetricData_
  ScanByDescending`'s `if len(...) >= 2 { assert... }` guard, `TestHandler_GetMetricStatistics_
  WithDimensions`'s bare `assert.Equal(200, ...)`); these were fixed too so they keep exercising
  real stored-data paths instead of decaying into no-op assertions.
- **Trap for the next auditor**: `TestSweepExpiredMetrics_TwoPhase`'s "fresh datapoint survives
  sweep" case and any test using `RecentTestAnchor()`/`time.Now()`-relative timestamps for
  non-eviction scenarios must go through real `PutMetricData` (not `StoreDatumForTest`) — the
  bypass exists solely to model already-aged data, not as a general test convenience.

### Four gopherstack-invented operations, deleted (2026-07-24 pass)

While field-diffing `PutMetricStream` against the real SDK (to confirm `OutputFormat`'s enum
values), the op list itself was diffed too
(`ls $(go env GOMODCACHE)/github.com/aws/aws-sdk-go-v2/service/cloudwatch@v1.55.1/api_op_*.go`),
which turned up four routed CloudWatch operations that **do not exist** in the real
`aws-sdk-go-v2/service/cloudwatch` module at all:

- **`UpdateAlarmMuteRule`**, **`UpdateInsightRule`**, **`UpdateMetricStream`** — none of these
  exist. Real CloudWatch's `PutAlarmMuteRule`/`PutInsightRule`/`PutMetricStream` are each
  documented as create-or-update in their own SDK doc comments (e.g. `PutMetricStreamInput.Name`:
  "If you are updating a metric stream, specify the name of that stream here"). A real SDK client
  has no `Update*` method to call for any of these three resources — it always calls the `Put*`
  method, whether creating or updating. gopherstack had invented a parallel `Update*` op for each
  that additionally required the resource to already exist (`ResourceNotFoundException` if not),
  which is behavior no real client can trigger or would ever depend on. Deleted: the op consts,
  routing (`handler.go` XML dispatch + `rpcv2cbor.go` CBOR dispatch), the `handleUpdate*`/
  `cborUpdate*` handler functions, and their `GetSupportedOperations()` entries. The `Put*`
  handlers already had correct create-or-update semantics (`Put*Internal` upserts by key), so no
  backend behavior was lost — only the redundant, non-real second entry point.
- **`PutMetricFilter`/`DescribeMetricFilters`/`DeleteMetricFilter`/`TestMetricFilter`** — this
  whole family is a **CloudWatch Logs** concept (`logs:PutMetricFilter` etc.), not a CloudWatch
  (`monitoring:`) one. Confirmed absent from `aws-sdk-go-v2/service/cloudwatch`'s op list and
  present in `aws-sdk-go-v2/service/cloudwatchlogs`'s instead (both were already in the local
  module cache, so both were read directly). `services/cloudwatch/models.go`'s own doc comment on
  the (now-deleted) `MetricFilter` struct even said *"represents a CloudWatch **Logs** metric
  filter"* — a self-admission that this was misplaced. `services/cloudwatchlogs/` already
  implements all four ops for real (`metric_filters.go`, `handler_metric_filters.go`,
  `rpcv2cbor_metric_filters.go` there), so nothing was lost — the cloudwatch-package copies were a
  pure duplicate that no real AWS SDK client could ever reach (a client calling
  `cloudwatchClient.PutMetricFilter` doesn't compile — the method doesn't exist on that client;
  only `cloudwatchlogsClient.PutMetricFilter` does). Deleted: `metric_filters.go`,
  `metric_filters_test.go`, `handler_metric_filters.go`, `handler_metric_filters_test.go`,
  `rpcv2cbor_metric_filters.go`, the `MetricFilter`/`MetricTransformation` types and
  `ErrMetricFilterNotFound` from `models.go`/`errors.go`, the `metricFilters` `store.Table` field
  and its registration in `store.go`/`store_setup.go`, the four methods from the `StorageBackend`
  interface in `interfaces.go`, and the op consts/routing in `handler.go`/`rpcv2cbor.go`. A
  `persistence_test.go` round-trip test for `metricFilters` was also removed — it was the *only*
  place `metricFilters` touched persistence at all; `persistence.go`'s real `backendSnapshot`
  never included it, so no live snapshot format is affected by the removal.

**Why `TestSDKCompleteness` (`sdk_completeness_test.go`, `pkgs/sdkcheck`) didn't catch this**: that
test only checks that `GetSupportedOperations()` is a **superset** of the real SDK's op list (every
real op must be handled) — it does not flag *extra* entries beyond what the SDK defines. That
asymmetry is why four invented ops could sit in the routing table, `GetSupportedOperations()`, and
a whole implementation family for an unknown number of prior passes without any test failing. Worth
keeping in mind for other services: `sdkcheck.CheckCompleteness` passing is necessary but not
sufficient evidence of a clean op surface — the op *list* itself needs an occasional read-through
against the SDK's `api_op_*.go` file listing, not just per-op field-diffing.

**Test fallout**: table-driven `Update*/success` and `Update*/not found` cases in
`handler_metric_streams_test.go`, `handler_insight_rules_test.go`,
`handler_alarm_mute_rules_test.go`, and `rpcv2cbor_test.go` were rewritten as `Put*/updates
existing` cases (re-PUTting an existing resource name, proving create-or-update semantics on the
one real op) instead of being simply deleted, so create-or-update behavior stays under test.
`insight_rule_validation_test.go`'s own new tests (written earlier in this same pass, before the
op-list diff) originally exercised `UpdateInsightRule` too and were fixed the same way.
`handler_test.go#TestCloudWatchHandler_NewOpsInSupportedOperations` gained explicit
`require.NotContains` assertions for all three deleted `Update*` op names, and
`rpcv2cbor_test.go`'s three `Update*/not found` cases were repurposed as `Update*/not a real op`
(same `wantCode: http.StatusBadRequest`, now asserting `InvalidAction` rather than
`ResourceNotFoundException` under the hood — both happen to be 400, so the numeric assertion
didn't need to change, only its meaning).

## 2026-08-20 — gopherstack-jqh2 pass 4: dispatchCBOR shape-3 bug found and fixed

Re-extracted all 50 CloudWatch ops from `cloudwatch@v1.66.3`'s
`request_snapshot/*.request.snap` files (this SDK version is schema-driven —
no hand-written `serializers.go` — so the snapshots are the authoritative
source instead) and cross-diffed against CloudWatch's three op-name tables:
`GetSupportedOperations()`'s literal slice, the query/form dispatch chain
(`dispatchFormAction` et al, handler.go), and `dispatchCBOR`'s chain
(rpcv2cbor.go).

`GetSupportedOperations()` and the form dispatch chain both matched the SDK
50/50. `dispatchCBOR` did not: **`StartMetricStreams` and
`StopMetricStreams` had no case anywhere in the CBOR dispatch chain**
(`dispatchCBOR` → `dispatchDashboardCBOR` → `dispatchResourceManagementCBOR`
→ `dispatchAlarmCBOR` → `dispatchExtendedCBOR` →
`dispatchAnomalyMetricStreamCBOR`/`dispatchInsightRuleCBOR`), even though
both were correctly listed in `GetSupportedOperations` and correctly wired
into the form dispatch chain. Since `cloudwatch@v1.66.3`'s real client
speaks rpc-v2-cbor exclusively (confirmed via `request_snapshot/`, no
`awsQuery` serializer exists in this SDK version), every real
`StartMetricStreams`/`StopMetricStreams` call from a real client landed in
`dispatchInsightRuleCBOR`'s default case: `InvalidAction: unknown
operation: <Op>`. This is the gopherstack-jqh2 shape-3 bug (a parallel
resolution table drifting from the real dispatch) — RouteMatcher and
ExtractOperation both looked correct because they only check
`GetSupportedOperations` membership, not `dispatchCBOR` completeness.

**Fix**: added `cborStartMetricStreams`/`cborStopMetricStreams` to
`rpcv2cbor_metric_streams.go` (input: `Names []string`, output: empty map,
matching the real `StartMetricStreamsInput`/`StopMetricStreamsInput` shape
and `request_snapshot`/`response_snapshot`) and wired both into
`dispatchAnomalyMetricStreamCBOR`.

**Proven by hand-revert**: reverted both changed files to `git show HEAD`,
confirmed `TestSDK_StartStopMetricStreams` (new, drives a real
`aws-sdk-go-v2/service/cloudwatch` client through both ops) failed with the
exact predicted error (`api error InvalidAction: unknown operation:
StartMetricStreams`), then restored both files byte-identical
(`md5sum`-verified) before reapplying the fix.

**New tests**: `handler_sdk_route_table_test.go`
(`TestExtractOperation_SDKRouteTable`) drives all 50 ops through the real
`Handler()` and asserts none falls through to the `InvalidAction`
dispatch-miss sentinel — this is what would have caught the bug above, since
a test that only checks `ExtractOperation`/`RouteMatcher` (gopherstack-ey26)
would not have (see that reasoning in the test's doc comment).
`sdk_metric_streams_start_stop_test.go`
(`TestSDK_StartStopMetricStreams`) is the real-SDK-client regression test
for this specific bug. No stale PARITY.md entries found otherwise.

### gopherstack-r80d batch 33: required-response-member sweep (2026-08-21)

Module resolved deliberately: `services/cloudwatch` has no `dirModuleOverride`
entry, so it maps directly to `aws-sdk-go-v2/service/cloudwatch` — confirmed
distinct from sibling `services/cloudwatchlogs` (module `cloudwatchlogs`, a
separate pinned version, off-limits this batch per a concurrent agent) and
from `eventbridge` (there is no `cloudwatchevents` service directory in this
repo at all — that name doesn't exist here). `go.mod` pins
`cloudwatch@v1.66.3`.

**Protocol, re-confirmed against this exact pinned version**: `api_client.go:214`
sets `options.Protocol = rpcv2.NewCBOR(schemas.GraniteServiceVersion20100801)`
unconditionally — this SDK client speaks **rpc-v2-cbor only**, not query/XML.
gopherstack's own XML path (`handler*.go`) is a hand-maintained compatibility
shim for older/raw clients, not something the pinned real SDK client ever
exercises; all three fixes below were made in both encoders, but only the
CBOR path is provable via `newTestHandlerAndClient`'s real
`aws-sdk-go-v2/service/cloudwatch` client.

`cmd/requiredoutputfields` reports 4 required output fields / 3
ops-with-required (`DescribeAlarmContributors.AlarmContributors`,
`GetDataset.{Arn,DatasetId}`, `GetOTelEnrichment.Status`) — all three already
correct, re-verified unconditionally emitted. The real surface is far larger
one level down: an AST-style read of every domain struct reachable through
this service's other 47 (zero-top-level-required) ops found `InsightRule` (4
required), `MuteTargets`/`Rule` (1 each, via `GetAlarmMuteRule`),
`MetricStreamStatisticsConfiguration` (2, via `GetMetricStream`),
`InsightRuleContributor`/`InsightRuleMetricDatapoint` (3/1, via
`GetInsightRuleReport`), `Tag` (2, via `ListTagsForResource`), matching this
batch's exact target shape (a wrapper field optional at its own op's
top level, wrapping a type that itself declares required members).

**2 bugs found and fixed, both proven via real `aws-sdk-go-v2/service/cloudwatch`
client round trips** (`wire_output_required_r80d_test.go`), hand-reverted to
`git show HEAD:<path>` / confirmed-failing / restored, md5sum-verified
byte-identical:

1. `GetAlarmMuteRule.MuteTargets` (`buildAlarmMuteRuleCBOR`,
   `rpcv2cbor_alarm_mute_rules.go`, and the XML twin in
   `handler_alarm_mute_rules.go`) gated emission on
   `len(rule.AlarmNames) > 0`. `types.MuteTargets.AlarmNames` ([]string) is
   required, but the real client-side validator (`validateMuteTargets`,
   cloudwatch@v1.66.3 validators.go:1418-1425) only null-checks it, so a real
   client can legally `PutAlarmMuteRule` with `MuteTargets: {AlarmNames: []}`.
   gopherstack's own `PutAlarmMuteRule` (alarm_mute_rules.go) never rejects
   an empty list either — a genuinely reachable state, previously
   collapsed into "MuteTargets never set at all". Fixed by gating on
   `rule.AlarmNames != nil` instead; also removed the `,omitempty` json tag
   from `AlarmMuteRule.AlarmNames` in `models.go` so the same nil/empty
   distinction survives a persistence snapshot round trip. The provable,
   observable difference is the wrapper object's own presence
   (`*types.MuteTargets` nil vs non-nil) — see the note below on why
   `AlarmNames`'s own nil-vs-empty is not independently observable through
   this client.
2. `GetMetricStream.StatisticsConfigurations` was structurally absent from
   gopherstack's model: `PutMetricStreamInput.StatisticsConfigurations`
   (whose members `AdditionalStatistics`/`IncludeMetrics` are each required,
   `types.MetricStreamStatisticsConfiguration`, types/types.go:3270) was never
   parsed on `PutMetricStream`, never stored on `MetricStream`, never emitted
   on `GetMetricStream` — no struct field existed at all. A real client
   configuring additional per-metric statistics had that configuration
   silently discarded with no error. Fixed by adding
   `MetricStreamStatisticsConfiguration`/`MetricStreamStatisticsMetric` domain
   types (`models.go`) and threading them through both `PutMetricStream` and
   `GetMetricStream` in the CBOR path (`rpcv2cbor_metric_streams.go`); the
   `MetricStream`/`PutMetricStream`/`GetMetricStream` signatures did not
   change (both take `*MetricStream` already), so no cross-file signature
   fallout.

**1 fixed but explicitly NOT counted as proven** (disqualified by the
provability rule, not by reachability): `GetInsightRuleReport`'s per-
contributor `types.InsightRuleContributor.Datapoints` (required,
schemas.go:1085) was never emitted at all — `cborGetInsightRuleReport`
(`rpcv2cbor_insight_rules.go`) built each contributor map with only
`Keys`/`ApproximateAggregateValue`. This backend has no per-timestamp
breakdown to offer (`aggregateContributorRecord` only accumulates a single
range-wide sum, matching the pre-existing, already-disclosed limitation on
the top-level `MetricDatapoints` field), so the fix emits an honest empty
list rather than fabricating data. **This is not provable via a real
`aws-sdk-go-v2` client round trip**: instrumenting the actual decoded value
showed the rpc-v2-cbor deserializer collapses a present-but-zero-length list
to a nil Go slice *identically* to an absent key — confirmed for both this
field and, independently, for `MuteTargets.AlarmNames` (a non-nil empty
`[]string` sent by a real client still decodes back as `nil`). A real client
of this exact SDK version cannot tell "the key was present with zero
elements" from "the key was never sent" for any list-typed field. This is a
genuinely new wrinkle for this campaign, worth carrying forward: **for
cloudwatch's rpc-v2-cbor protocol, a required list-typed member's
"present-but-empty vs absent" distinction is unobservable through the pinned
real client** — only a *wrapping object's own presence* (a struct pointer,
as in finding 1 above) remains provable. Fixed for wire correctness anyway,
per "fix for correctness if you like, but do not count them."

**Rejected, not bugs**: `InsightRule.{Definition,Name,Schema,State}` (all
`*string`, required) are always written unconditionally in both encoders;
`State` can be stored as an empty string when a real client omits the
(non-required) `RuleState` on `PutInsightRule`, but the field is still always
*present* on the wire (never omitted) — an empty *value* is a data-
correctness/defaulting question, not this campaign's dropped-required-member
class. `AlarmContributor` (already fixed under gopherstack-kb66) re-verified
still correct: `ContributorId`/`ContributorAttributes`/`StateReason` all
unconditionally written, `ContributorAttributes` built via `make(cbor.Map,
len(...))` so it's never nil even when empty.

**Wrapped-type-shape mechanism test (per batch's brief)**: selected by
building a small scratch tool (not committed — a one-off script layered on
`cmd/requiredoutputfields`'s own extraction logic) that, for every service,
finds ops whose `<Op>Output` declares **zero** top-level required members but
has a non-slice field whose own type — or, one hop further, a field of
*that* type — declares ≥2 required members: the exact `GetScheduleOutput` →
`Target` → `EcsParameters.NetworkConfiguration.AwsvpcConfiguration` shape
batch 32 named. Restricting to non-slice (singular `Get`-style) output
fields deliberately excludes the already-well-trodden "List op returns
`[]DomainStruct`" shape (covered by this campaign's ordinary per-op domain-
struct reads since batch 1). Ranked candidates among services with **zero**
required output fields at the flat `cmd/requiredoutputfields` level (so
genuinely invisible to that ranking) and not on this batch's off-limits list:
`fsx` (22 hits — `Backup`/`DataRepositoryTask` direct, plus many generic
`->Tag` hits since every taggable FSx resource wraps the common two-required-
member `Tag` shape) and `codebuild` (22 hits, all substantive:
`Project`/`Build`/`BuildBatch`/`Webhook`/`Sandbox` wrapping
`ProjectEnvironment`(3 required)/`ProjectSourceVersion`(2)/
`ScopeConfiguration`(2)). Both were selected but **not yet audited this
batch** — see the recommendation below; time this batch went to `cloudwatch`
first per the brief's explicit instruction, and the tapering-decision
question the brief poses is answered without needing to burn further budget
on them (see recommendation).

**Instrument validation**: cross-checked `cmd/requiredoutputfields`'s own
4-field/3-op count for cloudwatch by hand-reading all 50 `api_op_*.go`
`<Op>Output` struct bodies directly (not just the tool's summary) — agreed
exactly, no discrepancy. The scratch wrapped-type-shape tool was itself
validated against the known scheduler ground truth (`GetSchedule` → `Target`,
`required-in-type=2`) before being trusted for ranking fsx/codebuild.

**Gates**: `go build`, `go vet` (default + `-tags e2e` + `-tags integration`),
`gofmt -l`, `go test -race -count=1`, `golangci-lint run` all green, scoped
to `services/cloudwatch`; 0 banned nolints, 0 new nolints (two `govet` shadow
warnings were fixed by renaming, not suppressed). No exported signature
changed.

## 2026-08-22 — gopherstack-jodk: query/XML Result-wrapper family (DeleteDashboards and 13 siblings)

Found via the same PR #2433 `terraform-tests (2)` CI shard, reproducing
against the real terraform AWS provider: `tofu destroy` on an
`aws_cloudwatch_dashboard` failed with `deserialization failed, failed to
decode response body, DeleteDashboardsResult node not found` against a 200
response. `handler_dashboards.go`'s `DeleteDashboards` emitted
`<DeleteDashboardsResponse>` with no inner `<DeleteDashboardsResult>`.

This corrects r80d batch 33's conclusion that cloudwatch's query/XML path is
dead code because the pinned client (`cloudwatch@v1.66.3`) speaks rpc-v2-cbor
exclusively (`api_client.go:214`). True of the pinned client, false as a
general conclusion: the terraform AWS provider pins its own, older SDK that
still speaks query/XML for cloudwatch, so that path is live. Both the
query/XML and rpc-v2-cbor paths are fixed this pass, not just one.

Root cause, traced through the real generated query-protocol deserializers
still present for other services (`cloudformation@v1.76.1/deserializers.go`):
`NodeDecoder.GetElement(wrapper)` (`smithy-go@1.27.6/encoding/xml/xml_decoder.go:82`)
returns exactly `"<wrapper> node not found"` when the wrapper element is
absent — the literal error reported in CI. Current aws-sdk-go-v2 codegen
skips this lookup entirely for operations whose output shape has zero
members (a "discard body" optimization, confirmed present for both classic
`aws-sdk-go@v1.55.8` and every currently-cached `aws-sdk-go-v2` query-
protocol service), which is why no cached SDK version reproduces the bug
end-to-end here — terraform's own, older pinned SDK predates that
optimization and always calls `GetElement`, so it does.

Swept every cloudwatch query/XML op against botocore's authoritative
`cloudwatch/2010-08-01/service-2.json` (`output`/`resultWrapper` keys — the
model AWS generates every SDK, including the real server, from) to
determine which ops need a `<XResult>` wrapper: any op with a declared
output shape gets one, even with zero members; any op with no output shape
at all correctly has none. Found 14 ops missing the wrapper despite having a
declared output shape — DeleteDashboards (reported), AssociateDatasetKmsKey,
DisassociateDatasetKmsKey, DeleteAnomalyDetector, DeleteMetricStream,
PutInsightRule, StartMetricStreams, StopMetricStreams, StartOTelEnrichment,
StopOTelEnrichment, TagResource, UntagResource — plus two that were also
missing real output data: PutAnomalyDetector (silently dropped
`AnomalyDetectorId`) and PutMetricStream (silently dropped `Arn`), both now
populated and returned. Confirmed correct and left unchanged: every op with
no output shape at all (DeleteAlarms, SetAlarmState, PutMetricData,
DisableAlarmActions, EnableAlarmActions, PutMetricAlarm, PutCompositeAlarm,
PutAlarmMuteRule, DeleteAlarmMuteRule, PutLogAlarm), and every op that
already had a correctly-populated Result (GetDashboard, ListDashboards,
PutDashboard, GetDataset, DescribeAnomalyDetectors, GetAlarmMuteRule,
ListAlarmMuteRules, the DescribeAlarm* family, the Enable/Disable/Delete/
Describe/PutManaged InsightRules family, GetInsightRuleReport,
GetMetricStatistics, ListMetrics, GetMetricData, ListMetricStreams,
GetMetricStream, GetOTelEnrichment, GetMetricWidgetImage,
ListTagsForResource).

Not a regression: every file touched this pass has no commits since this
branch's merge-base with `origin/main`. This is a long-standing wire-shape
gap the pinned rpc-v2-cbor client could never surface — only reachable by a
real query/XML client.

`handler_dashboards_test.go`'s `DeleteDashboards/success` case previously
asserted only `Contains(body, "DeleteDashboardsResponse")`, true of both the
broken and fixed output — corrected to also require the literal
`<DeleteDashboardsResult` substring. New `query_result_wrapper_test.go`
decodes each fixed op's real response through smithy-go's actual
`encoding/xml` `GetElement` lookup — the same primitive the generated
deserializers use — rather than any string match; reproduces the verbatim
reported error against unfixed code for `deletedashboards` and passes
against the fix, across all 14 ops.

**Gates**: `go build`, `go vet`, `gofmt -l`, `go test -race`, `golangci-lint
run` all green, scoped to `services/cloudwatch`; `make build-check` green.
2 `fieldalignment` findings fixed by reordering struct fields (zero-sized
`Result` field was landing last, forcing 8 bytes of trailing padding), not
suppressed. 0 new nolints. No exported signature changed.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404 on
its legacy form-urlencoded (Query/XML) branch, and a second, unrelated silent-empty-input
bug on its rpc-v2-cbor branch**: same survey/rationale as gopherstack-3a8t (see
autoscaling's entry). `RouteMatcher`'s form-urlencoded branch now falls back to
`service.MatchesUserAgentMarker(r.Header, "api/cloudwatch")` (verified against the pinned
`cloudwatch@v1.66.3/api_client.go:719` `AddSDKAgentKeyValue` call) only on the `ReadBody`
failure branch; `ExtractOperation`/`ExtractResource`/`handleFormRequest` migrated off a
confused mix of `r.ParseForm()` and `httputils.ReadBody` (the pre-existing code called
`r.ParseForm()` first and fell back to `httputils.ReadBody` only on error, relying on a
stale "ParseForm is idempotent; RouteMatcher may have already called it" comment -- false:
only the *first* `ParseForm()` call surfaces a read error, a second silently sees a
cached-empty, non-nil `r.PostForm`) onto `httputils.ReadBody`+`url.ParseQuery` consistently.

**The pinned aws-sdk-go-v2 cloudwatch client (v1.66.3) sets `options.Protocol =
rpcv2.NewCBOR(...)` unconditionally** (`api_client.go:214`), so every operation a real Go
SDK client issues uses rpc-v2-cbor, never the legacy form-urlencoded protocol the
RouteMatcher fix above targets -- that branch still matters for the AWS CLI, boto3, or an
older (<1.55) SDK, but no real client available here can drive it, so the corresponding
test (`TestRouteMatcher_OversizedFormBodyRoutesInsteadOf404`) sends a raw form-urlencoded
POST with the real `api/cloudwatch` User-Agent marker rather than going through the SDK
client, still routed through `service.NewRegistry`/`service.NewServiceRouter`.

The CBOR path a real client actually hits (`handleTargetRequest` -> `dispatchCBOR`) has
its own, separate bug: on an oversized body it silently treats `err != nil` the same as
`len(body) == 0` and proceeds with an *empty* input map rather than surfacing the read
failure -- not the 404 this issue tracks (RouteMatcher's CBOR branch matches on
`X-Amz-Target`/path alone, never reading the body), but a legitimate empty-input bug.
**Left unfixed**: out of this issue's scope (it is not a masked-404, and dispatching with
a silently-empty input is a distinct correctness bug worth its own pass); filed as a
follow-up. `handleCBOR` (the direct rpc-v2-cbor path, used when `X-Amz-Target` is absent)
already reads the body itself with its own cap and already returns a typed
`SerializationException` on failure -- confirmed via
`TestHandler_CBOROversizedBodyAlreadyTyped`, unaffected by any change here.

Two test-level `req.ParseForm()`/`r.ParseForm()` pre-drain calls (`handler_test.go`'s
`postForm`, `provider_test.go`'s `cwServer`) broke once `Handler()` stopped relying on
`r.ParseForm()`'s idempotent-on-success caching; removed, same rationale as
cloudformation's entry. `TestCloudWatchHandler_ExtractOperation_ParseFormError` and
`TestCloudWatchHandler_ExtractResource_ParseFormError` tested a malformed *URL* query
string (`r.ParseForm()` merges URL query params with the POST body; `httputils.ReadBody`+
`url.ParseQuery` reads only the body) -- updated to put the malformed encoding in the body
instead, preserving the tests' original intent (a `url.ParseQuery` failure returns empty).

Proof: `TestRouteMatcher_OversizedFormBodyRoutesInsteadOf404` (raw HTTP, see above) confirms
the RouteMatcher fix; `TestHandler_NormalSizedBodyStillRoutes` (real SDK client, rpc-v2-cbor)
is the regression guard. Gates: `go build`, `go vet`, `gofmt -l` (clean), `go test -race
./services/cloudwatch/...` (pass), `golangci-lint run ./services/cloudwatch/...` (0 issues).

**2026-08-22 (gopherstack-vgnp) -- `handleTargetRequest` conflated a body-read failure with
an empty body**: filed by the ifzn sweep as "the CBOR path a real client actually hits";
that framing does not hold up. Verified against the pinned client's actual wire behavior
(`smithy-go@v1.27.6/transport/http/protocol/rpcv2/rpcv2.go:66-68`,
`Protocol.SerializeRequest`): it builds `req.URL.Path =
"/service/GraniteServiceVersion20100801/operation/{op}"` unconditionally and never sets
`X-Amz-Target` -- that header belongs to the distinct `awsjson` protocol family
(`smithy-go@v1.27.6/transport/http/protocol/awsjson/awsjson.go`), which no cached cloudwatch
SDK version (checked v1.55.1 through the pinned v1.66.3) has ever used. `isCBORRequest`
(rpcv2cbor.go) matches that path prefix, so every real client request routes to `handleCBOR`,
never to `handleFormRequest`/`handleTargetRequest` -- confirmed independently by
`sdk_roundtrip_helper_test.go`'s own doc comment ("this round trip exercises rpcv2cbor.go,
not the legacy XML/form handler.go path") and by `TestHandler_CBOROversizedBodyAlreadyTyped`,
which the ifzn entry above already documents as passing, unaffected, against `handleCBOR`.

So `handleTargetRequest` is reachable only by a raw request that sets `X-Amz-Target` while
avoiding the CBOR path prefix -- no real SDK client can drive it, the same shape as the
form-urlencoded branch the ifzn sweep fixed. The bug itself is real regardless: on a
`ReadBody` failure it fell into the same `err != nil || len(body) == 0` branch as a
genuinely empty body and dispatched with a silently-empty input map instead of surfacing the
read failure. Fixed by splitting the `err != nil` case out to return a typed
`SerializationException`/400 (`h.cborError`, matching `handleCBOR`'s own convention),
leaving `len(body) == 0` to dispatch with an empty map as before.

Proof: `TestHandleTargetRequest_DistinguishesReadFailureFromEmptyBody` (raw HTTP with
`X-Amz-Target`, since no real client reaches this branch) has two subtests -- an oversized
body now gets `SerializationException`/400 (fails pre-fix: got 200 with the exception-type
header empty), and a genuinely empty body still gets a normal 200 dispatch, proving the fix
distinguishes the two rather than just rejecting everything.
`TestHandleTargetRequest_ValidCBORBodyStillDispatches` is the regression guard for the
success path. Gates: `go build`, `go vet`, `gofmt -l` (clean), `go test -race
./services/cloudwatch/...` (pass), `golangci-lint run ./services/cloudwatch/...` (0 issues,
0 new nolints). No exported signature changed.

## 2026-08-29 -- exhaustive indexed-list/filter-key request-parameter sweep

**Protocol confirmed first, per the campaign's explicit warning for this
service:** `cloudwatch@v1.66.3`'s pinned SDK client sets
`options.Protocol = rpcv2.NewCBOR(...)` (`api_client.go:214`) and has no
`serializers.go` at all -- request/response field mapping instead comes from
the generated `schemas` package (`AddMember("FieldName", ...)` calls) plus
each type's own `Serialize`/`SerializeMembers` methods. `handler.go`
confirms real dispatch: `isCBORRequest(r)` routes to `handleCBOR`/
`dispatchCBOR` (`handler.go:243-254,344-346`); the form-encoded
`vals url.Values` handlers (`handler_alarms.go`, `handler_metrics.go`, etc.)
are the classic Query/XML path, reachable only by a hand-built legacy
request, never by a real `aws-sdk-go-v2` client at this pinned version. **All
verification effort this pass went into the CBOR path**, since that's the
only one both live and modeled by the pinned SDK.

**Every list-cardinality read on the CBOR path checked against its
operation's real Go input struct + `schemas.go` member name, 0 bugs found.**
~35 call sites across `cborStrList` (8 distinct fields --
`AlarmNames`/`AlarmActions`/`OKActions`/`InsufficientDataActions`/
`AlarmTypes`/`DashboardNames`/`LogGroupIdentifiers`/`MetricNames`/
`AdditionalStatistics`/`Statistics`/`ExtendedStatistics`/`Statuses`/`Names`,
each op's own struct checked rather than inferred from a sibling),
`cborDimensions` (7 sites, always the literal `"Dimensions"` key, matching
`types.Dimension.{Name,Value}`), `cborFloatList` (`MetricDatum.Values`/
`Counts`), `parseMetricDataQueries` (`GetMetricData.MetricDataQueries` plus
the nested `MetricStat.{Stat,Period,Metric}`/`Metric.{Namespace,MetricName,
Dimensions}` chain), `cborMetricStreamFilters`/
`cborMetricStreamStatisticsConfigurations` (`PutMetricStream`'s nested
`IncludeFilters`/`ExcludeFilters`/`StatisticsConfigurations`, down to
`IncludeMetrics[].{MetricName,Namespace}`), `cborMuteTargetAlarmNames`
(`MuteTargets.AlarmNames`, confirmed against `schemas.MuteTargets_AlarmNames`),
plain `Tags`/`TagKeys` on the three tag ops, `MetricData` on `PutMetricData`,
and `ManagedRules` on `PutManagedInsightRules`. Every key matched exactly;
no cardinality mistakes (no scalar-getter used on a list field or vice
versa) found anywhere in this set.

**FIXED 2026-08-30 (gopherstack-p1ph):** `PutMetricAlarmInput.Metrics
[]types.MetricDataQuery` (metric-math alarms) is a real, modeled field that
`cborPutMetricAlarm` never read at all -- while the **dead** legacy XML
`handlePutMetricAlarm` (`handler_alarms.go:51`) parsed it via
`parseMetricDataQueriesFromForm`, so the unreachable path had strictly more
feature coverage than the one real clients hit. Confirmed from the pinned
SDK schema that `PutMetricAlarmInput`'s `"Metrics"` member and
`GetMetricDataInput`'s `"MetricDataQueries"` member both point at the same
`_MetricDataQueries` shape (schemas.go:4205,4487), so `parseMetricDataQueries`
(previously hardcoded to the `"MetricDataQueries"` key) was generalized to
take the key as a parameter and is now called with `"Metrics"` from
`cborPutMetricAlarm` too. The read side (`buildMetricAlarmCBOR`, shared by
`DescribeAlarms` and `DescribeAlarmsForMetric`) gained a new
`buildMetricDataQueriesCBOR` so a write-then-read round trip through a real
`aws-sdk-go-v2` client preserves the full nested structure (`Id`,
`Expression`, `Label`, `AccountId`, `ReturnData`, and `MetricStat.{Metric.
{Namespace,MetricName,Dimensions},Period,Stat}`). Proven by
`metric_math_alarm_p1ph_test.go`'s `TestPutMetricAlarm_Metrics_RealClient_RoundTrip`,
which failed against unmodified code (0 metrics came back) before the fix.
Still unread on the CBOR path, deliberately left alone: `MetricStat.Unit` --
this repo's own `MetricStat` model (`models.go`) has no `Unit` field at all,
a gap that predates this fix and matches the legacy XML parser's identical
omission, so adding it would be a new feature, not this bug's fix.

**Dead legacy XML/Query path, spot-checked but not exhaustively
cross-referenced:** the pinned SDK has no serializer for this protocol at
all (it's not what `aws-sdk-go-v2` cloudwatch@v1.55+ ever sends), so there
is no "real SDK" to check these call sites against the way the campaign
otherwise requires. Read `parseMemberList`/`parseCWTagsFromForm`/
`parseCWTagKeysFromForm`/`parseDimensionsFromForm` (the direct form-path
analogs of the CBOR helpers above) and confirmed they consistently iterate
every `.member.N` entry (no shape-2 truncation-at-first-element bug), but
did not verify their key spelling against anything authoritative, since
nothing authoritative for this protocol is pinned in this repo. Given they
are unreachable by any real typed client, this is deliberately left
unresolved rather than guessed at.

**Coverage: N-of-N for the CBOR path (~35 of 35 list-cardinality sites);
the dead XML path was read for the same shape-2 pattern but explicitly not
graded pass/fail against a serializer that doesn't exist for it.** No code
changes in this service this pass -- the live path enumeration found
nothing to fix, which is itself informative given how much of this bug
class showed up when the same method was pointed at ec2's Query/XML surface.

## 2026-08-29 (pagination-arithmetic sweep, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

Census: `contributors.go`/`interfaces.go`/`alarm_mute_rules.go`/`alarm_history.go`/
`metrics.go`/`metric_streams.go`/`anomaly_detectors.go`/`insight_rules.go`/
`dashboards.go` all call `pkgs/page.New` directly — clamped offset tokens, no
independent arithmetic. Two genuinely hand-rolled cursors exist: `paginateAlarmResults`
(`alarms.go`, `DescribeAlarms`'s combined metric/composite/log-alarm page window) clamps
its offset via `min(page.DecodeToken(nextToken), combinedTotal)` before ever indexing —
safe against Class A, and being purely positional (not identity-matched) it cannot
express Class B/C either. `paginateMetricData`/`decodeMetricDataToken` (`metricdata.go`,
`GetMetricData`'s datapoint-budget pagination) decodes a `{ResultIndex, PointOffset}`
cursor, clamps both to `>= 0`, and its consuming loop (`for i := cursor.ResultIndex;
i < len(all); i++`) degrades to an empty, cursor-less result when `ResultIndex` is
past the end rather than panicking or looping. `pagination.go`'s
`signPageToken`/`parseSignedPageToken` are unused by any current op (grep-confirmed) —
dead code, not a live bug surface. Verdict: correct, no bug found.

Added `pagination_arithmetic_test.go`: a real `aws-sdk-go-v2` typed-client boundary
walk over `DescribeAlarms` (N=7 metric alarms, page=3 via `MaxRecords`,
`assert.ElementsMatch` against the full set) — the one hand-rolled cursor in this
service without pre-existing typed-client-level pagination coverage.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` — all clean
(`./services/cloudwatch/...`).

## 2026-08-30 -- gopherstack-uox6: value-semantics filter audit (iam/cloudwatch/resourcegroupstaggingapi pass)

Audited filter/matcher VALUE SEMANTICS (not shape) across alarms, alarm history,
condition-style matchers, metric-stream filters, and the alarm-evaluation threshold
operators, per bd gopherstack-uox6's "a parameter that is read, applied, and wrong"
class. One bug found and fixed; several matchers verified member-by-member and left
alone.

**Bug: `DescribeAlarmHistory`'s `AlarmTypes` filter was dead on the only reachable
wire.** `cborDescribeAlarmHistory` (`rpcv2cbor_alarm_history.go`) read a singular
`"AlarmType"` CBOR key that no real client ever sends -- `aws-sdk-go-v2` serializes
`DescribeAlarmHistoryInput.AlarmTypes` (a list) under the key `"AlarmTypes"`
(`cloudwatch@v1.66.3/api_op_DescribeAlarmHistory.go:53,92`). The backend's own
`DescribeAlarmHistory(alarmName, alarmType string, ...)` then treated the
permanently-empty result as "match every type", so the operation's documented default
("If you omit this parameter, only metric alarms are returned") was inverted:
composite/log alarm history always leaked into an unfiltered call, and any explicit
`AlarmTypes` filter a real client sent was silently ignored. Same shape as
`DescribeAlarms`' AlarmTypes default bug (gopherstack-yvb7), now found in its sibling
operation. Fixed: backend signature is now `alarmTypes []string`, matching
`DescribeAlarms`' `toSet`/`includeMetric := len(typeSet) == 0 || typeSet["MetricAlarm"]`
pattern; the live CBOR handler now reads `cborStrList(input, "AlarmTypes")`; the dead
legacy XML handler was updated for consistency to `parseMemberList(form,
"AlarmTypes.")`. Test: `alarm_history_alarmtypes_realclient_test.go`, a real
`aws-sdk-go-v2` client round trip asserting both directions (composite alarm absent
from an unfiltered call; metric alarm absent from an explicit `AlarmTypes:
[CompositeAlarm]` call) -- confirmed failing against the unmodified code first.

**Verified correct, left alone:** `alarm_eval.go`'s `breachesThreshold` -- all seven
`ComparisonOperator` enum members (incl. the three anomaly-detection operators)
checked member-by-member against `types/enums.go`; strict vs. or-equal-to boundaries
match each operator's own name exactly. `GetMetricStatistics`/`GetMetricData`'s bucket
window (`metrics.go:400`, `populateBuckets`) -- confirmed StartTime inclusive / EndTime
exclusive against the SDK doc comment's explicit "The value specified is
inclusive"/"exclusive" wording. `PutMetricData`'s Timestamp acceptance window
(two-weeks-past / two-hours-future, `validMetricTimestamp`) -- inclusive both ends,
matching the AWS API page's "as much as" wording. `metric_streams.go`'s
`streamAllowsMetric`/`filterIncludesMetric`/`filterExcludesMetric` -- OR-across-filters,
OR-across-MetricNames, empty-MetricNames-means-whole-namespace, and the
IncludeFilters/ExcludeFilters mutual-exclusivity validation, all correct.
`ListMetrics`' `RecentlyActive=PT3H` window -- inclusive boundary, consistent with
every other recency check in `metrics.go`.

**Gap, not implemented:** `DescribeAlarmsForMetric`'s own doc says "To filter the
results, specify a statistic, period, or unit" but the backend never reads
Statistic/Period/ExtendedStatistic/Unit at all (shape gap, not a wrong-semantics bug --
out of this pass's scope). Its `Dimensions` matcher (`dimsContainAll`) does a
subset/superset match; the SDK doc comment ("If the metric has any associated
dimensions, you must specify them in order for the call to succeed") is genuinely
ambiguous about whether an exact dimension-set match is required -- left as documented
existing behaviour rather than guessed at. `DescribeAlarmHistory`'s `AlarmContributorId`
and `ScanBy` parameters are also unread (shape gaps, not fixed this pass).

`iam` and `resourcegroupstaggingapi` matchers audited this pass (IAM condition
operators in `conditions.go`, `PathPrefix`/`OnlyAttached`/`PolicyUsageFilter` in
`handler_list_filters.go`, `resourcegroupstaggingapi`'s `TagFilters`/
`ResourceTypeFilters` AND/OR combining in `get_resources.go`) came back clean --
see those services' own PARITY.md entries.

Gates: `go build`, `go vet`, `go test -race -count=1` all clean on
`./services/cloudwatch/...`; repo-wide `go build ./...`/`go vet ./...` clean (no
cross-service callers of the changed `DescribeAlarmHistory` signature).

## 2026-09-08 -- gopherstack-cqzt: partial audit of the previously-unreached surface

Continuation of the 2026-09-04 pass that exhausted budget before reaching this
list: `GetMetricData`/`GetMetricStatistics` and `DescribeInsightRules`/
`DescribeAnomalyDetectors`/`ListDashboards`/`ListMetricStreams` input handling;
delete/update preconditions and ghost rows after delete; the alarm state
machine (`TreatMissingData`/`DatapointsToAlarm`); and resource leaks (alarm
history/metric datapoint growth, evaluation and metric-stream tickers).

**Covered, found clean:**
- **Resource leaks.** `janitor.go`'s two tickers (`worker.NewGroup` +
  `g.Ticker(...)`, `<-ctx.Done(); g.Stop()`) are ctx-cancellation-aware and
  match the repo's janitor convention. `SweepExpiredMetrics` (`metrics.go:328`)
  time-bounds metric datapoint growth (15-day retention,
  `cwMetricRetentionDays`) via a snapshot-then-apply two-phase sweep.
  `appendHistory` (`alarm_history.go:108`) caps per-alarm history at
  `cwMaxAlarmHistory`, trimming the oldest entries. `DeleteAlarms` already
  deletes the per-alarm history map entry (`alarm_history` cannot outlive its
  alarm). No metric-stream delivery ticker exists (delivery is synchronous,
  inline in `PutMetricData`) so there is nothing to leak there. No new
  goroutines/tickers found outside `janitor.go`.
- **The nil-on-write fall-through defect class** (the elasticache/pinpoint/
  apigatewayv2/etc. shape where a local error-response-writer returns nil on
  success and a caller's `if err != nil` after storing that return value never
  fires). cloudwatch's two writer helpers, `xmlError` (`handler.go:638`) and
  `cborError` (`rpcv2cbor.go:273`), both do exist and both do return `nil`
  unconditionally after writing. Checked **every** call site
  (`grep -n "xmlError(\|cborError("` across all non-test `.go` files, ~140
  matches): every single one is `return h.xmlError(...)` / `return
  h.cborError(...)` directly, with the sole non-`return` exception being
  `handler.go:387`'s `return true, h.cborError(...)` (a two-value return, same
  direct-propagation shape). No call site stores the result in a variable and
  branches on it. **This defect class does not exist in cloudwatch.**
- **The alarm state machine.** `evaluateMetricAlarmState`
  (`alarm_eval.go:132`) and `countBreachingPeriods` implement all four
  `TreatMissingData` modes; `missing` (default) returns `INSUFFICIENT_DATA`
  when there aren't enough real datapoints to reach `DatapointsToAlarm`,
  `breaching`/`notBreaching` count/exclude synthetic breaches per AWS's
  documented meaning, and `ignore` maintains the current state only when
  *zero* real data exists in the window (matching "the current alarm state is
  maintained" when there is nothing to evaluate). `DatapointsToAlarm`
  defaults to `EvaluationPeriods` when unset (M=N alarm) and `PutMetricAlarm`
  already validates `DatapointsToAlarm <= EvaluationPeriods`
  (`alarms.go:27`). Composite-alarm cycle handling
  (`evalCompositeRuleDepth`) treats a cycle as `INSUFFICIENT_DATA` rather than
  infinite-looping. Not independently re-verified against a fresh external
  source beyond what a prior pass already confirmed (see the
  "alarm-evaluation-state-machine" family note above) — no new bug found here.

**Fixed, each with a failing-first regression test (see per-op PARITY notes
above for detail):**
1. `DeleteMetricStream` (XML path) cleaned up tags under a hardcoded,
   region/account-less ARN (`"arn:aws:cloudwatch::metric-stream/"+name`) that
   never matched the real ARN (`arn.Build("cloudwatch", region, accountID,
   ...)`) `PutMetricStream` actually returns to callers — tags survived
   delete as a ghost row. The CBOR path did no cleanup at all. Both now fetch
   the stream for its real `.Arn` before deleting.
2. `DeleteInsightRules` (both XML and CBOR) never cleaned up tags at all,
   unlike its three taggable-resource siblings (alarms, dashboards, metric
   streams). Fixed with a new `insightRuleARNs` helper.

Both fixes strengthen a **pre-existing but non-assertive test**:
`TestHandler_DeleteMetricStream_CleansUpTags` previously only checked the
PUT/DELETE HTTP status codes and never called `TagResource`/
`ListTagsForResource` despite its name claiming to test tag cleanup — it could
not have caught either bug. It now tags the stream by its real ARN, confirms
the tag is visible, deletes, and confirms the tag is gone. A companion test,
`TestHandler_DeleteInsightRules_CleansUpTags`, was added new (no prior test
covered insight-rule tag cleanup at all). Both fixes were confirmed to fail
against unmodified code first (exact output in the bd issue / session
transcript), then pass after the fix; both re-run 10x under `-race` with no
flakes (fresh handler+backend per test, no shared/global state touched).

**Found, NOT fixed (see `DescribeAnomalyDetectors` row above for detail):**
`AnomalyDetectorIds`/`AnomalyDetectorTypes` are real modeled
`DescribeAnomalyDetectorsInput` filters that both the XML and CBOR handlers
silently ignore — every unfiltered detector is returned regardless of what a
caller filters for. Not fixed: `DescribeAnomalyDetectors`'s backend method has
a positional signature with roughly 10 call sites across this package's tests
(two of them real `aws-sdk-go-v2` client round trips), and threading a new
filter through all of them was judged too invasive for this pass's remaining
budget on a P3, read-op (lowest-priority-bucket) finding. Left as a flagged
gap for a dedicated pass rather than a rushed signature change.

**NOT reached this pass** (still open from the original gopherstack-cqzt
scope): `GetMetricData`/`GetMetricStatistics` input-handling validation beyond
what earlier passes already covered (see "Verified correct, left alone"
above); `DescribeInsightRules`/`ListDashboards`/`ListMetricStreams` input
handling; the "at most one composite alarm per `DeleteAlarms` call" and
"100-alarm-per-call" limits documented on `DeleteAlarms` (currently
unenforced — gopherstack always deletes everything requested); the documented
"cannot delete a composite alarm that is part of a dependency cycle"
precondition (currently unenforced; cycles are only handled at *evaluation*
time, not at delete time). None of these were touched code-wise this pass.

Gates for this pass: `GOTOOLCHAIN=go1.27.0 golangci-lint run
./services/cloudwatch/...` — 0 issues. `GOTOOLCHAIN=go1.27.0 go test -race
./services/cloudwatch/...` — all pass. No persisted-type fields were added, so
`pkgs/persistence` was not touched and its golden was not run.
