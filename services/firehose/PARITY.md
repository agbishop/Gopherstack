# PARITY MANIFEST — services/firehose
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/firehose/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: firehose
sdk_module: aws-sdk-go-v2/service/firehose@v1.46.4
last_audit_commit: 3b475e203
last_audit_date: 2026-09-04
overall: A            # all 10 real SDK destination-configuration types now implemented; remaining gaps are documented data-movement-mechanics simplifications, not wire-shape bugs.
                      # 2026-09-06 pass (bd gopherstack-pe7x): fixed the CloudWatchLoggingOptions
                      # gap disclosed by the 2026-09-04 pass below -- delivery failures now actually
                      # reach the emulated CloudWatch Logs backend via SetCWLogsBackend/
                      # wireFirehoseCWLogs, not just a local slog warning. See the gaps entry below.
                      # 2026-09-04 pass (bd gopherstack-o4ny): closed the gopherstack-rop lead below --
                      # wired cli.go's wireFirehoseKinesisSource (called from
                      # wireStorageAndSecretsIntegrations) so SetKinesisBackend is now actually called
                      # in a real running server, and a KinesisStreamAsSource delivery stream really
                      # polls and ingests records from its source Kinesis stream. See kinesis_source ops
                      # entry and the matching gaps entry below.
                      # 2026-09-04 pass (bd gopherstack-rop): confirmed and fixed the services/kinesis
                      # audit's lead -- KinesisStreamAsSource has never actually polled Kinesis in a
                      # real running server (SetKinesisBackend is only ever called from this service's
                      # own tests, not from cli.go); a client can create such a stream and it silently,
                      # permanently never ingests anything. Fixed the honesty half in scope (a warning
                      # log on create, matching the Redshift precedent) and documented the cli.go wiring
                      # half as a new gap; the fix itself is cli.go, out of this pass's scope. Also found
                      # and fixed a real resource leak: Reset() (reachable via the production
                      # /_gopherstack/reset endpoint) never cancelled running Kinesis pollers, so any
                      # live poller outlived a Reset() forever. Also newly disclosed (not fixed, same
                      # deferred-wiring class as Redshift/Iceberg/Snowflake): CloudWatchLoggingOptions
                      # never actually writes to the emulated CloudWatch Logs backend.
                      # 2026-08-07 pass (bd gopherstack-ohdc): found and fixed a genuine silent-breakage
                      # bug in Redshift delivery -- deliverToRedshift constructed a real
                      # aws-sdk-go-v2/service/redshiftdata client via sdk_rddata.NewFromConfig with no
                      # endpoint override and no credentials, meaning every Redshift delivery attempt
                      # either hung on the default credential chain or failed against real AWS, not this
                      # emulator's own redshiftdata service -- Redshift delivery had never actually worked
                      # end to end despite ops/gaps previously describing it as "executes a synthesized
                      # INSERT statement" (true only in the sense that the code tried to, not that it
                      # succeeded). Replaced with the same in-process interface pattern S3Storer/
                      # LambdaInvoker already use (RedshiftDataExecutor + SetRedshiftDataBackend), and
                      # implemented real two-hop delivery: records are staged to the destination's
                      # required S3Configuration bucket via the existing writeRecordsToBucket helper,
                      # then a COPY command referencing the staged S3 object and the configured
                      # CopyCommand (DataTableName/DataTableColumns/CopyOptions) plus RoleARN credentials
                      # is issued via the new executor, with the existing exponential-backoff retry loop
                      # preserved. Wiring SetRedshiftDataBackend to the local redshiftdata backend in
                      # cli.go is out of this pass's scope (cli.go forbidden) -- same deferred-wiring
                      # pattern already established for cloudwatch's metric-stream Firehose delivery gap;
                      # unlike before, this is now a documented, honest no-op (logged once) rather than a
                      # silent live-network call that looked like real delivery and wasn't.

ops:
  CreateDeliveryStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "response is DeliveryStreamARN only, matches SDK. Added Iceberg/Snowflake/legacy-Elasticsearch destination-configuration parsing this pass; added the at-most-one-destination validation that was previously missing (see Notes). FIXED 2026-08-21 (gopherstack-us9u kind-mismatch sweep) -- MSKSourceConfiguration.ReadFromTimestamp was string; the real client serializes it as a JSON number (serializers.go: ok.Double(smithytime.FormatEpochSeconds(...))), so any real client setting an MSK source's ReadFromTimestamp failed CreateDeliveryStream's request decode outright (json: cannot unmarshal number into Go struct field ...ReadFromTimestamp of type string). Fixed by changing mskSourceConfigurationInput.ReadFromTimestamp to float64 (matching MSKSourceDescription's response-side fix below). KinesisStreamSourceDescription.DeliveryStartTimestamp is the same shape but never assigned anywhere in this backend (dead field, always omitted via omitempty) -- examined and left as-is; no live call path can be shown to fail on it today, see gopherstack-us9u notes for the follow-up issue tracking it for whenever Kinesis-source DeliveryStartTimestamp gets implemented. FIXED 2026-08-21 (gopherstack-r80d batch 28) -- see DescribeDeliveryStream's note; the request-side fields feeding this bug (S3DestinationConfiguration/ExtendedS3DestinationConfiguration.BufferingHints/EncryptionConfiguration/S3BackupConfiguration) are parsed here, buildS3DestinationDescription/buildS3BackupDescription now apply real-SDK defaults at this same choke point (shared with UpdateDestination). FIXED 2026-08-20: HttpEndpoint/Amazonopensearchservice/Splunk's single S3 bucket used the wrong wire key ('S3BackupConfiguration' instead of 'S3Configuration') — see 2026-08-20 Notes. FIXED 2026-08-23: AmazonOpenSearchServerlessDestinationConfiguration (the unimplemented 11th destination type) had no field in createDeliveryStreamInput at all, so a real client naming it as the sole destination was silently accept-and-dropped -- validateSingleDestination saw zero destinations and let the call through, creating a stream with NO destination and no error. Now detected (json.RawMessage presence marker) and rejected explicitly with InvalidArgumentException. See 2026-08-23 Notes. FIXED 2026-08-29 (write-only-state sweep): three real, accepted CreateDeliveryStreamInput members were silently dropped in their entirety -- createDeliveryStreamInput had no field for DeliveryStreamEncryptionConfigurationInput, DirectPutSourceConfiguration, or DatabaseSourceConfiguration at all (serializers.go:3813,3818,3822-area). DeliveryStreamEncryptionConfigurationInput was the highest-severity of the three: a client encrypting a stream at creation time (rather than a separate StartDeliveryStreamEncryption call) got a stream that was never actually encrypted -- s.Encryption stayed nil, so DescribeDeliveryStream's DeliveryStreamEncryptionConfiguration stayed absent and PutRecord/PutRecordBatch's Encrypted field stayed false, silently. Fixed by adding the field and routing it through the existing StartDeliveryStreamEncryption backend method (validated pre-create via the new shared validateEncryptionConfigInput, so an invalid CUSTOMER_MANAGED_CMK/KeyARN combination fails atomically rather than leaving a half-created stream). DirectPutSourceConfiguration.ThroughputHintInMBs and the full DatabaseSourceConfiguration wire shape (preview API; Databases/Tables/Columns include-exclude lists, auth/VPC configuration, SurrogateKeys, etc.) are now accepted, stored, and echoed via SourceDescription.DirectPutSourceDescription/DatabaseSourceDescription -- same documented-simplification pattern as MSK (wire shape real, no polling/replication mechanics). See wire_field_fixes_test.go. CONFIRMED 2026-09-07 (gopherstack-t2wb errtargetaudit sweep) -- a class A finding flagged this op referencing a ResourceNotFoundException sentinel (via TagDeliveryStream/StartDeliveryStreamEncryption, called immediately after this op's own successful create); false positive, guard cannot fire on the just-created name in any real single-client call path. No fix. See Notes."}
  DeleteDeliveryStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade-cleans all destination pointers, Tags registry, pending-flush watch entry, and Kinesis poller on delete — verified no ghost state survives across the 5 new destination fields added this pass."}
  DescribeDeliveryStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "Destinations[] wrapper extended this pass with IcebergDestinationDescription/SnowflakeDestinationDescription/ElasticsearchDestinationDescription entries, exact-case wire keys verified against deserializers.go. Snowflake's write-only PrivateKey/KeyPassphrase are correctly never echoed back (matches real SDK, which has no such fields on the Description type). FIXED 2026-08-21 (gopherstack-us9u) -- MSKSourceDescription.ReadFromTimestamp changed to float64; see CreateDeliveryStream's note (request+response share this fix). Proven via a real aws-sdk-go-v2/service/firehose client round trip through both ops (wire_msk_timestamp_test.go), hand-reverted/confirmed-failing (request: json: cannot unmarshal number into Go struct field ...ReadFromTimestamp of type string)/restored, md5sum-verified byte-identical. FIXED 2026-08-21 (gopherstack-r80d batch 28, required-output-member cut) -- 3 bugs in the S3-family Destinations[] entries, all traced through buildS3DestinationDescription/buildS3BackupDescription (types.go:2763 S3DestinationDescription's own required set): (1) BufferingHints/EncryptionConfiguration are *BufferingHints/*EncryptionConfiguration (required, optional on input per validateS3DestinationConfiguration/validateExtendedS3DestinationConfiguration only null-checking RoleARN/BucketARN) -- gopherstack passed the nil pointers straight through and both were tagged omitempty, so any client that simply never set these two common optional fields got a response missing both required members; fixed by defaulting to AWS's documented values (BufferingHints{SizeInMBs:5,IntervalInSeconds:300}, EncryptionConfiguration{NoEncryptionConfig:\"NoEncryption\"}) in buildS3DestinationDescription. (2) BucketARN/RoleARN are required *string on the real type but the real client-side validator only null-checks them, not their content, so a client can legally send an explicit empty string; gopherstack's non-pointer BucketARN/RoleARN fields were tagged omitempty, dropping the key entirely for that value -- omitempty removed (same 'client only null-checks the pointer' class the cognitoidp batch of this campaign established). (3) structurally-absent class: the real SDK's S3BackupConfiguration/S3BackupDescription fields (used by every backup-capable destination: S3, Redshift, OpenSearch, Elasticsearch, Splunk) are literally typed as S3DestinationConfiguration/S3DestinationDescription (types.go:1496,1575,2568,2621) -- the exact same required set as a primary S3 destination -- but gopherstack modeled the backup slot as its own narrower S3BackupDescription struct with no EncryptionConfiguration field at all, so any backup-enabled destination unconditionally dropped this required member on every single call, not merely when a client omitted it. Added the field to both s3BackupInput (request) and S3BackupDescription (response) and routed it through the same buildS3BackupDescription default. CompressionFormat (non-pointer CompressionFormat enum on the real type) was also defaulted to UNCOMPRESSED for correctness but is NOT counted as a proven bug -- omitted and present-empty decode identically for any real client, same as State in kafka's Configuration fix earlier this campaign. All 3 counted fixes proven via real aws-sdk-go-v2/service/firehose client round trips (wire_output_required_r80d_test.go), hand-reverted (all 3 touched files together via git show HEAD:<path>)/confirmed-failing/restored, md5sum-verified byte-identical. FIXED 2026-08-20: HttpEndpoint/Amazonopensearchservice/Splunk/Elasticsearch's single S3 bucket was returned under wire key 'S3BackupDescription' but the real deserializer reads 'S3DestinationDescription' for these 4 families — see 2026-08-20 Notes. FIXED 2026-08-29: DeliveryStreamEncryptionConfiguration/Source.DirectPutSourceDescription/Source.DatabaseSourceDescription now round-trip real values instead of staying permanently absent -- this op's own read path (deliveryStreamDescriptionFields.EncryptionConfiguration: s.Encryption, Source: s.Source) was already correct; the bug was entirely on CreateDeliveryStream's write side never populating those fields to begin with. See CreateDeliveryStream's note and wire_field_fixes_test.go."}
  ListDeliveryStreams: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20: DeliveryStreamType filter now accepts all 4 real enum values (DirectPut, KinesisStreamAsSource, MSKAsSource, DatabaseAsSource) — previously rejected the latter 2 with ErrValidation even though they are valid SDK enum values. FIXED 2026-09-07 (gopherstack-t2wb errtargetaudit sweep): this op's real declared error set is UnknownError only (deserializers.go) -- it cannot legitimately reject any input at all, so the 2026-08-20 fix did not go far enough. Deleted isValidDeliveryStreamType entirely; an unrecognized DeliveryStreamType filter value now just matches no stream instead of erroring, matching the declared-set evidence. See Notes."}
  PutRecord: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: Encrypted (optional bool) now populated from the stream's live SSE status via a new IsStreamEncrypted backend method (kept PutRecord's own signature unchanged — cli.go's snsFirehosePutterAdapter forwards PutRecordBatch's (int, error) return directly and could not be touched). RecordId (required *string) confirmed always populated via newRecordID; PutRecordBatchResponseEntry (checked for PutRecordBatch below) and this op's own required set re-verified against the real SDK's zero-required-member domain structs during the 2026-08-21 gopherstack-r80d batch-28 required-output sweep -- no bug here."}
  PutRecordBatch: {wire: ok, errors: ok, state: ok, persist: ok, note: "FailedPutCount always 0 — every record that reaches the backend has already passed validation, matching how this emulator models delivery (no partial-batch throttling). FIXED this pass: Encrypted now populated, same mechanism as PutRecord. Re-verified 2026-08-21 (gopherstack-r80d batch 28): RequestResponses always a non-nil make(...) slice, matching the required-array convention; PutRecordBatchResponseEntry itself declares zero required members in the real SDK (confirmed via AST walk of types.go) -- no bug here."}
  ListTagsForDeliveryStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "Re-verified 2026-08-21 (gopherstack-r80d batch 28): Tags/HasMoreTags always emitted (non-nil make(...) slice, no omitempty on the bool); tags.KV.Key carries no omitempty, matching the real Tag type's sole required member. No bug."}
  TagDeliveryStream: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagDeliveryStream: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "extended this pass with IcebergDestinationUpdate/SnowflakeDestinationUpdate/ElasticsearchDestinationUpdate, sharing the existing exactly-one-destination / CurrentDeliveryStreamVersionId optimistic-concurrency enforcement. FIXED 2026-08-21 (gopherstack-r80d batch 28) -- shares buildS3DestinationDescription/buildS3BackupDescription with CreateDeliveryStream, so the same 3 required-output-member fixes documented on DescribeDeliveryStream's note apply here too (this op itself returns an empty body, matching the real SDK's UpdateDestinationOutput, which has no members at all). FIXED 2026-08-20: the nested S3 bucket field on HttpEndpoint/Amazonopensearchservice/Splunk/Elasticsearch/Snowflake/Redshift/ExtendedS3 Update payloads used the Create-only wire key ('S3Configuration'/'S3BackupConfiguration') instead of the real Update key ('S3Update'/'S3BackupUpdate'), so a real client's Update-shaped bucket change was silently dropped — see 2026-08-20 Notes, the campaign's single biggest finding for this service. FIXED 2026-08-23: AmazonOpenSearchServerlessDestinationUpdate had no field in updateDestinationInput either -- a real client supplying only that key fell through to applyDestinationUpdate's generic 'exactly one destination update must be specified, got 0', which is misleading (the caller did supply one) though not state-corrupting. Now detected and rejected with an accurate 'not supported by this emulator' message. See 2026-08-23 Notes."}
  StartDeliveryStreamEncryption: {wire: ok, errors: ok, state: ok, persist: ok}
  StopDeliveryStreamEncryption: {wire: ok, errors: ok, state: ok, persist: ok}

families:
  destination_delivery: {status: ok, note: "All 10 real SDK destination-configuration types now field-diffed and implemented: S3, ExtendedS3, HttpEndpoint, Redshift, Amazonopensearchservice, legacy Elasticsearch (NEW this pass), Splunk, Iceberg (NEW), Snowflake (NEW). S3/HTTP/Redshift/OpenSearch/Elasticsearch/Splunk delivery pipelines verified as real (Lambda transform, dynamic partitioning, S3 backup, error-output routing, retry/backoff) — not disguised no-ops. Elasticsearch reuses the OpenSearch bulk-API delivery path (the two share an identical wire protocol; only the Firehose destination-configuration shape differs). Iceberg/Snowflake land processed records into their required S3Configuration staging bucket via the same writeRecordsToBucket helper S3 delivery uses — genuine state mutation, not a stub — but neither drives a real Iceberg/Glue-catalog commit or Snowflake Snowpipe-Streaming ingest; see gaps (same documented-simplification pattern as the pre-existing Redshift gap). AmazonOpenSearchServerless (a distinct 11th real SDK destination-configuration type, `AmazonOpenSearchServerlessDestinationConfiguration`) remains unimplemented — out of scope for this pass's explicit destination list, not field-diffed, do not mark ok."}
  kinesis_source: {status: ok, note: "KinesisStreamAsSource polling code (launchKinesisPoller) is real and well covered by kinesis_source_test.go. FIXED 2026-09-04 (gopherstack-o4ny): SetKinesisBackend is now actually called from cli.go -- wireFirehoseKinesisSource (cli.go, called from wireStorageAndSecretsIntegrations alongside wireFirehoseDelivery) wires the real Kinesis InMemoryBackend into Firehose via a kinesisStreamReaderAdapter (shared with kinesisanalyticsbackend's DiscoverInputSchema sampling -- both need the same narrow ListShards/GetShardIterator/GetRecords shape over the same real Kinesis backend), so shouldPoll now actually fires in a real running server and a KinesisStreamAsSource stream ingests records from its source stream for real. Previously (gopherstack-rop) SetKinesisBackend was only ever called from this service's own tests, so b.kinesisBackend was always nil in production; that pass added a CreateDeliveryStream WarnContext log for the nil-backend case (kept -- still correct for a deliberately-unwired backend, e.g. in tests), covered by TestFirehose_KinesisSource_NoBackendLogsWarning. The wiring fix itself is covered by TestFirehose_KinesisSource_Wiring in cli_test.go (asserts SetKinesisBackend's effect: a poller now starts and PollerCount goes to 1 immediately after CreateDeliveryStream on a wired backend) plus the pre-existing kinesis_source_test.go poller-mechanics tests."}

gaps:
  - >
    FIXED 2026-08-07 (bd gopherstack-ohdc): Redshift delivery now models AWS's actual
    two-hop delivery for real -- records are staged to the destination's required
    S3Configuration bucket via writeRecordsToBucket, then a COPY command referencing the
    staged S3 object and CopyCommand (DataTableName/DataTableColumns/CopyOptions/RoleARN
    credentials) is built and issued through a new RedshiftDataExecutor interface (mirroring
    the existing S3Storer/LambdaInvoker in-process pattern), replacing the previous
    implementation which constructed a live aws-sdk-go-v2/service/redshiftdata client
    pointed at real AWS with no credentials -- a genuinely silent bug: every Redshift
    delivery attempt in any environment before this fix would fail against real AWS
    infrastructure rather than deliver anywhere, despite looking like working code (see
    "overall" note above). Remaining gap: SetRedshiftDataBackend is not wired to the local
    redshiftdata backend in cli.go (forbidden in this pass's scope), so the COPY step is a
    documented, explicitly-logged no-op until a future pass wires it there -- staging to S3
    is real and unconditional regardless of wiring. Same deferred-wiring shape as the
    cloudwatch metric-stream-to-Firehose gap below.
  - >
    Iceberg and Snowflake destinations (new this pass) land processed records into their
    required S3Configuration staging bucket rather than driving a real Apache Iceberg/Glue
    Data Catalog commit or a real Snowflake Snowpipe Streaming ingest — this backend has no
    Iceberg-table or Snowflake-account backend to connect to. Wire shape for
    CreateDeliveryStream/UpdateDestination/DescribeDeliveryStream is fully field-diffed and
    correct (including CatalogConfiguration, DestinationTableConfigurationList,
    SchemaEvolutionConfiguration, TableCreationConfiguration for Iceberg, and
    SecretsManagerConfiguration/SnowflakeRoleConfiguration/SnowflakeVpcConfiguration for
    Snowflake); only the data-movement mechanics diverge, same documented-simplification
    pattern as the existing Redshift gap above. Deferred — no bd id filed yet.
  - >
    Legacy Elasticsearch (ElasticsearchDestinationConfiguration, new this pass) and the
    pre-existing Amazonopensearchservice family both omit VpcConfiguration/
    VpcConfigurationDescription (private-VPC ENI delivery) and DocumentIdOptions
    (Firehose-generated vs. OpenSearch-generated document IDs) — both are real, optional
    SDK fields on those destination types that are not modeled. Newly identified this pass;
    not a regression (OpenSearch was previously marked ok without this having been
    field-diffed). Deferred — low-traffic advanced configuration, no bd id filed yet.
  - >
    AmazonOpenSearchServerlessDestinationConfiguration (a real, distinct 11th destination
    type in the SDK, separate from Amazonopensearchservice) is still not implemented as a
    real delivery pipeline. FIXED 2026-08-23 (this pass): the accept-and-drop half of this
    gap -- CreateDeliveryStream/UpdateDestination had no field at all for this key, so a
    real client naming it as the SOLE destination got json.Unmarshal's silent-drop, then
    validateSingleDestination/applyDestinationUpdate saw zero destinations and let the call
    through, creating (or leaving) a stream with NO destination configured and no error --
    is now closed: both ops detect the key's presence (via a json.RawMessage marker field)
    and reject explicitly with ErrValidation ("... is not supported by this emulator")
    instead of silently succeeding. See CreateDeliveryStream/UpdateDestination ops entries
    and the 2026-08-23 Notes section below. The remaining, still-open half (a real
    OpenSearch-Serverless delivery pipeline) is unchanged and correctly still deferred.
  - >
    IDENTIFIED 2026-09-04 (gopherstack-rop, the services/kinesis audit's lead), FIXED
    2026-09-04 (gopherstack-o4ny): KinesisStreamAsSource had never actually polled Kinesis
    in a real running gopherstack server. The wire shape
    (SourceDescription.KinesisStreamSourceDescription) and the background poller
    (launchKinesisPoller/pollKinesisStream/pollKinesisShard in kinesis_source.go) were both
    real and well-tested in isolation, but SetKinesisBackend -- the only way b.kinesisBackend
    is ever set -- was called exclusively from this service's own tests, never from cli.go.
    CreateDeliveryStream's shouldPoll gate requires b.kinesisBackend != nil, so in production
    it was always false: a client could CreateDeliveryStream with
    DeliveryStreamType=KinesisStreamAsSource and a valid KinesisStreamSourceDescription, get
    back 200 OK and a real ARN, and that stream would never deliver a single record for as
    long as it existed -- a silent, permanent, client-observable no-op, exactly the no-stub
    violation parity-principles.md prohibits. gopherstack-rop fixed the honesty half in scope
    at the time (a warning log on create, matching the Redshift precedent; kept -- still
    correct for a deliberately-unwired backend, e.g. tests). gopherstack-o4ny fixed the wiring
    itself: cli.go's wireFirehoseKinesisSource (called from wireStorageAndSecretsIntegrations
    alongside wireFirehoseDelivery) now wires the real Kinesis InMemoryBackend into Firehose
    via SetKinesisBackend, using a kinesisStreamReaderAdapter shared with
    kinesisanalyticsbackend's DiscoverInputSchema sampling wiring (both need the same narrow
    ListShards/GetShardIterator/GetRecords shape over the same real Kinesis backend). Proof:
    TestInitializeServices_FirehoseKinesisSourceWiring (root package) drives the actual
    initializeServices composition root, puts a record into a real Kinesis stream, and
    verifies it lands in the destination S3 bucket through the real poller -- hand-reverted
    (git show HEAD:cli.go > cli.go) it fails (no S3 object ever appears; 10s Eventually
    timeout), restored it passes.
  - >
    FIXED 2026-09-06 (bd gopherstack-pe7x): CloudWatchLoggingOptions previously only
    reached logDeliveryIssue (flush.go) as a local slog WarnContext record, never writing
    an event to the emulated CloudWatch Logs backend's log group/stream. Added
    firehose.CWLogsBackend (EnsureLogGroupAndStream/PutLogLines, mirroring
    lambda.CWLogsBackend) plus SetCWLogsBackend, and wired it in cli.go's
    wireFirehoseCWLogs, reusing the existing cwLogsAdapter (lambda's adapter already
    matched the shape exactly). logDeliveryIssue now also calls
    deliverCWLogEvent, which ensures the destination's LogGroupName/LogStreamName exist
    and writes the same failure message via PutLogLines. Unwired (no SetCWLogsBackend
    call, e.g. every test backend) stays a silent no-op -- delivery still proceeds
    normally. See TestLambdaTransformError_DeliversCloudWatchLogEvent and
    TestLambdaTransformError_UnwiredCloudWatchLogsStaysPermissive (flush_test.go).

deferred:
  - Redshift RedshiftDataExecutor cli.go wiring (mechanics implemented 2026-08-07, see gaps)
  - Iceberg/Snowflake real catalog-commit / Snowpipe-Streaming ingest mechanics (see gaps)
  - Elasticsearch/OpenSearch VpcConfiguration and DocumentIdOptions fields (see gaps)
  - AmazonOpenSearchServerlessDestinationConfiguration real delivery pipeline (see gaps; the
    accept-and-drop request-side bug is now fixed, only the pipeline itself remains deferred)
  - MSK source ingestion path (present via SourceDescription wire shape, CreateDeliveryStream/
    DescribeDeliveryStream round-trip correctly). Real polling/ingestion is genuinely
    unimplemented: unlike KinesisStreamAsSource (wired via the KinesisReader interface, set
    by cli.go's service-wiring step), there is no MSK/Kafka backend wiring — adding one would
    require a new KafkaReader-style interface plus cli.go changes to wire services/kafka's
    backend in, and this pass's instructions explicitly forbid editing cli.go. Left exactly as
    found; not reclassified to ok.
  - Database source ingestion path (FIXED 2026-08-29: wire shape -- DatabaseSourceConfiguration/
    DatabaseSourceDescription, previously entirely unmodeled -- now round-trips correctly
    through CreateDeliveryStream/DescribeDeliveryStream, same as MSK above). Real
    snapshot/CDC polling against an actual MySQL/PostgreSQL endpoint is genuinely
    unimplemented and out of scope: DatabaseSourceDescription.SnapshotInfo is always an
    empty slice (honest -- no snapshot has ever been taken -- not fabricated), and there is
    no database-source backend wiring, same structural gap class as MSK.

leaks: {status: "fixed this pass", note: "FIXED 2026-09-04 (gopherstack-rop): Kinesis poller cancel funcs were tracked per region/name and cancelled on DeleteDeliveryStream, but NOT on Reset() -- the prior note's 'cancelled on DeleteDeliveryStream' claim was true but incomplete, since Reset() (reachable in production via the /_gopherstack/reset admin endpoint, cli.go's buildResetHandler) only cleared the streams table and closed Tags registries, leaving every b.pollerCancel entry both un-cancelled and un-forgotten: any live Kinesis-source poller goroutine kept running forever against a stream that had just been deleted out from under it (injectKinesisRecord's ErrNotFound on every attempt, logged forever, never terminating). Fixed by having Reset() cancel and clear every tracked poller, plus reset the pendingFlush/sortedNamesCache maps that had the same problem (stale region/name entries surviving a Reset with no compensating cleanup, unlike DeleteDeliveryStream which already clears both). Proof: kinesis_source_test.go's TestFirehose_KinesisSource_ResetCancelsPoller, hand-reverted (git show HEAD:services/firehose/store.go), confirmed failing (PollerCount stayed 1 after Reset instead of dropping to 0), restored, md5sum byte-identical. Prior-pass claims otherwise stand: tags.Tags registries closed on Delete/Reset; streamCopy (store.go) deep-copies all destination pointer fields correctly."}
---

## Notes

### 2026-09-07 (gopherstack-t2wb -- errtargetaudit class A findings, first triage of this block)

No prior error-envelope/errtargetaudit entry existed for this service; genuinely
untriaged. `GOTOOLCHAIN=go1.26.6 go run ./cmd/errtargetaudit`: `operations with SDK
ground truth: 124, resolved: 12, with an emission found: 11` -- coverage warning (only
10% of ops resolved to a handler; treated as unverified, not a signal this service is
thin). `class A findings (2)`.

Protocol confirmed AWS JSON 1.1 (`Firehose_20150804.<Op>` target,
`application/x-amz-json-1.1`): `services/firehose/handler.go`'s `handleError` emits
`{"__type": "<Code>Exception", "message": "..."}` off one global sentinel table
(`ErrNotFound` -> `ResourceNotFoundException`/404, `ErrAlreadyExists` ->
`ResourceInUseException`/400, `ErrValidation`/`awserr.ErrInvalidParameter`/JSON decode
errors -> `InvalidArgumentException`/400) -- exactly the global-sentinel-map shape
flagged by gopherstack-hdvu, so each finding below was checked for whether the firing
call site's *own* declared set (not just the sentinel's typical mapping) actually
allows the code, and whether the guard could fire in that call path at all.

Verified both findings against `deserializers.go`'s per-op typed-error switch (`awk
"/deserializeOpError<Op>\(/,/^}/" deserializers.go | grep -oE '"[A-Za-z0-9]+"'`, pinned
`firehose@v1.46.4`) rather than trusting the tool's classification:

- **`CreateDeliveryStream` / `ResourceNotFoundException`
  (`services/firehose/encryption.go:37`, `services/firehose/tags.go:32`) -- false
  positive, class 4 (guard cannot fire: resource created moments earlier in the same
  request).** Raw extraction for `CreateDeliveryStream`: `UnknownError`,
  `InvalidArgumentException`, `InvalidKMSResourceException`, `LimitExceededException`,
  `ResourceInUseException` -- no `ResourceNotFoundException`. The sentinel references
  are `StartDeliveryStreamEncryption`'s and `TagDeliveryStream`'s own (legitimate,
  correctly-declared) not-found guards, reached one hop away via
  `handler_delivery_streams.go`'s `handleCreateDeliveryStream`, which calls
  `h.Backend.TagDeliveryStream`/`h.Backend.StartDeliveryStreamEncryption` with the exact
  same `DeliveryStreamName` immediately after `h.Backend.CreateDeliveryStream` has just
  inserted a stream under that same name. Each backend method takes its own
  lock-then-release cycle (coarse `*lockmetrics.RWMutex` in `store.go`), so there is a
  narrow theoretical window for a concurrent `DeleteDeliveryStream` on that exact
  brand-new name to race it, but that is not a code path any single real client request
  can hit, and matches why the real op's own declared set has no such code at all (this
  is the same shape as elb's `CreateLoadBalancer`/`LoadBalancerNotFound` dismissal from
  the same campaign). No fix applied.
- **`ListDeliveryStreams` / `InvalidArgumentException`
  (`services/firehose/handler_delivery_streams.go:1084`, pre-fix line) -- real bug,
  FIXED.** Raw extraction for `ListDeliveryStreams`: `UnknownError` only -- the op
  declares no exception beyond that, so per this campaign's rule it cannot legitimately
  reject any input (the cloudcontrol `ListResourceRequests` precedent). The pre-existing
  `isValidDeliveryStreamType` validator rejected an unrecognized `DeliveryStreamType`
  filter value with `InvalidArgumentException`/400; deleted the validator (and the two
  enum constants -- `deliveryStreamTypeMSKSource`/`deliveryStreamTypeDatabaseSource` --
  that existed only to feed it) rather than remapping it. An unrecognized filter value
  now simply matches no stored stream and returns an empty (or partial) list, same as
  `ListDeliveryStreamsByType`'s existing string-equality filter already does for any
  other non-matching value -- no separate code path was added.
  `TestListDeliveryStreams_TypeFilter`'s final block previously asserted the wrong
  behavior (`400`) with no note that it was pinning a known-bad response; corrected to
  assert `200` and that the bogus filter matches neither existing stream. Neutered:
  reverted the `handler_delivery_streams.go` diff, confirmed it still built, ran the
  test -- failed exactly as expected at the new `require.Equal(t, http.StatusOK,
  recInvalid.Code)` line (`expected: 200, actual: 400`) -- then restored the fix.

No field was added to any persisted struct, so the `pkgs/persistence` snapshot-version
guard was not run.

Gates: `go build ./services/firehose/...` clean; `go test -race -count=1
./services/firehose/...` ok (1.8s); `golangci-lint run services/firehose/...` 0 issues.
Re-ran the tool after the fix: `with an emission found: 10`, `class A findings (1)` --
the dismissed `CreateDeliveryStream`/`ResourceNotFoundException` finding above;
`ListDeliveryStreams`'s finding is gone.

### 2026-08-29 pass: write-only-state sweep -- CreateDeliveryStream silently dropped three real request members

Sixteenth service swept this campaign against an already-deeply-audited (A-graded, six
prior campaign passes) manifest; found bugs anyway, per the campaign's "a prior pass does
not mean a service is done" rule. Method: enumerated every field of the real
`CreateDeliveryStreamInput` (aws-sdk-go-v2/service/firehose@v1.46.4/api_op_CreateDeliveryStream.go)
and diffed it field-by-field against `createDeliveryStreamInput` (handler_delivery_streams.go)
rather than trusting the existing field list, since every prior pass's field-diff had
implicitly assumed that list was complete.

Three real, accepted request members had no field in `createDeliveryStreamInput` at all
(silently dropped by `json.Unmarshal`, not merely mis-keyed):

1. **`DeliveryStreamEncryptionConfigurationInput`** (serializers.go:3818) -- a client can
   encrypt a stream at creation time instead of a separate `StartDeliveryStreamEncryption`
   call. Silently dropped meant the stream was never actually encrypted:
   `DescribeDeliveryStream`'s `DeliveryStreamEncryptionConfiguration` stayed absent and
   `PutRecord`/`PutRecordBatch`'s `Encrypted` field (added in an earlier pass, correctly
   reading `s.Encryption`) stayed `false` -- the read path was already right, the write
   path never populated it. Highest-severity of the three: a security-relevant field,
   silently ignored, no error. Fixed by adding the field and routing it through the
   existing `StartDeliveryStreamEncryption` backend method (shared validation extracted
   into `validateEncryptionConfigInput`, checked *before* `CreateDeliveryStream` runs so an
   invalid `CUSTOMER_MANAGED_CMK`/missing-`KeyARN` request fails atomically instead of
   leaving a stream created but not encrypted).
2. **`DirectPutSourceConfiguration`** (`ThroughputHintInMBs`) -- small, single-field,
   previously entirely unmodeled; `SourceDescription.DirectPutSourceDescription` was never
   emitted.
3. **`DatabaseSourceConfiguration`** -- a real, distinct source type (preview API; MySQL/
   PostgreSQL CDC) with a full nested wire shape (`DatabaseSourceAuthenticationConfiguration`,
   `DatabaseSourceVPCConfiguration`, `Databases`/`Tables`/`Columns` include-exclude lists,
   `SurrogateKeys`, `SSLMode`, etc.) that had zero representation anywhere in this service --
   no type, no field, no case. `ListDeliveryStreams` already validated the `DatabaseAsSource`
   `DeliveryStreamType` enum value (an earlier pass), which made the gap easy to
   miss-as-covered; the actual source configuration itself was never wired. Fixed by adding
   the full wire shape (`DatabaseSourceDescription` and nested types in models.go,
   `databaseSourceConfigurationInput` and nested input structs in
   handler_delivery_streams.go) with real accept/store/echo, same documented-simplification
   pattern as MSK: wire shape is real and field-diffed against
   `awsAwsjson11_serializeDocumentDatabaseSourceConfiguration`/
   `awsAwsjson11_deserializeDocumentDatabaseSourceDescription`, but no actual database
   connectivity/snapshot/CDC polling exists (`SnapshotInfo` always empty, honestly).

**Reverse-direction check (per the primer's "ask whether each response member is
computable" method)**: confirmed `DescribeDeliveryStream`'s own read path
(`deliveryStreamDescriptionFields.EncryptionConfiguration: s.Encryption`,
`Source: s.Source`) was already correct before this pass -- the bug was purely on the
write side (`CreateDeliveryStream` never populating `s.Encryption`/`s.Source` for these
three cases), not a paired read-side bug. No sibling fields of the ones touched needed a
matching fix: `StartDeliveryStreamEncryption`/`StopDeliveryStreamEncryption` were already
correct and are now reused, not duplicated.

**Proof**: `wire_field_fixes_test.go`, three tests driving the real
`aws-sdk-go-v2/service/firehose` client's `CreateDeliveryStream` through to
`DescribeDeliveryStream`/`PutRecord` for each fix (`TestCreateDeliveryStream_
EncryptionConfigurationRoundTrip`, `..._DirectPutSourceConfigurationRoundTrip`, `..._
DatabaseSourceConfigurationRoundTrip`, the last asserting non-empty
`Databases`/`Tables`/`Columns` include/exclude collections and the nested auth/VPC
configuration per the campaign's never-assert-over-an-empty-collection rule). All three
hand-reverted (`git show HEAD:<path>` restore of `handler_delivery_streams.go`/`models.go`/
`encryption.go`, confirmed all three fail with the exact predicted symptom -- nil
`DeliveryStreamEncryptionConfiguration`/`DirectPutSourceDescription`/
`DatabaseSourceDescription`), restored, `md5sum`-verified byte-identical against the
scratchpad backup taken before the revert.

**Gates**: `go build ./services/firehose/...`, `go vet`, `go test -race -count=1
./services/firehose/...` (pass), `golangci-lint run ./services/firehose/...` (0 issues,
`--fix` applied for fieldalignment on the new structs; two lines carry `nolint:lll` for
AWS's own long field names, same pattern as every other AWS-field-name line in this file).

**Ops not reached this pass**: no full per-op re-sweep of the other 14 ops was performed --
this pass targeted the write-only-state method specifically (every `CreateDeliveryStreamInput`
member vs. `createDeliveryStreamInput`), not a from-scratch field-diff of every op's full
shape (those were already covered by the six prior passes listed above and not re-verified
here beyond spot-checking the two touched ops' read paths). `UpdateDestination` was not
touched: `DatabaseSourceConfiguration`/`DirectPutSourceConfiguration`/encryption are
Create-only inputs in the real API (no corresponding Update op member), confirmed by their
absence from `UpdateDestinationInput`'s field list.

### 2026-08-23 pass: AmazonOpenSearchServerlessDestinationConfiguration accept-and-drop fixed

The pre-existing gap note for the unimplemented 11th destination type
(`AmazonOpenSearchServerlessDestinationConfiguration`/`-Update`) only disclosed that the
real delivery pipeline was unbuilt; it did not distinguish that from what actually happens
on the wire when a real client sends the key. Checked: `createDeliveryStreamInput` and
`updateDestinationInput` had no field for either wire key at all, so `json.Unmarshal`
silently dropped it. On `CreateDeliveryStream`, `validateSingleDestination` only rejects
`provided > 1`; a request naming this destination as its *only* one saw `provided == 0` and
was let through, creating a delivery stream with **zero** destinations and returning 200 —
a real accept-and-drop bug (the client believes OpenSearch-Serverless delivery is
configured; nothing is, and nothing signals that). On `UpdateDestination`,
`applyDestinationUpdate` requires exactly one; the same silent drop produced
`provided == 0`, so the call still failed, but with a misleading "got 0" message given the
caller *did* supply a destination.

Fixed by adding a `json.RawMessage` presence-marker field for each wire key
(`createDeliveryStreamInput.AmazonOpenSearchServerlessDestinationConfiguration`,
`updateDestinationInput.AmazonOpenSearchServerlessDestinationUpdate`) and rejecting
explicitly with `ErrValidation` ("... is not supported by this emulator") the moment either
is non-nil — before the request reaches `validateSingleDestination`/the backend at all.
This is the same "detect and reject rather than silently drop" pattern already established
in this campaign for unsupported tagged-union variants (e.g. route53resolver's
`FirewallAdvancedContentCategory`/`FirewallAdvancedThreatCategory`/
`PartnerThreatProtection`). The real delivery-pipeline gap itself (no OpenSearch-Serverless
backend to write to) is unchanged and remains correctly deferred — this fix only closes the
silent-success/misleading-error half.

Proof: `wire_aoss_destination_test.go` drives both ops through a real
`aws-sdk-go-v2/service/firehose` client. `TestCreateDeliveryStream_AmazonOpenSearchServerless_RejectedNotSilentlyDropped`
asserts `CreateDeliveryStream` returns an error and no stream is left behind — hand-reverted
to the pre-fix files (`cp` from `git show HEAD:...`), confirmed the call succeeds with **no
error** and a real `DeliveryStreamARN` (the exact silent-success bug), restored, `md5sum`
byte-identical. `TestUpdateDestination_AmazonOpenSearchServerless_RejectedNotConfusing`
asserts the rejection message says "not supported by this emulator" — hand-reverted,
confirmed it instead says "exactly one destination update must be specified, got 0",
restored, byte-identical. Gates: `go build ./services/firehose/...`, `go vet`, `gofmt -l`
(clean), `go test -race ./services/firehose/...` (pass), `golangci-lint run
./services/firehose/...` (0 issues, after manually reordering `updateDestinationInput`'s
fields to satisfy `fieldalignment` rather than running the `-fix` tool). No persisted
struct changed — the new fields are request-only DTOs (`createDeliveryStreamInput`/
`updateDestinationInput`), never stored in `backendSnapshot`; no snapshot version bump
needed.

### 2026-07-23 pass: added Iceberg/Snowflake/legacy-Elasticsearch destinations, CreateDeliveryStream validation, PutRecord Encrypted field

This pass brought the destination-family surface up to true parity against
`aws-sdk-go-v2/service/firehose@v1.42.11`. Enumerated the real SDK's destination-
configuration types directly from `types/types.go` (`grep 'type.*DestinationConfiguration
struct'`): `AmazonOpenSearchServerless`, `Amazonopensearchservice`, `Elasticsearch`,
`ExtendedS3`, `HttpEndpoint`, `Iceberg`, `Redshift`, `S3`, `Snowflake`, `Splunk` — 10 total.
gopherstack previously implemented 6 of these (S3/ExtendedS3, HttpEndpoint, Redshift,
Amazonopensearchservice, Splunk); **Iceberg, Snowflake, and legacy Elasticsearch were
entirely missing** — no types, no routing, no delivery. `AmazonOpenSearchServerless` remains
unimplemented (out of this pass's explicit scope; recorded as a gap, not silently dropped).

- **Iceberg / Snowflake**: full field-diffed wire shapes added for CreateDeliveryStream,
  UpdateDestination, and DescribeDeliveryStream (`IcebergDestinationConfiguration/-Update/
  -Description`, `SnowflakeDestinationConfiguration/-Update/-Description`, plus every nested
  type: `CatalogConfiguration`, `DestinationTableConfiguration`, `PartitionSpec`/
  `PartitionField`, `SchemaEvolutionConfiguration`, `TableCreationConfiguration`,
  `SnowflakeBufferingHints`, `SnowflakeRetryOptions`, `SecretsManagerConfiguration`,
  `SnowflakeRoleConfiguration`, `SnowflakeVpcConfiguration`), verified field-by-field against
  `serializers.go`/`deserializers.go`. Snowflake's write-only `PrivateKey`/`KeyPassphrase`
  input fields are correctly *not* stored on the Description type returned by Describe,
  matching the real SDK (`SnowflakeDestinationDescription` has no such fields — credentials
  are accepted but never echoed back). Delivery lands processed records into the
  destination's required `S3Configuration` bucket (real state mutation via the same
  `writeRecordsToBucket` helper S3 delivery uses) rather than driving an actual Iceberg/Glue
  commit or Snowflake ingest, which this backend has no connectivity to model — documented as
  a gap using the same pattern as the pre-existing Redshift INSERT-vs-COPY gap.
- **Legacy Elasticsearch**: `ElasticsearchDestinationConfiguration/-Update/-Description` is a
  real, wire-distinct API family from `Amazonopensearchservice` (confirmed via
  `deserializers.go` case `"ElasticsearchDestinationDescription"` and
  `serializers.go` keys `"ElasticsearchDestinationConfiguration"`/
  `"ElasticsearchDestinationUpdate"`) — not a gopherstack invention, and not the same thing as
  the existing OpenSearch family's doc-comment aside ("OpenSearch (Elasticsearch)"). Added
  with its own types and wire keys; delivery reuses `deliverToOpenSearch` since Elasticsearch
  and OpenSearch share an identical `_bulk` NDJSON wire protocol. While implementing this,
  identified that both Elasticsearch and the pre-existing Amazonopensearchservice family omit
  `VpcConfiguration`/`DocumentIdOptions` (real, optional SDK fields) — recorded as a new gap
  rather than silently carried forward, per the "independently field-diff and record what you
  verify" instruction.
- **CreateDeliveryStream single-destination validation** (previously an open gap): AWS
  rejects a `CreateDeliveryStream` request naming more than one destination configuration;
  gopherstack had no such check. Added `validateSingleDestination`, counting the
  S3/ExtendedS3 pair as one slot (real AWS treats them as mutually exclusive aliases for the
  same destination, matching the pre-existing `rawS3 := ExtendedS3 ?? S3` precedence logic).
- **PutRecord/PutRecordBatch `Encrypted` field** (previously an open gap): both real
  `PutRecordOutput`/`PutRecordBatchOutput` carry an optional `Encrypted *bool` reflecting the
  stream's live SSE status. Implemented via a new `InMemoryBackend.IsStreamEncrypted` method
  called separately by the handler *instead of* changing `PutRecord`/`PutRecordBatch`'s own
  signatures — `cli.go`'s `snsFirehosePutterAdapter.PutRecordBatch` forwards
  `a.backend.PutRecordBatch(...)` directly and depends on its existing `(int, error)` return
  shape; changing that signature would have broken the whole-repo build, and this pass's
  instructions forbid editing `cli.go`. (First attempt did change the signature and broke
  `go build ./...`; caught before returning, reverted, redone via the additive-method
  approach.)
- **Isolation fix**: `store.go`'s `streamCopy` (used by `DescribeDeliveryStream` and
  `AddStreamInternal`) did a field-by-field shallow copy that, before this pass, would have
  left the 3 new destination-pointer fields (Elasticsearch/Iceberg/Snowflake) aliased between
  the backend's live state and every caller's returned copy — the same class of bug the
  existing S3/HTTP/Redshift/OpenSearch/Splunk fields were already guarded against. Fixed by
  adding matching deep-copy blocks for all 3 new fields.
- **Lint**: adding the 8-field `IcebergDestinationDescription`/`icebergDestinationInput`
  structs pushed `currentDestinationID`/`hasActiveDestinationLocked`'s per-type branch chains
  over the cyclop complexity budget (18 and 22 respectively, limit 15). Decomposed rather than
  suppressed: `currentDestinationID`/`setDestinationID` now share a single
  `activeDestinationIDField` lookup instead of two parallel switch statements, and
  `hasActiveDestinationLocked` is split into `hasCoreActiveDestinationLocked` (S3/HTTP/
  Redshift — needs the lock, checks `b.s3`) and `hasSearchOrLakeActiveDestination`
  (OpenSearch/Elasticsearch/Splunk/Iceberg/Snowflake — pure function of stream state). Also
  ran `fieldalignment -fix ./services/firehose/...` to clear 2 govet fieldalignment findings
  on the new Iceberg structs. No `nolint:cyclop/gocyclo/gocognit/funlen` added anywhere.

Not attempted this pass (see gaps/deferred): Redshift real S3-staging+COPY mechanics (already
deferred, no change in scope/effort this pass), MSK source real polling (needs a new
KafkaReader-style interface and `cli.go` wiring, which this pass's instructions forbid
touching), `AmazonOpenSearchServerlessDestinationConfiguration` (11th real destination type,
out of this pass's explicit scope).

### CRITICAL (fixed this pass): DescribeDeliveryStream destination wrapping was entirely wrong shape

Real AWS `DeliveryStreamDescription` carries **one** field, `Destinations` (a list of
`DestinationDescription`), where each entry has a `DestinationId` plus exactly one
populated type-specific field (`ExtendedS3DestinationDescription`,
`HttpEndpointDestinationDescription`, `RedshiftDestinationDescription`,
`AmazonopensearchserviceDestinationDescription`, `SplunkDestinationDescription`, etc. —
confirmed against `aws-sdk-go-v2/service/firehose@v1.42.11/deserializers.go`,
`awsAwsjson11_deserializeDocumentDeliveryStreamDescription` /
`...DestinationDescription`).

Before this pass, gopherstack's handler emitted **five separate top-level list fields**
(`S3DestinationDescriptions`, `HTTPEndpointDestinationDescriptions`,
`RedshiftDestinationDescriptions`, `AmazonOpenSearchServiceDestinationDescriptions`,
`SplunkDestinationDescriptions`) that do not exist anywhere in the real API. Because
`aws-sdk-go-v2`'s deserializer switches on exact JSON key names and silently discards
unknown keys (`default: _, _ = key, value`), **every real SDK client calling
DescribeDeliveryStream against gopherstack got back zero destinations, regardless of how
the stream was actually configured** — `DeliveryStreamDescription.Destinations` was
always `nil`/empty. This is a client-breaking bug matching the exact bug class called out
in `.claude/memories/parity-principles.md` (missing/incorrect response-root nesting), and
was undetected because the service's own unit tests (`audit_firehose_test.go`,
`parity_b_test.go`, `handler_test.go`, `handler_accuracy_batch2_test.go`) all asserted
against the wrong (self-consistent but AWS-incorrect) field names directly, rather than
round-tripping through the real SDK's deserializer — exactly the trap Principle #3 warns
about ("Unit tests are not parity proof").

Fix: `handler.go` now builds a single `Destinations []destinationDescriptionOutput` list
(one entry per configured destination — in practice always ≤1, since
`applyDestinationUpdate` enforces exactly one destination type per stream after the first
`UpdateDestination` call), with the correct exact-case wire keys, including the
AWS-idiosyncratic `AmazonopensearchserviceDestinationDescription` casing (not
`AmazonOpenSearchServiceDestinationDescription`) and `HttpEndpointDestinationDescription`
(not `HTTPEndpointDestinationDescription`). `RedshiftDestinationDescription` did not
previously track its own `DestinationId`; `currentDestinationID`/`setDestinationID` in
backend.go were extended to cover it (S3/HTTP/OpenSearch/Splunk already did).

### Fixed: DeliveryStreamEncryptionConfiguration key name

`DescribeDeliveryStream` returned the encryption block under `"EncryptionConfiguration"`;
the real wire key is `"DeliveryStreamEncryptionConfiguration"` (confirmed via
`awsAwsjson11_deserializeDocumentDeliveryStreamDescription`, case
`"DeliveryStreamEncryptionConfiguration"`). Inner fields (`Status`, `KeyType`, `KeyARN`,
`FailureDescription`) were already correct.

### Fixed: Redshift `CopyCommand` / `S3Configuration` nesting (input AND output)

Real AWS `RedshiftDestinationConfiguration`/`RedshiftDestinationDescription` requires:
- `CopyCommand: {DataTableName, DataTableColumns, CopyOptions}` — nested, not flat.
- `S3Configuration` — a **required**, separate S3 destination distinct from
  `S3BackupConfiguration` (the intermediate staging bucket Redshift's COPY reads from).

gopherstack previously modeled `DataTableName`/`DataTableColumns`/`CopyOptions` as flat
fields directly on the Redshift destination object and had no `S3Configuration` field at
all. A real SDK `CreateDeliveryStream`/`UpdateDestination` request nests these under
`CopyCommand`, so the flat fields were silently never populated from real requests
(`DataTableName` etc. always ended up empty), and `S3Configuration` was dropped entirely.
Fixed: `backend.go` gained `RedshiftCopyCommand` and `RedshiftDestinationDescription.
S3Destination`/`.CopyCommand`; `handler.go`'s `redshiftDestinationInput` /
`buildRedshiftDestination` updated to parse and round-trip both correctly. Verified
against `aws-sdk-go-v2/service/firehose@v1.42.11/serializers.go`
(`awsAwsjson11_serializeDocumentRedshiftDestinationConfiguration`) and `deserializers.go`
(`awsAwsjson11_deserializeDocumentRedshiftDestinationDescription`,
`awsAwsjson11_deserializeDocumentCopyCommand`).

### Confirmed correct (no change needed)

- `AmazonopensearchserviceDestinationConfiguration` as the **input** key for
  CreateDeliveryStream/UpdateDestination: gopherstack's struct tag reads
  `AmazonOpenSearchServiceDestinationConfiguration`. This differs in *capitalization
  pattern* from the real wire key but is character-for-character identical when
  lower-cased, and Go's `encoding/json.Unmarshal` falls back to case-insensitive field
  matching — so real SDK requests parse correctly today. Left as-is (cosmetic only, not a
  bug); flagged here so a future auditor doesn't waste time re-deriving this.
- S3/HTTP/OpenSearch/Splunk nested field names (`BufferingHints`, `CloudWatchLoggingOptions`,
  `ProcessingConfiguration`, `RetryOptions`, `S3BackupDescription`/`S3BackupMode`,
  `EncryptionConfiguration`, `DynamicPartitioningConfiguration`,
  `DataFormatConversionConfiguration`, etc.) were checked field-by-field against the SDK
  deserializers and match exactly.
- Timestamps: `CreateTimestamp`/`LastUpdateTimestamp` correctly emitted as epoch-second
  JSON numbers (`time.Unix()`), matching `smithytime.ParseEpochSeconds` on the SDK side.
- Tag/list pagination cursors (`ExclusiveStartTagKey`, `ExclusiveStartDeliveryStreamName`,
  `Limit`/`HasMore*`) match the SDK request/response shapes.
- PutRecord/PutRecordBatch base64 `Data` decoding, 1000KB per-record / 500-record /
  4MiB-batch limits, and error codes (`InvalidArgumentException`) all verified correct.
- `S3DestinationDescription`/`HTTPEndpointDestinationDescription`/etc. each carry their own
  `DestinationID` field (wire tag `DestinationId`) for internal UpdateDestination
  bookkeeping. Real AWS only carries `DestinationId` on the *enclosing*
  `DestinationDescription` wrapper, not nested inside each type-specific description, so
  this produces one harmless extra field per destination in the response body. Real SDK
  clients ignore unrecognized keys (deserializer `default:` branch), so this does not
  break clients; left in place because removing it would require restructuring how
  `UpdateDestination` version-tracks the active destination ID, and the correct
  wrapper-level `DestinationId` (via `destinationIDOrDefault`) is what
  `destinationDescriptionOutput.DestinationID` actually carries.

### Error handling

`handleError` covers `ErrNotFound` → `ResourceNotFoundException` (404),
`ErrAlreadyExists` → `ResourceInUseException` (400), `errUnknownAction` →
`UnknownOperationException` (400), and a shared `InvalidArgumentException` (400) bucket
for `ErrValidation`/`awserr.ErrInvalidParameter`/JSON decode errors — this also correctly
covers `ErrRecordTooLarge`/`ErrBatchTooLarge` since both wrap
`awserr.ErrInvalidParameter`. No missing `errCodeLookup`-style gap found (all sentinel
errors route through the switch above; nothing falls through to the generic 500 bucket
except genuinely unexpected internal errors).

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, unbumped retype

`firehoseSnapshotVersion` bumped 1 -> 2. `d83f4b5d3` retyped
`MSKSourceDescription.ReadFromTimestamp` (nested inside the registered `streams` table's
value type via `DeliveryStream.Source`) from `string` to `float64`, matching the real
deserializer's epoch-seconds number, without bumping the snapshot version. A pre-fix (v1)
snapshot's non-empty `"ReadFromTimestamp"` string no longer unmarshals into the new
float64 field at all -- `RestoreAll` now errors outright rather than silently losing data,
but the whole backend then fails to restore, which the version guard exists to convert
into a clean, recoverable "discard and start empty" instead. (The sibling request-side DTO
`mskSourceConfigurationInput`, changed in the same commit, is a handler-only unmarshal
target, never persisted, so it carries no snapshot-compatibility concern of its own.)

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration.

**Proof:** `TestInMemoryBackend_RestoreV1MSKReadFromTimestampDiscarded` (persistence_test.go)
builds a v1-shaped `streams` snapshot with a string-valued `ReadFromTimestamp` and asserts
`Restore` succeeds (discarding cleanly) rather than erroring. Hand-reverted to version 1:
the same test then fails with `Restore` returning `json: cannot unmarshal string into Go
struct field MSKSourceDescription.source.MSKSourceDescription.ReadFromTimestamp of type
float64`, confirming the symptom; restored and `md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).
### 2026-08-20 pass: systemic Create-vs-Update S3 wire-key mismatch across 6 destination families, response-side S3 key mismatch across 4, Redshift/OrcSerDe missing members, ListDeliveryStreams enum gap

Wrapper-key/nested-shape sweep against `aws-sdk-go-v2/service/firehose@v1.46.4`
(unchanged since the 2026-08-07 audit; only the sdk_module pin, not the local
files, needed re-verification — confirmed no local drift via
`git diff 198990e82..05693f4fa -- services/firehose/` touching only delivery
mechanics files (`delivery_redshift.go`, `delivery_s3.go`, `store.go`, `flush.go`,
`interfaces.go`), not any wire-shape file). Protocol confirmed JSON-RPC 1.1
(`awsjson11`, `application/x-amz-json-1.1`, `X-Amz-Target:
Firehose_20150804.<Op>` — `deserializers.go` prefix `awsAwsjson11_`).

**Root cause, found by reading every destination family's real
`serializers.go`/`deserializers.go` function body rather than trusting the
prior pass's field lists**: for HttpEndpoint, Amazonopensearchservice,
Splunk, Elasticsearch, and Snowflake, the destination's single S3 bucket is
wire-keyed **differently on every one of Create / Update / Describe**:

| family | Create key | Update key | Describe key |
|---|---|---|---|
| HttpEndpoint | `S3Configuration` | `S3Update` | `S3DestinationDescription` |
| Amazonopensearchservice | `S3Configuration` | `S3Update` | `S3DestinationDescription` |
| Splunk | `S3Configuration` | `S3Update` | `S3DestinationDescription` |
| Elasticsearch | `S3Configuration` | `S3Update` | `S3DestinationDescription` |
| Snowflake | `S3Configuration` | `S3Update` | `S3DestinationDescription` (already correct pre-pass) |
| Redshift | `S3Configuration`/`S3BackupConfiguration` | `S3Update`/`S3BackupUpdate` | `S3DestinationDescription`/`S3BackupDescription` (already correct) |
| ExtendedS3 (own backup) | `S3BackupConfiguration` | `S3BackupUpdate` | `S3BackupDescription` (already correct) |
| Iceberg | `S3Configuration` (both ops) | (same) | `S3DestinationDescription` (already correct) |

Confirmed by reading `awsAwsjson11_serializeDocumentHttpEndpointDestinationConfiguration`
(serializers.go:2075), `...HttpEndpointDestinationUpdate`,
`...AmazonopensearchserviceDestinationConfiguration`/`-Update`,
`...SplunkDestinationConfiguration`/`-Update`,
`...ElasticsearchDestinationConfiguration`/`-Update`,
`...SnowflakeDestinationConfiguration`/`-Update`,
`...RedshiftDestinationConfiguration`/`-Update` (2834/2915),
`...ExtendedS3DestinationConfiguration`/`-Update`, plus the matching
`awsAwsjson11_deserializeDocument<Family>DestinationDescription` functions
(each has a literal `case "S3DestinationDescription":` for the 5 single-bucket
families, not `"S3BackupDescription"`).

Before this pass, `handler_delivery_streams.go`'s shared parsing structs
(`httpEndpointDestinationInput`, `openSearchDestinationInput`,
`splunkDestinationInput`, `elasticsearchDestinationInput`,
`snowflakeDestinationInput`, `redshiftDestinationInput`, `s3DestinationInput`)
each carried exactly **one** fixed JSON tag per S3 sub-field, reused verbatim
for both the Create and Update top-level wire objects (a deliberate,
otherwise-reasonable simplification — most sibling fields genuinely share a
name across Create/Update). That assumption is false for every nested S3
bucket field in this service:

- **Create-side bug (3 families)**: `httpEndpointDestinationInput`,
  `openSearchDestinationInput`, `splunkDestinationInput` tagged their S3
  field `json:"S3BackupConfiguration"` — the real Create wire key is
  `S3Configuration` (confirmed "This member is required" in
  `types.HttpEndpointDestinationConfiguration.S3Configuration`'s doc comment).
  A real client's `CreateDeliveryStream` for these 3 families could never
  populate the backup/only S3 bucket at all.
- **Update-side bug (6 families + ExtendedS3's own backup)**: every family's
  nested S3 field(s) only recognized the Create-side key name, so a real
  client's `UpdateDestination` (which the SDK serializes under `S3Update`/
  `S3BackupUpdate`) left the bucket **silently unchanged**.
- **Response-side bug (4 families)**: `HTTPEndpointDestinationDescription`,
  `OpenSearchDestinationDescription`, `ElasticsearchDestinationDescription`,
  `SplunkDestinationDescription` echoed the bucket under wire key
  `S3BackupDescription`; the real deserializer's `case` list uses
  `S3DestinationDescription` for these 4 (confirmed
  `awsAwsjson11_deserializeDocumentHttpEndpointDestinationDescription` etc.)
  — so even a correctly-stored bucket would never reach a real SDK client's
  `DescribeDeliveryStreamOutput`.

**Fix**: each affected input struct gained a second field for the Update-only
key (`S3Update`/`S3BackupUpdate`), with build functions (`buildHTTPEndpointDestination`,
`buildOpenSearchDestination`, `buildSplunkDestination`,
`buildElasticsearchDestination`, `buildSnowflakeDestination`,
`buildRedshiftDestination`, `buildS3DestinationDescription`) preferring
whichever is non-nil (a real client sends exactly one). The 3 Create-key
fields were renamed to the correct `S3Configuration` tag. The 4 response
struct tags were changed from `S3BackupDescription` to
`S3DestinationDescription`. Grepped every existing test asserting these keys
(`handler_delivery_streams_test.go`, `destination_elasticsearch_test.go`) —
one self-consistent-but-wrong test per bug (HTTP endpoint Create using
`S3BackupConfiguration`, Elasticsearch Describe asserting
`d["S3BackupDescription"]`) corrected to the real wire keys.

**Severity note per the campaign's severity rule**: these are not fabricated
fields — they are real fields under the wrong key, so nothing was
"harmlessly" extra; the effect is the opposite of harmless: the backup/
staging bucket for 6 of firehose's destination families was **unreachable
from a real SDK client in at least one direction** (Create, Update, or
Describe) before this fix, and for HttpEndpoint/OpenSearch/Splunk in **three**
directions.

**Secondary findings, same read-the-whole-struct method**:

- `RedshiftDestinationConfiguration`/`-Description` (types.go) has real
  `CloudWatchLoggingOptions`/`SecretsManagerConfiguration` fields gopherstack
  had never modeled at all (present on every other destination family;
  Redshift alone was missing them) — added to `redshiftDestinationInput`,
  `RedshiftDestinationDescription`, and `buildRedshiftDestination`.
- `types.OrcSerDe` has a real `PaddingTolerance *float64` field absent from
  gopherstack's `OrcSerDe` (formats.go) — added.
- `ListDeliveryStreams`'s `DeliveryStreamType` filter validator
  (`isValidDeliveryStreamType`) only accepted 2 of the real enum's 4 values
  (`DirectPut`, `KinesisStreamAsSource`) — `MSKAsSource`/`DatabaseAsSource`
  (confirmed real values, `types/enums.go`) were rejected with
  `ErrValidation` instead of returning a (possibly empty) filtered list.
  Fixed by extending the switch to all 4.
- Verified plain (deprecated, non-Extended) `S3DestinationConfiguration`
  genuinely lacks `ProcessingConfiguration`/`DynamicPartitioningConfiguration`/
  `DataFormatConversionConfiguration`/`FileExtension`/`CustomTimeZone`/
  `S3BackupMode`/`S3BackupConfiguration` (all Extended-only) — gopherstack's
  shared `s3DestinationInput` supersets both shapes, which is harmless since a
  real client using the deprecated field never sends those keys anyway; not
  reclassified as a bug.
- `ProcessingConfiguration`/`Processor`/`ProcessorParameter` field sets,
  `ProcessorType`/`ProcessorParameterName` enum value spellings, and the two
  `DataFormatConversionConfiguration` SerDe unions (`HiveJsonSerDe`/
  `OpenXJsonSerDe` deserializer, `ParquetSerDe`/`OrcSerDe` serializer, both
  discriminated by which of the 2 struct pointers is non-nil, matching the
  real SDK's own union modeling) verified field-by-field against
  `types.go`/`enums.go` — all correct except the `OrcSerDe.PaddingTolerance`
  gap above.
- `VpcConfiguration`/`VpcConfigurationDescription`: confirmed **absent on
  both the request and response side** for Elasticsearch/Amazonopensearchservice
  (already a disclosed gap from 2026-07-23) — no request-only leak, since
  gopherstack has no `VpcConfiguration`-shaped field anywhere in this service.
- `ListDeliveryStreams`' `DeliveryStreamNames []string` + `HasMoreDeliveryStreams
  bool` shape (a bare string list plus a bool, not objects, not a NextToken) —
  confirmed correct against `api_op_ListDeliveryStreams.go`.
- Iceberg's Update variant is the sole family whose S3 field keeps the
  Create-style key `S3Configuration` on both ops (real SDK; confirmed
  `awsAwsjson11_serializeDocumentIcebergDestinationUpdate`) — no change
  needed there, called out explicitly to avoid a future pass "fixing" it by
  false generalization from the other 6 families.

**Every fix proven by hand-revert** (`cp` to scratchpad, restore original
content, re-run the new SDK round-trip tests, confirm failure, restore fix,
confirm pass) — see `services/firehose/wire_sdk_roundtrip_test.go`, a new
file exercising the real `aws-sdk-go-v2/service/firehose` client end-to-end
through `pkgs/service`'s router (not gopherstack's own JSON tags), covering
all 7 affected families' Update-path S3 key, the Redshift
CloudWatchLoggingOptions/SecretsManagerConfiguration round-trip, the OrcSerDe
PaddingTolerance round-trip, and all 4 `DeliveryStreamType` filter values.

Not reached this pass: `AmazonOpenSearchServerlessDestinationConfiguration`
(11th destination family, out of scope, pre-existing disclosed gap), MSK/
Database source real polling (structural gap, `cli.go` wiring forbidden),
Redshift `RedshiftDataExecutor` `cli.go` wiring (pre-existing disclosed gap).
