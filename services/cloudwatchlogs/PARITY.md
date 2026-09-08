---
service: cloudwatchlogs
sdk_module: aws-sdk-go-v2/service/cloudwatchlogs@v1.86.0
last_audit_commit: 3884816a
last_audit_date: 2026-08-13
overall: A            # 2026-08-13 (gopherstack-wl0s): GetLogFields never read dataSourceType
                       # from the request body at all (not even a field on the decode struct),
                       # so it was silently unused rather than required (validateOpGetLogFieldsInput
                       # marks it required). Fixed with a presence check. ListAggregateLogGroupSummaries
                       # discarded its whole request body (`_ []byte` handler param), so groupBy
                       # (also required) was silently unused; fixed with presence + enum validation
                       # against types.ListAggregateLogGroupSummariesGroupBy.Values(). Reading the
                       # full operation also caught a materially worse bug sitting beside the
                       # reported one: the response was wrapped as "logGroupSummaries", a key the
                       # real ListAggregateLogGroupSummariesOutput shape does not have at all
                       # (confirmed against awsAwsjson11_deserializeOpDocumentListAggregateLogGroupSummariesOutput,
                       # which only recognizes "aggregateLogGroupSummaries") -- so populated summaries
                       # never actually round-tripped to a real SDK client before this fix, regardless
                       # of groupBy. Both ops previously had no ValidationException sentinel in this
                       # service (errors.go only had ErrValidation -> InvalidParameterException, which
                       # is what most other ops' declared error sets use); ListAggregateLogGroupSummaries'
                       # own awsAwsjson11_deserializeOpErrorListAggregateLogGroupSummaries switch
                       # declares ValidationException instead, so a new ErrValidationException sentinel
                       # was added rather than reusing ErrValidation. See ops entries below.
ops:
  CreateLookupTable: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (2026-08-30, exhaustive field sweep, gopherstack-wksweep-cwl): CreateLookupTable took no ctx/region parameter at all and built its ARN via lookupTableARN(b.region, name) -- b.region is this InMemoryBackend instance's constant default-region config field, never the per-request region every other resource in this package derives from ctx (getRegion(ctx, b.region), used consistently by log groups/streams/subscription filters/metric filters/log events/syslog configurations -- confirmed by reading each). Two regions creating a same-named lookup table therefore collided on one storage key (LookupTable is keyed by ARN, lookupTableKeyFn): the second create failed with ResourceAlreadyExistsException even though it addressed a distinct regional resource in real AWS -- a storage key missing the region dimension its own resource is scoped by, the exact class already seen elsewhere in this campaign (RDS/other services' version-omitted keys silently overwriting). Fixed: CreateLookupTable now takes ctx, derives region := getRegion(ctx, b.region), and builds the ARN from that. Proven via TestCloudWatchLogsRegionIsolation_LookupTable (isolation_test.go, mirrors the pre-existing TestCloudWatchLogsRegionIsolation for LogGroup), hand-reverted (lookupTableARN(b.region, name) again) and confirmed to fail against unfixed code with the exact ResourceAlreadyExistsException collision. parity-4: field-diffed against aws-sdk-go-v2@v1.80.0 api_op_CreateLookupTable.go/types.LookupTable. CreateLookupTableInput.TableBody is a plain *string of CSV content (verified against serializers.go: tableBody is serialized as a bare JSON string, no S3 reference anywhere in this op's input or output) -- so this backend genuinely parses the CSV (encoding/csv) rather than modeling a reference to data it never reads: the header row becomes TableFields, subsequent rows are counted into RecordsCount, and len(tableBody) becomes SizeBytes. Name validated against the documented alphanumeric+underscore/256-char charset; body validated against the documented 10 MB limit and real CSV syntax (malformed CSV -> InvalidParameterException). ARN is constructed as arn:{partition}:logs:{region}:{account}:lookup-table:{name} via pkgs/arn -- no ARN pattern is embedded anywhere in the SDK module (no smithy model shipped, no doc-comment pattern), so this mirrors the existing log-group ARN convention (arn.Build + \"log-group:\"+name) rather than an AWS-confirmed pattern; flagged here for anyone who later finds an authoritative pattern to check against. Response is create-only (createdAt/lookupTableArn), matching CreateLookupTableOutput exactly (no echoed metadata). Tags are accepted and stored via the handler-level tag store (h.setTags, keyed by lookupTableArn) exactly like log group tags, since types.LookupTable/GetLookupTableOutput have no Tags field of their own -- tags are wire-visible only via the generic ListTagsForResource/TagResource/UntagResource ops, which already existed. FIXED (gopherstack-enpq, 2026-08-22): CreateLookupTableInput.QueryId (api_op_CreateLookupTable.go:55, \"You must specify either tableBody or queryId, but not both\") had no Go field at all -- the doc-prescribed query-results-populate-the-table path was structurally unreachable; a caller supplying only QueryId always fell through to \"tableBody is required\" even though validateOpCreateLookupTableInput does not require either field client-side, so the request reaches the wire unmodified. Fixed via resolveLookupTableBody/lookupTableBodyFromQuery: QueryId now fetches the completed query's [][]ResultField and renders real CSV content (header from the first result row's field order, one row per result), with both-set and neither-set both rejected as InvalidParameterException. Proven via TestCreateLookupTable_FromQueryID and TestCreateLookupTable_TableBodyAndQueryIDMutualExclusion (real aws-sdk-go-v2 client), hand-reverted and confirmed to fail against unfixed code."}
  GetLookupTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-4: full-content shape (description/kmsKeyId/lastUpdatedTime/lookupTableArn/lookupTableName/sizeBytes/tableBody) field-diffed against GetLookupTableOutput; unlike DescribeLookupTables this includes tableBody."}
  UpdateLookupTable: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "parity-4: full replacement of TableBody (re-parsed, TableFields/RecordsCount/SizeBytes recomputed) per the doc comment (\"This is a full replacement operation\"); Description/KmsKeyId are optional *string on the real input (nil = leave unchanged), modeled the same way here rather than collapsing to plain strings (which would make \"omitted\" and \"explicitly cleared\" indistinguishable over JSON). FIXED (gopherstack-enpq, 2026-08-22): same missing-QueryId bug as CreateLookupTable (api_op_UpdateLookupTable.go:47, same doc-prescribed either/or), same resolveLookupTableBody fix."}
  DeleteLookupTable: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeLookupTables: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (2026-08-30, exhaustive field sweep): same region-scoping bug as CreateLookupTable -- DescribeLookupTables took no ctx/region and walked every stored LookupTable regardless of region, so a caller in one region saw every other region's lookup tables too. Now takes ctx, derives region, and filters via a new lookupTableARNRegion helper (pkgs/arn has Build but no parser, so this is a local, minimal region-segment extractor, not a general ARN parser) comparing each stored table's ARN region segment against the request's. Covered by the same TestCloudWatchLogsRegionIsolation_LookupTable. parity-4: field-diffed against types.LookupTable -- this list shape deliberately excludes tableBody (metadata only: description/kmsKeyId/lastUpdatedTime/lookupTableArn/lookupTableName/recordsCount/sizeBytes/tableFields), matching the real SDK type used by DescribeLookupTablesOutput.LookupTables (distinct from GetLookupTableOutput's full-content shape). lookupTableNamePrefix filter and maxResults(default 50/max 100 per the doc comment)/nextToken pagination implemented via the same base64-index-cursor helpers (encodeNextToken/parseNextToken) every other paginated op in this package already uses."}
  PutSyslogConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-4: field-diffed against api_op_PutSyslogConfiguration.go/types.SyslogConfiguration. Real AWS PutSyslogConfigurationInput/DeleteSyslogConfigurationInput both require only LogGroupIdentifier (VpcEndpointId is optional on both, per the real validator -- validateOpPutSyslogConfigurationInput only requires LogGroupIdentifier), so this backend models at most one syslog configuration per log group, keyed by normalized log group identifier -- the same per-log-group-identifier keying this codebase already uses for IndexPolicy/Transformer (store_setup.go's indexPolicyKeyFn/transformerKeyFn). Improvement over those pre-existing sibling ops: this validates the log group actually exists (region-scoped groupGet lookup) and returns ResourceNotFoundException otherwise, rather than accepting an arbitrary string as those two do -- a deliberate, real behavior difference specifically called for this pass, not a pre-existing gap being silently carried forward. SourceType is always \"VPCE\" (the only real types.SyslogSourceType enum member). VpcEndpointId itself is accepted/stored/returned as an opaque string, not cross-validated against real EC2 VPC-endpoint state -- there is no VPC-endpoint modeling anywhere in this service (or a cross-service validation pattern anywhere in this codebase for ARN/ID references into another service's resources), matching how this service already treats KmsKeyId/RoleArn/DestinationArn (stored, not validated)."}
  ListSyslogConfigurations: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-4: field-diffed against types.SyslogConfiguration (createdAt/logGroupArn/sourceType/vpcEndpointId). Optional logGroupIdentifier/vpcEndpointId filters plus nextToken/maxResults pagination implemented."}
  DeleteSyslogConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-4: optional vpcEndpointId scoping parameter -- when supplied it must match the stored configuration's VPC endpoint or the delete is treated as not-found, matching the real input accepting both LogGroupIdentifier(required)+VpcEndpointId(optional) as a compound identify-then-delete key."}
  GetStorageTierPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-4: field-diffed against api_op_GetStorageTierPolicy.go -- GetStorageTierPolicyInput has ZERO fields (verified: an empty struct, no LogGroupIdentifier or any other member), confirming this is an account-level singleton, not per-log-group as might be assumed from this service's existing per-log-group LogGroupClass modeling. Before any PutStorageTierPolicy call, real AWS's default active tier is STANDARD with LastUpdatedTime absent (GetStorageTierPolicyOutput.LastUpdatedTime is a nilable *int64) -- this backend synthesizes exactly that default (StorageTier=STANDARD, LastUpdatedTime omitted from the wire response) rather than requiring a Put first or fabricating a timestamp."}
  PutStorageTierPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "parity-4: field-diffed against api_op_PutStorageTierPolicy.go/types.StorageTier -- PutStorageTierPolicyInput carries only StorageTier (no LogGroupIdentifier), confirming the account-level scope. StorageTier validated against the real 2-member enum (STANDARD/INTELLIGENT_TIERING); invalid values rejected with InvalidParameterException matching the real required-field validator (validateOpPutStorageTierPolicyInput)."}
  PutLogEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: added the two documented batch-shape constraints that were previously unenforced -- (1) a batch whose events are not in non-decreasing timestamp order now fails the whole request (InvalidParameterException) via validateChronologicalOrder, matching \"A batch of log events in a single request must be in a chronological order. Otherwise, the operation fails.\"; (2) a batch whose *valid* (post too-old/too-new/expired classification) events span more than 24 hours now fails the whole request via validateEventSpan, matching \"the time span in a single batch cannot exceed 24 hours.\" Both bypass synthetic (pre-2001-epoch) timestamps for fixture-friendliness, consistent with classifyLogEvents' existing bypass. Two existing tests (TestJanitor_SweepRetention, TestJanitor_SweepUpdatesStreamMetadata) sent a single batch spanning ~10 days, which real AWS would reject outright; split into two individually-valid PutLogEvents calls. Sequence-token/RejectedLogEventsInfo fixes from the prior pass unchanged."}
  GetLogEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "startFromHead/nextToken precedence and stable-at-boundary forward/backward tokens verified correct."}
  FilterLogEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "cross-stream interleave + stable timestamp sort verified; logStreamNames/logStreamNamePrefix mutual-exclusion validated; searchedLogStreams correctly always empty (AWS deprecated this field)."}
  GetLogGroupFields: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed: previously a disguised stub always returning the 4 static built-in fields at 100% regardless of actual log content, and didn't accept logGroupIdentifier or time at all. Now does real percentage-based sampling: logGroupIdentifier is accepted (via normalizeLogGroupIdentifier) alongside logGroupName; time (epoch *seconds*, unlike almost every other timestamp field in this API) centers an 8-minute-either-side window per the doc comment, or defaults to the most recent 15 minutes; every stored event in-window is sampled, the 4 built-in fields plus any JSON top-level keys (via the existing jsonMessageFields helper) are counted, and Percent is computed per-field over the sampled count, sorted descending. Zero sampled events now correctly returns an empty list rather than fabricating 100%-present built-in fields. Synthetic (pre-2001) event timestamps bypass the window, matching this file's existing test-fixture convention."}
  GetLogFields: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-wl0s): dataSourceName presence was already checked (via the LogGroupName/LogGroupIdentifier/DataSourceName fallback chain), but dataSourceType -- also required per validateOpGetLogFieldsInput -- was not even a field on the decode struct, so it was silently ignored regardless of presence. Now required-present (InvalidParameterException otherwise, matching this op's own awsAwsjson11_deserializeOpErrorGetLogFields switch). DataSourceType has no types.X enum on the real SDK (declared *string, not an enum type), so this is a presence check only, not enum-validated."}
  CreateLogGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteLogGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades streams/events/subscription filters/metric filters."}
  DescribeLogGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAggregateLogGroupSummaries: {wire: fixed, errors: fixed, state: ok, persist: n/a, note: "two bugs fixed together (gopherstack-wl0s), both in handleListAggregateLogGroupSummaries: (1) the handler discarded its whole request body (`_ []byte` param), so groupBy -- required per validateOpListAggregateLogGroupSummariesInput -- was silently unused; now required-present and enum-validated against types.ListAggregateLogGroupSummariesGroupBy.Values() (DATA_SOURCE_NAME_TYPE_AND_FORMAT, DATA_SOURCE_NAME_AND_TYPE), rejecting with ValidationException (this op's own declared client-error code, confirmed against awsAwsjson11_deserializeOpErrorListAggregateLogGroupSummaries -- distinct from the InvalidParameterException most other ops in this service use). (2) MATERIALLY WORSE, found while reading the whole operation: the response was wrapped under \"logGroupSummaries\", a key the real ListAggregateLogGroupSummariesOutput shape does not have at all -- confirmed against awsAwsjson11_deserializeOpDocumentListAggregateLogGroupSummariesOutput, which only recognizes \"aggregateLogGroupSummaries\". A real SDK client's AggregateLogGroupSummaries field was always nil/empty regardless of what this backend returned, for every caller, unconditionally -- not a validation gap but a total wire-shape break. Fixed to emit \"aggregateLogGroupSummaries\". FIXED, THIRD BUG (gopherstack-enpq, 2026-08-22): even after the wrapper-key fix, the array ELEMENT shape itself was still completely fabricated -- the previous AggregateLogGroupSummary modeled per-log-group fields (logGroupName/logGroupArn/logGroupClass/storedBytes/logEventCount) that do not exist anywhere on the real types.AggregateLogGroupSummary at all (confirmed against awsAwsjson11_deserializeDocumentAggregateLogGroupSummary, which only recognizes \"groupingIdentifiers\"/\"logGroupCount\"). types.AggregateLogGroupSummary is a GROUPED BUCKET (one entry per distinct data-source characteristic under the requested groupBy), not a per-log-group record, so every prior response returned N entries (one per log group) each with GroupingIdentifiers/LogGroupCount permanently nil/empty for a real client -- a total, previously-undetected wire-shape break that survived two prior structfielddiff passes on this exact op. This backend has no per-log-group data-source classification (dataSource.Name/Type/Format) to group by, so the honest fix returns a single bucket covering all log groups in the region (GroupingIdentifiers empty -- not fabricated, LogGroupCount real), or an empty list when there are zero log groups. Proven via TestListAggregateLogGroupSummaries_RealShape (real aws-sdk-go-v2 client), hand-reverted and confirmed to fail against unfixed code (2 raw per-log-group entries, both with LogGroupCount nil). Grouping by data-source characteristic itself remains unimplemented -- disclosed, not fabricated; see gaps."}
  CreateLogStream: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteLogStream: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeLogStreams: {wire: ok, errors: ok, state: ok, persist: ok, note: "orderBy=LastEventTime + prefix and descending + orderBy=LogStreamName rejection rules match AWS."}
  PutLogGroupDeletionProtection: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-7rq1): request member was `deletionProtected`, a gopherstack-invented name -- the real PutLogGroupDeletionProtectionRequest member (logs/2014-03-28/service-2.json) is `deletionProtectionEnabled`. A real client's flag was silently dropped by json.Unmarshal (wrong key = zero value = false), so every real PutLogGroupDeletionProtection call silently disabled protection regardless of the caller's intent. Fixed the json tag; both existing tests (which asserted only HTTP 200, not backend state, and used the wrong key) and a new state-asserting test cover it."}
  PutRetentionPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRetentionPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutSubscriptionFilter: {wire: ok, errors: ok, state: ok, persist: ok, note: "2-filter-per-group cap and update-in-place-by-name verified."}
  DescribeSubscriptionFilters: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteSubscriptionFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  PutMetricFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeMetricFilters: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-uox6 (2026-08-30): FilterNamePrefix's own doc comment says CloudWatch Logs applies it only when logGroupName is also given; this backend applied it unconditionally, so filterNamePrefix-without-logGroupName wrongly narrowed a global listing instead of being a no-op. Fixed by clearing the effective prefix when logGroupName is empty."}
  DeleteMetricFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  TestMetricFilter: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed: ExtractedValues was always {} (disguised stub -- computed nothing from the pattern's named fields). Now extracts every $-referenced field for JSON and space-delimited patterns."}
  ListTagsLogGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagLogGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagLogGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateExportTask: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeExportTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: types.ExportTask nests Status as {code, message} (types.ExportTaskStatus) and Creation/CompletionTime under executionInfo (types.ExportTaskExecutionInfo); this handler previously serialized the internal flat ExportTask model directly onto the wire (status as a bare string, creationTime/completionTime/statusMessage as flat top-level keys), which a real SDK client's generated deserializer would silently read as nil for all four fields. Added toWireExportTask/wireExportTask (handler_export_tasks.go) to map correctly; the internal flat model is unchanged (still used for backend state/persistence)."}
  CancelExportTask: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateImportTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: initial status was the shared completenessStatusActive constant (\"ACTIVE\"), which is correct for IntegrationStatus but is not a member of types.ImportStatus (IN_PROGRESS/CANCELLED/COMPLETED/FAILED) at all. Now uses a dedicated importStatusInProgress=\"IN_PROGRESS\" constant."}
  DescribeImportTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: the ImportTask model serialized its status field as \"status\", but the real wire key (types.Import.ImportStatus) is \"importStatus\" -- a real SDK client's ImportStatus field would always deserialize empty/unrecognized. Also excluded ImportRoleArn from the wire (json:\"-\"): it is accepted on CreateImportTask for this backend's own bookkeeping but is not a field on the real Import describe/list type at all."}
  CancelImportTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: accepted-for-cancellation check compared against \"ACTIVE\" (see CreateImportTask note) instead of \"IN_PROGRESS\"; a real import task (always created with status IN_PROGRESS) could therefore never actually be cancelled through this backend before this fix. Output wire shape (importId/importStatus/creationTime/lastUpdatedTime) was already correct."}
  PutDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: response silently omitted accessPolicy and creationTime (both real flat fields on types.Destination); added destinationWireShape helper, used by PutDestination and DescribeDestinations."}
  DescribeDestinations: {wire: ok, errors: ok, state: ok, persist: ok, note: "same creationTime/accessPolicy fix as PutDestination. DestinationNamePrefix's PascalCase wire key (verified against the real serializer -- this operation is a smithy-model exception to the otherwise-universal lowerCamelCase convention) was already correct. fixed this pass: Limit/NextToken pagination (real DescribeDestinationsInput/Output, confirmed via api_op_DescribeDestinations.go) now implemented via the same base64-index-cursor helpers every other paginated op in this package uses -- a previous revision took no limit/nextToken at all and always returned the complete unpaginated list."}
  DeleteDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  PutDestinationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutDeliveryDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: the handler built its response by hand and only ever included name/arn/outputFormat, silently dropping the target resource ARN, deliveryDestinationType, and tags from every response. The target ARN is also real-AWS-nested under deliveryDestinationConfiguration.destinationResourceArn, not a flat string (the DeliveryDestination model's own json tag, deliveryDestinationConfiguration on a bare string field, was wrong for the same reason, though it was never actually used for wire serialization). Added deliveryDestinationType as a real accepted+persisted+validated (S3/CWL/FH/XRAY) input, and deliveryDestinationWireShape to build the correct nested response for Put/Get/Describe."}
  GetDeliveryDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as PutDeliveryDestination."}
  DescribeDeliveryDestinations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30, exhaustive field sweep): DescribeDeliveryDestinationsInput.Limit/NextToken (api_op_DescribeDeliveryDestinations.go, both real optional members) were decoded nowhere -- the handler discarded its whole request body (`_ []byte`) and the backend method took no paging arguments, so every call always returned the complete unpaginated list. Now decodes limit/nextToken and paginates via the new shared paginateRange helper (store.go; same defaultDescribeLimit fallback as DescribeDestinations, which already had this fix). Proven via TestDescribeDeliveryDestinations_FullPagination (real client, 9 destinations/page size 4), hand-reverted and confirmed to fail against unfixed code (9 back in one page instead of <=4). same fix as PutDeliveryDestination -- previously this list endpoint returned only name+arn per entry, nothing else."}
  DeleteDeliveryDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  PutDeliveryDestinationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDeliveryDestinationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDeliveryDestinationPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutDeliverySource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass -- CRITICAL: the input parser read \"resourceArns\" (plural array), but the real wire key (verified against the serializer) is \"resourceArn\" (singular string). A real SDK client's request always sent \"resourceArn\", so this backend's ResourceArns was always empty for every real client call -- the resource ARN was silently dropped, not just mis-shaped in the response. Also added service (aws-sdk-go-v2 types.DeliverySource.Service, \"the Amazon Web Services service that is sending logs\"): confirmed NOT client-supplied on PutDeliverySourceInput, so it is now derived server-side from the resource ARN's service segment via serviceFromARN, matching real AWS. Response previously returned only name+arn; now uses deliverySourceWireShape (name/arn/logType/resourceArns/service/tags)."}
  GetDeliverySource: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as PutDeliverySource."}
  DescribeDeliverySources: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-30, exhaustive field sweep): same Limit/NextToken-ignored bug as DescribeDeliveryDestinations (api_op_DescribeDeliverySources.go). Proven via TestDescribeDeliverySources_FullPagination, hand-reverted and confirmed to fail against unfixed code. same fix as PutDeliverySource -- previously this list endpoint returned only name+arn per entry."}
  DeleteDeliverySource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateLogAnomalyDetector: {wire: ok, errors: ok, state: ok, persist: ok}
  GetLogAnomalyDetector: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: LogAnomalyDetector.DetectorStatus serialized as \"detectorStatus\"; the real wire key (types.AnomalyDetector.AnomalyDetectorStatus / GetLogAnomalyDetectorOutput.AnomalyDetectorStatus) is \"anomalyDetectorStatus\" -- a real SDK client's status field always deserialized empty. Field renamed to AnomalyDetectorStatus (Go field + json tag both fixed) for consistency with the rest of this model. Also removed two orphaned gopherstack-invented fields with no wire representation anywhere in the real SDK and no readers anywhere in this codebase (de-stub hygiene): EvaluationLookback (\"evaluationLookback\") and FilterAnomalies (\"filterAnomalies\") -- neither exists in types.AnomalyDetector, any api_op_*AnomalyDetector*.go input, or any SDK doc comment."}
  ListLogAnomalyDetectors: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateLogAnomalyDetector: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: Enabled is a *required* field on the real UpdateLogAnomalyDetectorInput (\"Use this parameter to pause or restart the anomaly detector\"), used to set/clear types.AnomalyDetectorStatusPaused -- this backend didn't accept or act on it at all, meaning a detector could never actually be paused/resumed through this API despite PAUSED being a real, reachable status value. Now enabled=false sets AnomalyDetectorStatus=PAUSED; enabled=true resumes a paused detector to ANALYZING (a no-op if not currently paused, e.g. still INITIALIZING)."}
  DeleteLogAnomalyDetector: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAnomalies: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-enpq, cmd/structfielddiff): Anomaly had no Go field at all for Histogram/LogSamples/PatternId/PatternString/PatternTokens (all `required` on the real types.Anomaly) or the optional IsPatternLevelSuppression/PatternRegex/Priority/Suppressed/SuppressedUntil -- reverting the added fields fails to compile (anomaly_detectors.go references them), the same strength of confirmation as sns's XMLOriginationPhone fix. The pre-existing SuppressedState field used a made-up \"suppressedState\" wire key holding the raw suppressionType request value; the real member is \"state\" (types.Anomaly.State, values Active/Suppressed/Baseline), so a real client's State field always deserialized empty. This backend has no pattern-detection engine (anomalies are only ever seeded via the AddAnomalyInternal test seam, never generated from real log content), so Histogram/LogSamples/PatternString/PatternTokens content is only ever present if caller-seeded -- disclosed, not fabricated; see gaps."}
  UpdateAnomaly: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "fixed (gopherstack-enpq): suppressionType==\"\" (the real un-suppress signal per this op's own doc comment: 'to end the suppression... omit the suppressionType and suppressionPeriod parameters') was previously treated as the SUPPRESS branch (inverted check against a gopherstack-invented \"NO_SUPPRESSION\" sentinel with no wire representation at all), so a real client ending a suppression was instead left suppressed with a freshly bumped SuppressedDate. Reverting just this branch (keeping the new Anomaly fields, so it still compiles) reproduces the bug as an assertion failure, not a compile error -- confirmed independently of the wire-shape fix above. suppressionType is now validated against the real 2-member enum (LIMITED/INFINITE; types.SuppressionType.Values()), rejecting the old \"NO_SUPPRESSION\" convention. FIXED (gopherstack-enpq, 2026-08-22): PatternId (api_op_UpdateAnomaly.go:12-19, \"You must specify either anomalyId or patternId, but you can't specify both\") had no Go field at all, and anomalyId was unconditionally required -- the doc-prescribed pattern-suppression path (\"If you suppress a pattern, CloudWatch Logs won't report any anomalies related to that pattern\") was structurally unreachable, always rejected with \"anomalyId is required\" even though validateOpUpdateAnomalyInput only requires AnomalyDetectorArn client-side. Fixed: PatternId now suppresses/unsuppresses every stored anomaly sharing that pattern (via the existing anomalyByDetector index), with both-set and neither-set rejected as InvalidParameterException. Proven via TestUpdateAnomaly_PatternID and TestUpdateAnomaly_AnomalyIDAndPatternIDMutualExclusion (real aws-sdk-go-v2 client), hand-reverted and confirmed to fail against unfixed code. Not implemented: Baseline (mark as baseline behavior) and SuppressionPeriod (limited-duration expiry -> SuppressedUntil) -- two more real UpdateAnomalyInput members, disclosed not fixed; see gaps."}
  CreateScheduledQuery: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: executionRoleArn and queryLanguage are both real required CreateScheduledQueryInput members (confirmed via api_op_CreateScheduledQuery.go); a previous revision accepted executionRoleArn from the wire but discarded it via a `_` parameter (never stored, never returned) and never modeled queryLanguage at all, so a required member's absence was silently accepted rather than rejected. Now both are validated required, plus description/destinationConfiguration/logGroupIdentifiers/timezone/endTimeOffset/startTimeOffset/scheduleStartTime/scheduleEndTime are accepted and stored (bundled behind a new ScheduledQueryCreateParams struct to avoid an unwieldy positional-parameter signature). Follow-up (gopherstack-09o8, sdk_module v1.81.1): DestinationConfiguration's LookupTableConfiguration alternative member (types.LookupTableConfiguration, types.go:1561) is now modeled too, alongside the pre-existing S3Configuration -- neither member is required by the real type (validateDestinationConfiguration has no top-level required check, validators.go:2451), so a config with neither set is accepted, matching AWS. LookupTableConfiguration's required members (tableName/roleArn) are accepted from the wire and stored verbatim; unlike S3Configuration this backend does not additionally validate their presence (matching the pre-existing lack of nested S3Configuration validation, not a new gap introduced here)."}
  GetScheduledQuery: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass (2 bugs): (1) ScheduledQuery.Arn previously serialized as \"arn\"; the real wire key (GetScheduledQueryOutput.ScheduledQueryArn) is \"scheduledQueryArn\" -- fixed in an earlier pass. (2) fixed this pass: the response was wrapped under a \"scheduledQuery\" key with no real wire representation at all -- GetScheduledQueryOutput's members (confirmed via deserializers.go's awsAwsjson11_deserializeOpDocumentGetScheduledQueryOutput) sit flat at the top level of the response. The ScheduledQuery model previously covered only 6 of GetScheduledQueryOutput's ~20 members; now covers the full set (description, destinationConfiguration, executionRoleArn, lastExecutionStatus/lastTriggeredTime/lastUpdatedTime, logGroupIdentifiers, queryLanguage, scheduleType, scheduleEndTime/scheduleStartTime, startTimeOffset/endTimeOffset, timezone). scheduleType is always CUSTOMER_MANAGED (not client-settable; AWS_MANAGED queries are pre-provisioned by AWS, not created through this API). Follow-up (gopherstack-09o8): destinationConfiguration now round-trips LookupTableConfiguration (tableName/roleArn/description/kmsKeyId/tags) as well as S3Configuration -- see CreateScheduledQuery note."}
  ListScheduledQueries: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: real ListScheduledQueriesOutput.ScheduledQueries is []types.ScheduledQuerySummary, a distinct, narrower shape than GetScheduledQueryOutput (no queryString/executionRoleArn/queryLanguage/logGroupIdentifiers/description/endTimeOffset/startTimeOffset/scheduleStartTime/scheduleEndTime, confirmed via types.ScheduledQuerySummary) -- a previous revision reused the full Get shape here, over-sharing fields real AWS never returns from List. New scheduledQuerySummaryToWire renders the correct narrower shape. destinationConfiguration is passed through wholesale (types.ScheduledQuerySummary carries it too), so it also now covers LookupTableConfiguration (gopherstack-09o8)."}
  UpdateScheduledQuery: {wire: ok, errors: ok, state: ok, persist: ok, note: "still state-only, not the real API's full-replace UpdateScheduledQueryInput (executionRoleArn/queryLanguage/queryString/scheduleExpression all required, plus the same optional set as Create) -- see gaps below. lastUpdatedTime now bumped on every state change. Because this op never accepted destinationConfiguration at all, the LookupTableConfiguration addition (gopherstack-09o8) does not touch it; it is unaffected by, not fixed by, that change."}
  DeleteScheduledQuery: {wire: ok, errors: ok, state: ok, persist: ok}
  GetScheduledQueryHistory: {wire: ok, errors: ok, state: ok, persist: ok}
  PutResourcePolicy: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "fixed (gopherstack-enpq, second pass): the prior pass's structfielddiff sweep classified this whole family (\"account policies / data protection/resource/index policies / transformers / integrations\") as spot-checked-flat and moved on without an op-by-op field diff -- that shortcut missed real gaps. types.ResourcePolicy.ResourceArn/PolicyScope/RevisionId/LastUpdatedTime (deserializers.go:awsAwsjson11_deserializeDocumentResourcePolicy) had no Go field at all; ResourceArn in particular is a whole real feature (\"one per LogGroup resourceARN\", PutResourcePolicy doc comment) that was silently unreachable -- a caller-supplied resourceArn was accepted by the wire body but the handler never read it. Now models the real account-vs-resource scope split (keyed by resourceArn when present, else policyName, matching AWS's own \"a maximum of 10 policies without resourceARN and one per LogGroup resourceARN\" limit), generates an incrementing RevisionId, and enforces ExpectedRevisionId concurrency per the input's own doc comment (\"Required when resourceArn is provided to prevent concurrent modifications\")."}
  DescribeResourcePolicies: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "fixed (gopherstack-enpq, second pass): PolicyScope/ResourceArn input filters were both unmodeled -- every call returned every policy regardless of scope. DescribeResourcePoliciesInput's own doc comment says PolicyScope \"defaults to ACCOUNT\" when omitted, which this backend now honors; ResourceArn does an exact lookup against the resource-scoped policy on that ARN. CORRECTED (2026-08-30, sort-totality pass): the \"Limit/NextToken pagination still not implemented\" line above is stale -- Limit/NextToken were implemented by the later pagination_sweep entry (2026-08-28/29, see below) and are live in the code today (parseNextToken/encodeNextToken). What this pass actually found and fixed: for the RESOURCE scope, PolicyName is not unique (PutResourcePolicy keys resource-scoped policies by policyName+resourceArn, so two different resourceArns can legitimately share a PolicyName) and the sort was `PolicyName` alone with no secondary key, sourced from store.Table.All() (unordered map iteration) -- a genuine record-dropped-or-duplicated-across-a-page-boundary bug, not just theoretical: TestDescribeResourcePoliciesSortIsTotal (pagination_sort_totality_test.go) reproduces it against unfixed code. Fixed by adding ResourceArn as the tiebreak."}
  DeleteResourcePolicy: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "fixed (gopherstack-enpq, second pass): ResourceArn (real DeleteResourcePolicyInput member, needed to address a resource-scoped policy at all since those are no longer keyed by name alone) and ExpectedRevisionId (concurrency check, \"Required when deleting a resource-scoped policy\") were both accepted on the wire and silently ignored -- any caller could delete any policy by name with no conflict check. Both now wired through PutResourcePolicy's shared key/revision helpers."}
  PutIndexPolicy: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-enpq, second pass): types.IndexPolicy.Source had no Go field at all, so a real client's Source field always deserialized empty even though it is always present on the wire. Now always LOG_GROUP (the only source this op can produce; PolicyName is correctly left unset here too, since the real type's own doc comment says log-group-level index policy responses don't carry a PolicyName -- only account-level ones, created via PutAccountPolicy's FIELD_INDEX_POLICY type, do). DescribeIndexPolicies does not fall back to an account-level FIELD_INDEX_POLICY when no log-group-level policy exists, despite this backend supporting that PolicyType on PutAccountPolicy -- disclosed, not fixed; see gaps."}
  DescribeIndexPolicies: {wire: fixed, errors: fixed, state: ok, persist: n/a, note: "FIXED (2026-08-30, exhaustive field sweep, gopherstack-wksweep-cwl): total unfiltered-list bug -- the handler discarded its whole request body (`_ []byte`) and DescribeIndexPolicies() took no arguments, always returning every stored index policy for every log group regardless of what the caller asked about. LogGroupIdentifiers is a REQUIRED member on the real DescribeIndexPoliciesInput (api_op_DescribeIndexPolicies.go) that scopes the response to only those log groups -- a required field was silently accepted as absent, and the response was an unfiltered full list rather than the requested subset, the dominant bug shape this pass hunted for. Fixed: DescribeIndexPolicies now takes logGroupIdentifiers []string (required, InvalidParameterException if empty) plus nextToken/limit, filtering to only the requested identifiers before the existing LogGroupIdentifier sort; also added NextToken pagination (the real output carries one, previously never implemented -- no documented default, so this follows the same defaultDescribeLimit fallback DescribeResourcePolicies/GetQueryResults/ListLogGroupsForQuery use). Proven via TestDescribeIndexPolicies_FiltersByLogGroupIdentifiers (real aws-sdk-go-v2 client; two log groups seeded, requesting one returns exactly one, and an empty LogGroupIdentifiers request errors) and TestHandler_IndexPolicy's DescribeIndexPolicies/MissingLogGroupIdentifiers case; both hand-reverted (body discarded again, filter/limit args ignored again) and confirmed to fail against unfixed code (2 policies back instead of 1)."}
  PutQueryDefinition: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-enpq, second pass): Parameters ([]types.QueryParameter, a real accepted PutQueryDefinitionInput member per api_op_PutQueryDefinition.go) was dropped entirely -- a real client's parameterized-query placeholders never round-tripped. QueryLanguage also added to the QueryDefinition model, always CWLI since PutQueryDefinitionInput itself has no queryLanguage member to set it from."}
  DescribeQueryDefinitions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-enpq, second pass): now echoes Parameters/QueryLanguage from the shared QueryDefinition model -- see PutQueryDefinition."}
  DisassociateSourceFromS3TableIntegration: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "fixed (gopherstack-enpq, second pass): total stub before this fix -- handler took no request body param at all and unconditionally returned an empty success response, so the association this op is named for was never actually removed from b.s3TableIntegrations (a real, if quiet, permissiveness/data-integrity bug: repeated Disassociate calls, or one made in error, never had any effect to undo) and the required DisassociateSourceFromS3TableIntegrationOutput.Identifier member was never populated. Now reads Identifier, deletes the matching s3TableIntegrationEntry (new ErrS3TableIntegrationNotFound sentinel if it doesn't exist), and echoes Identifier back."}
  AssociateSourceToS3TableIntegration: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (gopherstack-enpq, 2026-08-22): DataSource.Name/Type (types.DataSource, real accepted AssociateSourceToS3TableIntegrationInput member) were parsed off the wire by the handler but then discarded entirely by the backend (`integrationArn, _, _ string` -- both args unread), so an association's data source was never actually stored anywhere; the sibling ListSourcesForS3TableIntegration bug (below) meant this went unnoticed since nothing ever read the association back either. s3TableIntegrationEntry now carries DataSourceName/DataSourceType/CreatedTimeStamp (purely additive persisted fields, no cwlSnapshotVersion bump -- guard-verified via pkgs/persistence's TestSnapshotVersionGuard -update)."}
  ListSourcesForS3TableIntegration: {wire: fixed, errors: fixed, state: fixed, persist: n/a, note: "FIXED (gopherstack-enpq, 2026-08-22): total stub before this fix -- handler took no request body param at all (`_ []byte`) and unconditionally returned an empty \"sources\" list, so an association genuinely stored by AssociateSourceToS3TableIntegration could never actually be observed through this op regardless of which integrationArn a real caller listed (present but never populated on any read path). Also: IntegrationArn is required on the real input (validateOpListSourcesForS3TableIntegrationInput) but the pre-fix stub silently accepted an empty body -- an existing test (TestHandler_S3TableIntegrationSourceOperations/ListSourcesForS3TableIntegration/ReturnsEmpty) asserted 200+empty-list for exactly that request, ratifying a call shape a real client's own client-side validator refuses to send; corrected to assert 400 (renamed .../MissingArn) and a new .../ReturnsEmptyForUnknownArn case added. Now filters by integrationArn, paginates via maxResults/nextToken (real input members, 1-100 range per the doc comment), and renders the real types.S3TableIntegrationSource shape (createdTimeStamp/dataSource{name,type}/identifier/status) -- status is always ACTIVE (this backend has no health-check/failure modeling for these associations, matching the same always-ACTIVE pattern used elsewhere in this codebase for unmonitored resources). Proven via TestListSourcesForS3TableIntegration_RealRoundTrip (real aws-sdk-go-v2 client), hand-reverted and confirmed to fail against unfixed code (0 sources instead of 1). ParentSourceIdentifier/StatusReason left unmodeled -- disclosed, not fabricated; see gaps."}
  GetDataProtectionPolicy: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-enpq, second pass): GetDataProtectionPolicyOutput.LastUpdatedTime had no Go field at all -- confirmed against the raw structfielddiff dump, which lists it as a real *int64 output member alongside LogGroupIdentifier/PolicyDocument. Now stamped by PutDataProtectionPolicy and returned by Get."}
  DescribeAccountPolicies: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED (2026-08-30, exhaustive field sweep, gopherstack-wksweep-cwl): AccountPolicy is keyed by PolicyName+\":\"+PolicyType (accountPolicyKeyFn) -- a caller can legitimately have several account policies sharing one PolicyName across different PolicyTypes (e.g. one DATA_PROTECTION_POLICY and one SUBSCRIPTION_FILTER_POLICY both named \"default\"), since PolicyType is part of the real key. DescribeAccountPolicies sorted only by PolicyName, a deliberately non-unique key in that scenario, over store.Table.All()'s unordered map walk -- the tie-prone-sort-over-non-deterministic-input shape that drops/duplicates records across a pagination boundary, same class already fixed for DescribeResourcePolicies (ResourceArn tiebreak) and DescribeQueryDefinitions. Fixed by adding PolicyType as the tiebreak. Proven via TestDescribeAccountPoliciesSortIsTotal (pagination_sort_totality_test.go, 3 policies same name/different types, 30-attempt repeated small-page walk per this file's existing walkAndVerify convention), hand-reverted (tiebreak removed) and confirmed to fail against unfixed code on the very first attempt (one policy returned on two different pages)."}
families:
  metric-filter-emission: {status: ok, note: "fixed (internal PutLogEvents dispatch, not an SDK op): emitMetricFilterMatches previously emitted matchCount copies of one static value regardless of MetricValue's per-event field reference (a disguised stub -- '$field' values were never actually read from the matched log event, just defaulted to 1.0/DefaultValue). Now extracts the referenced field ($name for space-delimited patterns, $.path for JSON patterns) per matched event via new compiledFilterPattern.extract; a matched-but-non-numeric-or-absent field now correctly emits no data point rather than fabricating one. Also fixed emitted Unit being hardcoded to \"\" instead of the configured MetricTransformation.Unit."}
  janitor-retention-sweep: {status: ok, note: "two-phase read-then-write lock, worker.NewGroup ticker is ctx-cancel safe, telemetry recorded. No leak."}
  subscription-filter-delivery: {status: ok, note: "goroutines bounded by workerSem + backend WaitGroup + service ctx; Close()/Drain() wait for in-flight deliveries. No leak found."}
  insights (StartQuery/GetQueryResults/StopQuery/DescribeQueries/query language): {status: ok, note: "lightly reviewed only (large ~2500 LOC subsystem across insights_*.go); query TTL eviction (evictByTTL) and cap enforcement (enforceCap) present and bounded; not exhaustively re-audited op-by-op this pass -- see deferred."}
  export/import tasks / deliveries / anomaly detectors / scheduled queries: {status: ok, note: "genuinely field-diffed and fixed this pass -- see the individual ops entries above for CreateExportTask/DescribeExportTasks/CreateImportTask/DescribeImportTasks/CancelImportTask/PutDeliveryDestination*/PutDeliverySource*/GetLogAnomalyDetector/UpdateLogAnomalyDetector/GetScheduledQuery. Several real bugs found and fixed: nested-vs-flat wire shape (ExportTask, DeliveryDestination), wrong wire key (importStatus, anomalyDetectorStatus, scheduledQueryArn, deliveryDestinationConfiguration.destinationResourceArn), wrong input wire key that silently dropped a real field entirely (PutDeliverySource's resourceArn), invented status enum values not in the real SDK (ImportStatus ACTIVE/SUCCEEDED -> IN_PROGRESS/COMPLETED), and orphaned invented fields with zero wire representation (LogAnomalyDetector.EvaluationLookback/FilterAnomalies). Follow-up pass closed the four gaps this note used to list: CreateDelivery now accepts FieldDelimiter/RecordFields/S3DeliveryConfiguration at creation time (UpdateDeliveryConfiguration also gained S3DeliveryConfiguration, which it was real-API-eligible for but hadn't implemented either); Delivery's fabricated CreationTime field is now excluded from the wire (json:\"-\") since real types.Delivery has no such member; AccountPolicy now carries AccountId/LastUpdatedTime; DescribeDestinations now implements Limit/NextToken pagination; ScheduledQuery now models the full GetScheduledQueryOutput field set and the Get/List wire-shape bugs (wrapper key, over-shared List fields) described below are fixed. UpdateScheduledQuery's state-only-update limitation (the real op is a full-replace requiring executionRoleArn/queryLanguage/queryString/scheduleExpression on every call) remains open -- see gaps."}
  account policies / data protection/resource/index policies / transformers / integrations: {status: ok, note: "spot-checked (CreateExportTask/runExport does a real synchronous S3 write via injectable ExportSink when configured; ApplyTransformer/applyJSONProcessor implements real addKeys/deleteKeys/renameKeys-style JSON processors, not a stub); AccountPolicy/ResourcePolicy/Transformer/GetIntegrationOutput top-level shapes spot-checked as flat (no nested-object bugs found), but not exhaustively re-audited op-by-op this pass -- see deferred."}
  PutIntegration: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-4ggy: ResourceConfig (a required PutIntegrationInput member, a union whose only member is OpenSearchResourceConfig) was dropped entirely -- request only read integrationName/integrationType. Now required, its own required members (dataSourceRoleArn/dashboardViewerPrincipals/retentionDays -- validateOpenSearchResourceConfig, validators.go) validated, and stored on CWLIntegration.OpenSearchResourceConfig (real, non-json:\"-\" tag so it survives Snapshot/Restore). Not surfaced on PutIntegrationOutput/GetIntegrationOutput -- neither carries ResourceConfig back on the wire, only the separate server-computed IntegrationDetails (GetIntegrationOutput), which this backend still does not model since there is no real OpenSearch Service collection/workspace behind it (pre-existing gap, unaffected by this fix)."}
  PutBearerTokenAuthentication: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-4ggy: total stub before this fix (body param `_ []byte`, always returned success, no backend effect). LogGroupIdentifier/BearerTokenAuthenticationEnabled (both required, validateOpPutBearerTokenAuthenticationInput) now validated; the log group must exist (ResourceNotFoundException otherwise) and the flag is stored on LogGroup.BearerTokenAuthenticationEnabled -- a real types.LogGroup field (types.go:1366) that DescribeLogGroups/ListLogGroups previously never modeled or echoed at all, now wired through since LogGroup is marshaled directly for those responses."}
  StartLiveTail: {status: ok, note: "explicitly validation-only (log-group-identifier existence check) with a documented comment explaining the streaming HTTP/2 transport can't be served by this request/response handler -- an honest declared limitation, not a silent stub."}
  lookup tables / syslog configurations / storage tier policy (parity-4 SDK-bump additions): {status: ok, note: "10 new ops (CreateLookupTable/GetLookupTable/UpdateLookupTable/DeleteLookupTable/DescribeLookupTables, PutSyslogConfiguration/ListSyslogConfigurations/DeleteSyslogConfiguration, GetStorageTierPolicy/PutStorageTierPolicy), all newly implemented for real (lookup_tables.go, syslog_configurations.go, policies.go, handler_lookup_tables.go, handler_syslog_configurations.go, handler_storage_tier_policy.go) against aws-sdk-go-v2@v1.80.0 (bumped from v1.64.0). Two findings worth flagging for future auditors who might assume otherwise from the task framing alone: (1) lookup tables do NOT reference S3 -- CreateLookupTableInput/UpdateLookupTableInput both carry TableBody as a plain CSV *string (verified against serializers.go), so this backend parses real CSV content rather than modeling an S3 reference it would need chaos/network plumbing to honestly resolve; (2) the storage tier policy is account-level, NOT per-log-group -- GetStorageTierPolicyInput is a zero-field struct and PutStorageTierPolicyInput carries only StorageTier, confirmed by reading the real Input structs directly, so it is intentionally kept independent of LogGroup.LogGroupClass rather than invented as a per-group attribute. See the individual ops entries above for full field-diff detail per op."}
  pagination_sweep: {status: fixed, note: "2026-08-28/29 (wrapper-key-sweep-rds-cloudwatch-sqs-sns pagination pass): audited every op with a page-size + continuation member (List*/Describe*/GetQueryResults) against the pinned SDK. Three ops accepted Limit/MaxItems/MaxResults + NextToken on the real wire but decoded neither, always returning everything in one call: DescribeResourcePolicies (api_op_DescribeResourcePolicies.go:29-42, no documented default -- now falls back to this service's existing defaultDescribeLimit=50 convention, same as DescribeLogStreams et al.); GetQueryResults (api_op_GetQueryResults.go:56-66, documented 'maximum is 10,000 log events per request' -- now the default/max page size, maxGetQueryResultsItems); ListLogGroupsForQuery (no documented default -- same defaultDescribeLimit=50 fallback). All three fixed at the handler layer (parseNextToken/encodeNextToken, the shared index-token helpers store.go already provides) without touching the underlying backend methods' full-result-set signatures. TestDescribeResourcePolicies_FullPagination/TestGetQueryResults_FullPagination/TestListLogGroupsForQuery_FullPagination (wire_field_fixes_test.go) each create more records than one page holds, drive the real SDK client through the full pagination loop asserting per-page truncation plus a duplicate-free/complete union, and were hand-verified to fail against unfixed code (page sizes of 9/25/9 instead of the requested 4/10/4). Everything else audited (DescribeLogGroups/DescribeLogStreams/DescribeSubscriptionFilters/DescribeMetricFilters/DescribeExportTasks/DescribeImportTasks/DescribeDeliveries/DescribeDestinations/DescribeQueries/DescribeQueryDefinitions/DescribeLookupTables/ListLogAnomalyDetectors/ListAnomalies/ListLogGroups/ListScheduledQueries/ListSourcesForS3TableIntegration/ListSyslogConfigurations) already shares the same parseNextToken/encodeNextToken/defaultDescribeLimit convention and correctly truncates+resumes+emits-only-when-truncated. ListAggregateLogGroupSummaries has no Limit/NextToken in this backend's signature by design: the backend always collapses to at most one summary bucket (no per-log-group data-source classification to group by), so a token could never be meaningful; DescribeConfigurationTemplates/DescribeImportTaskBatches are disclosed structural void-results (no create op backs either)."}
gaps:
  - RESOLVED (gopherstack-09o8): types.DestinationConfiguration's LookupTableConfiguration member (added since v1.80.0, alternative to S3Configuration -- neither is `required` on the real type) is now modeled: ScheduledQueryDestinationConfig gained a LookupTableConfiguration field (models.go), mirroring types.LookupTableConfiguration's tableName/roleArn/description/kmsKeyId/tags (types.go:1561, field names/wire keys confirmed against serializers.go/deserializers.go). Threaded through Create/Get/List, which already passed the whole DestinationConfiguration through unmodified. UpdateScheduledQuery does not carry destinationConfiguration at all (pre-existing, separate-scope gap -- see the UpdateScheduledQuery gap entry above) so is unaffected. Round-tripped in TestHandler_ScheduledQuery_DestinationConfiguration and TestInMemoryBackend_SnapshotRestore_ScheduledQueryLookupTableDestination; added as an additive omitempty field, cwlSnapshotVersion unchanged (older snapshots decode fine with the field simply absent).
  - MetricTransformation.Dimensions is accepted, validated on the wire, and persisted on the MetricFilter, but is never forwarded to the emitted CloudWatch metric: the MetricEmitter interface (backend.go) only carries namespace/name/value/unit, and its real implementation is wired in cli.go's wireCWLogsMetricEmitter, which is out of scope for this pass (SHARED FILE). Extending the interface + cli.go wiring to carry dimensions is a real fix but requires touching cli.go. (bd: gopherstack-b14)
    RECONFIRMED (2026-08-30, exhaustive field sweep, gopherstack-wksweep-cwl): still real, still open. Verified by reading, not inherited: MetricTransformation.Dimensions is decoded (handler_metric_filters.go), stored verbatim (metric_filters.go:153, transformations slice passed through wholesale to PutMetricFilter), but emitMetricFilterMatches/metricTransformationValue (metric_filters.go) never reference `.Dimensions` at all, and interfaces.go's MetricEmitter.EmitMetric signature genuinely has no dimensions parameter to carry it through even if they did. This is a real layer-boundary gap (the fix needs cli.go, a shared file outside this pass's scope), not fabricated -- left open per this pass's restraint instruction. MetricTransformation.DefaultValue is a related but separate, already-documented (see the "Trap" note in the file-level comments below) intentional non-implementation: it is AWS's periodic-no-match-emission value, which this backend has no scheduler to drive, and metricTransformationValue correctly never reads it.
  - RESOLVED (follow-up pass): ScheduledQuery previously modeled only a subset of GetScheduledQueryOutput (arn/name/queryString/scheduleExpression/state/creationTime) and Get's response was wrapped under a non-existent "scheduledQuery" key. Now models the full field set (description, destinationConfiguration, executionRoleArn, lastExecutionStatus/lastTriggeredTime/lastUpdatedTime, logGroupIdentifiers, queryLanguage, scheduleType, scheduleEndTime/scheduleStartTime, startTimeOffset/endTimeOffset, timezone), Get returns it flat, List renders the real, narrower ScheduledQuerySummary shape via a separate scheduledQuerySummaryToWire, and Create validates the real required executionRoleArn/queryLanguage/scheduleExpression members. Still open: UpdateScheduledQuery remains state-only rather than the real API's full-replace semantics (UpdateScheduledQueryInput requires executionRoleArn/queryLanguage/queryString/scheduleExpression on every call, plus the same optional field set as Create) -- a distinct, separate-scope reshape from the field-completeness gap just closed. (bd: gopherstack-b14)
  - RESOLVED (follow-up pass): CreateDelivery now accepts FieldDelimiter, RecordFields, and S3DeliveryConfiguration at creation time (all real CreateDeliveryInput members, confirmed via serializers.go), rather than only via the separate UpdateDeliveryConfiguration op, which also gained S3DeliveryConfiguration support it was real-API-eligible for but hadn't implemented. Delivery's CreationTime field (no equivalent on real types.Delivery) is now excluded from the wire via json:"-", matching the same bookkeeping-only pattern used elsewhere in this codebase (e.g. inspector2's FindingsReport.CreatedAt). RESOLVED 2026-08-23 (manifest-harvest pass): Delivery now carries DeliveryDestinationType (models.go), populated at CreateDelivery time by a new deliveryDestinationByArnLocked lookup (deliveries.go) matching the client-supplied deliveryDestinationArn against the stored DeliveryDestination's own Arn/DeliveryDestinationType -- confirmed against types.Delivery.DeliveryDestinationType's doc comment ("Displays whether the delivery destination associated with this delivery is CloudWatch Logs, Amazon S3, Firehose, or X-Ray", cloudwatchlogs@v1.81.1 types/types.go:539-540) and the deserializer's "deliveryDestinationType" wire key (deserializers.go:16747). CreateDelivery does not validate deliveryDestinationArn against an existing destination (pre-existing behavior, unchanged this pass -- an unknown ARN just leaves DeliveryDestinationType empty rather than erroring); UpdateDeliveryConfiguration does not touch deliveryDestinationArn so no re-derivation is needed there. Proven via new TestCreateDelivery_DeliveryDestinationType (deliveries_test.go), hand-reverted to confirm the pre-fix code doesn't even compile against the new field reference, restored, md5sum byte-identical. (bd: gopherstack-b14)
  - RESOLVED (follow-up pass): AccountPolicy now carries AccountId and LastUpdatedTime, both real flat fields on types.AccountPolicy, populated by PutAccountPolicy. (bd: gopherstack-b14)
  - RESOLVED (follow-up pass): DescribeDestinations now implements Limit/NextToken pagination (real DescribeDestinationsInput/Output members, confirmed via api_op_DescribeDestinations.go) via the same base64-index-cursor helpers every other paginated op in this package uses. (bd: gopherstack-b14)
  - SyslogConfiguration's VpcEndpointId is accepted/stored/returned as an opaque string, never cross-validated against real EC2 VPC-endpoint state -- there is no VPC-endpoint modeling anywhere in this service, and no established cross-service ARN/ID validation pattern anywhere in this codebase to reuse (this backend already treats KmsKeyId/RoleArn/DestinationArn the same way elsewhere). Not implemented this pass; would require either a cross-service backend dependency or a shared registry that does not currently exist.
  - LookupTable's ARN (arn:{partition}:logs:{region}:{account}:lookup-table:{name}) is constructed by analogy to this codebase's existing log-group ARN convention, not confirmed against an authoritative AWS source: no smithy model ships with the installed aws-sdk-go-v2 module and no ARN pattern appears in any doc comment for LookupTableArn. If a future pass finds the real pattern differs, only lookupTableARN (lookup_tables.go) needs to change.
  - IndexPolicy/Transformer (pre-existing, prior pass) still accept any logGroupIdentifier string without checking it resolves to a real log group, unlike the new PutSyslogConfiguration added this pass (which does validate). Noted here for consistency awareness, not fixed this pass (out of scope: pre-existing ops, not part of the parity-4 SDK-bump op set this pass covers).
  - "2026-08-14 (gopherstack-enpq): mechanical struct-field diff (cmd/structfielddiff) against aws-sdk-go-v2/service/cloudwatchlogs@v1.81.1, all 118 ops and every nested type expanded (Processor union, DeliverySource, ConfigurationTemplate, Import*, Anomaly*, MetricFilter/SubscriptionFilter, etc.), a different method than this file's op-by-op audits. 118 of the raw op-level hits were pure ResultMetadata noise; of the remaining 39 ops with a real candidate field miss, hand-verified one by one against the real SDK source and this backend's handlers: 2 ops (ListAnomalies, UpdateAnomaly) got a real fix (see their ops entries above -- Anomaly's missing content fields + wrong \"suppressedState\" key, and UpdateAnomaly's inverted suppress/unsuppress branch), 3 fields were confirmed non-issues, and the rest were disclosed rather than fabricated -- see the following gap entries, all filed this pass. (bd: gopherstack-enpq)"
  - "FALSE POSITIVE, not a gap (gopherstack-enpq): GetTransformer/PutTransformer/TestTransformer's Processor union members (Grok/DateTimeConverter/ParseJSON/ParseKeyValue/ParseToOCSF/etc.) look missing from a struct-field diff, but Transformer.Processors is stored/returned as raw []map[string]any, so every processor type's fields round-trip on the wire regardless of which subset ApplyTransformer's own doc comment says it functionally implements (addKeys/deleteKeys/renameKeys/lowerCaseString/upperCaseString/copyValue) -- a documented functional-completeness question, not a wire-shape bug."
  - "Non-issue (gopherstack-enpq): PutQueryDefinitionInput.ClientToken is unmodeled, but accept-and-ignore is correct AWS idempotency-token behavior for a backend with no idempotency cache to key it against. QueryStatistics.EstimatedBytesSkipped/EstimatedRecordsSkipped/LogGroupsScanned and GetQueryResultsOutput.EncryptionKey are also unmodeled, but this backend has no field-index-based scan-skipping or query-result encryption, so these are always meaningfully zero/nil whether or not the wire key is explicitly sent -- no observable client-visible difference either way."
  - "DISCLOSED, not fixed (gopherstack-enpq): UpdateAnomaly does not accept AnomalyId/PatternId mutual exclusion, Baseline (mark as baseline behavior), or SuppressionPeriod (limited-duration expiry -> SuppressedUntil) -- three more real UpdateAnomalyInput members beyond the suppress/unsuppress fix above. STALE re: AnomalyId/PatternId -- flagged by cmd/staleclaims (gopherstack-anjf): FIXED (gopherstack-enpq, 2026-08-22), see the UpdateAnomaly ops: entry above (\"had no Go field at all... structurally unreachable, always rejected\"). Baseline and SuppressionPeriod remain genuinely not implemented -- see the same ops: entry's own \"Not implemented: Baseline... and SuppressionPeriod...\" note."
  - "DISCLOSED, not fixed (gopherstack-enpq): Anomaly's Histogram/LogSamples/PatternString/PatternTokens are only ever populated when a caller seeds them via AddAnomalyInternal -- this backend has no pattern-detection engine to generate them from real log content, same class as sts's already-disclosed JWT-payload-size gap. The wire shape itself is now correct (see ListAnomalies fix above); only the analysis content is unimplementable without inventing it."
  - "DISCLOSED, not fixed (gopherstack-enpq): PutMetricFilter/DescribeMetricFilters and PutSubscriptionFilter/DescribeSubscriptionFilters do not accept, store, or echo ApplyOnTransformedLogs, EmitSystemFieldDimensions (EmitSystemFields on the subscription-filter side), or FieldSelectionCriteria -- the transformed-logs metric/subscription routing feature family, added to the real API since this file's last field-level pass on these four ops."
  - "DISCLOSED, not fixed (gopherstack-enpq): PutLogEvents does not accept Entity (Attributes/KeyAttributes, OTel entity correlation); PutLogEventsOutput.RejectedEntityInfo is never populated as a result."
  - "RESOLVED (gopherstack-enpq, second pass): ResourcePolicy now carries ResourceArn/PolicyScope/RevisionId/LastUpdatedTime, and Put/DeleteResourcePolicy enforce ExpectedRevisionId concurrency -- see PutResourcePolicy/DescribeResourcePolicies/DeleteResourcePolicy op entries above. The prior pass's disclosure only named RevisionId/ExpectedRevisionId; ResourceArn/PolicyScope (a whole real feature, resource-scoped policies) were missed entirely because this family was spot-checked rather than field-diffed op-by-op -- see the dated note below."
  - "DISCLOSED, not fixed (gopherstack-enpq): DescribeLogGroups/ListLogGroups do not accept IncludeLinkedAccounts, LogGroupNamePattern (glob name search), DataSources, FieldIndexNames, or LogGroupTags filters; LogGroup.DataProtectionStatus/InheritedProperties are not modeled on output. The cross-account \"linked accounts\" half is structural -- there is no CloudWatch Logs cross-account-observability-link model anywhere in this backend to source it from."
  - "Benign, not a real behavioral gap (gopherstack-enpq): FilterLogEvents/GetLogEvents/GetLogObject/GetLogRecord do not accept Unmask. PutDataProtectionPolicy stores a policy document but this backend never actually redacts log content against it, so a real client's masked-vs-unmasked view is identical either way today; the real gap is that masking itself is unimplemented, not the flag."
  - "DISCLOSED, not fixed (gopherstack-enpq): GetQueryResultsInput.MaxItems is not accepted, so Insights query results are never truncated. DescribeQueries's QueryInfo.QueryDuration is not computed; UserIdentity needs a caller-identity model this backend does not have (same blocker as gopherstack-cu4g). GetScheduledQueryHistoryInput.ExecutionStatuses filter is not accepted; TriggerHistoryRecord/ScheduledQueryDestination's ErrorMessage/TriggeredTimestamp/ProcessedIdentifier members are not modeled."
  - "DISCLOSED, not fixed (gopherstack-enpq): import tasks -- CreateImportTaskInput.ImportFilter (EndEventTime/StartEventTime) is not accepted; Import/CancelImportTaskOutput's ImportStatistics(.BytesImported)/ErrorMessage are not modeled; DescribeImportTaskBatches remains validation-only (pre-existing, already documented in its own doc comment) rather than modeling real per-task import batches."
  - "DISCLOSED, not fixed (gopherstack-enpq): PutDeliverySourceInput.DeliverySourceConfiguration (per-log-type config key/value pairs) is not accepted, stored, or echoed; DeliverySource.Status/StatusReason are not modeled (StatusReason=RESOURCE_DELETED specifically needs cross-service resource-deletion tracking this backend does not have). PutDestinationPolicyInput.ForceUpdate is also unmodeled, but low-impact: on real AWS it only bypasses an idempotency check this backend never performs in the first place."
  - "2026-08-21 (gopherstack-enpq, second pass): the prior pass's own gaps entry (above, dated 2026-08-14) said 39 of 118 ops had a real structfielddiff candidate and were hand-verified one by one -- but the op-level table only carries individual entries for 2 of those 39 (ListAnomalies, UpdateAnomaly); the remaining ~37, including this whole family (\"account policies / data protection/resource/index policies / transformers / integrations\"), were folded into a single spot-checked-flat family note rather than genuinely diffed op-by-op, per that note's own honest caveat. Re-running the raw structfielddiff dump against this family surfaced 5 more real bugs the spot-check missed: PutResourcePolicy/DescribeResourcePolicies/DeleteResourcePolicy (ResourceArn/PolicyScope/RevisionId/LastUpdatedTime all missing -- see RESOLVED gap above), PutIndexPolicy (Source missing), PutQueryDefinition/DescribeQueryDefinitions (Parameters/QueryLanguage missing), DisassociateSourceFromS3TableIntegration (total no-op stub, both a permissiveness bug and a missing required output member), and GetDataProtectionPolicy (LastUpdatedTime missing). All 5 fixed and round-trip-tested against the real aws-sdk-go-v2 client; see the individual op entries above. Lesson for future passes: a family-level \"spot-checked flat, not exhaustively re-audited\" note is not equivalent to running structfielddiff against that family -- it looks similar in the table but is a materially weaker check. (bd: gopherstack-enpq)"
  - "STALE (2026-08-30, sort-totality pass): the DescribeResourcePolicies Limit/NextToken gap this line described was closed by the later pagination_sweep entry (2026-08-28/29, see op-level note above); kept here struck through only to preserve the audit trail rather than silently deleting a superseded claim."
  - "DISCLOSED, not fixed (gopherstack-enpq, second pass): DescribeIndexPolicies does not fall back to an account-level FIELD_INDEX_POLICY (PutAccountPolicy) when a log group has no policy of its own, despite this backend already supporting FIELD_INDEX_POLICY as a PutAccountPolicy type -- per DescribeIndexPolicies' own doc comment (\"If a specified log group doesn't have a log-group level index policy, but an account-wide index policy applies to it, that account-wide policy is returned\"). Would require DescribeIndexPolicies to additionally query the account-policies store and reason about applicability (scope ALL vs SELECTION_CRITERIA), not attempted this pass."
  - "DISCLOSED, not fixed (gopherstack-enpq, second pass): DescribeConfigurationTemplates and DescribeFieldIndexes are unconditional empty-list stubs. DescribeConfigurationTemplates is meant to return AWS's own static catalog of supported delivery-destination/log-type template combinations (not per-account state), which this backend would have to fabricate wholesale rather than derive from anything it models -- the no-stub rule favors the honest empty list over invented catalog data. DescribeFieldIndexes needs a field-indexing engine this backend does not have (same family as the already-disclosed FieldIndexNames filter gap)."
  - "2026-08-22 (gopherstack-enpq, third pass): the prior two passes' own gaps entries said this service was fully swept by cmd/structfielddiff across all 118 ops -- overstated in the same way the 2026-08-14 kinesis pass's ledger was: both compared field lists, but never asked whether an op could be called the way its own doc prose prescribes (kinesis's lesson: nine ops silently ignored the recommended StreamARN parameter). Applying that lens to cloudwatchlogs found 4 more real bugs the prior structural passes missed: CreateLookupTable/UpdateLookupTable had no QueryId field at all (doc: 'you must specify either tableBody or queryId, but not both' -- the query-results-populate-the-table path was structurally unreachable), UpdateAnomaly had no PatternId field and unconditionally required AnomalyId (doc: 'you must specify either anomalyId or patternId' -- the pattern-suppression path was structurally unreachable), AggregateLogGroupSummary modeled fabricated per-log-group fields that do not exist on the real type at all (a wire-shape break that survived the wrapper-key fix from an earlier pass because nobody re-checked the array ELEMENT shape, only the wrapper), and ListSourcesForS3TableIntegration was a total empty-list stub despite AssociateSourceToS3TableIntegration genuinely storing data underneath it (present but never read back). All 4 fixed and round-trip-tested against the real aws-sdk-go-v2 client, each hand-reverted and confirmed to fail against unfixed code; see the individual op entries above. Lesson reaffirmed: a structural field-diff sweep, however thorough, does not by itself catch (a) doc-prescribed alternate identification paths that are simply absent as fields, or (b) a correct-looking wrapper hiding a still-wrong element shape underneath. (bd: gopherstack-enpq)"
  - "DISCLOSED, not fixed (gopherstack-enpq, third pass): ListIntegrations does not accept IntegrationNamePrefix/IntegrationStatus/IntegrationType (all real, optional ListIntegrationsInput filter members) -- the handler discards its whole request body. Low-impact: this op's own doc comment says 'Currently, only one integration can be created in an account,' so there is at most one row to filter in the first place."
  - "DISCLOSED, not fixed (gopherstack-enpq, third pass): S3TableIntegrationSource's ParentSourceIdentifier and StatusReason (real, optional types.S3TableIntegrationSource members) are not modeled -- this backend does not model nested/derived associations or a health-check-driven failure reason, so every association is a top-level, unconditionally-ACTIVE entry."
  - "2026-08-30 (gopherstack-wksweep-cwl): first genuine EXHAUSTIVE field sweep of this service, as distinct from every prior structfielddiff/manifest-harvest pass -- a go/types-based scanner (scratch tool, not committed) loaded this package, found every json.Unmarshal(body,&X) call site inside a handle* method, recursively expanded X's struct fields (and any nested struct-typed field, cycle-guarded), and reported which field Vars are never referenced by a SelectorExpr anywhere else in the package. Result: 118 dispatch-table entries (confirmed by a temporary test printing len(h.ops)/GetSupportedOperations(), matching exactly, deleted after use, not taken from this file), 105 top-level decode structs / 293 fields on the first pass, 114 structs / 323 fields after adding nested-struct expansion, 18 fields flagged never-read. All 18 were hand-verified: DeliveryS3Configuration.SuffixPath/EnableHiveCompatiblePath, ScheduledQueryDestinationConfig.S3Configuration/LookupTableConfiguration and their nested fields, and QueryParameter.Name/DefaultValue/Description are whole-struct/whole-slice passthroughs (stored and echoed verbatim, e.g. deliveries.go's `S3DeliveryConfiguration: s3Config`, scheduled_queries.go's `DestinationConfiguration: p.DestinationConfiguration`, query_definitions.go's `slices.Clone(parameters)`) -- the scanner's known blind spot (field-level scan can't see through a whole-value copy), confirmed benign by reading each assignment. MetricTransformation.Dimensions/DefaultValue are the pre-existing, still-open metric-emitter gap and the documented DefaultValue non-implementation respectively -- see their entries above. TOOL BLIND SPOT FOUND AND WORKED AROUND: the scanner only matched `*types.Named` struct types, so `var input struct{...}` (an ANONYMOUS struct literal, used by ~13 handlers: handleDescribeConfigurationTemplates/handleDescribeFieldIndexes/handleDescribeImportTaskBatches/handleGetLogFields/handleGetLogObject/handleGetStorageTierPolicy/handleListAggregateLogGroupSummaries/handleListIntegrations/handleStartLiveTail/handleTestTransformer among others) never appeared in its output at all and had to be hand-enumerated separately by diffing the dispatch-table function-name set against the scanner's covered-function set. That hand pass found handleTestTransformer decoding a `LogGroupIdentifier` field with literally no member on the real TestTransformerInput (verified against api_op_TestTransformer.go: only LogEventMessages/TransformerConfig exist) and never read anywhere -- deleted (de-stub hygiene, not a behavioral bug: it was never used regardless of presence). The same hand pass, cross-referenced against each op's own real SDK Input struct (not inferred from a sibling), found three real PRIMARY-class bugs the field-diff/passthrough analysis alone could not have caught, because the request body was discarded ENTIRELY (`_ []byte`) rather than partially misread: DescribeIndexPolicies ignored its required LogGroupIdentifiers filter (unfiltered full list -- the dominant bug shape), and DescribeDeliveryDestinations/DescribeDeliverySources both ignored real Limit/NextToken members. All three fixed; see their ops: entries. A fourth bug, unrelated to field-reading, was found by checking whether a storage key carries the dimension its resource is scoped by (this pass's explicitly-directed hunt, given lookup tables' status as a scoped/versioned concept): CreateLookupTable/DescribeLookupTables used the backend's constant default region instead of the per-request ctx-derived region every sibling resource type uses, so two regions' same-named lookup tables collided on one key -- see CreateLookupTable/DescribeLookupTables ops: entries. A fifth bug was found reviewing every sort.Slice call in the package for non-unique-key-over-map-walk instability (the ordering class this pass also hunted): DescribeAccountPolicies' PolicyName-only sort was non-total because AccountPolicy's real key is PolicyName+PolicyType -- see the DescribeAccountPolicies ops: entry and the corrected sort-ordering note above (this file itself had wrongly claimed that sort was unique-by-construction). Host-prefix reachability (GetLogObject/StartLiveTail's real 'stream-' prefix, api_op_GetLogObject.go:161/api_op_StartLiveTail.go:225) and the metric-dimensions layer-boundary gap were both independently reconfirmed by reading, not inherited from this file's prior claims -- both still accurate. No storage key omitting a required scope dimension was found anywhere else (resource policies' resourceArn+revisionId, per-log-group IndexPolicy/Transformer/SyslogConfiguration keys, StorageTierPolicy's genuine account-level singleton status were all re-verified against their code, not assumed). DescribeConfigurationTemplates/DescribeFieldIndexes reconfirmed as honest structural void-results (no create op backs either, confirmed by grepping the full 118-op dispatch table). No handler discarding its entire request remains except the three legitimately-argument-free/low-impact cases (handleGetStorageTierPolicy -- real input is a zero-field struct; handleDescribeConfigurationTemplates/handleDescribeFieldIndexes -- structural stubs above; handleListIntegrations -- disclosed, low-impact per its own 'only one integration' doc comment). (bd: gopherstack-wksweep-cwl)"
deferred:
  - Insights query language/stages/parser correctness (insights_expr.go, insights_parse.go, insights_parser.go, insights_stages.go, insights_stats.go) -- not re-verified op-by-op against CloudWatch Logs Insights query syntax this pass.
  - Transformers, Integrations (PutIntegration/GetIntegration/ListIntegrations), Account Policies (top-level shapes spot-checked flat/no-nested-object-bugs, not exhaustively re-audited field-by-field op-by-op) -- see the "account policies, data protection/resource/index policies, transformers, integrations" family note. Resource Policies and Index Policies were subsequently field-diffed for real (gopherstack-enpq, second pass, 2026-08-21) and are no longer deferred -- see their op entries and the dated gaps note.
  - StartLiveTail streaming transport (intentionally out of scope; validation-only by design).
leaks: {status: clean, note: "Only one goroutine spawn site (scheduleFilterDelivery for subscription filter delivery), bounded by a semaphore + backend WaitGroup + ctx cancellation; Close()/Drain() join in-flight work. Janitor ticker is ctx-cancel safe via pkgs/worker. No unbounded per-request goroutines found in the areas audited this pass."}
---

## Notes

**2026-08-15 (gopherstack-3gbe):** investigated whether CloudWatch Logs
shares Omics' (gopherstack-keee) client-side host-prefix-rewrite
reachability gap. It does: **2 ops, one literal prefix, `stream-`**
(GetLogObject `api_op_GetLogObject.go:161`, StartLiveTail
`api_op_StartLiveTail.go:225`), confirmed against the pinned
`cloudwatchlogs@v1.81.1` module, exactly matching gopherstack-3gbe's filing.

No routing/auth code needed changing. `Handler.RouteMatcher`
(`handler.go:228`) matches on the `X-Amz-Target` header prefix
`"Logs_20140328."`, never `Host` or `Path`, so header-based dispatch is
structurally immune to the path-collision class this bug family could
otherwise cause. The reachability gap is a pure client-side DNS/dial
failure, same as Omics.

**This family is not the same shape as the other four services in
gopherstack-3gbe's filing.** Both real GetLogObject and StartLiveTail
responses are Smithy event streams (`GetLogObjectEventStream` /
`StartLiveTailResponseStream`), and this handler deliberately returns a
plain unary JSON body instead of real event-stream framing --
`handleStartLiveTail`'s existing doc comment already documents this as "a
streaming (HTTP/2 event-stream) operation that cannot be meaningfully
emulated over the standard unary JSON response". Confirmed live this pass:
once the dial problem is solved, an unmodified client's happy-path
StartLiveTail call still fails client-side with `unexpected output result
type: <nil>`, because the SDK's event-stream deserializer has nothing to
unpack. That is a separate, pre-existing, already-documented gap, not a
host-prefix-reachability bug -- out of scope here.

Added `host_prefix_reachability_test.go`: a before-fix test proving the
unmodified client can't dial either op, and an after-fix test that, via a
redial-to-the-real-listener transport (real, un-disabled rewrite left
intact on the wire), proves the request *does* reach gopherstack and gets
correctly authenticated/routed/validated -- both ops return the
correctly-typed AWS error (InvalidParameterException /
ResourceNotFoundException) for bad/missing input, decoded via the SDK's
ordinary unary-JSON error path, which is unaffected by the happy path's
event-stream gap. Gates green: build, vet, race, `go fix -diff` (no diff),
golangci-lint (0 findings).

- **2026-07-25 (parity-4 SDK-bump pass): implemented 10 new operations that appeared when
  the vendored aws-sdk-go-v2/service/cloudwatchlogs module was bumped from v1.64.0 to
  v1.80.0 -- three new families: lookup tables (CreateLookupTable/GetLookupTable/
  UpdateLookupTable/DeleteLookupTable/DescribeLookupTables), syslog configurations
  (PutSyslogConfiguration/ListSyslogConfigurations/DeleteSyslogConfiguration), and the
  account-level storage tier policy (GetStorageTierPolicy/PutStorageTierPolicy). All 10 are
  real implementations (new backend state, real CSV parsing, real validation, real
  persistence via the existing store.Registry "clean table" convention), not additions to
  the `notImplemented` tracking list. Two assumptions in the initial task framing turned out
  to be wrong once checked against the real SDK types directly, and are corrected here
  rather than silently followed: lookup tables carry their CSV content directly in
  CreateLookupTableInput/UpdateLookupTableInput.TableBody (a plain `*string`) with **no S3
  reference anywhere in the op's real input or output shape** (verified against
  serializers.go, which serializes `tableBody` as a bare JSON string) -- so this backend
  genuinely parses the CSV via `encoding/csv` rather than modeling an S3 reference it would
  need to fake; and the storage tier policy is **account-level, not per-log-group**
  (`GetStorageTierPolicyInput` is a zero-field struct, `PutStorageTierPolicyInput` carries
  only `StorageTier`, both confirmed by reading the real Input structs) -- kept as an
  independent concept from `LogGroup.LogGroupClass` rather than wired together, since
  nothing in the real API connects the two. `PutSyslogConfiguration` (the one family that
  genuinely does reference a log group) validates the log group actually exists via a
  region-scoped `groupGet` lookup and returns `ResourceNotFoundException` otherwise -- a
  real improvement in reference-validation rigor over this codebase's pre-existing
  `IndexPolicy`/`Transformer` completeness-pass ops, which accept any `logGroupIdentifier`
  string unchecked (noted as a `gaps` entry for awareness, not fixed here since those are
  out of scope for this pass). `DeleteLogGroup` now cascades to remove that log group's
  syslog configuration, matching its existing cascade to streams/subscription filters/metric
  filters. `go build`/`go vet`/`go test -race`/`gofmt`/`golangci-lint` all clean; 0 banned
  (`cyclop`/`gocyclo`/`gocognit`/`funlen`) nolints, same as before this pass. Overall grade
  held at A: the new surface is genuinely field-diffed and implemented, not stubbed, with
  honestly documented, narrow remaining gaps (VpcEndpointId not cross-validated against
  EC2 VPC-endpoint state -- no such modeling exists anywhere in this codebase to reuse; the
  LookupTable ARN pattern constructed by analogy to the log-group convention rather than
  confirmed against an authoritative source, since no smithy model ships with the installed
  SDK module).

- **2026-07-23 re-audit (this pass): closed the two remaining `gaps` from the prior pass
  (GetLogGroupFields sampling, PutLogEvents chronological-order/24h-span) and did a genuine
  field-diff (not a spot-check) of the previously-deferred export/import task, delivery,
  anomaly detector, and scheduled query families against the real SDK types.** This turned up
  several real, previously-unverified bugs beyond what the "spot-checked" status in the prior
  pass's `families` entry implied:
  - **Nested-vs-flat wire shape**, the most common bug class found this pass: `ExportTask`
    (`status`/`executionInfo` serialized as flat scalars instead of `{code,message}` /
    `{creationTime,completionTime}` objects) and `DeliveryDestination` (target ARN serialized
    as a flat string instead of nested under `deliveryDestinationConfiguration.destinationResourceArn`).
    Both were caught by comparing this package's hand-rolled `map[string]any` / struct-tag
    response construction against each type's real deserializer switch statement in
    aws-sdk-go-v2's generated `deserializers.go` -- the SDK's own struct field *doc comments*
    don't show wire nesting, only the deserializer's `case "key":` structure does.
  - **Wrong wire key** (right field, wrong JSON name): `ImportTask` used `"status"` instead of
    `"importStatus"`, `LogAnomalyDetector` used `"detectorStatus"` instead of
    `"anomalyDetectorStatus"`, `ScheduledQuery` used `"arn"` instead of `"scheduledQueryArn"`.
    Each of these meant a real SDK client's corresponding Go field always deserialized to its
    zero value from this backend's responses.
  - **Wrong wire key on the *input* side, more severe than an output-shape bug**:
    `PutDeliverySource`'s handler parsed `"resourceArns"` (a plural array key this backend
    invented) instead of the real `"resourceArn"` (a singular string, per
    `PutDeliverySourceInput.ResourceArn *string` and the real serializer). A real SDK client's
    request always carries `resourceArn`, so this backend's `ResourceArns` was silently empty
    for *every* real caller, not just mis-shaped in the response -- the resource association
    was completely dropped, not just displayed wrong.
  - **Invented enum values not in the real SDK**: `ImportTask`'s initial/cancellable status
    reused the shared `completenessStatusActive` constant (`"ACTIVE"`) -- correct for
    `IntegrationStatus`, but `"ACTIVE"` is not a member of `types.ImportStatus`
    (`IN_PROGRESS`/`CANCELLED`/`COMPLETED`/`FAILED`) at all, so `CancelImportTask`'s
    accepted-state check could never match a real import task's real status. A handler test
    also asserted a seeded status of `"SUCCEEDED"`, likewise not a real `ImportStatus` member
    (real: `COMPLETED`); fixed to `IN_PROGRESS`/`COMPLETED`.
  - **Orphaned invented fields with zero wire representation**: `LogAnomalyDetector` carried
    `EvaluationLookback`/`FilterAnomalies` fields with no equivalent anywhere in the real SDK
    (not in `types.AnomalyDetector`, not in any `api_op_*AnomalyDetector*.go` input, not in any
    doc comment) and no readers anywhere in this codebase either -- pure dead weight, removed
    (de-stub hygiene, same category as the `ErrInvalidSequenceToken` cleanup in the prior pass).
  - **A real feature gap disguised as "the field is just missing"**: `UpdateLogAnomalyDetector`
    never accepted `Enabled`, a *required* field on the real `UpdateLogAnomalyDetectorInput`
    used to pause/resume a detector. Since `PAUSED` is a real, reachable `AnomalyDetectorStatus`
    value that nothing in this backend ever set, a detector could never actually be paused
    through this API even though the status existed. Implemented: `enabled=false` -> `PAUSED`,
    `enabled=true` on a paused detector -> `ANALYZING` (steady-state resume, not a restart back
    to `INITIALIZING`).

  Two existing tests (`TestJanitor_SweepRetention`, `TestJanitor_SweepUpdatesStreamMetadata`)
  sent a single `PutLogEvents` batch spanning ~10 days to exercise retention-sweep eviction --
  real AWS would reject that whole batch outright once the 24-hour-span check went in, so they
  were split into two individually-valid calls (an old-events call, then a separate recent-event
  call) rather than weakening the new validation to keep the old test shape.

  Not everything found was fixed this pass -- see `gaps` for `ScheduledQuery`'s still-missing
  `GetScheduledQueryOutput` fields (including `executionRoleArn`, which is accepted as
  `CreateScheduledQuery` input and then silently discarded, never stored), `CreateDelivery`'s
  missing `FieldDelimiter`/`RecordFields`/`S3DeliveryConfiguration` input fields,
  `AccountPolicy`'s missing `AccountId`/`LastUpdatedTime`, and `DescribeDestinations`'s missing
  pagination -- these are real gaps, correctly still open, not reclassified to `ok`.
  (All four were closed in a later follow-up pass -- see the corresponding `gaps` entries above,
  now marked RESOLVED, for what changed.)
  `go build`/`go vet`/`go test -race`/`gofmt`/`golangci-lint` all clean before and after this
  pass; 0 banned (`cyclop`/`gocyclo`/`gocognit`/`funlen`) nolints, same as before.

- **PutLogEvents sequenceToken is a no-op today.** aws-sdk-go-v2 v1.64.0's own doc comments
  (api_op_PutLogEvents.go, types.InputLogEvent.UploadSequenceToken, types.InvalidSequenceTokenException)
  are explicit and repeated: "The sequence token is now ignored in PutLogEvents actions.
  PutLogEvents actions are always accepted and never return InvalidSequenceTokenException or
  DataAlreadyAcceptedException even if the sequence token is not valid." A previous audit pass
  (see the now-updated `ops_batch2_audit_test.go` comment) had deliberately added strict
  sequence-token validation, modeling the *pre-2022* contract. This sweep removed that
  validation; NextSequenceToken is still computed and returned (many older SDKs/tools still read
  it), it just no longer gates acceptance. If a future auditor sees "AWS returns
  InvalidSequenceTokenException for a stale token" in an older blog post or Stack Overflow
  answer, that's describing pre-Feb-2022 behavior -- trust the SDK doc comment over that.

- **RejectedLogEventsInfo field is `TooOldLogEventEndIndex`, not `...StartIndex`.** The real
  wire key is `tooOldLogEventEndIndex` (aws-sdk-go-v2 types.RejectedLogEventsInfo), and per the
  field doc it's an *exclusive end* index ("too-old events form a prefix of the batch; this is
  one past the last of them"), exactly mirroring `ExpiredLogEventEndIndex`'s semantics. A real
  SDK client unmarshalling our old `tooOldLogEventStartIndex` key would have silently gotten
  `nil` for `TooOldLogEventEndIndex`. Both `TooOldLogEventEndIndex` and `ExpiredLogEventEndIndex`
  are computed as `(highest matching event index) + 1`, not the first-matching index -- watch for
  this if the field is ever renamed again.

- **Metric filter `MetricValue` field-reference extraction is real, not literal-only.**
  `MetricTransformation.MetricValue` can be a literal number (published as-is) or a
  `$`-prefixed field reference: `$name` for a *named* field in a space-delimited pattern
  (`[ip, level, size]` makes `$ip`/`$level`/`$size` addressable) or `$.path` for a JSON
  selector pattern (`{ $.level = "ERROR" }` makes any `$.foo.bar` addressable, not just paths
  that appear in the match condition). This is implemented via
  `compiledFilterPattern.extract`/`extractString`/`extractValue`, fed by
  `compileJSONFilterPatternExtract` (filter_pattern_json.go) and
  `compileSpaceFilterPatternExtract` + `spaceFieldIndex` (filter_pattern_space.go, which now
  also tracks each `spaceTerm.name` -- previously discarded entirely, even for bare/unconditioned
  terms). **Trap:** `MetricTransformation.DefaultValue` is documented as "the value to emit when
  a filter pattern does NOT match a log event" (i.e. a substitute for a quiet period with zero
  matches), **not** a fallback for a matched event whose referenced field happens to be missing
  or non-numeric. The previous implementation conflated the two and used DefaultValue (or a bare
  `1.0`) as an extraction-failure fallback, fabricating metric data points that real CloudWatch
  Logs would never emit. The fix intentionally emits *nothing* for an extraction failure on a
  matched event; genuinely wiring "publish DefaultValue once per period when this filter had zero
  matches" would require a periodic scheduler (not per-PutLogEvents-call logic) and is left as a
  gap, not attempted this pass.

- **`putLogEventsMaxMessageBytes = 256 * 1024` looks suspicious next to aws-sdk-go-v2's doc
  comment ("Each log event can be no larger than 1 MB") but was deliberately left unchanged.**
  The older, still-maintained aws-sdk-go v1 model (`service/cloudwatchlogs/api.go`) explicitly
  says "Each log event can be no larger than 256 KB" in the same field's doc comment, matching
  this codebase's long-standing constant and the widely-documented CloudWatch Logs event-size
  quota. Treat the v2 SDK's "1 MB" doc text as smithy-model doc drift (likely bled in from the
  *batch* size limit description) rather than a real per-event limit increase, unless a more
  authoritative source (e.g. the AWS Service Quotas console page) is checked and says otherwise.

- **`compileFilterPattern`'s per-pattern-kind dispatch (`{`/`[`/plain-text) is the single
  source of truth for "does this pattern have addressable fields."** Anything touching
  metric-value extraction or `TestMetricFilter.ExtractedValues` should go through
  `compiledFilterPattern.extractString`/`extractValue` or `patternFieldRefs`, not re-parse the
  pattern text directly -- the existing JSON lexer/space-term parsing is reused rather than
  duplicated so the match and extract paths can't drift out of sync.

- **2026-07-11 re-audit (this pass): the diff from 17a215e4 to this commit is the "Phase 3.3"
  datalayer refactor** (backend.go/store_setup.go/region_accessors.go/persistence.go/
  janitor.go/export.go), replacing every hand-rolled `map[string]*T` / nested
  `map[string]map[string]*T` resource field with `pkgs/store.Table[T]` (+ `store.Index` for
  the region-qualified "dirty" tables: groups, streams, subscriptionFilters, metricFilters;
  events now live inline on `LogStream.events` instead of a separate `events` map). Verified
  op-by-op that every accessor was mechanically translated with no behavior change: locking
  discipline (coarse `b.mu` still guards every Table/Index access, matching `pkgs/store`'s
  "no internal locking" contract), in-place index-key-stable mutation (`PutSubscriptionFilter`
  update-in-path, `DeleteLogStream`/`applyEvictionPlan` decrementing `group.StoredBytes` through
  the same pointer), copy-before-delete-while-iterating on every `Index.Get()` consumer
  (`deleteStreamsInGroup` et al.), and DTO round-tripping of the four now-unexported identity
  fields (`region`, `logGroupName`, inline `events`) through `persistence.go`'s
  `logGroupSnapshot`/`logStreamSnapshot`/`subscriptionFilterSnapshot`/`metricFilterSnapshot`.
  No regression found; `go build`/`go vet`/`go test -race`/`golangci-lint` all clean before and
  after this pass. One hygiene fix made: `ErrInvalidSequenceToken` (backend.go) was an orphaned
  error var -- never returned by any op (PutLogEvents stopped validating sequenceToken in the
  prior sweep, see the "PutLogEvents sequenceToken is a no-op today" note above) and its
  `handler.go` errType mapping was already removed in that same prior sweep, so the var itself
  was dead code left behind; removed it (de-stub hygiene: no orphaned symbols).

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, three unbumped shape changes

`cwlSnapshotVersion` bumped 1 -> 2. Three registered tables' value types changed shape with
no bump applied, found via `pkgs/persistence`'s snapshot-version guard extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration:

- **`ca3afb3ca`**: `IndexPolicy.LastUpdated` (`time.Time`) retyped to `LastUpdateTime`
  (`int64`), same wire key `"lastUpdateTime"`. `PutIndexPolicy` genuinely sets this to
  `time.Now()`, so a pre-fix snapshot's RFC3339 string no longer unmarshals into the new
  `int64` field at all -- an outright decode error (loud) that takes down the whole restore.
- **`9f62f7f5d`**: `ScheduledQuery.Arn` retagged `"arn"` -> `ScheduledQueryArn` `"scheduledQueryArn"`.
  `scheduledQueryKeyFn` keys the `scheduledQueries` table on exactly this field, so a pre-fix
  snapshot's ARN silently decodes empty (silent, not loud) and every restored scheduled query
  collides onto the same `""` key -- the same collapse-to-one-record class as bedrock's
  `Flow`/`Prompt` bug (gopherstack-hjdd, same session).
- **`9f62f7f5d`**: `ImportTask.Status` retagged `"status"` -> `"importStatus"`, and
  `LogAnomalyDetector.DetectorStatus` retagged `"detectorStatus"` -> `AnomalyDetectorStatus`
  `"anomalyDetectorStatus"`. Neither field is part of its table's key, so both are narrower
  silent losses (the status field alone decodes empty) rather than a key collision.

**Examined and disqualified as bump candidates:**

- `357edbc07`'s `Delivery.CreationTime` `"creationTime"` -> `"-"` only affects an internal
  bookkeeping field used solely to order `DescribeDeliveries` results -- its own doc comment
  discloses it has no real-AWS wire counterpart at all ("an earlier revision fabricated one").
  Losing it on restore reorders a list; it does not destroy any field a real AWS client would
  observe on `Delivery` itself.
- `9f62f7f5d`'s `ImportTask.ImportRoleArn` `"importRoleArn"` -> `"-"` is a real bug but a
  different class: `json:"-"` excludes the field from **every** future snapshot unconditionally,
  old or new, so bumping the version constant cannot fix it (a fresh snapshot taken after the
  bump would still lose it). Filed separately rather than folded into this bump; needs its own
  fix (giving `ImportTask` a persistence-only DTO, mirroring `logGroupSnapshot` et al., that
  carries `ImportRoleArn` under an exported key distinct from the wire response's own shape).
- `567e2c4f8`'s `Anomaly` field renames (`suppressedState` -> `state`, plus several
  previously-absent required members) are moot for persistence: `Anomaly` is registered on
  `b.ephemeralRegistry` (`b.anomalies`), never included in `backendSnapshot` -- confirmed by
  `Snapshot`'s own doc comment, which lists `b.ephemeralRegistry` among the state deliberately
  excluded from every snapshot.

**Proof:** `TestInMemoryBackend_RestoreV1IndexPolicyLastUpdateTimeDiscarded` (persistence_test.go)
builds a v1-shaped `indexPolicies` snapshot with an RFC3339-string `lastUpdateTime` and asserts
`Restore` succeeds (discarding cleanly) rather than erroring; hand-reverted to version 1, the
same test fails with `Restore` returning `json: cannot unmarshal string into Go struct field
IndexPolicy.lastUpdateTime of type int64`, confirming the symptom.
`TestInMemoryBackend_RestoreV1ScheduledQueryArnDiscarded` builds a v1-shaped `scheduledQueries`
snapshot tagged `"arn"` and asserts `ListScheduledQueries` returns empty after restore;
hand-reverted, the same snapshot instead restores one scheduled query with `ScheduledQueryArn`
silently empty (while `Name`/`QueryString`/`State`, never renamed, restore correctly) --
confirming the predicted key-collision symptom. Both hand-reverts restored and
`md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).

## 2026-08-22 (gopherstack-enpq, third pass): doc-prescribed-usage sweep, four real bugs the field-diff passes missed

Continues gopherstack-enpq. `cmd/structfielddiff` had already been run against
this service twice (2026-08-14, 2026-08-21), covering all 118 ops by
comparing SDK field lists against this backend's structs. This pass applied
a different lens instead, carried over from the same day's kinesis pass on
this same bd issue: **read the SDK's doc prose for every op and ask whether
it can actually be called the way AWS's own documentation prescribes**, not
just whether the field lists match. kinesis's prior sweep had made the same
mistake -- comparing fields but never checking nine ops' documented "use
StreamARN" alternate path -- and this pass found the identical class of bug
here, plus one the field-diff method structurally cannot see at all.

**Module resolved:** `services/cloudwatchlogs` maps directly to
`aws-sdk-go-v2/service/cloudwatchlogs` -- confirmed absent from
`dirModuleOverride` in `cmd/structfielddiff`/`cmd/overwidecandidates`/
`cmd/requiredoutputfields`, and cross-checked against `go.mod`
(`v1.81.1`, matching this file's own `sdk_module` front matter) and the
pinned copy under `$(go env GOMODCACHE)`. This is a **different module and
directory** from the sibling `services/cloudwatch` (CloudWatch metrics/
alarms); there is no `services/cloudwatchevents` directory in this repo at
all.

**Collision check:** `git status --short services/cloudwatchlogs/` was
clean at the start of this session (a concurrent `cmd/keycheck` sweep was
touching other services), and re-checked clean after every fix in this pass.

**Four real bugs, all newly found this pass:**

1. **`CreateLookupTable`/`UpdateLookupTable`** -- `QueryId` (`*string`,
   `api_op_CreateLookupTable.go:55`/`api_op_UpdateLookupTable.go:47`: "You
   must specify either tableBody or queryId, but not both") had no Go field
   at all. `validateOpCreateLookupTableInput`/`validateOpUpdateLookupTableInput`
   require neither field client-side, so a real caller populating a lookup
   table from a completed query's results -- the doc-prescribed alternate to
   raw CSV -- reaches the wire unmodified and always fell through to
   "tableBody is required". Structurally absent, the same shape as kinesis's
   missing `StreamARN`. Fixed via `resolveLookupTableBody`/
   `lookupTableBodyFromQuery`: `QueryId` now renders real CSV from the
   query's `[][]ResultField` (header from the first row's field order).
2. **`UpdateAnomaly`** -- `PatternId` (`*string`, `api_op_UpdateAnomaly.go:12-19`:
   "You must specify either anomalyId or patternId... If you suppress a
   pattern, CloudWatch Logs won't report any anomalies related to that
   pattern") had no Go field at all, and `AnomalyId` was unconditionally
   required. `validateOpUpdateAnomalyInput` only requires
   `AnomalyDetectorArn` client-side, so the pattern-suppression path was
   structurally unreachable, always rejected with "anomalyId is required".
   Fixed: `PatternId` now suppresses/unsuppresses every stored anomaly
   sharing that pattern via the existing `anomalyByDetector` index.
3. **`ListAggregateLogGroupSummaries`** -- a prior pass (2026-08-14,
   gopherstack-wl0s) fixed this op's response *wrapper* key
   (`"logGroupSummaries"` -> `"aggregateLogGroupSummaries"`), but never
   re-checked the array *element* shape underneath: `AggregateLogGroupSummary`
   modeled fabricated per-log-group fields (`logGroupName`/`logGroupArn`/
   `logGroupClass`/`storedBytes`/`logEventCount`) that do not exist anywhere
   on the real `types.AggregateLogGroupSummary` at all (confirmed against
   `awsAwsjson11_deserializeDocumentAggregateLogGroupSummary`, which only
   recognizes `groupingIdentifiers`/`logGroupCount`). The real type is a
   **grouped bucket** (one entry per distinct data-source characteristic
   under the requested `groupBy`), not a per-log-group record -- so every
   response after the wrapper-key fix still returned N per-log-group entries
   with `GroupingIdentifiers`/`LogGroupCount` permanently nil/empty for a
   real client. This is the lesson worth naming: a correct-looking wrapper
   fix can hide a still-wrong shape underneath, and no structural field-diff
   catches it because the *wrapper's* declared fields (which this backend's
   invented per-log-group fields resembled in spirit) were never diffed
   against the *array element's* real type. This backend has no per-log-group
   data-source classification to group by, so the honest fix returns a
   single bucket covering all log groups (`GroupingIdentifiers` empty --
   disclosed, not fabricated; `LogGroupCount` real), or an empty list for
   zero log groups.
4. **`ListSourcesForS3TableIntegration`** -- an unconditional empty-list stub
   (`_ []byte` handler param) despite `AssociateSourceToS3TableIntegration`
   genuinely storing associations in `b.s3TableIntegrations` -- present but
   never populated on any read path, and structurally unreachable no matter
   what a caller associated. `AssociateSourceToS3TableIntegration` itself had
   a paired bug: `DataSourceName`/`DataSourceType` were parsed off the wire
   by the handler but discarded by the backend (`integrationArn, _, _
   string`), so even a fixed List op would have had nothing real to show.
   Both fixed together: `s3TableIntegrationEntry` gained `DataSourceName`/
   `DataSourceType`/`CreatedTimeStamp` (purely additive, `cwlSnapshotVersion`
   unchanged -- verified via `pkgs/persistence`'s `TestSnapshotVersionGuard
   -update`, which refuses to write the golden if a bump were actually
   warranted), and `ListSourcesForS3TableIntegration` now filters by the
   required `integrationArn`, paginates via `maxResults`/`nextToken`, and
   renders the real `types.S3TableIntegrationSource` shape.

**Test that ratified a defect, found and corrected:**
`TestHandler_S3TableIntegrationSourceOperations`'s
`ListSourcesForS3TableIntegration/ReturnsEmpty` case sent a request with no
`integrationArn` at all and asserted `200 OK` + empty list -- a call shape a
real client's own client-side validator (`IntegrationArn` is `required` on
`ListSourcesForS3TableIntegrationInput`) refuses to ever send. Renamed to
`.../ReturnsEmptyForUnknownArn` (now supplies a real, unmatched
`integrationArn`) and a new `.../MissingArn` case added asserting `400`.

**Rejected candidates (disqualifying rule in parens):** `ListIntegrations`'s
missing `IntegrationNamePrefix`/`IntegrationStatus`/`IntegrationType` filters
and `S3TableIntegrationSource`'s missing `ParentSourceIdentifier`/
`StatusReason` were both confirmed real but **disclosed, not fixed** (low
value / no backing model to derive them from honestly -- see `gaps`, not a
provability disqualification).

**Checked the other direction** (required-by-SDK-but-ignored, vs.
demanded-but-not-required): no new finding this pass -- `AssociateKmsKey`/
`DisassociateKmsKey`'s `ResourceIdentifier` alternate-target path (a similar
either/or shape to the four bugs above) was already correctly modeled from
an earlier pass, confirmed by direct inspection rather than assumed.

**Proof, every fix:** a real `aws-sdk-go-v2` client round trip
(`TestCreateLookupTable_FromQueryID`,
`TestCreateLookupTable_TableBodyAndQueryIDMutualExclusion`,
`TestUpdateAnomaly_PatternID`,
`TestUpdateAnomaly_AnomalyIDAndPatternIDMutualExclusion`,
`TestListAggregateLogGroupSummaries_RealShape`,
`TestListSourcesForS3TableIntegration_RealRoundTrip`), each hand-reverted
(`git show HEAD:<path>` restored into the scratch dir, symptom reproduced,
then `cp` back and `md5sum`-verified byte-identical) and confirmed to fail
against the unfixed code before restoring.

**Gates:** `go build ./...`, `go vet ./services/cloudwatchlogs/...`,
`gofmt -l` (clean), `go test ./services/cloudwatchlogs/... -race` (pass),
`go test ./pkgs/... -race` (pass, after `TestSnapshotVersionGuard -update`),
`go fix -diff` (no diff), `golangci-lint run ./services/cloudwatchlogs/...`
(0 issues after fixing 9 real findings -- golines, gosec G115, 2x
fieldalignment, 3x shadow, 2x testifylint `error-is-as` -- by refactoring,
not `//nolint`), `make build-check` (clean; `StorageBackend.UpdateAnomaly`'s
signature changed to add `patternID`).

Sweep status: 118/118 ops now covered by at least one structural pass (two
prior) plus this pass's doc-prose pass on every op with an either/or or
alternate-identifier doc pattern. Service remains grade A; four more real
gaps disclosed (see `gaps`), none rising to a grade change.

## gopherstack-o7gx follow-up (2026-08-22): default error path emitted an unmodeled InternalServerError

`handler.go`'s `handleError` default branch wrote `errType =
"InternalServerError"` for any unclassified 500. `cloudwatchlogs@v1.81.1`
does model an `InternalServerException` (`types/errors.go:97-116`,
`ErrorFault: FaultServer`), but it's wired into only 9 of 118 operation
error switches in `deserializers.go`. `ServiceUnavailableException`
(`types/errors.go:399-419`, also `FaultServer`) is wired into 101 of 118 --
including `CreateLogGroup`'s own `awsAwsjson11_deserializeOpErrorCreateLogGroup`
-- making it the real dominant 5xx fault for this service, not
`InternalServerException` (an easy mistake: both are legitimately-modeled
5xx faults for *some* cloudwatchlogs operations, but only one is the
service-wide default). Plain `"InternalServerError"` matches neither and
appears in no cloudwatchlogs SDK file at all.

Fixed to `errType = "ServiceUnavailableException"`. Proven with a real
`aws-sdk-go-v2/service/cloudwatchlogs` client's `CreateLogGroup`, whose
outgoing JSON body is corrupted to invalid syntax via a Finalize middleware
(`corruptJSONBody` -- a spec-compliant client can never organically send
malformed JSON, so this stands in for wire-level corruption; same technique
already used by `services/ce`, `services/dynamodb`, `services/apigateway`).
cloudwatchlogs's `handleError` has no `json.SyntaxError`/
`json.UnmarshalTypeError` case of its own, so the corrupted body's
unmarshal failure falls straight to default.
`TestCreateLogGroup_MalformedBodySurfacesServiceUnavailableException`
(`handler_error_type_test.go`, new) asserts `apiErr.ErrorCode() ==
"ServiceUnavailableException"` and
`errors.As(err, &types.ServiceUnavailableException{})` with `ErrorFault()
== smithy.FaultServer`; confirmed it fails pre-fix with the old
`"InternalServerError"` code (hand-reverted, byte-identical restore
after).

## 2026-08-28 — wrapper-key-sweep: request-side member-name/shape bugs (acceptguard)

`cmd/acceptguard` flagged two request-side bugs in `services/cloudwatchlogs/`
where the handler decoded a member real AWS never sends:

1. `DeleteScheduledQuery`, `UpdateScheduledQuery`, `GetScheduledQuery`, and
   `GetScheduledQueryHistory` all read `scheduledQueryArn` from the request
   body. The real member on all four Input types is `Identifier`
   (`cloudwatchlogs@v1.81.1` `api_op_{Delete,Update,Get,GetHistory}ScheduledQuery.go`,
   confirmed against each op's own `awsAwsjson11_serializeOpDocument*Input`
   in `serializers.go` -- wire key `"identifier"`). A real client sending
   `identifier` left the field permanently empty, so these four ops could
   never resolve the query a real client asked for -- the highest-value bug
   in this pass. Fixed by renaming the wire struct field/JSON tag to
   `Identifier`/`identifier` in `handler_scheduled_queries.go`; no alias
   kept, since nothing in-repo (UI, tests) depended on the old name --
   `ui/`'s `ScheduledQueryArn` usage under `timestreamquery/` is an unrelated
   service (Timestream Query's own, differently-shaped, `ScheduledQueryArn`
   member is real for *that* service).
2. `ListLogAnomalyDetectors` read a `filterLogGroupArnList` array. The real
   member is singular `FilterLogGroupArn *string`
   (`api_op_ListLogAnomalyDetectors.go`, wire key `"filterLogGroupArn"`).
   Fixed by changing the wire field to a single string and wrapping it in a
   one-element slice before calling the (unchanged) backend, which already
   took `[]string`.

Both proven via a real `aws-sdk-go-v2/service/cloudwatchlogs` client round
trip in `wire_field_fixes_test.go` (new): `TestScheduledQuery_IdentifierRealClient`
(Create → Get/Update/GetHistory/Delete all addressed by `Identifier`) and
`TestListLogAnomalyDetectors_FilterLogGroupArnRealClient` (two detectors on
different log groups, `FilterLogGroupArn` returns only the matching one).
Hand-reverted `handler_scheduled_queries.go`/`handler_anomaly_detectors.go`
only, confirmed both tests fail pre-fix (`GetScheduledQuery` 400
`InvalidParameterException: scheduledQueryArn is required`; filter returned
both detectors instead of one), restored the fix.

`handler_scheduled_queries_test.go`'s `TestHandler_GetScheduledQuery_WireShape`
and `TestHandler_ScheduledQuery_DestinationConfiguration` sent the wrong
`scheduledQueryArn` request key directly as raw JSON -- updated both to send
`identifier`, the real member.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/cloudwatchlogs/...`).

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

Audited every pagination helper: `paginateStreams`/`paginateGroups` (both correct,
boundary-tested) and the ~19 handler-level clones of the same
`parseNextToken`/encodeNextToken`/start-end` block (`ListLogAnomalyDetectors`,
`ListAnomalies`, `DescribeAccountPolicies`, `DescribeDeliveries`, `DescribeDestinations`,
`DescribeExportTasks`, `DescribeImportTasks`, `ListSourcesForS3TableIntegration`,
`ListSyslogConfigurations`, `DescribeLookupTables`, `DescribeResourcePolicies`,
`DescribeMetricFilters`, `DescribeQueries`, `ListScheduledQueries`,
`GetScheduledQueryHistory`, `DescribeSubscriptionFilters`, plus handler-level
`GetQueryResults`/`ListLogGroupsForQuery`) — all read and confirmed byte-identical and
correct; no shared helper factored out, but no bug either (contrast with `services/workspaces`,
which had the same "many shallow copies" shape but a real missing-cursor bug in each copy).

**Bug found and fixed:** `InMemoryBackend.GetLogEvents` (`log_events.go`) — its bidirectional
forward/backward pagination computed `startIdx` from `nextToken` with no upper-bound clamp,
unlike every sibling method above (which all guard `if startIdx >= len(all) { return empty }`
before slicing). A `nextToken` naming an offset past the current event count — e.g. minted
before the retention janitor swept older events out from under it, or simply a
corrupted/replayed token — panicked with "slice bounds out of range" on
`filtered[startIdx:end]`. Fixed by clamping `startIdx = min(startIdx, len(filtered))` before
computing `end`. `FilterLogEvents`'s equivalent pagination was checked and found
self-correcting by construction (its `end`/`startIdx` clamps compose safely even though
computed in an unusual order) — no change needed there.

Proof: `TestCloudWatchLogsBackend_GetLogEvents_StaleTokenPastEnd` (log_events_test.go, unit)
and `TestGetLogEvents_SDKRoundTrip_StaleNextTokenDoesNotPanic` (pagination_sdk_roundtrip_test.go,
real `aws-sdk-go-v2/service/cloudwatchlogs` client) both reproduce the panic pre-fix.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/cloudwatchlogs/...`).

## 2026-08-30 sort-totality sweep (wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

Audited every `sort.Slice`/`sort.SliceStable` call for whether its comparator
is a *total* order, not just whether the pagination arithmetic around it is
correct (the 2026-08-29 sweep above checked the arithmetic; this pass asked
a different question). The mechanism: a listing sorts on a field that admits
ties, with no secondary key, over `store.Table.All()` (unordered map
iteration per Go's randomised range), so two calls in the same paginated
walk can disagree about relative order and drop or duplicate a record across
a page boundary with nothing changed in between.

**Fixed (non-total sort, tiebreak added):**

- `ListLogAnomalyDetectors` — sorted on `CreationTimeStamp` alone; two
  detectors created in the same millisecond tied. Added `AnomalyDetectorArn`
  (the table's own key) as tiebreak.
- `ListAnomalies` — sorted on `FirstSeen` alone. Added `AnomalyID` tiebreak.
- `DescribeDeliveries` — sorted on `CreationTime` alone. Added `ID` tiebreak.
- `DescribeExportTasks` / `DescribeImportTasks` — both sorted on
  `CreationTime` alone. Added `TaskID`/`ImportID` tiebreak respectively.
  (`DescribeExportTasks` also decomposed: the CreationTime→Status aging loop
  was pulled into `advanceExportTaskStatesLocked`, since adding the tiebreak
  pushed the combined function's gocognit complexity from 20 to 22 —
  confirmed by lint against the pre-fix file before decomposing, not
  guessed.)
- `ListSourcesForS3TableIntegration` — sorted on `CreatedTimeStamp` alone.
  Added `ID` tiebreak.
- `ListScheduledQueries` — sorted on `CreationTime` alone. Added
  `ScheduledQueryArn` tiebreak.
- `DescribeResourcePolicies` — sorted on `PolicyName` alone, but PolicyName
  is only unique *within* the ACCOUNT scope; two RESOURCE-scoped policies
  (keyed by `policyName+resourceArn`) can legitimately share a PolicyName.
  Added `ResourceArn` tiebreak. See the corrected op-level note above — an
  earlier note claiming this op's pagination itself was unimplemented was
  stale (fixed by the 2026-08-28/29 pass) and has been corrected in place.
- `DescribeQueryDefinitions` — sorted on `Name` alone; `PutQueryDefinition`
  does not (and per real AWS should not) enforce name uniqueness, only
  `QueryDefinitionID` uniqueness. Added `QueryDefinitionID` tiebreak.
- `DescribeLogStreams(orderBy=LastEventTime)` — the "caller selects the sort
  attribute" shape called out for this audit: the default order
  (`LogStreamName`, the table's own primary key) was already total, but the
  `LastEventTime` branch had no secondary key at all, and streams with no
  events share `LastEventTimestamp == nil` (0) by construction, so a tie is
  the ordinary case for freshly created streams, not a contrived one. Added
  `LogStreamName` tiebreak. Note: this source is `streamsByGroup.Get`
  (an `Index`, insertion-order-stable absent an intervening delete), not
  `Table.All()`, so a static-state 30x walk does not observe instability the
  way the map-backed sites above do — the fix is still correct on its own
  terms (the comparator was non-total regardless), see `pagination_sort_totality_test.go`'s
  doc comment on this test for the full reasoning.

**Confirmed correct, left unfixed (evidence, not presumption):**

- `FilterLogEvents`'s cross-stream timestamp interleave and `exportWindowEvents`
  (export.go) — both sort a per-stream `events []*OutputLogEvent` slice field
  that is strictly append-ordered (never rebuilt from a map/index), using
  `sort.SliceStable`, with the stream visitation order itself
  (`filterStreamOrderLocked`) sorted on `LogStreamName` (a unique key). Same
  shape as the `ram`-listings precedent from the prior pass: append-ordered
  source + stable sort means a tied-timestamp pair's relative order is a
  fixed function of insertion order, reproducible across repeated calls with
  no intervening mutation. Not fixed; not observably unstable.
- `GetScheduledQueryHistory` — sorts `ScheduledQueryRunSummary.InvocationTime`
  descending, source is `history.Runs`, an append-only slice field
  (`history.Runs = append(history.Runs, &r)`, never rebuilt from a table).
  Same append-ordered-source reasoning as above; not fixed.
- `GetLogGroupFields` — sorts `Percent` then `Name`; not paginated (single
  full response), and its `fieldCounts` map keys (Name) are inherently
  unique, so the existing `Name` tiebreak already makes this total.
- Every `Name`/`FilterName`/`DestinationName`/`LogGroupName`/`LogGroupIdentifier`-keyed
  sort not listed above (~~`data_protection.go`,~~ `destinations.go`,
  `subscription_filters.go`, `log_groups.go`, `lookup_tables.go`,
  `syslog_configurations.go`, `integrations.go`'s `ListIntegrations`,
  `deliveries.go`'s Describe*Destinations/*Sources) sorts on the same field
  the backing `store.Table`'s `keyFn` uses as the primary key (confirmed
  against `store_setup.go`), so it is unique by construction — a duplicate
  value cannot exist because `Table.Put` would overwrite it. No tie is
  possible; nothing to fix.
  CORRECTED (2026-08-30, exhaustive field sweep, gopherstack-wksweep-cwl):
  `data_protection.go` was wrongly included in this "unique by construction"
  list -- `accountPolicyKeyFn` (`store_setup.go`) is
  `p.PolicyName + ":" + p.PolicyType`, not `PolicyName` alone, so two
  `AccountPolicy` rows CAN legitimately share a `PolicyName` (different
  `PolicyType`) and `DescribeAccountPolicies`' `PolicyName`-only sort was
  genuinely non-total. This was a real, reproducible bug (not merely
  theoretical) -- see the fixed `DescribeAccountPolicies` ops: entry above
  and `TestDescribeAccountPoliciesSortIsTotal`. Struck through rather than
  silently deleted, per this file's own audit-trail convention for a
  superseded claim.
- The Insights query-language `sort` command (`insights.go`'s `sortStage`,
  `sort.SliceStable`) was reviewed and judged out of scope for this class:
  it orders one query execution's already-fixed result set once, not a
  resource listing paginated with a cursor across independent calls.

**Existing test-suite weakness confirmed:** the pagination arithmetic tests
from the 2026-08-28/29 sweep (`wire_field_fixes_test.go`) assert page sizes
and truncation/duplicate-free-union only for the three ops they targeted —
none of the existing pagination tests in this package construct a tie group
and compare item *identity* across a full multi-page walk with a small page
size, which is exactly the shape needed to catch this class. New tests
(`pagination_sort_totality_test.go`) fill that gap for the ops fixed above,
looping each 30x per the reasoning that map-iteration instability shows up
across separate calls, not within one.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/cloudwatchlogs/...`).

## 2026-08-30 (gopherstack-uox6, value-semantics pass)

Audited this service's hand-rolled matchers/filters/comparisons for a
different question than every prior sweep: not "is the field read" but "is
the field applied correctly." This axis was declared closed by earlier
field-coverage passes; it is not the same axis.

**Matchers audited** (all against the pinned `cloudwatchlogs@v1.81.1` SDK,
doc comments unless noted): `compileFilterPattern`/`compiledFilterPattern.matches`
(filter_patterns.go) — required/exclude/optional term combining, `?`-ignored-
when-combined-with-other-terms rule, quoted-exact vs wildcard-regex term
compilation; `compileSpaceFilterPattern` (filter_pattern_space.go) — `[...]`
positional/ellipsis alignment, `=`/`!=`/`<`/`<=`/`>`/`>=` operators, `*`
wildcard on `=`; `compileJSONFilterPattern` (filter_pattern_json.go) — `{...}`
selector AST, `&&`/`||`, exists/not-exists, wildcard string equality, numeric
comparators; `FilterLogEvents`'s StartTime/EndTime bounds (log_events.go) —
confirmed inclusive on both ends per `api_op_FilterLogEvents.go`'s "Events
with a timestamp before/later than this time are not returned" wording;
`metricFilterMatches`/`DescribeMetricFilters` (metric_filters.go);
`DescribeSubscriptionFilters`'s prefix filter (subscription_filters.go);
`DescribeLogStreams`'s orderBy/descending validation (log_streams.go). All
were correct **except** the one below.

**Bug found — under-application direction, new to this class's catalog.**
Every prior instance of "a documented modifier ignored" had the modifier
under-applied (negation, `?`, an operator) so real matches were missed. This
one runs the other way: `DescribeMetricFiltersInput.FilterNamePrefix`'s doc
comment says the prefix is used "only if you also include the logGroupName
parameter" — i.e. it must be a no-op without a log group, not a global name
filter. `metric_filters.go`'s `DescribeMetricFilters` applied it
unconditionally, so a caller passing `filterNamePrefix` alone (no
`logGroupName`) got results wrongly narrowed to that prefix instead of every
metric filter in the account. Fixed by zeroing the effective prefix when
`logGroupName` is empty. Test: `TestDescribeMetricFilters_FilterNamePrefixIgnoredWithoutLogGroupName`
(`metric_filters_prefix_scope_test.go`), drives the real SDK client, confirmed
failing pre-fix (only the prefix-matching filter was returned; the other was
wrongly excluded).

**Every StartTime/EndTime-shaped comparison checked for format**: all of
FilterLogEvents/GetLogEvents/metric-filter time windows compare epoch-
milliseconds `int64` against `OutputLogEvent.Timestamp` (also epoch-ms) — no
format mismatch found anywhere in this service; the self-inconsistent
nanoseconds-vs-seconds shape from ec2/sagemaker does not recur here.

**Adjacent, out-of-class finding, NOT fixed, flagged for the field-coverage
owner instead of acted on here**: `filterLogEventsInput` (handler_log_events.go)
has no `StartFromHead` field at all, though `FilterLogEventsInput` (the real
SDK type) documents one affecting sort direction. This is a field never
decoded, not a field read-and-misapplied — the wrong axis for this pass, and
PARITY's existing `FilterLogEvents: {wire: ok, ...}` line does not disclose
it. Recorded here rather than silently left for a future sweep to rediscover.

**Web pages fetched: 0.** Everything needed for cloudwatchlogs' filter/time
semantics was already in the pinned SDK's Go doc comments.

Gates re-run after this pass: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./services/cloudwatchlogs/...`, `golangci-lint run
./services/cloudwatchlogs/...` — all clean.

## 2026-08-31 cmd/errtargetaudit sweep: real-code-wrong-operation class, 22 findings, all real

`go run ./cmd/errtargetaudit -dir cloudwatchlogs` (219/230 real declared codes
resolved for real cloudwatchlogs ground truth, ignore the header's inflated
230/258-style module list which also drags in `s3`'s SDK for `export_s3.go`'s
import) reported 22 class-A findings: a real, correctly-spelled code
(`InvalidParameterException`) sent by 21 operations that don't declare it,
plus one (`ResourceNotFoundException`) sent by an operation that declares
neither.

**THE FAMILY.** cloudwatchlogs' Delivery/DeliveryDestination/DeliverySource/
ScheduledQuery/S3TableIntegration operations (21 of them: AssociateSourceToS3TableIntegration,
CreateDelivery, CreateScheduledQuery, DeleteDelivery,
DeleteDeliveryDestination, DeleteDeliveryDestinationPolicy,
DeleteDeliverySource, DeleteScheduledQuery,
DisassociateSourceFromS3TableIntegration, GetDelivery,
GetDeliveryDestination, GetDeliveryDestinationPolicy, GetDeliverySource,
GetScheduledQuery, GetScheduledQueryHistory, ListSourcesForS3TableIntegration,
PutDeliveryDestination, PutDeliveryDestinationPolicy, PutDeliverySource,
UpdateDeliveryConfiguration, UpdateScheduledQuery) all declare
`ValidationException` for parameter/JSON-shape failures in their own
`awsAwsjson11_deserializeOpError<Op>` switch (confirmed per-op against
`aws-sdk-go-v2/service/cloudwatchlogs@v1.81.1/deserializers.go`) — none of
them declares `InvalidParameterException`, the older code the rest of this
service's ops use. The emulator's `ErrValidation` sentinel
(`InvalidParameterException`) was reused at every one of these 37 call sites
across `deliveries.go`, `scheduled_queries.go`, `integrations.go`,
`handler_deliveries.go`, `handler_integrations.go`. A second, unused-until-now
sentinel, `ErrValidationException` (`ValidationException`), already existed
in `errors.go` and was already wired into `handleError` — but its doc comment
claimed it covered only "a small set of operations... e.g.
ListAggregateLogGroupSummaries", undercounting its real scope by ~20
operations (the campaign's recurring false-doc-comment pattern; corrected).
Fixed by switching all 37 call sites to `ErrValidationException`, per-op, not
by touching the shared sentinel other legitimate `InvalidParameterException`
callers (PutIntegration/GetIntegration/DeleteIntegration, whose deserializers
genuinely include `InvalidParameterException`) still use.

**ONE SITE INSIDE THE SAME FAMILY NEEDED A THIRD CODE, NOT THE FAMILY'S
DEFAULT.** `CreateScheduledQuery`'s quota check ("scheduled query limit
exceeded") was also routed through `ErrValidation`. Its own deserializer
declares `ServiceQuotaExceededException` specifically (alongside
`ValidationException`), a semantically exact match a blanket
`ErrValidation`→`ErrValidationException` swap would have missed. Added a new
sentinel, `ErrScheduledQueryLimitExceeded` (`ServiceQuotaExceededException`),
used only at that one call site.

**PutDestinationPolicy is the sibling-trap shape.** It shares the
`ErrDestinationNotFound` sentinel (`ResourceNotFoundException`) with
`DeleteDestination`, correct for `DeleteDestination` (declares
`ResourceNotFoundException`) but not for `PutDestinationPolicy`, whose own
declared set is `{InvalidParameterException, OperationAbortedException,
ServiceUnavailableException}` — no not-found type at all. Overridden at the
`PutDestinationPolicy` call site only (now `ErrValidation` →
`InvalidParameterException`, 400 instead of 404); `ErrDestinationNotFound`
itself is untouched and stays correct for `DeleteDestination`.

**Existing tests corrected, assertion counts identical (one assertion each,
before and after):** `deliveries_test.go` (GetDelivery/`get_empty_id`,
PutDeliveryDestination×2, PutDeliverySource/`put_empty_name_errors`),
`scheduled_queries_test.go` (UpdateScheduledQuery, GetScheduledQuery,
GetScheduledQueryHistory), `destinations_test.go`
(`put_policy_not_found_errors`, sentinel corrected from
`ErrDestinationNotFound` to `ErrValidation`). One HTTP-level test asserted
only a status code and could not have detected this class regardless of
value: `handler_destinations_test.go`'s `PutDestinationPolicy/NotFound`
expected 404; corrected to 400 and renamed
`PutDestinationPolicy/InvalidParameter`. All four failed against the
pre-fix source (confirmed by running the suite before touching the tests);
zero assertions dropped.

Gates: `go build ./services/cloudwatchlogs/...`, `go vet
./services/cloudwatchlogs/...`, `go test -race -count=1
./services/cloudwatchlogs/...` (pass), `golangci-lint run
./services/cloudwatchlogs/...` (0 issues).
