---
service: iotanalytics
sdk_module: aws-sdk-go-v2/service/iotanalytics@v1.32.0
last_audit_commit: bb9dd1e99
last_audit_date: 2026-09-04  # gopherstack-2wb parity sweep: fixed CreatePipeline/UpdatePipeline
  # missing PipelineActivities 2-25/channel+datastore shape validation -- see
  # CreatePipeline/UpdatePipeline ops entries and the 2026-09-04 Notes section below.
overall: A            # wrapper-key/nested-shape sweep: fixed DescribeChannel/DescribeDatastore statistics sibling-key nesting, CreateDatastore/DescribeDatastore datastorePartitions wire key, 4 fabricated summary ARNs, fabricated GetDatasetContent versionId, fabricated IotSiteWise roleArn -- zero remaining wrapper-key bugs found
                       # ---- query/header-to-non-string-field sweep (2026-08-29) ----
                       # Hunted for query/header/path values fed into a non-string Go field
                       # without conversion. No merging-into-JSON-body pattern (query values are
                       # read individually, not merged into the JSON body then unmarshaled).
                       # Inventoried every non-string query member across all 34 ops:
                       # maxResults/*int32 (5 List ops, correct via parsePagination),
                       # maxMessages/*int32 and includeStatistics/bool (correct),
                       # scheduledBefore/scheduledOnOrAfter (*time.Time, correct via
                       # parseQueryDateTime). Found and fixed one SILENT (inert) bug:
                       # SampleChannelData's StartTime/EndTime (*time.Time) were declared but
                       # never read -- see SampleChannelData row. Also hardened
                       # DescribeChannel/DescribeDatastore's includeStatistics from a naive =="true"
                       # (correct only by accident, since the real SDK always emits lowercase) to
                       # strconv.ParseBool -- see those rows. No hard-fail (500) bugs found.
ops:
  CreateChannel: {wire: ok, errors: ok, state: ok, persist: ok, note: "now validates tags (key/value charset, aws: prefix, max 50) before create, matching TagResource"}
  DescribeChannel: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20: statistics was nested inside the channel object; AWS returns it as a sibling top-level member (deserializers.go:1851 awsRestjson1_deserializeOpDocumentDescribeChannelOutput has separate channel/statistics cases; awsRestjson1_deserializeDocumentChannel has no statistics case at all). Hardened 2026-08-29: includeStatistics (real bool query param, api_op_DescribeChannel.go:46) was compared with a naive == \"true\", correct only because the real SDK's Boolean() query encoder always emits lowercase (smithy-go@v1.27.6 httpbinding/query.go:43-45) -- correct by accident, not construction. Now strconv.ParseBool via queryBool (handler.go), so a non-Go caller sending \"TRUE\"/\"1\" also works."}
  UpdateChannel: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteChannel: {wire: ok, errors: ok, state: ok, persist: ok}
  ListChannels: {wire: ok, errors: ok, state: ok, persist: ok, note: "cursor pagination correct: Snapshot() is Name-ascending, cursor thresholds on Name. FIXED 2026-08-20: channelSummary fabricated a channelArn member ChannelSummary doesn't have (deserializers.go:5795 awsRestjson1_deserializeDocumentChannelSummary has no arn case) -- removed"}
  SampleChannelData: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (query/header-to-non-string-field sweep): StartTime/EndTime (real *time.Time query params, api_op_SampleChannelData.go:45,56, serializers.go:2184-2192) were never read at all -- messages carried no arrival timestamp, so every stored message always came back regardless of the time window a client asked for. Backend now records each message's arrival time (ChannelMessage.ArrivedAt, whole-second resolution matching every other stored timestamp in this backend) and SampleChannelData filters by [startTime, endTime] when set. Snapshot version bumped 1->2 (channelMessages value shape changed [][]byte -> []ChannelMessage). Proven by TestSampleChannelData_StartTimeExcludesEarlierMessages (wire_field_fixes_test.go), driven through the real SDK client."}
  CreateDatastore: {wire: ok, errors: ok, state: ok, persist: ok, note: "now validates tags before create (see CreateChannel). FIXED 2026-08-20: request read the partitions member under the wrong wire key 'partitions'; AWS's key is 'datastorePartitions' (serializers.go:583 awsRestjson1_serializeOpDocumentCreateDatastoreInput) -- a real client's DatastorePartitions was silently dropped on create"}
  DescribeDatastore: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20: statistics was nested inside the datastore object; AWS returns it as a sibling top-level member (deserializers.go:2184 awsRestjson1_deserializeOpDocumentDescribeDatastoreOutput has separate datastore/statistics cases). FIXED 2026-08-20: datastoreDetail also emitted partitions under 'partitions' instead of AWS's 'datastorePartitions' (deserializers.go:7177 awsRestjson1_deserializeDocumentDatastore) -- a real client's Datastore.DatastorePartitions stayed nil even when the backend had partitions stored. Hardened 2026-08-29: same includeStatistics accident-correct-boolean fix as DescribeChannel -- see that row."}
  UpdateDatastore: {wire: ok, errors: ok, state: ok, persist: ok, note: "updateDatastoreRequest still accepts a 'partitions' body field UpdateDatastoreInput has no real counterpart for (api_op_UpdateDatastore.go:32 UpdateDatastoreInput: DatastoreStorage/FileFormatConfiguration/RetentionPeriod only, no partitions member) -- disclosed, not fixed, see Notes"}
  DeleteDatastore: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDatastores: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20: datastoreSummary fabricated a datastoreArn member DatastoreSummary doesn't have (deserializers.go:7677 awsRestjson1_deserializeDocumentDatastoreSummary has no arn case) -- removed. FIXED 2026-08-23: datastoreSummary was also missing DatastoreSummary's real datastorePartitions/fileFormatType members (types.go:952). Confirmed against the real *DatastoreSummary type (not the Datastore detail type, which has no fileFormatType member at all -- types.go:707) before adding: datastorePartitions now round-trips from Datastore.Partitions, fileFormatType is derived from FileFormatConfiguration (defaulting to JSON, matching 'The default file format is JSON' in api_op_CreateDatastore.go's doc comment, since gopherstack never persisted a resolved format type before). See TestListDatastores_SummaryCarriesPartitionsAndFileFormatType (list_summaries_missing_members_test.go)."}
  CreateDataset: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now validates tags before create (see CreateChannel). FIXED 2026-08-23 (manifest-harvest pass): CreateDatasetInput/CreateDatasetOutput/Dataset's real retentionPeriod member (deserializers.go:6271 awsRestjson1_deserializeDocumentDataset, api_op_CreateDataset.go:75/117) was accepted on the wire (createDatasetRequest had no field for it) then silently dropped -- an accept-and-drop gap, same class as Datastore's already-correctly-modeled RetentionPeriod (this backend already had the RetentionPeriod type + validateRetentionPeriod/cloneRetentionPeriod helpers from Datastore; Dataset just never used them). Fixed: Dataset gained RetentionPeriod, CreateDataset validates and stores it, both CreateDatasetOutput and DescribeDatasetOutput echo it. DatasetSummary (ListDatasets) correctly has no retentionPeriod member on the real SDK type -- not part of this fix. Verified via TestDataset_RetentionPeriod_RoundTrips, driven through the real aws-sdk-go-v2 client, hand-reverted (datasets.go/handler_datasets.go/models.go/interfaces.go) to confirm it fails against unfixed code (nil RetentionPeriod on both responses), restored, md5sum identical. Additive-only struct field; pkgs/persistence snapshot-version guard confirmed no bump needed."}
  DescribeDataset: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same Dataset.retentionPeriod fix as CreateDataset -- see above."}
  UpdateDataset: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDataset: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDatasets: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20: datasetSummary fabricated a datasetArn member DatasetSummary doesn't have (deserializers.go:7011 awsRestjson1_deserializeDocumentDatasetSummary has no arn case) -- removed. FIXED 2026-08-23: datasetSummary was also missing DatasetSummary's real actions/triggers members (types.go:652). Confirmed against the real *DatasetSummary type before adding: actions uses the narrower datasetActionSummary shape (types.go:522 DatasetActionSummary -- actionName/actionType only, ActionType derived from which of QueryAction/ContainerAction the full DatasetAction carried, not the full action body), triggers reuses DatasetTrigger unchanged (same type real AWS uses in both the summary and detail view). See TestListDatasets_SummaryCarriesActionsAndTriggers (list_summaries_missing_members_test.go)."}
  CreatePipeline: {wire: ok, errors: ok, state: ok, persist: ok, note: "now validates tags before create (see CreateChannel). FIXED 2026-09-04 (gopherstack-2wb): CreatePipelineInput.PipelineActivities is documented as \"The list can be 2-25 PipelineActivity objects and must contain both a channel and a datastore activity. Each entry in the list must contain only one activity\" (api_op_CreatePipeline.go) -- the SDK's client-side validator (validatePipelineActivity, validators.go) only checks each activity's own required sub-fields, not this aggregate shape, so a real typed client could send (and gopherstack silently accepted) a pipeline with zero, one, >25, or channel-only/datastore-only activities. Now enforced server-side via validatePipelineActivities (store.go), returning InvalidRequestException. See TestCreatePipeline_RequiresChannelAndDatastoreActivity (wire_field_fixes_test.go)."}
  DescribePipeline: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePipeline: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-09-04 (gopherstack-2wb): same PipelineActivities 2-25/channel+datastore shape as CreatePipeline is documented as required on UpdatePipelineInput too (api_op_UpdatePipeline.go) -- now enforced when a non-nil activities slice is supplied. A nil/omitted activities body still leaves the pipeline unchanged (a real client always sends this required field; nil is only reachable via a raw HTTP caller omitting it)."}
  DeletePipeline: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPipelines: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20: pipelineSummary fabricated a pipelineArn member PipelineSummary doesn't have (deserializers.go:9310 awsRestjson1_deserializeDocumentPipelineSummary has no arn case) -- removed"}
  StartPipelineReprocessing: {wire: ok, errors: ok, state: ok, persist: ok}
  CancelPipelineReprocessing: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchPutMessage: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDatasetContent: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED: now accepts and honors an explicit versionId body field (CreateDatasetContentInput.VersionId), previously silently discarded -- duplicate explicit versionId against the same dataset now returns ResourceAlreadyExistsException instead of being accepted. Still always synchronously SUCCEEDED (no CREATING/FAILED simulation) -- acceptable simplification, see Notes"}
  GetDatasetContent: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED: now honors $LATEST / $LATEST_SUCCEEDED (uppercase, as sent by the SDK) in addition to an omitted versionId; previously matched only a non-wire-accurate lowercase '$latest'. FIXED 2026-08-20: response fabricated a versionId member GetDatasetContentOutput doesn't have (deserializers.go:2681 awsRestjson1_deserializeOpDocumentGetDatasetContentOutput: entries/status/timestamp only) -- removed, along with the test that asserted on its presence"}
  ListDatasetContents: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED: pagination cursor was VersionID-threshold (random UUID, unrelated to the CreationTime-descending sort) -- now offset-based. FIXED: underlying sort used slices.SortFunc (unstable) over second-resolution timestamps, so tied entries could reorder between calls -- now slices.SortStableFunc with a reversed-input tiebreak (see Notes). FIXED: scheduleTime was missing entirely from DatasetContentSummary (a real field, distinct from creationTime) and the scheduledBefore/scheduledOnOrAfter query filters were unimplemented -- both now implemented (see Notes)"}
  DeleteDatasetContent: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED: omitted versionId previously deleted ALL content versions; AWS defaults to $LATEST_SUCCEEDED (exactly one version). Now also honors explicit $LATEST / $LATEST_SUCCEEDED"}
  DescribeLoggingOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  PutLoggingOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  RunPipelineActivity: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED: addAttributes/removeAttributes/selectAttributes/filter/math now perform real per-activity transforms (see Notes and pipeline_expr.go); channel/datastore remain pass-through (correct: real source/sink activities); lambda/deviceRegistryEnrich/deviceShadowEnrich now invoke the real Lambda/IoT backends when cli.go's wireIoTAnalyticsCrossService has wired them (see Notes); math now supports the documented math-operators-functions.html function library; filter's LIKE/IN/BETWEEN remain unimplemented -- see items_still_open"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  routing: {status: ok, note: "RouteMatcher + parseIoTAnalyticsPath verified path-prefix and HTTP-method-for-method against every awsRestjson1_serializeOpHttpBindings*/request.Method in aws-sdk-go-v2/service/iotanalytics@v1.32.0/serializers.go -- all 34 ops match (paths, GET/POST/PUT/DELETE, query param names incl. includeStatistics/maxMessages/maxResults/nextToken/resourceArn/tagKeys/versionId/scheduledBefore/scheduledOnOrAfter)"}
  timestamps: {status: ok, note: "creationTime/lastUpdateTime/lastMessageArrivalTime/completionTime/startTime/endTime/scheduleTime all epoch-seconds JSON numbers (awstime-equivalent; models.go epochSeconds), matches smithytime.ParseEpochSeconds/FormatEpochSeconds in the real deserializers/serializers"}
gaps:
  - "GetDatasetContent always returns an empty entries array (no S3-backed data URIs) since this backend has no S3 delivery integration -- consistent with CreateDatasetContent's synchronous SUCCEEDED simulation, not tracked as a bug."
  - "items_still_open: RunPipelineActivity's lambda/deviceRegistryEnrich/deviceShadowEnrich now invoke the real Lambda/IoT backends (cli.go's wireIoTAnalyticsCrossService -> InMemoryBackend.SetLambdaBackend/SetThingRegistry/SetThingShadowStore, following the same LambdaInvoker pattern SNS/Firehose/SecretsManager already use). lambda batches payloads by BatchSize and round-trips a JSON object array through InvokeFunction, matching the documented contract (docs.aws.amazon.com/iotanalytics/latest/userguide/pipeline-activities-lambda.html: \"the Lambda function must receive and return a JSON object array\"); deviceRegistryEnrich/deviceShadowEnrich call iot:DescribeThing/iot:GetThingShadow and store the result under Attribute (CloudFormation docs for AWS::IoTAnalytics::Pipeline DeviceRegistryEnrich/DeviceShadowEnrich). A missing Thing/shadow or a Lambda invoke error fails the RunPipelineActivity call (ErrPipelineActivityFailed) rather than silently passing the message through, since these AWS calls genuinely fail when their target doesn't exist. Only remaining gap: when no Lambda/IoT backend is registered in a given deployment (SetLambdaBackend/SetThingRegistry/SetThingShadowStore never called), these activities still pass through unchanged -- there is nothing to invoke."
  - "items_still_open: RunPipelineActivity's math expression language (pipeline_expr.go) now additionally implements AWS's documented function library (docs.aws.amazon.com/iotanalytics/latest/userguide/math-operators-functions.html: abs/acos/asin/atan/atan2/ceil/cos/cosh/exp/ln/log/mod/power/round/sign/sin/sinh/sqrt/tan/tanh/trunc). filter/math still do NOT implement LIKE, IN, or BETWEEN. Reason: unlike the math function library, no citable AWS documentation for filter's operators beyond '=, !=, <, <=, >, >=, AND, OR, NOT' was found (docs.aws.amazon.com/iotanalytics/latest/APIReference/API_RunPipelineActivity.html and the userguide's pipeline-activities-filter.html describe it only as \"an expression that looks like an SQL WHERE clause\", with no operator/function reference page equivalent to math's) -- extending the grammar with LIKE/IN/BETWEEN would be inventing behavior against an unpublished spec, not closing a documented gap."
deferred: []
leaks: {status: clean, note: "no goroutines/janitors owned by this backend; svcCtx is only used to seed test helpers (AddChannelInternal etc.). pipeline_expr.go's tokenizer/parser/evaluator is pure, synchronous, and per-call -- no new goroutines, tickers, or shared mutable state introduced."}
---

## Notes

- **2026-08-20 wrapper-key/nested-shape sweep (this pass).** Confirmed protocol REST-JSON
  from `deserializers.go`'s `awsRestjson1_*` prefix (168 matching funcs) and `api_client.go`.
  All 34 SDK ops (`ls api_op_*.go`) match `GetSupportedOperations` one-for-one
  (`sdk_completeness_test.go`, `handler_sdk_route_table_test.go`). Confirmed every op's live
  deserializer path calls its `deserializeOpDocument<Op>Output` function directly (not the
  restjson1 flat-body dead-code trap that bit appmesh/glacier) -- iotanalytics has no flat ops.
  - **`DescribeChannel`/`DescribeDatastore` statistics sibling-key bug (fixed, the dominant
    find).** `DescribeChannelOutput`/`DescribeDatastoreOutput` each declare `Channel`/
    `Datastore` and `Statistics` as two SEPARATE top-level members
    (`api_op_DescribeChannel.go:51`, `api_op_DescribeDatastore.go:51`), and `types.Channel`/
    `types.Datastore` themselves have no `statistics` field at all
    (`awsRestjson1_deserializeDocumentChannel`/`awsRestjson1_deserializeDocumentDatastore`
    have no `"statistics"` case). gopherstack nested `statistics` inside the `channel`/
    `datastore` object instead, so a real typed client's `DescribeChannelOutput.Statistics`/
    `DescribeDatastoreOutput.Statistics` stayed `nil` even with `IncludeStatistics=true` --
    the includeStatistics HTTP binding itself was already correct (query param honored), only
    the response shape was wrong. Fixed by moving `Statistics` onto
    `describeChannelResponse`/`describeDatastoreResponse` as a sibling of `Channel`/
    `Datastore`, off `channelDetail`/`datastoreDetail` (`models.go`,
    `handler_channels.go:handleDescribeChannel`, `handler_datastores.go:handleDescribeDatastore`).
    Proven by `TestDescribeChannel_Statistics_SDKRoundTrip`/
    `TestDescribeDatastore_Statistics_SDKRoundTrip` (`wire_shape_sdk_roundtrip_test.go`) --
    hand-reverting either fix reproduces `out.Statistics == nil` through the real client.
    Two existing tests (`TestHandler_DescribeChannel_IncludeStatistics`,
    `TestHandler_DescribeDatastore_IncludeStatistics`) asserted `hasStats` on the WRONG nested
    location (`resp["channel"]["statistics"]`/`resp["datastore"]["statistics"]`) -- corrected
    to assert on `resp["statistics"]` (top-level).
  - **`CreateDatastore`/`DescribeDatastore` `datastorePartitions` wire key (fixed).** AWS's
    wire key for the partitions member is `"datastorePartitions"` on BOTH the create request
    (`awsRestjson1_serializeOpDocumentCreateDatastoreInput`, `serializers.go:583`) and the
    datastore detail response (`awsRestjson1_deserializeDocumentDatastore`,
    `deserializers.go:7177`), not `"partitions"`. gopherstack's `createDatastoreRequest` and
    `datastoreDetail` both used `"partitions"`, so a real client's serialized
    `datastorePartitions` was silently dropped on create (the field never reached the
    backend), AND even a correctly-stored value would have come back under the wrong key on
    describe, leaving `Datastore.DatastorePartitions` nil either way. Fixed in `models.go`.
    Proven by `TestCreateDatastore_Partitions_SDKRoundTrip`; hand-reverting the request-side
    key alone and the response-side key alone each independently reproduce
    `out.Datastore.DatastorePartitions == nil`.
  - **Fabricated ARN on all four `*Summary` shapes (fixed, pattern (a)).** AWS's
    `ChannelSummary`/`DatastoreSummary`/`DatasetSummary`/`PipelineSummary` deserializers
    (`deserializers.go:5795`/`7677`/`7011`/`9310`) declare NO arn member at all -- unlike
    each resource's full `Channel`/`Datastore`/`Dataset`/`Pipeline` detail type, which does
    have one. gopherstack's summary DTOs were generalized from the full detail type and
    carried a `channelArn`/`datastoreArn`/`datasetArn`/`pipelineArn` field none of the real
    summary shapes have. A real typed client can't observe this (the generated
    `types.*Summary` structs simply have no such field to decode into), so proof is a raw-body
    absence assertion: `TestListSummaries_NoFabricatedARN` (table-driven, all four ops).
    Hand-reverting the channel case alone reproduces `hasARN == true` on exactly that subtest
    while the other three (still fixed) continue to pass.
  - **Fabricated `versionId` on `GetDatasetContent` (fixed).** `GetDatasetContentOutput` has
    no `versionId` member (`awsRestjson1_deserializeOpDocumentGetDatasetContentOutput`,
    `deserializers.go:2681`: `entries`/`status`/`timestamp` only). Removed from
    `getDatasetContentResponse`. Two existing tests locked this bug in:
    `TestHandler_GetDatasetContent_VersionAndEntries` asserted `resp["versionId"]` was
    non-empty (now asserts it's ABSENT, plus checks `status`/`entries` are present); and
    `TestHandler_GetDatasetContent_MagicVersionStrings` compared the `$LATEST`/
    `$LATEST_SUCCEEDED` GET response's `versionId` against the create response's `versionId`
    (now asserts a 200 with `status.state == "SUCCEEDED"` instead, since an unresolved magic
    string 404s via `ErrDatasetContentNotFound` -- `datasets.go:GetDatasetContent` -- so a 200
    is itself proof the sentinel resolved to the existing content version).
  - **Fabricated `roleArn` on the IoT SiteWise nested storage variant (fixed, second-order
    pattern (a)).** `types.IotSiteWiseCustomerManagedDatastoreS3Storage` (the type nested
    under `datastoreStorage.iotSiteWiseMultiLayerStorage.customerManagedS3Storage`) has only
    `bucket`/`keyPrefix`
    (`awsRestjson1_deserializeDocumentIotSiteWiseCustomerManagedDatastoreS3Storage`,
    `deserializers.go:8400`) -- unlike the wider `CustomerManagedDatastoreS3Storage` used
    directly under `datastoreStorage.customerManagedS3`
    (`awsRestjson1_deserializeDocumentCustomerManagedDatastoreS3Storage`,
    `deserializers.go:6155`), which does have `roleArn`. gopherstack reused the wider Go type
    for both, so the nested SiteWise variant fabricated a `roleArn` key. Split into a new
    narrower `IotSiteWiseCustomerManagedS3Storage` type (`models.go`) with no `RoleArn` field.
    A real typed client can't observe this (the generated type has no field for it), so proof
    is a raw-body absence assertion: `TestDatastore_IotSiteWiseCustomerManagedS3Storage_NoRoleArn`.
  - **`PipelineActivity` union (verified CLEAN, no fix needed).** All 10 discriminator keys
    (`channel`/`lambda`/`datastore`/`addAttributes`/`removeAttributes`/`selectAttributes`/
    `filter`/`math`/`deviceRegistryEnrich`/`deviceShadowEnrich`) match
    `awsRestjson1_deserializeDocumentPipelineActivity` exactly, and every nested activity
    type's field set matches its own deserializer
    (`ChannelActivity`/`LambdaActivity`/`DatastoreActivity`/`AddAttributesActivity`/
    `RemoveAttributesActivity`/`SelectAttributesActivity`/`FilterActivity`/`MathActivity`/
    `DeviceRegistryEnrichActivity`/`DeviceShadowEnrichActivity`).
  - **`DatasetContentDeliveryRule`/`Destination` union and `FileFormatConfiguration` union
    (verified CLEAN).** `destination`/`entryName` match
    `awsRestjson1_deserializeDocumentDatasetContentDeliveryRule`; the destination union
    (`iotEventsDestinationConfiguration`/`s3DestinationConfiguration`) matches
    `awsRestjson1_deserializeDocumentDatasetContentDeliveryDestination`;
    `jsonConfiguration`/`parquetConfiguration` match
    `awsRestjson1_deserializeDocumentFileFormatConfiguration`.
  - **All other nested dataset/pipeline sub-shapes (verified CLEAN):**
    `DatasetAction`/`ContainerDatasetAction`/`SqlQueryDatasetAction`/`ResourceConfiguration`/
    `Variable`/`QueryFilter`/`DeltaTime`, `RetentionPeriod`, `VersioningConfiguration`,
    `DatastorePartitions`/`DatastorePartition`/`Partition`/`TimestampPartition`,
    `Column`/`SchemaDefinition`, `LateDataRule`/`LateDataRuleConfiguration`/
    `DeltaTimeSessionWindowConfiguration`, `Schedule`/`TriggeringDataset`,
    `BatchPutMessageErrorEntry`, `Tag`, `LoggingOptions`, `DatasetEntry`/
    `DatasetContentStatus`/`DatasetContentSummary` -- every field name and nesting checked
    against its own `awsRestjson1_deserializeDocument<Type>` and found matching.
  - **Enum values (spot-checked CLEAN):** `ChannelStatus`/`DatastoreStatus`/`DatasetStatus`
    (`CREATING`/`ACTIVE`/`DELETING`), `DatasetContentState`
    (`CREATING`/`SUCCEEDED`/`FAILED`), `ReprocessingStatus`
    (`RUNNING`/`SUCCEEDED`/`CANCELLED`/`FAILED`) all match the literal strings gopherstack
    emits (`store.go` `statusActive`/`statusSucceeded` consts, `pipelines.go` `"RUNNING"`/
    `"CANCELLED"`). Error codes (`ResourceNotFoundException`/`ResourceAlreadyExistsException`/
    `InvalidRequestException`) match `types/errors.go` `ErrorCode()` spellings exactly.
  - **Genuine gaps found, disclosed, NOT fixed** (Layer 3 members never emitted, out of scope
    as a hunt per this sweep's method, but surfaced incidentally while checking wrapper
    keys/wire shapes above): `Dataset`/`CreateDatasetInput`/`CreateDatasetOutput` all carry a
    real `retentionPeriod` member (`awsRestjson1_deserializeDocumentDataset`,
    `deserializers.go:6271`; `awsRestjson1_serializeOpDocumentCreateDatasetInput`,
    `serializers.go:359`) this backend's `Dataset` model has no field for at all -- adding it
    would require a model field plus create/update/describe wiring, out of scope for a
    wire-shape sweep. **FIXED 2026-08-23**: `DatastoreSummary` carries real
    `datastorePartitions`/`fileFormatType` members
    (`awsRestjson1_deserializeDocumentDatastoreSummary`) this backend's summary previously
    never emitted; `DatasetSummary` carried real `actions` (as the narrower
    `DatasetActionSummary`, `actionName`+`actionType` only, NOT the full `DatasetAction`) and
    `triggers` members it also never emitted. Both are now populated in `handleListDatastores`/
    `handleListDatasets` (`handler_datastores.go`/`handler_datasets.go`), verified against a
    real aws-sdk-go-v2 client round trip
    (`TestListDatastores_SummaryCarriesPartitionsAndFileFormatType`,
    `TestListDatasets_SummaryCarriesActionsAndTriggers`,
    `list_summaries_missing_members_test.go`). `updateDatastoreRequest.Partitions` (json key
    `"partitions"`, left unfixed) is a genuinely FABRICATED request member --
    `UpdateDatastoreInput` has no partitions member at all in the real SDK
    (`api_op_UpdateDatastore.go:32`: `DatastoreStorage`/`FileFormatConfiguration`/
    `RetentionPeriod` only) -- but since no real typed client can ever populate a field the
    generated `UpdateDatastoreInput` struct doesn't expose, this field is unreachable by any
    real client regardless of its wire key; disclosed rather than removed to keep this pass
    scoped to reachable, provable bugs.
  - **`last_audit_commit` provenance check:** prior value `be69d5ece` dates to 2026-07-23
    (`git show -s --format=%ad`), one day before the prior `last_audit_date` of 2026-07-24 --
    consistent, not suspicious (the schema only requires HEAD-when-written, not an
    iotanalytics-specific commit). None of the prior manifest's "FIXED" claims (versionId
    handling, `$LATEST`/`$LATEST_SUCCEEDED` magic strings, `ListDatasetContents`
    pagination/sort-stability/scheduleTime, `RunPipelineActivity` transforms/cross-service
    wiring/math functions, `DatastorePartitions` validation) are wire-shape claims this pass
    re-derives against -- they're backend-behavior fixes, and this pass's independent
    re-derivation of every wire shape from the SDK found them consistent with what's
    documented; none introduced a wrapper-key/nested-shape bug.
- Protocol: restjson1. Service ARNs: `arn:aws:iotanalytics:<region>:<account>:<type>/<name>` via `pkgs/arn.Build`.
- `/channels` path is shared with MediaPackage/MediaTailor matchers at the same routing
  priority; the RouteMatcher disambiguates via `httputils.ExtractServiceFromRequest` (SigV4
  service name), not path alone. Verified correct.
- **Tag validation gap (fixed):** `CreateChannel`/`CreateDatastore`/`CreateDataset`/
  `CreatePipeline` accepted arbitrary tags (bad charset, `aws:` prefix, >50 tags) at creation
  time even though the identical tags would be rejected by a subsequent `TagResource` call on
  the same resource. AWS validates tags identically regardless of which API attached them.
  Fixed by calling `validateTags(req.Tags)` in all four create handlers before conversion to
  the internal map; `validateTags` now also enforces the 50-tag cap on the incoming batch
  itself (in addition to `TagResource`'s existing incremental cap against the existing set).
- **`$LATEST` / `$LATEST_SUCCEEDED` versionId (fixed):** AWS's `GetDatasetContent` and
  `DeleteDatasetContent` accept the sentinel strings `$LATEST` and `$LATEST_SUCCEEDED`
  (uppercase, sent verbatim as the `versionId` query param by the SDK) and default to
  `$LATEST_SUCCEEDED` when the query param is omitted entirely. The old code matched only a
  lowercase `"$latest"` literal that no real client ever sends, so any client passing the
  actual AWS sentinel value fell through to exact-match-by-UUID and got a spurious 404. Worse,
  `DeleteDatasetContent` treated an omitted `versionId` as "delete every version for this
  dataset" -- AWS deletes exactly one (the latest `SUCCEEDED` version). Both are fixed in
  `backend.go` (`latestSucceededContent` helper, `deleteDatasetContentVersion` helper). Since
  `CreateDatasetContent` always synthesizes `Status: SUCCEEDED` synchronously, `$LATEST` and
  `$LATEST_SUCCEEDED` coincide in this backend today, but the distinct code paths are kept
  because `DatasetContent.Status` is a real field or future async simulation would silently
  regress this.
- **`ListDatasetContents` pagination cursor (fixed):** the handler compared
  `content.VersionID <= cursor` to decide what to skip, but `ListDatasetContents` sorts by
  `CreationTime` descending -- `VersionID` is a random UUID with no relation to that order.
  Unlike `ListChannels`/`ListDatastores`/`ListDatasets`/`ListPipelines` (whose `Name`-keyed
  `store.Table.Snapshot()` is naturally ascending by the same field used as the cursor), this
  meant a `nextToken` cursor from page 1 would skip or repeat an effectively arbitrary subset
  of page 2. Fixed by switching to an offset-encoded token (`encodeNextToken(strconv.Itoa(end))`)
  in `handleListDatasetContents`.
- **`ListDatasetContents` sort stability (fixed, found while fixing pagination):**
  `CreationTime` is `epochSeconds` (second resolution). Content versions created within the
  same wall-clock second (e.g. a tight test loop, or a client bursting `CreateDatasetContent`
  calls) tie on `CreationTime`. The prior code used `slices.SortFunc`, which the stdlib
  explicitly documents as **not stable** -- tied entries could come back in a different
  relative order on every call to `ListDatasetContents`, even with zero mutation in between.
  That nondeterminism alone would have broken correct pagination even after the cursor fix
  above (page 1 and page 2 are two separate backend calls). Fixed by reversing the
  creation-order copy before `slices.SortStableFunc`, which makes ties resolve
  deterministically as "most-recently-inserted first" and keeps repeated calls
  byte-for-byte identical.
- **`RunPipelineActivity` real per-activity transforms (fixed):** previously every activity
  type, including `addAttributes`/`removeAttributes`/`selectAttributes`/`filter`/`math`, was
  pass-through regardless of the requested activity -- a real gap, since AWS applies real
  transforms for these. `pipeline_expr.go` adds a self-contained tokenizer, recursive-descent
  parser, and evaluator for the SQL-like expression language `filter` and `math` carry
  (literals, message-attribute identifiers, `+ - * / %`, `= != <> < <= > >=`, `AND/OR/NOT`,
  parentheses). `pipelines.go` wires per-activity-type handling into `RunPipelineActivity`
  (now takes the typed `PipelineActivity` the client sent, not an untyped
  `map[string]any` the old code never even inspected):
  `addAttributes`/`removeAttributes`/`selectAttributes` mutate the decoded JSON message
  object; `filter` evaluates the expression per payload and drops non-matching (or
  unparsable) payloads, matching a real filter activity removing messages from the pipeline;
  `math` evaluates the expression and stores the numeric result under `Attribute`. A
  per-message failure (non-JSON payload, unknown attribute, malformed expression, type
  mismatch) is a soft failure -- the payload is left unchanged (transforms/math) or dropped
  (filter) rather than failing the whole `RunPipelineActivity` call, matching a single bad
  message failing only its own activity step. `channel`/`datastore` remain pass-through
  (correct: real source/sink activities).
- **`RunPipelineActivity` lambda/deviceRegistryEnrich/deviceShadowEnrich cross-service wiring
  (fixed):** these three activities were pass-through with a note claiming this backend "has
  no wiring" for cross-service calls -- the same stale claim this parity campaign found and
  fixed for sagemaker's S3 read, ELB's EC2/ACM/IAM checks, and glacier's S3 write-back. The
  wiring pattern already exists (SNS/Firehose/SecretsManager's `LambdaInvoker` +
  `SetLambdaBackend`, IoT's `DescribeThing`/`GetThingShadow` used elsewhere in `cli.go`) and
  applies here unchanged. `services/iotanalytics/interfaces.go` adds `LambdaInvoker`,
  `ThingRegistry`, `ThingShadowStore`; `cli.go`'s `wireIoTAnalyticsCrossService` (called from
  `wireStorageAndSecretsIntegrations`) wires the real Lambda backend directly (it already
  satisfies `LambdaInvoker`) and adapts the IoT backend's `DescribeThing`/`GetThingShadow`
  (`iotAnalyticsThingRegistryAdapter`/`iotAnalyticsThingShadowAdapter`) into the map-shaped
  interfaces `pipelines.go` uses. `RunPipelineActivity` now takes a `ctx` parameter to thread
  through to `InvokeFunction`. Per-activity behavior: `lambda` batches payloads by
  `BatchSize` and round-trips a JSON object array through `InvokeFunction("RequestResponse")`
  per AWS's documented contract; `deviceRegistryEnrich`/`deviceShadowEnrich` call
  `DescribeThing`/`GetThingShadow` once per activity call (the target `ThingName` is a fixed
  activity field, not per-message) and store the result under `Attribute` on every payload.
  Unlike the per-message soft-failure convention above, a missing Thing/shadow or a Lambda
  invoke/response error fails the whole call (`ErrPipelineActivityFailed`) rather than passing
  the message through unchanged -- a real AWS `iot:DescribeThing`/`iot:GetThingShadow`/Lambda
  invoke against a nonexistent target genuinely fails, and silently returning the original
  message would be the same silent-drop bug class this campaign has been hunting. When no
  Lambda/IoT backend is registered at all (`SetLambdaBackend`/`SetThingRegistry`/
  `SetThingShadowStore` never called), these three activities still pass through unchanged --
  there's nothing to invoke, which is an environment characteristic, not a bug. Proven by
  `cli_iotanalytics_lambda_iot_wiring_test.go`, which drives `initializeServices` (the actual
  `cli.go` composition root) rather than calling the wiring helper directly.
- **`RunPipelineActivity` math function library (fixed):** `math` only implemented arithmetic
  (`+ - * / %`); AWS documents a real function library at
  `math-operators-functions.html` (`abs/acos/asin/atan/atan2/ceil/cos/cosh/exp/ln/log/mod/
  power/round/sign/sin/sinh/sqrt/tan/tanh/trunc`, each `func(Decimal[, Decimal])` per that
  page's exact signatures -- `trunc` takes a second `int` argument, `atan2`/`mod`/`power` take
  two `Decimal`s, the rest take one). This was previously mischaracterized as part of an
  "undocumented AWS superset" alongside filter's `LIKE`/`IN`/`BETWEEN`; the math function list
  specifically is real, citable, and now implemented in `pipeline_expr.go`
  (`mathFuncs1`/`mathFuncs2`, `funcCallNode`). `filter`'s grammar beyond comparisons/logical
  operators remains genuinely undocumented (no equivalent operator/function reference page
  exists for filter) and is intentionally not extended -- see `items_still_open`.
- **`CreateDatasetContent` explicit `versionId` (fixed):** `CreateDatasetContentInput` has a
  real `versionId` body field the old handler never read (the handler didn't even parse a
  request body). Now `handleCreateDatasetContent` parses `createDatasetContentRequest` and
  `InMemoryBackend.CreateDatasetContent(datasetName, versionID string)` uses the caller's
  `versionID` when non-empty (generating a UUID only when it's empty, as before), rejecting a
  duplicate with `ErrAlreadyExists` (409) instead of silently accepting it. AWS's docs say
  specifying `versionId` requires the dataset to use a `DeltaTimer` filter; this backend
  accepts it unconditionally rather than modeling that restriction, since enforcing it would
  require simulating `DeltaTimer`-driven dataset content generation this backend does not
  otherwise implement.
- **`ListDatasetContents` `scheduleTime` field + `scheduledBefore`/`scheduledOnOrAfter`
  filters (fixed):** `DatasetContentSummary.ScheduleTime` ("the time the creation of the
  dataset contents was scheduled to start", distinct from `CreationTime`, "the actual time
  ... was started") was missing entirely from `DatasetContent`/`datasetContentSummary`, and
  the `scheduledBefore`/`scheduledOnOrAfter` query filters on `ListDatasetContentsInput` were
  unimplemented. `DatasetContent.ScheduleTime` is now set equal to `CreationTime` in
  `CreateDatasetContent` -- this backend only ever creates dataset content synchronously via
  a direct API call (no background cron simulation of a dataset's `Schedule` trigger), which
  is exactly the case where real AWS also sets `scheduleTime == creationTime` (a manually
  invoked `CreateDatasetContent` wasn't fired by a schedule). `handleListDatasetContents` now
  parses the `scheduledBefore`/`scheduledOnOrAfter` query params (RFC3339 date-time strings,
  matching `smithytime.FormatDateTime`) and filters on `ScheduleTime` before pagination.
- **`DatastorePartitions` validation (fixed):** `CreateDatastore`/`UpdateDatastore` accepted
  any `DatastorePartitions` shape, including partition entries with neither
  `attributePartition` nor `timestampPartition` set, both set, or a set variant with an empty
  `attributeName`. The real SDK's client-side validators (`validatePartition` /
  `validateTimestampPartition`) require `attributeName` on whichever variant is set; a raw
  HTTP caller bypassing SDK-side validation would still need to satisfy this server-side.
  `validateDatastorePartitions`/`validateDatastorePartitionEntry` in `store.go` now enforce
  exactly-one-variant-set plus a non-empty `attributeName`, returning `InvalidRequestException`
  otherwise. Partition count/nesting cardinality limits are not enforced -- no SDK client-side
  validator surfaces a specific limit to diff against, and AWS's server-side limits for a
  deprecated service are not independently documented; this is treated as an intentional
  non-issue rather than a gap.
- Persistence: `channels`/`datastores`/`datasets`/`pipelines` are `store.Table[T]` (key =
  `Name`, no secondary index needed -- `resolveARNResource` parses the ARN's resource segment
  back into a name rather than reverse-indexing). `tags`/`channelMessages`/`datasetContents`
  are plain maps folded into `backendSnapshot` alongside `registry.SnapshotAll()`. `Handler`
  delegates `Snapshot`/`Restore` to the backend via the `Snapshottable` interface -- verified
  present and wired correctly, nothing to fix.
- `TestSDKCompleteness` (sdk_completeness_test.go) confirms all 34 SDK ops are handled with
  zero entries in the `notImplemented` acknowledgement list.

## 2026-08-28 — wrapper-key-sweep: UpdateDatastore accepted a phantom partitions field (acceptguard)

acceptguard flagged `updateDatastoreRequest.Partitions` (`models.go:146`, read in
`handleUpdateDatastore`) as matching no member of any real Input in the module. Confirmed
against iotanalytics@v1.32.0's `UpdateDatastoreInput` (`api_op_UpdateDatastore.go`):
`DatastoreName`/`DatastoreStorage`/`FileFormatConfiguration`/`RetentionPeriod` only — no
partitions member at all. `CreateDatastoreInput` has `DatastorePartitions`; partitions are
settable only at creation and are immutable afterward, matching real AWS's documented
behavior for this field.

Fixed by removing `Partitions` from `updateDatastoreRequest` (`models.go`), the corresponding
parameter from `Backend.UpdateDatastore` (`datastores.go`, `interfaces.go`), and its call site
(`handler_datastores.go`); the now-dead `partitions != nil` clone/validate branches were
removed with it. `validateDatastorePartitions` remains, still used by `CreateDatastore`.

A typed-client fail-before test isn't constructible here — `UpdateDatastoreInput`'s Go struct
never had a partitions field to send incorrectly, so a real client's request is identical
before and after. Proof is a raw-body test instead
(`TestUpdateDatastore_RawPartitionsFieldIgnored`, `wire_field_fixes_test.go`, new file):
sending `{"partitions": {...}}` directly to `PUT /datastores/{name}` must not affect the stored
datastore. Hand-reverted `datastores.go`/`handler_datastores.go`/`interfaces.go`/`models.go`,
confirmed this test fails (the raw partitions key mutated the datastore), restored. A companion
real-SDK test (`TestUpdateDatastore_PartitionsImmutable`) proves the correct behavior a typed
client actually observes: partitions set at `CreateDatastore` survive an `UpdateDatastore` call
that changes an unrelated field.

**Test judgement**: `datastores_test.go`'s `TestInMemoryBackend_DatastorePartitionsValidation`
previously called `b.UpdateDatastore(..., tt.partitions)` for every non-error case, asserting
partitions could be set via update — this was itself testing the bug as correct behavior. The
call is removed; the test now only validates `CreateDatastore`'s partition-shape checks, and its
doc comment was corrected to say `UpdateDatastore` doesn't take partitions at all rather than
"validates" them.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` — all clean
(`./services/iotanalytics/...`).

## 2026-09-04 — parity sweep (gopherstack-2wb): CreatePipeline/UpdatePipeline missing PipelineActivities shape validation

`CreatePipelineInput`/`UpdatePipelineInput`'s `PipelineActivities` doc comment
(`api_op_CreatePipeline.go`, `api_op_UpdatePipeline.go`) states: "The list can be 2-25
PipelineActivity objects and must contain both a channel and a datastore activity. Each entry
in the list must contain only one activity." This is a real, citable business rule distinct
from a simple required-field check. The SDK's client-side validator
(`validatePipelineActivity`, `validators.go`) only walks each activity's own required
sub-fields (e.g. `ChannelActivity.ChannelName`/`Name`) — it never checks the aggregate list
shape — so a real typed client can construct and send a `CreatePipelineInput` with zero
activities, only a channel activity, only a datastore activity, or more than 25 entries, and
gopherstack accepted all of these silently before this fix.

Fixed by `validatePipelineActivities` (`store.go`): enforces 2-25 entries, exactly one
`Channel` activity, exactly one `Datastore` activity, and that no single entry sets more than
one activity union member. Wired into `CreatePipeline` (unconditional) and `UpdatePipeline`
(only when the caller supplies a non-nil activities slice — `UpdatePipelineInput.PipelineActivities`
is also `This member is required`, so a real client always sends it; nil/omitted is only
reachable via a raw HTTP caller, and is kept as "leave unchanged" for backward compatibility
with that path). Returns `InvalidRequestException` (`errCodeInvalidRequest`/`ErrValidation`),
which is in both ops' real error sets (confirmed against
`awsRestjson1_deserializeOpErrorCreatePipeline`/`...UpdatePipeline` in `deserializers.go`).

Proven by `TestCreatePipeline_RequiresChannelAndDatastoreActivity` (three subtests:
channel-only, datastore-only, empty) and `TestCreatePipeline_ChannelAndDatastoreActivity_Succeeds`
(`wire_field_fixes_test.go`), driven through the real aws-sdk-go-v2 client. Hand-reverted the
`validatePipelineActivities` call in `CreatePipeline` (`pipelines.go`), confirmed all three
`RequiresChannelAndDatastoreActivity` subtests fail ("An error is expected but got nil"),
restored (diff against backup identical).

**Blast radius**: this is a `CreatePipeline`/`UpdatePipeline` precondition that most of this
package's own test suite incidentally violated (seeding a pipeline with `nil` or
channel-only activities purely to exercise unrelated behavior — tags, pagination, persistence,
deep-copy, reprocessing). Updated: `AddPipelineInternal` (`pipelines.go`, the shared test-seed
helper) now seeds a valid channel+datastore pair; every direct `CreatePipeline(...)` call site
and raw-HTTP pipeline-creation body across `pipelines_test.go`, `store_test.go`,
`persistence_test.go`, `handler_test.go`, `handler_tags_test.go`, `handler_pipelines_test.go`,
`wire_shape_sdk_roundtrip_test.go`, and `test/integration/iotanalytics_test.go` now supplies a
minimal valid activity pair (two shared helpers: `validPipelineActivities()` for Go-level
`[]iotanalytics.PipelineActivity`, `validPipelineActivitiesBody()` for raw JSON bodies).
`test/integration/iotanalytics_test.go`'s `TestIntegration_IoTAnalytics_PipelineLifecycle`/
`_PipelineReprocessing` previously sent a channel-only `PipelineActivities` and asserted
`CreatePipeline` succeeded — this itself encoded the bug as correct behavior (a real AWS
endpoint would reject it); fixed by adding the missing datastore activity to both.

Gates: `go build ./...`, `go vet ./services/iotanalytics/...`,
`go test -race -count=1 ./services/iotanalytics/...`,
`golangci-lint run ./services/iotanalytics/...` (0 issues),
`golangci-lint run ./test/integration/...` (0 issues),
`go test -race -count=1 ./services/cloudformation/... ./services/iot/... ./services/lambda/... .`
— all pass (no cross-service wiring touches `CreatePipeline`'s shape, so no CFN teardown or
Lambda/IoT wiring regression).
