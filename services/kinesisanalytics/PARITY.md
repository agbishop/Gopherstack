---
service: kinesisanalytics
sdk_module: aws-sdk-go-v2/service/kinesisanalytics@v1.33.4
last_audit_commit: 17458c2f
last_audit_date: 2026-08-20
overall: A            # real fixes found: deleted three gopherstack-invented surfaces
                       # (ServiceExecutionRole/RuntimeEnvironment fields, five non-real
                       # ApplicationStatus constants, InputUpdate.InputStartingPositionConfiguration),
                       # and closed a whole class of missing required-field validation across
                       # Input/Output/ReferenceDataSource/InputProcessingConfiguration/SourceSchema
                       # that let malformed requests silently succeed instead of failing with
                       # InvalidArgumentException like real AWS.
ops:
  CreateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "ServiceExecutionRole request field DELETED (gopherstack-invented -- CreateApplicationInput has no such member in the real SDK, verified via grep across the whole module). Inputs[]/Outputs[] now route through the same hardened convertInputConfig/convertOutputConfig validation as AddApplicationInput/AddApplicationOutput (see families below)."}
  DeleteApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeApplication: {wire: fixed, errors: ok, state: ok, persist: ok, note: "ServiceExecutionRole/RuntimeEnvironment response fields DELETED (gopherstack-invented, present nowhere in ApplicationDetail). 2026-08-20: InputProcessingConfigurationDescription.InputLambdaProcessor wire key FIXED to InputLambdaProcessorDescription -- see dated entry below."}
  ListApplications: {wire: ok, errors: ok, state: ok, persist: ok, note: "HasMoreApplications/ExclusiveStartApplicationName pagination correct, no NextToken -- matches real ListApplicationsInput/Output exactly."}
  StartApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "READY->STARTING->RUNNING transition via launchTransition goroutine, correct."}
  StopApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "RUNNING->STOPPING->READY transition, correct."}
  UpdateApplication: {wire: fixed, errors: ok, state: ok, persist: ok, note: "InputUpdate.InputStartingPositionConfiguration field DELETED (gopherstack-invented -- the real InputUpdate shape has no such member; starting-position changes are only ever accepted via StartApplication's InputConfigurations). ReferenceDataSourceUpdate.ReferenceSchemaUpdate (a whole-object SourceSchema replace, per its doc) now runs through the same required-field validation as a fresh ReferenceSchema."}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  AddApplicationCloudWatchLoggingOption: {wire: ok, errors: ok, state: ok, persist: ok}
  AddApplicationInput: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "Input.InputSchema (a required member per validators.go's validateInput -- authoritative over the doc comment, which doesn't call it out) was never validated; a request omitting it was silently accepted with a nil InputSchema. Also added: KinesisStreamsInput/KinesisFirehoseInput.ResourceARN+RoleARN required-when-the-sub-object-is-present; InputProcessingConfiguration.InputLambdaProcessor required-when-InputProcessingConfiguration-is-present, and its own ResourceARN/RoleARN required."}
  AddApplicationInputProcessingConfiguration: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "InputId and InputProcessingConfiguration (both required members) were never validated -- a request omitting InputProcessingConfiguration silently cleared/no-op'd instead of being rejected. Now validates both, plus InputProcessingConfiguration.InputLambdaProcessor and its ResourceARN/RoleARN, matching validateOpAddApplicationInputProcessingConfigurationInput/validateInputProcessingConfiguration/validateInputLambdaProcessor."}
  AddApplicationOutput: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "KinesisStreamsOutput/KinesisFirehoseOutput/LambdaOutput.ResourceARN+RoleARN required-when-the-sub-object-is-present was never validated -- added, matching validateKinesisStreamsOutput/validateKinesisFirehoseOutput/validateLambdaOutput."}
  AddApplicationReferenceDataSource: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "ReferenceSchema's own required members (RecordFormat.RecordFormatType restricted to JSON/CSV, RecordColumns non-nil, each RecordColumn.Name/SqlType, JSON/CSVMappingParameters sub-fields) were never validated -- only top-level presence was checked. Added full validateSourceSchema-equivalent validation, shared with AddApplicationInput's InputSchema and UpdateApplication's ReferenceSchemaUpdate via the same convertSourceSchema helper."}
  DeleteApplicationCloudWatchLoggingOption: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteApplicationInputProcessingConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteApplicationOutput: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteApplicationReferenceDataSource: {wire: ok, errors: ok, state: ok, persist: ok}
  DiscoverInputSchema: {wire: ok, errors: fixed, state: ok, persist: n/a, note: "fixed twice over: (1) the request previously did zero validation and always returned a canned 200 OK schema regardless of input -- now enforces exactly-one-of-{ResourceARN+RoleARN, S3Configuration{BucketARN,FileKey,RoleARN}} plus InputProcessingConfiguration's usual required-field contract, rejecting malformed requests with InvalidArgumentException. (2) the successful-path response was ALSO fabricated even for well-formed requests (a canned COL_1/value1 schema for any ResourceARN/S3Configuration, including nonexistent ones) -- now DiscoverInputSchema actually samples real records via new SetKinesisStreamReader/SetS3ObjectReader hooks (discover_schema.go) and infers RecordColumns from them, returning UnableToDetectSchemaException (a real, previously-unused error on this op) when it cannot reach or sample the source. cli.go now wires both: S3 directly (kaBk.SetS3ObjectReader(s3Bk), no adapter -- s3.InMemoryBackend.GetObject satisfies S3ObjectReader with the real SDK types) and Kinesis via a new kinesisAnalyticsStreamReaderAdapter (cli.go) bridging onto KinesisStreamReader's narrow shape, both proven end to end through the real HTTP dispatch by TestInitializeServices_KinesisAnalyticsKinesisS3Wiring (cli_kinesisanalytics_kinesis_s3_wiring_test.go). A Firehose-sourced ResourceARN still has no reader at all -- see gaps -- and correctly returns UnableToDetectSchemaException rather than something misleading."}
families:
  requiredFieldValidation: {status: fixed, note: "A whole class of nested required-member validation gaps, all verified against aws-sdk-go-v2/service/kinesisanalytics/validators.go (the authoritative client-side validator source, distinct from -- and occasionally contradicting -- doc comments): Input.InputSchema; KinesisStreamsInput/KinesisFirehoseInput/KinesisStreamsOutput/KinesisFirehoseOutput/LambdaOutput.ResourceARN+RoleARN (required whenever their parent sub-object is supplied at all); InputProcessingConfiguration.InputLambdaProcessor (required whenever InputProcessingConfiguration is supplied) and its own ResourceARN/RoleARN; SourceSchema.RecordFormat.RecordFormatType (restricted to the real two-value RecordFormatType enum, JSON/CSV -- previously only enforced for Output.DestinationSchema, not for Input.InputSchema or ReferenceDataSource.ReferenceSchema); SourceSchema.RecordColumns (required, non-nil) and each RecordColumn's Name/SqlType; JSONMappingParameters.RecordRowPath and CSVMappingParameters.RecordRowDelimiter/RecordColumnDelimiter (required whenever their parent variant is supplied). Previously these gaps meant a malformed request (missing schema, missing role ARN on a nested Kinesis/Lambda sub-object, empty processing configuration, invalid record-format type) was silently accepted and stored with zero-valued/absent fields instead of being rejected with InvalidArgumentException -- a disguised-corruption bug in the same family as the UpdateApplication wire-shape bug fixed in a prior sweep. Centralized in new helpers (validateResourceRoleARN, convertInputProcessingConfig, convertSourceSchema + validateRecordFormatType/validateMappingParameters/validateRecordColumns in applications.go) shared across CreateApplication/AddApplicationInput/AddApplicationOutput/AddApplicationInputProcessingConfiguration/AddApplicationReferenceDataSource/UpdateApplication's ReferenceSchemaUpdate."}
  updateNestedPayloads: {status: ok, note: "InputUpdate/OutputUpdate/ReferenceDataSourceUpdate's Kinesis*/Lambda/S3/InputProcessingConfiguration/InputSchema/InputParallelism sub-objects all correctly carry AWS-suffixed field names (ResourceARNUpdate, RoleARNUpdate, BucketARNUpdate, FileKeyUpdate, ReferenceRoleARNUpdate, RecordColumnUpdates, RecordEncodingUpdate, RecordFormatUpdate, CountUpdate), each with its own dedicated Go type -- verified against aws-sdk-go-v2/service/kinesisanalytics/serializers.go's per-shape awsAwsjson11_serializeDocument* functions. InputSchemaUpdate is correctly applied as a field-by-field partial patch; ReferenceSchemaUpdate is correctly applied as a whole-object SourceSchema replace (confirmed via types.ReferenceDataSourceUpdate.ReferenceSchemaUpdate *SourceSchema)."}
gaps:
  - "DiscoverInputSchema now does real sampling+inference (discover_schema.go: newline-delimited-JSON sampling, per-key BOOLEAN/INTEGER/DOUBLE/VARCHAR(N) type inference, sorted-alphabetical column order) instead of a fixed synthetic schema, and cli.go wires both readers it needs: S3 directly (kaBk.SetS3ObjectReader(s3Bk), no adapter -- s3.InMemoryBackend.GetObject satisfies S3ObjectReader with the real SDK types) and Kinesis via kinesisAnalyticsStreamReaderAdapter (cli.go), which bridges kinesis.InMemoryBackend's real ctx+typed-struct ListShards/GetShardIterator/GetRecords (services/kinesis/records.go, shards.go) onto KinesisStreamReader's narrow (streamName string, limit int) shape. Both proven through the actual composition root (not the wiring helper called directly) by TestInitializeServices_KinesisAnalyticsKinesisS3Wiring (cli_kinesisanalytics_kinesis_s3_wiring_test.go), which deletes its own wireKinesisAnalyticsCrossService call site to confirm the test goes red. Firehose delivery streams as a DiscoverInputSchema source remain genuinely unimplemented, not just unwired: firehose.InMemoryBackend has no accessor to read back buffered/recently-ingested records at all (it's flush-oriented), and adding one is outside services/kinesisanalytics. A Firehose-sourced request (and any request before either reader existed) correctly reports UnableToDetectSchemaException (a real, previously-unused SDK error type for this exact op -- see errors.go) instead of fabricating a 200 -- covered by the same wiring test's firehose_source_reports_unable_to_detect_schema subtest."
  - "statusUpdating (\"UPDATING\", a real ApplicationStatus enum value per types/enums.go) is unused by design, not by omission: it is present in source (matches the wire enum exactly, not a gap in the enum itself), but UpdateApplication is genuinely synchronous here -- it validates, applies, and bumps ApplicationVersionId/LastUpdateTimestamp atomically under the backend lock before returning, so a client can never observe an intermediate state where those fields disagree. This is the same shape as the emrserverless-SUBMITTED and elasticsearch-Processing precedents judged legitimate simplifications: the transient state is unreachable because nothing async ever exists to be caught mid-transition, not because a field is missing or inconsistent. No code change made for this item."
deferred: []
leaks: {status: clean, note: "launchTransition/DeleteApplication background goroutines remain bounded by b.svcCtx (NewInMemoryBackendWithContext) and tracked in b.cancelFuncs, canceled on Reset(). No new goroutines, maps, or per-request state introduced this sweep -- all changes were request-validation logic in the existing conversion helpers (applications.go/handler_*.go), which return early with an error and mutate no backend state on the rejected path."}
---

## Notes

Protocol: **awsjson1.1**, single POST endpoint, `X-Amz-Target: KinesisAnalytics_20150814.<Op>`
dispatch (verified against handler.go's `kinesisanalyticsTargetPrefix` -- correctly uses the
older 20150814 date, not v2's 20180523). Timestamps (`CreateTimestamp`/`LastUpdateTimestamp`) are
epoch-seconds `float64` with sub-second precision, verified against
`aws-sdk-go-v2/service/kinesisanalytics` deserializers.go's `smithytime.ParseEpochSeconds` --
correct.

### Real bugs fixed this sweep

1. **Three gopherstack-invented surfaces DELETED**, none of which exist anywhere in
   `aws-sdk-go-v2/service/kinesisanalytics@v1.30.21` (verified by grepping the whole downloaded
   SDK module source, including generated serializers/deserializers/validators, not just
   `types.go`):
   - `Application.ServiceExecutionRole` / `Application.RuntimeEnvironment` and their mirrors on
     `createApplicationInput`/`applicationDetail`. `CreateApplicationInput` has no
     `ServiceExecutionRole` member at all (confirmed against `api_op_CreateApplication.go`), and
     neither field appears on `ApplicationDetail`. The only "RuntimeEnvironment" hits in the SDK
     module are unrelated client-config internals (`aws.RuntimeEnvironment` for
     `DefaultsMode` resolution in `options.go`/`api_client.go`), not an API field. Removed from
     `Application`/`createApplicationInput`/`applicationDetail` (models.go),
     `CreateApplication`'s signature (applications.go, store.go's `StorageBackend` interface),
     and `toApplicationDetail` (handler_applications.go). All backend/handler test call sites
     updated (export_test.go, persistence_test.go, isolation_test.go).
   - `statusAutoScaling`/`statusForceStopping`/`statusMaintenance`/`statusRollingBack`/
     `statusRolledBack` constants (store.go), marked `//nolint:deadcode "AWS status constant"`
     but not part of the real v1 `ApplicationStatus` enum (`types/enums.go`'s
     `ApplicationStatus.Values()` returns exactly `DELETING/STARTING/STOPPING/READY/RUNNING/
     UPDATING` -- six values, no more). These five were copied from kinesisanalyticsv2's larger,
     distinct `ApplicationStatus` enum. Deleted; the doc comment on the remaining six real
     constants now states the real enum explicitly so a future audit doesn't need to re-derive
     it.
   - `inputUpdate.InputStartingPositionConfiguration` (models.go) and its handling in
     `applyOneInputUpdate` (application_update.go): the real `InputUpdate` shape
     (`types.InputUpdate`) has exactly `InputId`/`InputParallelismUpdate`/
     `InputProcessingConfigurationUpdate`/`InputSchemaUpdate`/`KinesisFirehoseInputUpdate`/
     `KinesisStreamsInputUpdate`/`NamePrefixUpdate` -- no starting-position member. A real
     client's `UpdateApplication` call can never change an input's starting position; that's
     only reachable via `StartApplication`'s `InputConfigurations` (`types.InputConfiguration`,
     which legitimately does carry `InputStartingPositionConfiguration` -- confirmed distinct
     from `InputUpdate`). Deleted the field and its dead-in-practice handling.

2. **A whole class of missing required-field validation, closed** (applications.go,
   handler_inputs.go, handler_reference_data.go, application_update.go). Verified against
   `aws-sdk-go-v2/service/kinesisanalytics/validators.go`, which is the authoritative source for
   what AWS actually requires client-side (and, by strong inference, server-side) -- doc
   comments alone are occasionally wrong or incomplete (e.g. `Input.InputSchema`'s doc comment
   doesn't say "required" but `validateInput` unconditionally requires it). Before this sweep,
   several nested required members were accepted as absent/empty and silently stored that way
   (a `200 OK` with a corrupted/incomplete resource) instead of being rejected with
   `InvalidArgumentException` -- the same disguised-corruption bug class as the `UpdateApplication`
   nested-payload wire-shape bug fixed in a prior sweep, just at the required-field-presence
   layer instead of the field-naming layer. Fixed:
   - `Input.InputSchema` (required on both `CreateApplication`'s `Inputs[]` and
     `AddApplicationInput`, since both route through `convertInputConfig`).
   - `KinesisStreamsInput`/`KinesisFirehoseInput`/`KinesisStreamsOutput`/`KinesisFirehoseOutput`/
     `LambdaOutput`'s `ResourceARN`+`RoleARN`, required whenever that sub-object is supplied at
     all (new shared `validateResourceRoleARN` helper).
   - `InputProcessingConfiguration.InputLambdaProcessor`, required whenever
     `InputProcessingConfiguration` itself is supplied -- previously an empty
     `InputProcessingConfiguration{}` was silently dropped instead of rejected (new
     `convertInputProcessingConfig` helper, shared by `AddApplicationInput`,
     `AddApplicationInputProcessingConfiguration`, and `DiscoverInputSchema`'s optional
     processing-config field). `AddApplicationInputProcessingConfiguration` additionally never
     validated its own required `InputId`/`InputProcessingConfiguration` members at all.
   - `SourceSchema.RecordFormat.RecordFormatType` restricted to the real two-value
     `RecordFormatType` enum (`JSON`/`CSV`) -- previously only enforced on
     `Output.DestinationSchema`, not on `Input.InputSchema` or
     `ReferenceDataSource.ReferenceSchema`/`ReferenceSchemaUpdate`, despite all four using the
     identically-typed field. `SourceSchema.RecordColumns` (required, non-nil) and each
     `RecordColumn.Name`/`SqlType` (required), and `JSONMappingParameters.RecordRowPath`/
     `CSVMappingParameters.RecordRowDelimiter`/`RecordColumnDelimiter` (required whenever their
     parent variant is supplied) -- all previously unvalidated. Consolidated into
     `convertSourceSchema` (now returning an error), shared by `AddApplicationInput`'s
     `InputSchema`, `AddApplicationReferenceDataSource`'s `ReferenceSchema`, and
     `UpdateApplication`'s `ReferenceSchemaUpdate` (a whole-object replace per its doc, so the
     same required-field contract legitimately applies there too -- unlike `InputSchemaUpdate`,
     which is a genuine partial patch and was correctly left alone).

3. **`DiscoverInputSchema` accepted any request shape and always returned a canned success**
   (handler_inputs.go). An empty request, a request supplying both `ResourceARN` and
   `S3Configuration`, or a source missing its own required sub-fields (`RoleARN` for the
   streaming-source path; `BucketARN`/`FileKey`/`RoleARN` for `S3Configuration`, all three
   `This member is required` per `types.S3Configuration`) all incorrectly returned `200 OK`.
   Added `validateDiscoverInputSchemaInput`, enforcing exactly-one-of-{streaming source, S3
   source} plus each source's required sub-fields, rejecting malformed requests with
   `InvalidArgumentException` -- one of `DiscoverInputSchema`'s modeled errors (confirmed via
   `deserializers.go`'s `awsAwsjson11_deserializeOpErrorDiscoverInputSchema`, alongside
   `ResourceProvisionedThroughputExceededException`/`ServiceUnavailableException`/
   `UnableToDetectSchemaException`). The successful-path response content remains an
   intentionally fixed synthetic sample (see gaps) -- this fix is about the request side, not
   fabricating real schema inference.

4. **`DiscoverInputSchema`'s successful-path response was ALSO a fixed synthetic sample**, even
   for well-formed requests naming a source that plainly can't exist (e.g.
   `arn:...:stream/test`) -- confirmed by running `TestHandler_DiscoverInputSchema` against the
   pre-fix code: both the streaming-source and S3-source cases returned `200 OK` with the same
   canned `COL_1`/`value1` schema regardless of what `ResourceARN`/`S3Configuration` named. Added
   real sampling+inference (discover_schema.go): `KinesisStreamReader`/`S3ObjectReader` are new
   narrow interfaces (interfaces.go, mirroring services/cloudwatch's `FirehosePutter` /
   services/firehose's `KinesisReader`/`S3Storer` cross-service pattern) that a sibling backend
   satisfies once wired via the new `SetKinesisStreamReader`/`SetS3ObjectReader`; when wired,
   `DiscoverInputSchema` samples up to 10 newline-delimited-JSON records and infers a
   `RecordColumn` per observed key (BOOLEAN/INTEGER/DOUBLE/VARCHAR(N), sorted-alphabetical
   order). `s3.InMemoryBackend.GetObject` satisfies `S3ObjectReader` directly, with no adapter,
   proven by `TestS3ObjectReader_SatisfiedByRealS3Backend`. Neither hook is wired by cli.go (out
   of this pass's scope -- see gaps for exactly what wiring would close the gap), so in the
   shipped server every request now returns `UnableToDetectSchemaException` -- a real,
   previously-unused error on this exact op (see errors.go) -- rather than a fabricated 200.
   `TestDiscoverInputSchema_S3SamplesRealRecords`/`_KinesisSamplesRealRecords` prove the sampling
   +inference logic itself is correct once a reader is wired, using fake test doubles for the
   same reason services/firehose's own `KinesisReader` tests do (no cli.go-side adapter exists to
   bridge onto `kinesis.InMemoryBackend`'s real ctx+typed-struct methods).

### Verified clean (no bug, but worth recording so the next audit doesn't re-flag)

- **Route matcher / target prefix**: `KinesisAnalytics_20150814.` -- correctly uses the v1 date,
  distinct from kinesisanalyticsv2's `KinesisAnalyticsV2_20180523.`. `ExtractOperation` correctly
  falls back to `"Unknown"` when the header is absent or doesn't match, so the `dispatch` map
  lookup fails closed rather than routing garbage.
- **Persistence**: `Handler.Snapshot`/`Restore` (persistence.go) delegate to
  `InMemoryBackend.Snapshot`/`Restore`, which version-gate (`kinesisanalyticsSnapshotVersion`) and
  go through `store.Registry.SnapshotAll`/`RestoreAll` for the `apps` table -- confirmed via
  `TestInMemoryBackend_FullStateSnapshotRestoreRoundTrip` (persistence_test.go), which round-trips
  every sub-resource kind across two regions and survives the `CreateApplication` signature change
  from this sweep.
- **Lifecycle transitions**: `StartApplication` (READY -> STARTING -> RUNNING) and
  `StopApplication` (RUNNING -> STOPPING -> READY) both correctly gate on the real API's
  documented precondition, and `launchTransition`'s background goroutine actually advances the
  transient state after `transitionDelay` (50ms). `UpdateApplication`'s optimistic-concurrency
  check (`CurrentApplicationVersionId` vs `ApplicationVersionId`) is real and enforced, as is
  every `Add*`/`Delete*` sub-resource op's `checkAndBumpVersion` call.
- **ApplicationStatus enum**: the six remaining real constants
  (DELETING/STARTING/STOPPING/READY/RUNNING/UPDATING) match `types.ApplicationStatus`'s six real
  v1 values exactly -- `statusUpdating` is a correct, present enum value, just unreachable
  because `UpdateApplication` is genuinely synchronous (verdict: legitimate simplification, same
  class as emrserverless's SUBMITTED-only run status and elasticsearch's unreached Processing
  state -- see gaps for the reasoning).
- **Cascade cleanup on delete / no ghost rows**: inputs/outputs/reference-data-sources/tags are
  all plain fields embedded directly on `Application` (not separate top-level maps), so
  `DeleteApplication` removing the `Application` row from `b.apps` inherently removes every
  sub-resource with it -- there is no separate cleanup step to forget. Confirmed no orphaned
  per-sub-resource maps exist anywhere in store.go/store_setup.go.
- **ListApplications**: `ApplicationSummaries`/`HasMoreApplications` pagination shape, and the
  absence of a `NextToken`, matches `types.ApplicationSummary` / `ListApplicationsOutput` -- no
  cursor-based pagination in the real v1 API.

## kinesisanalytics (this session, 2026-08-20)

Wrapper-key/nested-shape sweep. Protocol confirmed **awsjson1.1** (JSON-RPC): `X-Amz-Target:
KinesisAnalytics_20150814.<Op>` (`serializers.go:60` for AddApplicationCloudWatchLoggingOption,
same pattern for all 19 ops), single `POST /` endpoint, no URL routing -- no cnhp trap applies
(that trap is restjson-specific; this service has none).

**V1/V2 twin check (the reason this service was targeted): CLEAN.** Unlike kinesisanalyticsv2's
`Input.InputSchema`/`ReferenceDataSource.ReferenceSchema` (both required, never modelled), V1's
equivalents ARE fully modelled and populated end to end: `models.go:449` (`applicationInputConfig.
InputSchema`), `models.go:526` (`referenceDataSourceConfig.ReferenceSchema`), backed by a complete
`SourceSchema`/`RecordColumn`/`RecordFormat`/`MappingParameters`/`JSONMappingParameters`/
`CSVMappingParameters` sub-tree (`models.go:122-157`) that matches
`aws-sdk-go-v2/service/kinesisanalytics/types/types.go:955-1202` field-for-field, with required-ness
enforced in `applications.go`'s `convertSourceSchema`/`validateRecordFormatType`/
`validateRecordColumns`/`validateMappingParameters` (carried over from the prior sweep). A prior
sweep (2026-07-24) had already closed the exact gap this campaign's V2 finding predicted might
exist here -- this session independently re-verified it against
`aws-sdk-go-v2/service/kinesisanalytics@v1.33.4` rather than trusting that stamp.

**Full field-list diff, every op's Input/Output struct against `types.go`/`api_op_*.go`, optional
included, types checked**: all 19 ops' request/response shapes match exactly --
`CreateApplication`/`DeleteApplication`/`DescribeApplication`/`ListApplications`/
`StartApplication`/`StopApplication`/`UpdateApplication`/`ListTagsForResource`/`TagResource`/
`UntagResource`/`AddApplicationCloudWatchLoggingOption`/`AddApplicationInput`/
`AddApplicationInputProcessingConfiguration`/`AddApplicationOutput`/
`AddApplicationReferenceDataSource`/`DeleteApplicationCloudWatchLoggingOption`/
`DeleteApplicationInputProcessingConfiguration`/`DeleteApplicationOutput`/
`DeleteApplicationReferenceDataSource`/`DiscoverInputSchema` -- each field-by-field against
`api_op_<Op>.go` (SDK v1.33.4). One real bug found (below), everything else already correct from
the 2026-07-24 sweep.

**Enums checked both directions**: `ApplicationStatus` (6/6 real values present, none invented --
`store.go:33-38` vs `types/enums.go:5-13`); `RecordFormatType` restricted to exactly JSON/CSV
(`applications.go:320-322` vs `types/enums.go:53-61`); `InputStartingPosition` (NOW/TRIM_HORIZON/
LAST_STOPPED_POINT) is passed through as an opaque string with no gopherstack-side validation or
invented values -- consistent with the SDK's own validators.go not requiring it client-side either
(the field is optional on `InputStartingPositionConfiguration`).

**`ApplicationDetail` vs `ApplicationSummary`**: distinct, correctly. `ApplicationSummary`
(`models.go:180-184`, 3 fields: ARN/Name/Status) exactly matches
`types.ApplicationSummary` (`types/types.go:84-102`, same 3 required fields, no
`ApplicationVersionId`/timestamps/descriptions). `applicationDetail` (`models.go:214-227`, 12
fields) exactly matches `types.ApplicationDetail` (`types/types.go:16-76`, same 12 fields). No
summary/full confusion.

**The nine Output/Description/Update variants** (KinesisStreams/KinesisFirehose/Lambda x
Output/OutputDescription/OutputUpdate): each checked against its own SDK type in `types/types.go`
(lines 622-834) -- all nine match field-for-field (ResourceARN+RoleARN required on the request
form, both optional on the Description form, both `...Update`-suffixed on the Update form). Clean,
unchanged from the prior sweep.

### Real bug found and fixed

1. **`InputProcessingConfigurationDescription.InputLambdaProcessor` -> wrong wire key, should be
   `InputLambdaProcessorDescription`** (case a: member generalized from a sibling -- here the
   request-side name `InputLambdaProcessor` was reused on the response-side Description type
   instead of that type's own, differently-named member).
   - **Wrong**: `models.go:47-50` (before fix) had
     `InputProcessingConfigurationDesc{ InputLambdaProcessor *LambdaProcessorDesc
     \`json:"InputLambdaProcessor,omitempty"\` }`.
   - **Real**: `types.InputProcessingConfigurationDescription` has exactly one member,
     `InputLambdaProcessorDescription *InputLambdaProcessorDescription`
     (`aws-sdk-go-v2/service/kinesisanalytics/types/types.go:458-466`), and the deserializer reads
     that exact key (`deserializers.go:3611-3636`,
     `awsAwsjson11_deserializeDocumentInputProcessingConfigurationDescription`'s `case
     "InputLambdaProcessorDescription":` at line 3633). The request-side sibling type,
     `types.InputProcessingConfiguration.InputLambdaProcessor` (`types/types.go:441-452`), does
     correctly use the shorter name -- the bug was applying that request-side name to the
     response-side Description type instead of its own distinct member name.
   - **Blast radius**: every `DescribeApplication` response for an input with a processing
     configuration attached (set via `CreateApplication`, `AddApplicationInput`, or
     `AddApplicationInputProcessingConfiguration`, and surfaced by `UpdateApplication`'s internal
     state too) silently dropped the Lambda preprocessor's ResourceARN/RoleARN from the real
     client's parsed response -- the real SDK's `InputLambdaProcessorDescription` field always
     came back `nil` even though gopherstack's internal state held the correct values.
   - **Files fixed**: `models.go` (field+tag rename, with a doc comment recording the distinction
     from the request-side sibling), `applications.go` (`convertInputProcessingConfig`,
     `copyInputDescs`), `application_update.go` (`applyOneInputUpdate`),
     `application_inputs_test.go` (four call sites updated to the renamed field).
   - **Test**: `wire_sdk_roundtrip_test.go`,
     `TestDescribeApplication_InputLambdaProcessorDescription_SDKRoundTrip` -- drives
     `CreateApplication` -> `AddApplicationInput` (with `InputProcessingConfiguration`) ->
     `DescribeApplication` through the real `aws-sdk-go-v2` client over the actual
     `pkgs/service` router, and asserts
     `InputProcessingConfigurationDescription.InputLambdaProcessorDescription` is non-nil with the
     correct ARNs.
   - **Hand-revert symptom**: reverting the JSON tag back to `"InputLambdaProcessor"` (verified via
     the mandated `cp`-based hand-revert, not git) reproduces the exact failure: the real SDK
     client's `InputLambdaProcessorDescription` field decodes as `nil`, failing
     `require.NotNil` in the round-trip test with no other symptom (still a `200 OK`, silent data
     loss).

### Families verified clean this session (already correct, re-confirmed against the pinned SDK)

`CloudWatchLoggingOption`/`Description`/`Update`; `ApplicationUpdate`;
`InputProcessingConfiguration`/`InputProcessingConfigurationUpdate` (request/update sides, as
opposed to the Description side above) -> `InputLambdaProcessor`/`InputLambdaProcessorUpdate`;
`InputStartingPositionConfiguration`; `InputParallelism`/`InputParallelismUpdate`;
`DiscoverInputSchema`'s `InputSchema`/`ParsedInputRecords`/`ProcessedInputRecords`/
`RawInputRecords` response shape (still backed by the real sampling/inference logic from the prior
sweep -- `discover_schema.go`, 346 lines, unchanged).

### Gaps disclosed, not fixed (out of scope for a wrapper-key/nested-shape sweep)

- Per-op error coverage is incomplete relative to each op's own
  `awsAwsjson11_deserializeOpError<Op>` switch: `UnsupportedOperationException` (present on most
  mutating ops' error switches), `CodeValidationException` (`CreateApplication`/
  `AddApplicationInput`/`UpdateApplication`), `InvalidApplicationConfigurationException`
  (`StartApplication`), and `ResourceProvisionedThroughputExceededException`/
  `ServiceUnavailableException` (`DiscoverInputSchema`) are real SDK-defined exceptions on these
  ops that gopherstack's `errors.go` does not model or ever return. This is a structural
  completeness gap (Layer 3), not a wrapper-key/nesting bug, and out of this sweep's scope.

### Provenance verdict

`last_audit_commit: 6e7056ac` (before this session's update) was committed **2026-07-24 01:48:59
-0500** (`git show -s --format=%ad 6e7056ac`), matching `last_audit_date: 2026-07-24` exactly --
no stale-stamp gap, no false accusation. This session advances the stamp to the current HEAD
(`17458c2f`, committed 2026-08-20 16:54:05 -0500) and today's date, as required.

### Gates (verbatim)

```
$ go build ./services/kinesisanalytics/...
(no output, exit 0)
$ go vet ./services/kinesisanalytics/...
(no output, exit 0)
$ go fix -diff ./services/kinesisanalytics/...
(no output, exit 0)
$ gofmt -l services/kinesisanalytics/
(no output, exit 0)
$ go test -race ./services/kinesisanalytics/...
ok  	github.com/blackbirdworks/gopherstack/services/kinesisanalytics	1.204s
$ golangci-lint run ./services/kinesisanalytics/...
0 issues.
$ git status --short
 M services/kinesisanalytics/application_inputs_test.go
 M services/kinesisanalytics/application_update.go
 M services/kinesisanalytics/applications.go
 M services/kinesisanalytics/models.go
 M services/kinesisanalytics/PARITY.md
?? services/kinesisanalytics/wire_sdk_roundtrip_test.go
(services/pipes/* and .claude/ dirty entries belong to a concurrent, unrelated
session -- confirmed not touched by this sweep)
```

### Follow-up pass (2026-08-29, gopherstack-6flj/21my wrapper-key/silent-drop sweep, V1-vs-V2 lens)

Paired with `services/kinesisanalyticsv2` under the explicit instruction to verify V1
(this package) and V2 do not share Go types or assume shape parity. **Confirmed 0
shared types**: `grep -rn "kinesisanalytics\"" services/kinesisanalyticsv2/*.go` and the
reverse grep against this package both come back empty (the only cross-hit is an
unrelated ARN-namespace string literal in `kinesisanalyticsv2/store.go:109`); each
package has its own `models.go` and is registered under its own SDK module
(`kinesisanalytics@v1.33.4` vs. `kinesisanalyticsv2@v1.41.4` per `go.mod`). No op-level
V1/V2 naming collision exists within either package for this concern to apply to.

Independently re-derived member lists from the pinned SDK's own
`awsAwsjson11_deserializeDocument*`/`serializeOpDocument*` case switches (not `types.go`)
and diffed against this package's structs:
- `ApplicationDetail`: **12 of 12** deserializer cases (`deserializers.go:2870`), matching
  `models.go:219-230` exactly.
- `InputDescription`: **9 of 9** (`deserializers.go:3400`), matching `models.go:81-91`
  exactly; `InputID`/`InputStartingPositionConfiguration` traced to their actual write
  sites (`application_inputs.go:32`, `applications.go:486,641-642`) -- genuinely wired,
  not present-but-unpopulated.
- `OutputDescription`: **6 of 6** (`deserializers.go:4133`), matching `models.go:117-124`
  exactly.
- `CreateApplicationInput` (request side): **7 of 7** serializer fields
  (`serializers.go:2350`), all read and acted on in `handleCreateApplication`
  (`handler_applications.go:9-77`).

No new bugs found in this package this pass -- every spot-check matched the prior
audit's claims exactly, both request and response direction. The paired sweep of
`services/kinesisanalyticsv2` did find one real bug (`UpdateApplication` accepting and
applying a gopherstack-invented `ApplicationDescription` request member); see that
package's PARITY.md. `last_audit_commit`/`last_audit_date` above intentionally left
unchanged (no code in this package changed this pass).

### 2026-08-31 (error-target sweep, gopherstack-uox6 class-A campaign)

`errtargetaudit -dir kinesisanalytics` flagged 3 findings (`DeleteApplication`,
`DescribeApplication`, `StopApplication`, all `code=InvalidArgumentException`), all
real. Verified against `kinesisanalytics@v1.33.4` deserializers.go
(`awsAwsjson11_deserializeOpError<Op>` switches): `DeleteApplication` declares
`ConcurrentModificationException`/`ResourceInUseException`/`ResourceNotFoundException`/
`UnsupportedOperationException`; `DescribeApplication` declares
`ResourceNotFoundException`/`UnsupportedOperationException`; `StopApplication` declares
`ResourceInUseException`/`ResourceNotFoundException`/`UnsupportedOperationException`
-- none of the three declare `InvalidArgumentException`.

All three handlers pre-checked `ApplicationName == ""` and returned the shared
`errApplicationName` sentinel (-> `InvalidArgumentException`), an invented check: the
client-side SDK validator only rejects a nil `*string`, so `""` reaches the handler,
and each backend method already has a natural not-found lookup
(`b.apps.Get(applicationKey(region, name))`) that returns `awserr.ErrNotFound` ->
`ResourceNotFoundException`, which all three operations do declare. Deleted the
pre-check at all three call sites; `errApplicationName` stays unchanged for its 12
other legitimate callers (all declare `InvalidArgumentException` correctly:
`CreateApplication`, `StartApplication`, `UpdateApplication`, and the
Add/DeleteApplication{CloudWatchLoggingOption,Output,ReferenceDataSource,
InputProcessingConfiguration} family).

A second, closely related bug surfaced incidentally while writing the test for
`DeleteApplication`: its handler also pre-checked `CreateTimestamp == 0` and returned
the same undeclared `InvalidArgumentException` -- but `CreateTimestamp` is a required
`*time.Time` client-side (nil-only validation), so an explicit epoch-0 timestamp is a
legitimate wire value, not a "missing" one. The backend's own comparison
(`app.CreateTimestamp != nil && createTimestamp.Unix() != app.CreateTimestamp.Unix()`)
already answers a mismatched/zero timestamp correctly via `ErrConcurrentUpdate` ->
`ConcurrentModificationException` (declared). Deleted this pre-check too.

Test-first: `undeclared_invalidargument_test.go` (real SDK client, `errors.As` against
`*types.ResourceNotFoundException`) confirmed failing against the unmodified tree
(got `*smithy.OperationError` wrapping `InvalidArgumentException` for all three, and
again for `DeleteApplication` after the first fix once the test's epoch-0
`CreateTimestamp` collided with the second bug), then passing after both fixes.

Gates: `go build ./services/kinesisanalytics/...`, `go vet
./services/kinesisanalytics/...`, `go test -race -count=1
./services/kinesisanalytics/...` (pass), `golangci-lint run
./services/kinesisanalytics/...` (0 issues).

### 2026-09-07 (gopherstack-725q: CodeValidationException / UnsupportedOperationException, verdict: no code change)

Revisited the two exceptions flagged as a structural gap in the 2026-08-20 entry above. No
`api_op_*.go` file declares either error directly (`grep -l <name> api_op_*.go` in the SDK module
comes back empty for both) -- the per-op error set lives in `deserializers.go`'s
`awsAwsjson11_deserializeOpError<Op>` switches. Extraction:

- `CodeValidationException`: `AddApplicationInput`, `CreateApplication`, `UpdateApplication`.
- `UnsupportedOperationException`: `AddApplicationCloudWatchLoggingOption`,
  `AddApplicationInput`, `AddApplicationInputProcessingConfiguration`, `AddApplicationOutput`,
  `AddApplicationReferenceDataSource`, `DeleteApplication`,
  `DeleteApplicationCloudWatchLoggingOption`, `DeleteApplicationInputProcessingConfiguration`,
  `DeleteApplicationOutput`, `DeleteApplicationReferenceDataSource`, `DescribeApplication`,
  `StartApplication`, `StopApplication`.

`types/errors.go` doc comments (verbatim):
- `CodeValidationException`: "User-provided application code (query) is invalid. This can be a
  simple syntax error."
- `UnsupportedOperationException`: "The request was rejected because a specified parameter is not
  supported or a specified resource is not valid for this operation."

**`CodeValidationException`: not implementable.** The related field, `CreateApplicationInput.
ApplicationCode` (`*string`, optional -- not `This member is required`), is documented only as
"One or more SQL statements that read input data, transform it, and generate output" -- no length
limit, no format constraint, nothing in `validators.go` (no client-side validator exists for this
field at all). The exception is specifically about the SQL *compiling*, not about presence or
size. Gopherstack has no SQL engine to compile against; returning this error would mean rejecting
requests based on invented heuristics (e.g. string-matching for "SELECT"), which is the fabrication
this issue explicitly rules out. Verdict unchanged from 2026-08-20: genuinely unimplementable, not
merely unscoped.

**`UnsupportedOperationException`: not implementable.** Checked whether any of its 13 declaring
ops has a documented, checkable state precondition distinct from the sibling
`ResourceInUseException` ("Application is not available for this operation," already implemented
via `ErrResourceInUse` in `applications.go`'s `StartApplication`/`StopApplication`/
`checkAndBumpVersion`). Two ops' own doc comments do state a state precondition --
`StartApplication`: "The application status must be READY for you to start an application";
`StopApplication`: "You can stop an application only if it is in the running state" -- but both
map onto `ResourceInUseException`'s wording exactly and are already the two existing
`ErrResourceInUse` checks (`applications.go:672`, `applications.go:702`). None of the other 11 ops'
doc comments state any precondition at all, and `DescribeApplication` -- a pure read with no state
constraint under any real AWS semantics -- still declares this exception, which rules out "checkable
application state" as a uniform, documented trigger across the declaring set. No length/enum/
required-field angle exists either (checked each declaring op's own doc comment and struct fields;
none mention one). Implementing anything here would mean inventing a condition (e.g. "only one
CloudWatch logging option," "can't add an already-added input") that is real AWS product behavior
but not documented anywhere in this SDK module, which the issue rules out.

**Verdict: neither exception is implementable from a documented, checkable constraint. No code
changed.** This confirms and closes out the 2026-08-20 "gaps disclosed, not fixed" entry's
open question for these two error types specifically -- recorded here as a permanent divergence,
not a to-do.

Gates: `GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/kinesisanalytics/...` (pass),
`GOTOOLCHAIN=go1.26.6 golangci-lint run services/kinesisanalytics/...` (0 issues) -- both run
against the unmodified tree since no fix was made.
