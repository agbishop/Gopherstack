---
service: dynamodb
sdk_module: aws-sdk-go-v2/service/dynamodb@v1.67.0   # version audited against (go.mod pin)
last_audit_commit: 97805509b
last_audit_date: 2026-08-23  # manifest-harvest pass: fixed UpdateGlobalTableSettings autoscaling
  # accept-and-drop gap and DisableKinesisStreamingDestination's never-echoed
  # EnableKinesisStreamingConfiguration -- see global_table_settings_autoscaling/
  # kinesis_streaming_disable_echo families below. Did not re-litigate
  # ConfirmRemoveSelfResourceAccess (no IAM evaluator, gopherstack-cu4g) or
  # UpdateTableReplicaAutoScaling's ReplicaUpdates (no per-replica field to
  # route into) -- both re-verified genuine in a prior pass.
overall: A   # gopherstack-rkmp deep pass (this audit, 2026-08-14): struct-field-diffed every wire model against the pinned SDK (see Notes) and fixed 3 more wire drops -- Query/Scan AttributesToGet (undeclared, and even where declared elsewhere the projection resolver never consulted it for these two ops), GSI/LSI IndexArn (+GSI IndexSizeBytes/Backfilling), ListBackups BackupSummary.BackupSizeBytes. PARITY.md itself was stale by 6 commits (7a2189b06..bc2e6285a) before this update -- see Notes. CONFIRMED FIXED, previously an open gap here: GSI/LSI Query full-scan (17c0ac7a7 added real per-GSI/LSI indexes; gopherstack-anlc verified 4.8-5.0us flat vs 1.82-28.0ms before). gopherstack-lze5 (2026-08-14, follow-up pass): PutItem/UpdateItem/DeleteItem's legacy Expected/ConditionalOperator/AttributeUpdates parameters -- the conditional-check-bypass and no-op-write bugs -- are now FIXED by translation into the existing expr evaluator. gopherstack-yvs8 (2026-08-14, follow-up to lze5): Query/Scan's legacy KeyConditions/QueryFilter/ScanFilter -- the "ScanFilter/QueryFilter silently returns every item" and "KeyConditions silently dropped" failure modes -- are now FIXED the same way (translation into KeyConditionExpression/FilterExpression, reusing the existing evaluator paths); see gaps for the KeySchema-reordering writeup. ReturnConsumedCapacity=INDEXES dead code (gopherstack-glfv) also still open -- see gaps.
protocol: json-1.0 (DynamoDB_20120810 targets)
families:
  item_crud:    {status: ok, note: PROVEN — condition eval, all ReturnValues, ItemCollectionMetrics/LSI 10GB, WCU/RCU formulas. 2026-08-13: GetItem's wire model (models.GetItemInput/GetItemOutput) was silently dropping ReturnConsumedCapacity, ConsistentRead, AttributesToGet on input and ConsumedCapacity on output even though the backend computed everything correctly -- fixed at the wire boundary in models/convert_ops.go, not the backend. Same day, separately: CreateTable dropped SSESpecification/OnDemandThroughput on input (7a2189b06); UpdateTable dropped DeletionProtectionEnabled/TableClass/BillingMode/SSESpecification (7a2189b06); DescribeBackup/DeleteBackup dropped two required SourceTableDetails members (bc2e6285a). 2026-08-14 (this audit): ListBackups' BackupSummary had no BackupSizeBytes field at all, even though CreateBackup/DescribeBackup's BackupDetails already carried it for the same backup via the real per-backup b.SizeBytes -- fixed in models/types.go + backup_ops.go's collectBackupSummaries. 2026-08-14 (gopherstack-lze5): PutItem/UpdateItem/DeleteItem's legacy Expected/ConditionalOperator (conditional-write) and UpdateItem's AttributeUpdates (legacy update) parameters were wire-serialized (confirmed against serializers.go) but declared nowhere in models/types.go, so a legacy client's conditional check or update silently never happened -- 200 OK either way. Fixed by translating them into an equivalent ConditionExpression/UpdateExpression (synthesized #name/:value placeholders) and reusing the exact same evaluator path PutItem/UpdateItem/DeleteItem already use for the modern expression API, rather than a second evaluation engine -- see gaps for the full writeup and citations. legacy_conditional_params_test.go drives the real aws-sdk-go-v2 client and asserts behaviour (blocked writes stay unchanged, ADD/DELETE/PUT actually mutate the item), each hand-verified to fail against unfixed code.}
  query_scan:   {status: ok, note: PROVEN pagination (LastEvaluatedKey w/ base-PK fusion for GSI/LSI, 1MB/Limit); FIXED Select/COUNT omits Items + Select constraint validation. 2026-08-13: ToSDKQueryInput never copied ReturnConsumedCapacity/Select despite models.QueryInput declaring ReturnConsumedCapacity (Select wasn't declared at all); models.ScanInput was missing ReturnConsumedCapacity/ConsistentRead/Select entirely and models.ScanOutput was missing ConsumedCapacity -- all fixed at the wire boundary. A real client could never get ConsumedCapacity from Query/Scan, nor a COUNT-only Scan/Query response, regardless of what it requested. GSI/LSI Query full-scan gap (previously documented here) is FIXED -- see overall. 2026-08-14 (this audit): AttributesToGet (the legacy pre-expression projection parameter, still real and wire-serialized per api_op_Query.go:92/api_op_Scan.go) was declared on neither models.QueryInput nor models.ScanInput, so it was silently dropped by json.Unmarshal regardless of what a client sent. Fixing the wire model alone was not enough: item_ops_query.go's collectQueryPage and item_ops_scan.go's doScan built their Projector from ProjectionExpression only, never falling back to AttributesToGet the way GetItem/BatchGetItem's resolveProjection() already did -- so even a correctly-wired AttributesToGet would have been silently ignored by the projection logic itself. Fixed both layers (models/types.go + convert_ops.go for the wire, item_ops_query.go/item_ops_scan.go for resolveProjection() reuse), and added the AttributesToGet+ProjectionExpression mutual-exclusion validation Query/Scan were missing (validateProjectionParams, already used by GetItem/BatchGetItem). Test: TestQueryScan_AttributesToGet_SurvivesWireConversion (hand-verified to fail against unfixed code: both subtests failed with "AttributesToGet should have excluded 'other'"). 2026-08-14 (gopherstack-yvs8): KeyConditions/QueryFilter (Query) and ScanFilter (Scan), the remaining legacy pre-expression parameters, were declared on neither models.QueryInput nor models.ScanInput (same wire-drop class as AttributesToGet above, and as Expected/AttributeUpdates on item_crud) -- fixed by adding models.LegacyCondition + the KeyConditions/QueryFilter/ScanFilter/ConditionalOperator fields, and translating them (legacy_query_scan.go) into KeyConditionExpression/FilterExpression through the same evaluator paths item_ops_query.go/item_ops_scan.go already use. See gaps for the KeySchema-reordering fix to item_ops_query.go's PK-position-dependent fast path, and citations.}
  batch:        {status: ok, note: FIXED BatchWriteItem duplicate-key validation (was missing; BatchGetItem had it). 2026-08-13: BatchGetItem/BatchWriteItem never called the throttler at all (db.throttler.ConsumeRead/ConsumeWrite) despite ProvisionedThroughputExceededException being a real, documented error for both ops (confirmed against deserializers.go's per-op error switch) -- provisioned-capacity tables never throttled batch calls even though every single-item op did; fixed with per-table charging mirroring the existing single-item formulas, PAY_PER_REQUEST still bypasses. Also: models.BatchGetItemInput/BatchWriteItemInput had no ReturnConsumedCapacity field at all (BatchWriteItemInput also missing ReturnItemCollectionMetrics) -- silently dropped on the wire regardless of client request; fixed in models/convert_ops.go. BatchWriteItemOutput.ItemCollectionMetrics (a real, conditionally-populated SDK field per api_op_BatchWriteItem.go) was never populated by the backend even when requested on an LSI table -- wired using the same per-item SizeEstimateRangeGB formula PutItem/DeleteItem already use. BatchExecuteStatement dropped per-statement ConsistentRead on input, already forwarded correctly once it arrived (fbc2cfe1f).}
  transactions: {status: ok, note: FIXED TransactWriteItems Update key-mutation — was NOT validated, silently corrupted pkIndex/pkskIndex (state corruption bug). 2026-07-24: FIXED gopherstack-daa — Put/Update/Delete/ConditionCheck now reject an ExpressionAttributeNames/Values placeholder unused by that item's expression(s), matching plain PutItem/UpdateItem/DeleteItem (checkUnusedExpressionAttributeNames/Values in expressions.go); Update correctly considers both UpdateExpression AND ConditionExpression when deciding "used". Enforced pre-lock in validateTransactWriteItems (transact_validation.go) so it's a plain ValidationException, not wrapped in CancellationReasons — matches AWS request-validation-time semantics. 2026-08-13: TransactWriteItems/TransactGetItems never called the throttler (same gap as BatchWriteItem/BatchGetItem above); fixed with per-table charging. ToSDKTransactWriteItemsInput had a dangling `// ReturnItemCollectionMetrics` comment instead of actually copying the field, so it was always dropped on the wire even though models.TransactWriteItemsInput declared it and TransactWriteItemsOutput.ItemCollectionMetrics (a real SDK field) was never populated by the backend regardless; both fixed.}
  streams:      {status: ok, note: PROVEN shard-iterator sequence clamping, trim-horizon; streamARNIndex now a store.Table, verified Put/Delete key derivation unchanged. 2026-07-24 (gopherstack-exg7): (1) DescribeStream's ShardFilter{Type:CHILD_SHARDS,ShardId} was accepted on the wire but silently ignored — now filters found.streamShards by ParentShardID (parseShardFilter/filterChildShards in streams_ops.go), rejecting unsupported filter Types and a missing ShardId with ValidationException; verified a filter that legitimately matches zero shards returns a real empty Shards list rather than the "stream just enabled" placeholder shard (buildSDKShardsList's synthesizePlaceholder flag). (2) ShardIteratorStore gained a clock-injection seam (now func() time.Time, SetClock/Now) — resolveIterator's expiry check now reads db.iteratorStore.Now() instead of time.Now() directly, so ExpiredIteratorException is exercised end-to-end via GetShardIterator -> advance fake clock -> GetRecords in a test, not just via the pre-existing ExpireAllShardIteratorsForTest backdate-hack. (3) De-duplicated the wire<->SDK AttributeValue conversion functions that were split across streams_ops.go (wire->SDK: toStreamAttributeValue/dispatchStreamType/buildSDKStreamItem/buildSDKRecord) and streams_wire.go (SDK->wire: FromStreamAttributeValue/FromStreamItem) — both directions (and their shared sentinel errors) now live together in streams_wire.go; streams_ops.go keeps only shard/record-management logic.}
  janitor_ttl:  {status: ok, note: PROVEN batched-lock, ctx-cancel, quickselect eviction, ring-buffer compaction}
  datalayer:    {status: ok, note: RE-AUDITED — ce30166a converted db.Tables/Backups/GlobalTables/exports/imports/streamARNIndex from raw maps to pkgs/store.Table+Index (composite key tableKey(region,name), region derived by parsing TableArn via tableRegion()). Verified every insertion site (CreateTable, RestoreTable, CreateGlobalTable replicas, cloneTableSchema, applyOneReplicaTableEntry) builds TableArn with the same region string used as the store key *before* Put, so tableRegion(t) round-trips correctly; TableArn is never mutated post-insert. No stale map-key leaks (tablesByRegion Index auto-empties groups on last delete, unlike the old per-region submap). Persistence snapshot reshaped map->sorted slice + added a schema version gate (old snapshots discarded cleanly on upgrade, matching the sqs/ec2 precedent) — intentional, not a parity bug.}
  admin_lists:  {status: ok, note: gopherstack-6flj (2026-08-15) wrapper-key sweep of all 22 List+Describe+Get ops (ListBackups/ListContributorInsights/ListExports/ListGlobalTables/ListImports/ListTables/ListTagsOfResource, the 13 Describe* ops, GetItem, GetResourcePolicy) — every top-level wrapper key diffed field-by-field against its own api_op_*.go Output struct in the pinned aws-sdk-go-v2/service/dynamodb@v1.63.1 module cache; all correct, no wrong/silent-empty key found, no shared-converter cross-op mismatch (exportTableToPointInTimeOutput is legitimately shared by ExportTableToPointInTime/DescribeExport — both real Outputs are ExportDescription-only). One real gap found and fixed: DescribeContributorInsightsOutput.LastUpdateDateTime (deserializers.go:18441, epoch-seconds) was entirely unmodeled — the backend never tracked when contributor insights was last toggled. Fixed by adding Table.ContributorInsightsLastUpdate (set in setContributorInsightsLocked on every UpdateContributorInsights call) and emitting it only once non-zero (a never-toggled table reports it absent, matching AWS's own "populated once an action has occurred" behavior, not a fabricated zero time). See gaps for FailureException (same struct, correctly left unmodeled).}
  autoscaling:  {status: fixed, note: "2026-08-21 (gopherstack-1vv2, InMemoryDB receiver-scope sweep): UpdateTableReplicaAutoScaling built a brand-new autoScalingSettings from only the current call's fields and assigned it wholesale over table.AutoScaling. GlobalSecondaryIndexUpdates and ProvisionedWriteCapacityAutoScalingUpdate are independently optional on the real input (api_op_UpdateTableReplicaAutoScaling.go) -- a call updating only one GSI's auto scaling settings silently wiped a previously-set table-level write-capacity autoscaling config, and vice versa. Fixed: autoScalingSettingsFromInput -> mergeAutoScalingSettingsFromInput, which merges into the existing table.AutoScaling (creating one only if nil) instead of replacing it. TestUpdateTableReplicaAutoScaling_WriteAndGSIUpdatesDontClobberEachOther (autoscaling_status_agreement_internal_test.go), hand-verified to fail against unfixed code. Other InMemoryDB Update* methods checked in the same sweep (UpdateContinuousBackups/UpdateContributorInsights/UpdateGlobalTable/UpdateGlobalTableSettings/UpdateItem/UpdateKinesisStreamingDestination/UpdateTable/UpdateTimeToLive) already merge field-by-field or are single-scalar toggles -- no further bugs of this shape found. See gaps: GlobalSecondaryIndexes autoscaling settings are stored but never echoed back on ReplicaAutoScalingDescription (a separate, pre-existing accept-and-drop gap, not touched by this fix)."}
  global_table_settings_autoscaling: {status: fixed, note: "2026-08-23 (manifest-harvest pass): UpdateGlobalTableSettingsInput's GlobalTableProvisionedWriteCapacityAutoScalingSettingsUpdate, GlobalTableGlobalSecondaryIndexSettingsUpdate (global, not per-replica, per-GSI write autoscaling), ReplicaSettingsUpdate[].ReplicaProvisionedReadCapacityAutoScalingSettingsUpdate, and ReplicaGlobalSecondaryIndexSettingsUpdate[].ProvisionedReadCapacityAutoScalingSettingsUpdate (api_op_UpdateGlobalTableSettings.go, types.go:2891/2962/1881) were all accepted on the wire (handler_global_tables.go's updateGlobalTableSettingsInput had no struct fields for any of them) then silently dropped -- an accept-and-drop wire gap, same class as UpdateTableReplicaAutoScaling's pre-1vv2-fix clobber bug but never wired at all rather than clobbered. Fixed: StoredGlobalTable gained WriteCapacityAutoScaling/GSIWriteCapacityAutoScaling, StoredReplicaSettings/StoredReplicaGSISettings gained ReadCapacityAutoScaling, all reusing the existing autoScalingThroughput persisted shape and throughputFromUpdate/sdkAutoScalingSettingsDescription converters UpdateTableReplicaAutoScaling already has (autoscaling.go) -- no new evaluator. Both UpdateGlobalTableSettings and DescribeGlobalTableSettings now echo the same stored settings (global write-capacity autoscaling applies uniformly across replicas, matching how WriteCapacityUnits already does, since it is a global-table-level setting in the v1 API, not per-replica). Verified via TestGlobalTableSettings_AutoScaling, driven through the real aws-sdk-go-v2 client, hand-reverted (services/dynamodb/{global_tables,handler_global_tables,store}.go) to confirm it fails against unfixed code (nil ReplicaProvisionedWriteCapacityAutoScalingSettings), restored, md5sum identical. Additive-only struct fields; pkgs/persistence snapshot-version guard confirmed no bump needed."}
  kinesis_streaming_disable_echo: {status: fixed, note: "2026-08-23 (manifest-harvest pass): DisableKinesisStreamingDestinationOutput.EnableKinesisStreamingConfiguration (deserializers.go:18931 -- a real modeled response member on Disable despite its SDK doc comment reading 'the destination for the Kinesis streaming information that is being enabled', a codegen doc-comment artifact shared with Enable/Update, not evidence the field is request-only) was never populated; DisableKinesisStreamingDestination always returned it as nil/absent even though the backend already tracked the destination's precision (KinesisDestinationEntry.Precision) right up until deleting it. Fixed: removeKinesisDestinationLocked now returns the removed entry's precision, echoed back as EnableKinesisStreamingConfiguration (defaulting to MILLISECOND, matching Enable/Describe's existing default). Verified via TestDisableKinesisStreamingDestination_EchoesConfig, hand-reverted (kinesis_streaming.go, handler_kinesis_streaming.go) to confirm nil response before the fix, restored, md5sum identical."}
  pagination_sweep: {status: fixed, note: "2026-08-28/29 (wrapper-key-sweep-rds-cloudwatch-sqs-sns pagination pass): audited every List/Describe/Query/Scan op with a page-size + continuation member against the pinned SDK. ListGlobalTables' applyGlobalTableLimit only capped the page when the caller supplied an explicit Limit; an omitted Limit (ListGlobalTablesInput.Limit doc, api_op_ListGlobalTables.go:35, 'if the parameter is not specified, DynamoDB defaults to 100') returned every global table uncapped with no LastEvaluatedGlobalTableName. Fixed: applyGlobalTableLimit now falls back to defaultListGlobalTablesLimit=100. TestListGlobalTables_DefaultLimitPagination (wire_field_fixes_test.go) creates 105 global tables, drives the real SDK client through the full pagination loop with no Limit set, and asserts each page is <=100 and the union is exactly the 105 names with no duplicates; hand-reverted to confirm it fails against unfixed code (page of 105), restored. Everything else audited CORRECT: Query/Scan's Limit-as-items-examined + post-limit-filter + ExclusiveStartKey/LastEvaluatedKey semantics (item_ops_query.go/item_ops_scan.go) match AWS's own documented 'LastEvaluatedKey may be non-nil with nothing left to return' behavior -- collectQueryPage emits LastEvaluatedKey whenever the Limit boundary is hit, including on the true last item (no i<len-1 guard, unlike scanPage's), which is correct-but-surprising, not a bug: a client resuming from that key gets an empty page with a nil LastEvaluatedKey next call, one harmless extra round trip, exactly the documented AWS gotcha. ListBackups/ListContributorInsights/ListExports/ListImports/ListTables all correctly consume+truncate+emit. ListTagsOfResource ignores NextToken/returns everything in one call by design: the real op has no MaxResults input member at all and no documented default page size to impose (DO-NOT-INVENT-A-PAGE-SIZE), so returning the full <=50-tag set in one page is the only non-fabricated implementation."}
gaps:
  - "2026-08-21 (gopherstack-1vv2): ReplicaAutoScalingDescription.GlobalSecondaryIndexes (types.go:2642) is
    never populated by UpdateTableReplicaAutoScaling or DescribeTableReplicaAutoScaling --
    replicaAutoScalingDescriptionsRLocked only ever echoes table-level Write settings per
    replica. Per-GSI autoscaling settings ARE stored (autoScalingSettings.GlobalSecondaryIndexes,
    now correctly merged rather than clobbered -- see autoscaling family) but a real client
    reading them back via Update or Describe always sees an empty list regardless of what was
    configured. Pre-existing, found while fixing the clobber bug above; not fixed here since it's
    an accept-and-drop wire gap, a different bug class from this pass's scope."
  - "2026-08-15 (gopherstack-6flj, disclosed, not fixed): DescribeContributorInsightsOutput.FailureException
    (types.FailureException{ExceptionName, ExceptionDescription}, api_op_DescribeContributorInsights.go)
    remains unmodeled. This backend's UpdateContributorInsights/DescribeContributorInsights
    never fail to enable/disable contributor insights (no IAM/service-limit failure
    model exists anywhere in this service), so there is no honest non-nil value to
    populate this field with -- always leaving it nil is the accurate representation,
    not a gap being papered over. LastUpdateDateTime (same struct) was the real,
    fixable gap and is now fixed -- see admin_lists family above."
  - "2026-08-14 (gopherstack-lze5, CORRECTNESS, PARTIALLY FIXED): Expected,
    ConditionalOperator, and AttributeUpdates (PutItem/UpdateItem/DeleteItem's
    legacy pre-expression parameters) are now implemented -- the
    conditional-check-bypass and no-op-write failure modes this issue was filed
    for. Fixed by translation, not a second evaluator: legacy_conditions.go
    converts each legacy Expected/Condition into an equivalent
    ConditionExpression fragment (aliased #name/:value placeholders synthesized
    per attribute, joined by ConditionalOperator's AND/OR, default AND -- see
    legacyConditionalJoiner) and each AttributeUpdates entry into an equivalent
    UpdateExpression fragment (PUT -> SET, DELETE w/o Value -> REMOVE, DELETE
    w/ a set Value -> DELETE, ADD -> ADD; action-semantics citations:
    types/types.go:197-269 AttributeValueUpdate doc), then hands the rewritten
    request to the SAME evaluator (services/dynamodb/expr, via the existing
    checkPutCondition/checkUpdateCondition/checkDeleteCondition/doUpdate) real
    PutItem/UpdateItem/DeleteItem already used for ConditionExpression/
    UpdateExpression. ComparisonOperator set: EQ/NE/LE/LT/GE/GT/NOT_NULL/NULL/
    CONTAINS/NOT_CONTAINS/BEGINS_WITH/IN/BETWEEN, all implemented (renderComparison,
    citing types/types.go:1279-1391 for operator semantics and arg counts).
    Expected's old Value/Exists style and its Value/Exists-vs-ComparisonOperator
    mutual exclusion cite types/types.go:1240-1256 verbatim. Mutual exclusion
    between legacy and expression parameters is enforced per-operation (any of
    Expected/ConditionalOperator/AttributeUpdates set alongside any of
    ConditionExpression/UpdateExpression -> ValidationException) -- this specific
    rejection is well-established real DynamoDB behavior but has no client-side
    SDK validation to cite a line number against, so the error wording is our
    own, not a verified verbatim AWS string. Tested driving the real
    aws-sdk-go-v2 client and asserting behaviour (ConditionalCheckFailedException
    + item unchanged on a failing Expected, ADD-on-number increments,
    ADD-on-set unions, DELETE-with-set-value subtracts, DELETE-without-value
    removes), not just call success -- legacy_conditional_params_test.go; each
    covered case was hand-verified to fail with unfixed code (e.g. 'An error is
    expected but got nil... expected: *types.ConditionalCheckFailedException').

    2026-08-14 (gopherstack-yvs8, follow-up pass, FIXED): KeyConditions,
    QueryFilter (Query) and ScanFilter (Scan) -- the remaining legacy
    parameters this issue named -- are now implemented, closing the "returns
    every item, caller believes it was filtered" failure mode for
    QueryFilter/ScanFilter and the "KeyConditions silently dropped" failure
    mode for Query. Two layers were wrong, same as every prior wire-drop in
    this family: (1) models.QueryInput/ScanInput didn't declare these fields
    at all, so json.Unmarshal dropped them before any backend code ran --
    fixed by adding models.LegacyCondition (mirrors types.Condition: just
    ComparisonOperator + AttributeValueList, confirmed against
    types/types.go:672-770, no Value/Exists shorthand unlike
    ExpectedAttributeValue) plus KeyConditions/QueryFilter/ConditionalOperator
    on models.QueryInput and ScanFilter/ConditionalOperator on
    models.ScanInput, wired through models/convert_ops.go's new
    toSDKLegacyConditions into the SDK struct's KeyConditions/QueryFilter/
    ScanFilter/ConditionalOperator fields (api_op_Query.go:98,284,316;
    api_op_Scan.go:95,248). (2) legacy_query_scan.go translates the now-arriving
    fields the same way legacy_conditions.go translates
    Expected/AttributeUpdates: QueryFilter/ScanFilter (combined per
    ConditionalOperator, default AND) become a FilterExpression fragment via
    translateLegacyFilterConditions, reusing renderComparison and the full
    12-operator mapping from legacy_conditions.go unchanged.

    KeyConditions -> KeyConditionExpression needed the reordering this issue
    flagged as the blocker: item_ops_query.go's
    filterCandidatesForKeyCondition/preParseQueryPKValue assume the first
    AND-clause of the expression is the partition-key equality condition, for
    their indexed-lookup fast path. A legacy KeyConditions map has no
    inherent order (Go maps don't), so translateKeyConditionsToKeyConditionExpression
    (legacy_query_scan.go) explicitly looks up the partition key and sort key
    by name against the resolved KeySchema (base table, or the named GSI/LSI
    via a new legacyKeySchemaForQuery, which takes its own short table.mu
    RLock -- this translation must run before snapshotTableForQuery's own
    lock cycle, which needs KeyConditionExpression already resolved to know
    what to snapshot) and always emits [partition-key clause, sort-key clause]
    in that order, regardless of the map's order. Tested with the sort key
    listed first in the Go map literal (TestQuery_LegacyKeyConditions/
    sort_key_listed_first_in_the_map...) -- the case that would silently
    break the existing fast path if reordering were skipped -- and it passes;
    hand-reverting either the wire-model fix or the translation/reordering
    logic reproduces the original bug (see below).

    Operator restrictions enforced: KeyConditions' partition-key entry must
    use EQ; its optional sort-key entry is restricted to
    EQ/LE/LT/GE/GT/BEGINS_WITH/BETWEEN (keyConditionsAllowedOps). This subset
    is documented in AWS's KeyConditions developer guide, which
    api_op_Query.go:281-284's KeyConditions field doc links to but does not
    inline -- disclosed in legacy_query_scan.go as our own transcription of
    that guide, not an SDK-cited fact, same honesty standard as the
    ConditionalOperator wording below. ConditionalOperator is associated with
    QueryFilter/ScanFilter only, not KeyConditions (KeyConditions is always
    ANDed) -- this does have a concrete citation: ConditionalOperator's own
    SDK doc comment says "Use FilterExpression instead"
    (api_op_Query.go:92-98, api_op_Scan.go:89-95), tying it to
    FilterExpression's legacy counterpart, not KeyConditionExpression's.

    Mutual exclusion enforced per parameter pair (KeyConditions vs
    KeyConditionExpression; QueryFilter/ConditionalOperator vs
    FilterExpression on Query; ScanFilter/ConditionalOperator vs
    FilterExpression on Scan) -- same disclosure as the Put/Update/Delete gap
    above: this is real DynamoDB behavior with no SDK-side validation to cite
    a line against, so the wording is ours.

    Tested driving the real aws-sdk-go-v2 client and asserting behaviour, not
    call success: a ScanFilter that excludes items returns fewer items than
    an unfiltered Scan (TestScan_LegacyScanFilter), a QueryFilter narrows a
    KeyConditions-matched set (TestQuery_LegacyQueryFilter), and a
    KeyConditions query with a sort-key BETWEEN/GT condition returns only the
    matching range, not the whole partition (TestQuery_LegacyKeyConditions) --
    legacy_query_scan_test.go. Hand-reverted both fix layers independently to
    confirm each is load-bearing: with models/types.go+convert_ops.go reverted
    to pre-fix (fields undeclared again), every filter/key-condition test
    failed with 4 items returned instead of 3 (or 2) -- the exact
    'ScanFilter/QueryFilter returns everything' and 'KeyConditions dropped'
    symptoms this issue was filed for. Restored byte-identical, then reverted
    item_ops_query.go/item_ops_scan.go's integration (translation layer)
    instead, wire layer left fixed: same failures reproduced (fields now
    reach the SDK struct but are never read). Restored byte-identical again;
    all gates green with both layers in place."
  - "2026-08-14 (gopherstack-rkmp/gopherstack-glfv, CORRECTNESS, flagged not fixed):
    ReturnConsumedCapacity=INDEXES never returns a per-index breakdown on any
    operation. capacity.go's buildConsumedCapacityWithIndexes/applyIndexBreakdowns
    correctly build types.ConsumedCapacity.Table/GlobalSecondaryIndexes/
    LocalSecondaryIndexes and are unit-tested in isolation, but grep confirms they
    are called from nowhere except export_test.go -- every real operation
    (PutItem/UpdateItem/DeleteItem/Query/Scan/BatchGetItem/BatchWriteItem/
    TransactGetItems/TransactWriteItems) builds a bare ConsumedCapacity{TableName,
    CapacityUnits, Read/WriteCapacityUnits} literal directly, so INDEXES and TOTAL
    produce byte-identical output everywhere. TestConsumedCapacityIndexes_PutItem
    is misleadingly named: despite the name and a GSI fixture, it actually requests
    TOTAL and never exercises the INDEXES path -- the same 'test looked like
    coverage and wasn't' pattern noted below for the pre-53cfd590b tests. Read-side
    fix (100% of RCU to the queried index) is straightforward; write-side fix
    (attributing WCU across every GSI/LSI a written item's key populates) needs
    AWS billing semantics not verified against a real account this pass, so it's
    flagged rather than guessed, per the no-fabrication rule."
  - "2026-08-14 (gopherstack-rkmp, minor/structural, not filed individually):
    struct-field-diffing every wire model against dynamodb@v1.63.1 turned up a
    long tail of fields absent because the underlying AWS feature has no backend
    model at all (same category as the SearchVectors gap below, not a wire drop):
    WarmThroughput and VectorIndexes on CreateTable/UpdateTable/GSI actions;
    GlobalTableWitnesses and MultiRegionConsistency (MRSC witness regions) on
    CreateTable/TableDescription; ResourcePolicy on CreateTableInput (resource-based
    policy IS modeled via the separate Put/GetResourcePolicy ops, just not the
    at-creation shortcut); VectorIndexOverride/LocalSecondaryIndexOverride on
    RestoreTableFromBackup/RestoreTableToPointInTime; several ReplicaDescription
    v2-global-table fields (ReplicaArn, KMSMasterKeyId, OnDemand/ProvisionedThroughputOverride,
    ReplicaStatusDescription/PercentProgress, ReplicaTableClassSummary,
    ReplicaInaccessibleDateTime); ProvisionedThroughputDescription's
    LastIncrease/DecreaseDateTime and NumberOfDecreasesToday (AWS itself rarely
    populates the latter post-2018 throttling changes); SSEDescription's
    InaccessibleEncryptionDateTime (only set when a KMS key becomes unreachable,
    a failure mode this backend doesn't model); BackupSummary/BackupDetails'
    BackupExpiryDateTime (only set on the SYSTEM auto-backups DynamoDB creates on
    table deletion with PITR enabled -- this backend only ever creates USER
    backups via CreateBackup, so there's genuinely no SYSTEM-backup expiry to
    report). None fabricated; all are honest absences, listed here so a future
    pass doesn't have to rediscover them by re-running the same diff."
  - "2026-08-05: SearchVectors (new in SDK v1.63.1) — DynamoDB vector indexes have no
    backend model here: CreateTable/UpdateTable have no field or code path that attaches a
    vector index to a table, so no vector index can ever exist in this backend. Fabricating
    similarity scores for a search against an index that was never created would violate
    the no-fabricated-data rule. search_vectors.go implements full request validation
    (TableName/IndexName/SearchVector/TopK required, matching the SDK's
    validateOpSearchVectorsInput) and a real table-existence check, then honestly returns
    ResourceNotFoundException for the named index — the same response real DynamoDB gives
    for any index name on a table with no vector indexes. Wire types/converters
    (SearchVectorsInput/Output, VectorCapacity, SearchResultItem) are implemented in full
    for shape-correctness even though the success path is never reached. Full vector-index
    support (CreateTable VectorIndex, index storage, real similarity scoring) is out of
    scope for this pass — tracked as a follow-up if vector search ever becomes a priority."
deferred:
  - expr/ lexer/parser/evaluator subpackage (has own aws_spec_test.go/evaluator_test.go) — not line-by-line re-audited this sweep; genuinely large surface, out of scope for this streams/transactions-focused follow-up pass. No known bugs, just not freshly field-diffed against the SDK this cycle.
  - PartiQL execution (partiql.go, ~37KB) — not re-audited this sweep, same reason as above.
leaks: {status: clean, note: TTL sweeper + stream trimming verified, ctx-cancel present. ShardIteratorStore's SetClock mutates the store's `now` field under s.mu (same lock Put/Get/Sweep already take), so the clock-injection seam introduces no new race — verified via `go test -race`.}
---

## Notes
- 2026-08-14 (gopherstack-rkmp): methodology for this audit was a mechanical
  struct-field diff, not another manual read-through: a small Go/AST program
  (not checked in) parsed every `*Input`/`*Output` struct from the pinned
  aws-sdk-go-v2/service/dynamodb@v1.63.1 (both the top-level api_op_*.go files
  and types/types.go) and every struct in services/dynamodb/models/types.go,
  normalized Go's `Id`/`Arn`/`Kms`/`Sse` vs `ID`/`ARN`/`KMS`/`SSE` naming
  variance, and reported SDK fields with no same-named counterpart in the
  matching gopherstack struct. This is exactly the "required response member
  never populated" bug class this pass was asked to hunt, made systematic
  instead of op-by-op. It over-reports (ResultMetadata is SDK-internal
  middleware state, not a wire field; a handful of hits were the Go-naming
  false positives the normalization pass didn't fully catch, e.g. TableId vs
  TableID before normalization was added) so every hit was hand-verified
  against the actual SDK serializer/type before being treated as a bug --
  three were real and fixed (AttributesToGet, IndexArn, BackupSizeBytes,
  see families above), two are real and flagged as feature work (see gaps),
  the rest are honest structural absences (see gaps) or SDK-internal noise.
  The diff also caught `SearchVectorsInput.SearchConditionExpression` as
  "declared but never read outside models/" -- checked against search_vectors.go
  and confirmed this is the ALREADY-documented SearchVectors gap below (the
  field is validated for wire-shape correctness but the success path that
  would consume it is never reached), not a new bug -- included here as a
  cross-check that the diff produces real signal rather than only false
  positives.
- 2026-08-13 (gopherstack-rkmp): two existing tests exercised the SDK-typed backend
  method directly (or patched the SDK struct after conversion) rather than going
  through the wire-format models.*Input -> ToSDK*Input path a real client's JSON
  body actually takes, which is exactly why they never caught the ReturnConsumedCapacity
  wire-drop bugs above: TestTransactWriteItems_ConsumedCapacity calls
  db.TransactWriteItems with a hand-built *sdk.TransactWriteItemsInput{ReturnConsumedCapacity: ...}
  (bypassing ToSDKTransactWriteItemsInput entirely), and TestQuery_ConsistentRead_ConsumedCapacity
  calls models.ToSDKQueryInput then manually overwrites
  sdkQuery.ReturnConsumedCapacity afterward (masking that ToSDKQueryInput itself
  never set it). Neither test was asserting anything false — they just couldn't see
  the gap they were standing next to. New tests (`*_SurvivesWireConversion`) added
  alongside each to close that blind spot; the old tests are left as-is since they
  still correctly cover the backend-level ConsumedCapacity math.
- BatchWriteItem rejects same-key Put+Delete / Put+Put / Delete+Delete in one call: "Provided list of item keys contains duplicates" (verified docs + boto3 history). A prior test asserted the opposite — corrected.
- Select=COUNT returns Count/ScannedCount only, Items omitted.
- Select=SPECIFIC_ATTRIBUTES requires a projection; ALL_PROJECTED_ATTRIBUTES invalid on bare table.
- 2026-07-11 re-audit: aws-sdk-go-v2/service/dynamodb bumped f459c9fa's v1.59.2 -> HEAD's v1.60.0 (e51c0de9); diffed api_op_*.go/types.go between the two module versions — zero surface change (v1.60.0's only changelog entry is "Add request serialization snapshot tests"), so no new-op audit was needed this cycle.
- 2026-07-11 re-audit: no real bugs found. All gates pass (build, vet, race tests, go fix -diff empty, golangci-lint 0 issues) with zero working-tree changes required.
- 2026-07-24 follow-up sweep: verified against dynamodbstreams@v1.35.0 (go.mod) that ShardFilter is the only new DescribeStreamInput field this backend hadn't wired up (types.ShardFilter{ShardId, Type}, only CHILD_SHARDS defined in ShardFilterType.Values()). services/dynamodbstreams (the sibling client-facing service that reads this backend's stream buffer) required no changes — it passes ShardFilter straight through the SDK input struct, which already carried the field; the gap was entirely on the dynamodb-backend side. `go build ./...` and `go test -race ./services/dynamodbstreams/...` both verified clean after the change.

## gopherstack-wlo1 (2026-08-22): CBOR request/response path had its own untyped dispatch errors, missed by c6554e9f8

`c6554e9f8` typed `Handler()`'s own dispatch errors (`writeDynamoDBDispatchError`)
but never touched `handleCBORRequest` (`cbor.go`), a separate hand-rolled path
branched into *before* `Handler()`'s own `httputils.ReadBody` call
(`if service.IsCBORRequest(c.Request()) { return h.handleCBORRequest(...) }`).
All three of its own failure branches (ReadBody failure, response
`json.Marshal` failure, `service.JSONToCBOR` encode failure) wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.

DynamoDB is JSON-RPC 1.0 (`dynamodb@v1.63.1` `awsAwsjson10_` prefix), and its
error deserializer (`deserializers.go:86-121`,
`awsAwsjson10_deserializeOpErrorBatchExecuteStatement`) always JSON-decodes the
response body via `json.NewDecoder` regardless of the *request's* content
type -- there is no CBOR handling anywhere in the error path. So a real client
saw `smithy.GenericAPIError{Code:"UnknownError"}` for every one of these three
failures on the CBOR wire option, same shape as `c6554e9f8`'s finding on the
main dispatch path.

Fixed: all three sites now call the existing `writeDynamoDBDispatchError(c,
http.StatusInternalServerError, "InternalFailure", "internal server error")`
helper (handler.go), the same helper and code `c6554e9f8` already established
for this class on the JSON path.

Proof: `aws-sdk-go-v2/service/dynamodb` never sends
`application/x-amz-cbor-1.1` itself (that wire option is used by other
language SDKs), so `TestHandleCBORRequest_OversizedBodySurfacesInternalFailure`
(`handler_cbor_dispatch_malformed_test.go`) drives a real client's `PutItem`
through a Finalize-stage middleware that rewrites the Content-Type header to
CBOR post-signing, with an item attribute large enough to exceed
`httputils.MaxRequestBodyBytes`, forcing the ReadBody-failure branch.
Hand-reverted `cbor.go` to `git show HEAD`, confirmed the test fails with
`*json.SyntaxError: "invalid character 'i' looking for beginning of value"`,
restored the fix, `md5sum`-confirmed byte-identical.

The other two branches (response marshal/CBOR-encode failure) are fixed
defensively for consistency with the same helper but not independently
proven by a client -- `response` is backend-constructed and `json.Marshal`
essentially cannot fail on it.

## 2026-08-29: error-path sweep (failure-side wire shape) -- one live bug fixed, rest confirmed clean

Campaign-wide hunt for the class distinct from wrapper-key/nesting sweeps:
what a client sees when a request *fails* -- HTTP status, AWS error code, and
whether the operation actually models that code, checked against each op's
own `awsAwsjson10_deserializeOpError<Op>` switch in `deserializers.go`
(dynamodb@v1.63.1), not the shared `types/errors.go` list.

**Error path**: every backend method returns a Go error, either a typed
`*Error{Type, Message}` (errors.go's `New*Exception` constructors, `Type`
holding the full `com.amazonaws.dynamodb.v20120810#<Name>` shape) or a plain
`errors.New`. `Handler.classifyError` (handler.go) maps a typed `*Error` to
HTTP 500 only for `InternalServerError`, HTTP 400 for every other type --
confirmed correct against the SDK: DynamoDB's JSON-RPC (awsjson10) protocol
never varies HTTP status per exception type on the real service either; the
client determines the concrete exception type purely from the body's
`__type`/`X-Amzn-ErrorType`, so a uniform 400 for all client-fault codes is
not a bug.

**58 ops' declared code sets extracted and spot-checked** against the
sentinels each backend method actually raises (BatchGetItem/BatchWriteItem/
Get/Put/Update/DeleteItem/Query/Scan/Transact*/Execute*/Create*/Delete*/
Describe*/List*/Update* families). No wrong-code or wrong-status findings
this pass; `NewDuplicateItemException` (`ExecuteStatement`'s own switch
models `DuplicateItemException`) and the Backup/Export/Import/GlobalTable
`Not*Found` constructors all match their respective op's own declared set.

**One real bug found and fixed**: `TransactWriteItems`' `ClientRequestToken`
idempotency tracking (`transact_ops.go`'s `txnTokens`) recorded only an
expiry, no record of what request actually committed under a token. AWS
raises `IdempotentParameterMismatchException` (confirmed modeled on this
op's own `awsAwsjson10_deserializeOpErrorTransactWriteItems` switch,
`deserializers.go`) when a caller reuses a `ClientRequestToken` with a
*different* request; gopherstack instead treated any second call with a
matching token as a matching replay and returned a bare empty success --
even when `TransactItems` was entirely different. This is the "AWS models
an error, gopherstack returns a bare success" direction of the class.

Fixed: `txnTokens` now stores a `txnTokenRecord{expiry, hash}` (`store.go`),
where `hash` is a SHA-256 of the JSON-encoded `TransactItems`
(`hashTransactWriteItems`, `transact_ops.go`) -- JSON's built-in map-key
sorting makes this deterministic regardless of item ordering. A token reused
with a mismatched hash now returns the new
`NewIdempotentParameterMismatchException` (`errors.go`). `janitor.go`'s
sweep/eviction logic (`evictOldestTokens`, `scanExpiredTxnTokensRLocked`)
updated for the new value type; `txnTokens` is not part of `persistence.go`'s
snapshot (idempotency tokens are process-local and short-TTL by design), so
no snapshot/restore format changed.

`ExecuteTransaction` (PartiQL) also models `IdempotentParameterMismatchException`
but its `ClientRequestToken` wire field is parsed
(`handler_execute_transaction.go`) and then never passed to the backend at
all -- **disclosed, not fixed**: this op has zero idempotency-token
plumbing to extend (unlike `TransactWriteItems`, which already had a
commit/pending token store this pass could add a hash to), and building
that from scratch is a larger, separate change than this pass's scope.

Proof: `TestTransactWriteItems_ReusedTokenDifferentPayload_IdempotentParameterMismatch`
(`transact_ops_wire_test.go`) drives the real `aws-sdk-go-v2` client through
two `TransactWriteItems` calls sharing one `ClientRequestToken` but different
items, asserts `errors.As` against `*types.IdempotentParameterMismatchException`,
and asserts the mismatched item was never written. Confirmed failing
(bare success, no error) against the pre-fix code by reverting `store.go`/
`janitor.go`/`transact_ops.go`/`errors.go` and re-running.

Gates: `go build`, `go vet ./...` (repo-wide, per this session's
signature-change caveat -- clean except an unrelated concurrently-edited
`services/apigateway` package), `go test -race -count=1 ./services/dynamodb/...`
(pass), `golangci-lint run --fix ./services/dynamodb/...` (0 issues).

## 2026-08-29: discarded-error sweep -- malformed ProjectionExpression/FilterExpression silently ignored, not rejected

Campaign-wide hunt for the class where a client-visible failure is
discarded (`_`) instead of reaching its designated place in the response --
distinct from the wrong-error-code sweep above.

**Confirmed bug, 5 call sites, one root cause**: `ParseProjector`
(`expressions.go`, wraps `expr.Parser.ParseProjection`) and `ParseConditionStr`
(wraps `expr.Parser.ParseCondition`) both return a real parse error for a
syntactically malformed `ProjectionExpression`/`FilterExpression` (e.g. an
unclosed `[` or a dangling operator) -- proven with `TestParser_Projection`-style
direct calls returning `err != nil`. Every caller discarded that error with
`_`, so `ParseProjector` fell back to a `Projector{}` and `ParseConditionStr`'s
nil `*ParsedCondition` (both explicitly treat nil as "no-op": `Project`
returns the item unchanged, `Evaluate` returns `true`, i.e. "matches
everything"). Net effect: a malformed `ProjectionExpression` silently returns
the **full unprojected item** (over-exposing attributes the caller asked to
exclude) and a malformed `FilterExpression` silently returns **every item
unfiltered**, instead of the `ValidationException` real DynamoDB raises. This
is reachable through the real typed SDK client -- confirmed against
`validateOpGetItemInput`/`validateOpQueryInput`/`validateOpScanInput`/
`validateOpBatchGetItemInput` in the pinned SDK's `validators.go`, none of
which parse expression syntax client-side.

`services/dynamodb/expressions.go:95`'s `projectItem` comment --
`// Return full item if projection fails? Or error? Standard seems to be
quiet.` -- was the source of the bug, not a description of correct
behaviour: the operation already has a designated place for this failure
(`ValidationException`, used by this same op's own `validateProjectionParams`
for the sibling ProjectionExpression/AttributesToGet-both-set case).

Fixed call sites, all now returning `NewValidationException("Invalid
ProjectionExpression: "+err)` / `"Invalid FilterExpression: "+err)`:
- `GetItem` via `projectItem` (`item_ops_crud.go`)
- `Query` via `collectQueryPage` (`item_ops_query.go`) -- both Projection and
  Filter
- `Scan` via `doScan` (`item_ops_scan.go`) -- both Projection and Filter
- `BatchGetItem` via `batchGetTable` (`item_ops_batch.go`) -- Projection only
  (BatchGetItem has no FilterExpression)
- `TransactGetItems` via `transactGetResponseItem` (`transact_ops.go`) --
  Projection only

`KeyConditionExpression` (Query) was already correct -- checked separately
(`item_ops_query.go:221`) and already returns `ValidationException` on parse
failure; only the *Filter*/*Projection* expressions on these five call sites
discarded their errors.

**Reviewed, not a bug**: `CalculateItemSize`'s error return is dead code
(`validation.go:140-152` -- every path returns `nil`), so its ~15 discarded
call sites across dynamodb are legitimate. `models.ToSDKItem`'s error
(malformed internally-stored attribute value) is discarded at several
Query/Scan/BatchGetItem/TransactGetItem read paths (`item_ops_query.go:562`,
`item_ops_scan.go:167,187`, `item_ops_batch.go:292`, `transact_ops.go:551`)
but is unreachable in practice: every write path (`FromSDKItem`,
`ValidateItemSize`) guarantees the wire-format invariant `ToSDKItem` assumes,
so this can only fail on an internal invariant violation elsewhere, not on
attacker-controlled input -- inconsistent with `GetItem`'s own `ToSDKItem`
call (which does check the error) but not a confirmed reachable bug. All
other `_`-discards found in `services/dynamodb` (~50 in total, grep count)
are comma-ok type assertions, non-error second/third return values
(`getPKAndSK`, `applyAutoScalingSettingsLocked`,
`contributorInsightsStateRLocked`), or best-effort logging/cleanup
(`json.Marshal` for debug logs, `gz.Close()`).

Proof: new table cases in `query_test.go` (`Malformed FilterExpression`,
`Malformed ProjectionExpression`), `scan_test.go` (same two), `batch_test.go`
(`MalformedProjectionExpression`), `transact_ops_test.go`
(`MalformedProjectionExpression`), and `projection_test.go`
(`TestProjection_MalformedExpression_ReturnsError`) -- each drives
`db.<Op>` with a real `aws-sdk-go-v2` input struct containing a syntactically
invalid expression and asserts the decoded error's message contains
`ValidationException`. Confirmed failing (silent full/unfiltered result, no
error) against the pre-fix code before the fix landed.

Gates: `go build ./services/dynamodb/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/dynamodb/...` (pass),
`golangci-lint run --fix ./services/dynamodb/...` then plain
`golangci-lint run ./services/dynamodb/...` (0 issues both times).

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

Audited every pagination helper for pure arithmetic (boundary correctness, exact division,
single-page, empty, cursor stability, stale-cursor behaviour): `findStartIndex` (`table_ops.go`
— `ListTables`) is correct and deletion-tolerant by construction (first-name-strictly-greater
search, so a since-deleted `ExclusiveStartTableName` still resumes correctly). `encode`/
`decodePartiQLNextToken` (`partiql.go`) are a plain encode/decode of the real
`LastEvaluatedKey`, not offset arithmetic — nothing to verify there beyond round-trip, which
holds. No bug found or fixed in either; both newly boundary-tested directly
(`pagination_arithmetic_internal_test.go`).

**Recorded, not fixed:** `paginateBackupSummaries` (`backup_ops.go` — `ListBackups`) resolves
`ExclusiveStartBackupArn` by exact ARN match and falls back to `start = 0` when the named
backup has since been deleted, restarting pagination from the beginning rather than resuming
past it — diverges from `findStartIndex`'s deletion-tolerant `>`-search pattern elsewhere in
this same package. Not changed: unlike the equivalent bug fixed in `services/dax`/`services/omics`
this pass, this list is sorted by a composite `(CreationDateTime, BackupArn)` key and the
cursor carries only the ARN half, so reconstructing the correct resume position for a deleted
ARN isn't a like-for-like `==` → `>=` swap — it would need the cursor to also encode the
creation time, which AWS's own `LastEvaluatedBackupArn` (a bare ARN string) leaves no room
for. AWS does not document `ListBackups`' behaviour for a stale `ExclusiveStartBackupArn`, so
the current behaviour is pinned by a test (`TestPaginateBackupSummaries_StaleCursorRestartsFromZero`)
rather than asserted correct, per this pass's instruction to record undefined behaviour instead
of inventing a rule for it. Worth a follow-up if the cursor's shape is ever revisited.

Gates: `go build ./services/dynamodb/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/dynamodb/...` (pass),
`golangci-lint run ./services/dynamodb/...` (0 issues).

## 2026-08-30 cross-call pagination-reproducibility audit (wrapper-key-sweep campaign)

Re-audited every `List`/`Query`/`Scan` op for the class this campaign's brief distinguishes
from the arithmetic sweep above: is the *complete sorted order* reproducible between two
calls with nothing changed in between (a `store.Table.All()`/map walk feeding a sort whose
key can tie drops or duplicates a record at a page boundary), not just whether the pagination
arithmetic/limit handling is correct. `ListTables` (`sort.Strings` on table names, globally
unique), `ListStreams` (sorted by stream ARN, unique per table), `ListImports`/`ListExports`
(sorted by ImportArn/ExportArn, unique), `ListGlobalTables`/`ListContributorInsights` (sorted
by table/global-table name, unique), and `ListBackups` (sorted by `(CreationDateTime,
BackupArn)`, ARN tiebreak already present — see the "Recorded, not fixed" entry above, which
is about a *different* class: cursor resumption after a deleted backup, not cross-call
ordering) all sort by a field that is the same table's own unique key, so no walk-order tie
is reachable regardless of `store.Table.All()`'s unspecified iteration order. `Query`/`Scan`
draw their candidate items from `table.Items` — a plain Go slice under `table.mu`, not a map
— so their traversal order is already stable across calls with no writes in between,
independent of any tie in the requested sort/index key (confirmed for the GSI/LSI case too,
where duplicate `(index PK, index SK)` pairs are legitimately possible in real DynamoDB: the
existing "base-PK fusion" `LastEvaluatedKey` handling, already proven correct per the
`query_scan` row above, sits on top of that same stable slice source). `ListTagsOfResource`
is correctly non-paginated by design (real op has no `MaxResults`/page-size member) and sorts
by the tag map's own key via `collections.SortedKeys`. No pagination-reproducibility bug
found in `services/dynamodb`; nothing changed. This confirms the brief's own note that this
service's one known-bad cursor (`ListBackups`' ARN-only `ExclusiveStartBackupArn`, unable to
reconstruct a deleted backup's `CreationDateTime` half) is a distinct, already-recorded,
deliberately-unfixed gap — not an instance of the cross-call reproducibility class audited
here.

## 2026-08-30 enumcheck typed-response-struct extension: one confirmed bug, five false positives

`cmd/enumcheck` was extended to see an enum value carried on a named response
struct's own composite literal (`SomeType{Field: value}` / `&SomeType{...}`),
not only a `map[string]any` entry — its previously documented blind spot.
Run against `services/dynamodb`, it surfaced 6 findings (all needs-review,
none confident); hand-checked against the pinned dynamodb@v1.63.1 SDK:

- **Confirmed bug, fixed**: `partiql.go`'s `partiqlValidationExceptionCode`
  (`handleBatchExecuteStatement`'s parameter-conversion-failure branch) emitted
  `BatchStatementError.Code = "ValidationException"`. The real
  `BatchStatementErrorCodeEnum` (types/enums.go) has no such member — the
  correct value is `"ValidationError"`. Fixed; covered by
  `TestBatchExecuteStatement_ParameterConversionFailure_ErrorCode`, which
  asserts against `types.BatchStatementErrorCodeEnumValidationError`, not a
  hardcoded string.
- **False positive** (`transact_ops.go:131`, `transact_validation.go:227`):
  `CancellationReason{Code: "None"}` — the real SDK types `CancellationReason.Code`
  as a plain `*string`, not an enum at all (confirmed in types/types.go).
- **False positive** (`global_tables.go:170`, `global_tables.go:541`,
  `replication.go:40`): `Table{Status: statusActive, ...}` — this repo's
  internal `Table` struct's `json:"Status"` tag exists for
  `persistence.go`'s snapshot serialization (save/restore to disk), not the
  AWS wire response; the real `TableDescription` response is built
  separately via `models.FromSDKTableDescription`, whose `TableStatus` field
  carries the correct wire key and value. The checker cannot distinguish a
  same-package tagged struct built for persistence from one built for the
  wire — a real, structural false-positive class this extension can produce,
  disclosed in `cmd/enumcheck`'s package doc.

## 2026-08-30 value-semantics pass (gopherstack-uox6): ListBackups TimeRangeLowerBound inclusivity bug

Targeted pass for gopherstack-uox6 ("a parameter that is read, applied, and
WRONG" -- shape checks are blind to this class). Read every `ComparisonOperator`
member (types/enums.go: EQ/NE/LE/LT/GE/GT/BETWEEN/NOT_NULL/NULL/CONTAINS/
NOT_CONTAINS/BEGINS_WITH/IN, 13 total) against `legacy_conditions.go`'s
`renderComparison` -- all 13 handled correctly (6 via `legacyBinarySymbols`,
3 via `legacyUnaryFuncs`, 4 via the switch), default case rejects an
unrecognized operator with `ValidationException` rather than silently
matching everything or nothing. `ConditionalOperator`'s default (unset) is
AND per `legacyConditionalJoiner`, matching AWS's documented default.
Confirmed Query's `FilterExpression` is applied strictly after
`KeyConditionExpression` resolves candidates (item_ops_query.go:634, inside
`collectQueryPage`, downstream of `filterCandidatesForKeyCondition`) and that
`ConsumedCapacity`/`ScannedCount` are computed from the key-condition-matched
candidate count (item_ops_query.go:93, before the filter runs), not reduced
by the filter -- matches AWS's documented "filter does not reduce consumed
read capacity". `Select`'s four documented values (ALL_ATTRIBUTES/
ALL_PROJECTED_ATTRIBUTES/SPECIFIC_ATTRIBUTES/COUNT) and their interaction
with index projection type and ProjectionExpression/AttributesToGet are all
enforced correctly in `validateSelectConstraints` (validation.go).

**Bug found and fixed**: `ListBackups`' `TimeRangeLowerBound` is documented
inclusive ("Only backups created after this time are listed. TimeRangeLowerBound
is inclusive.", api_op_ListBackups.go) but `collectBackupSummaries`
(backup_ops.go) excluded a backup created at *exactly* that boundary --
`!createdAt.After(lower)` continues (excludes) whenever `createdAt <= lower`,
which wrongly drops the equal-to-bound case. `TimeRangeUpperBound` (documented
exclusive) was already correct. Fixed to `createdAt.Before(lower)` (excludes
only strictly-earlier backups). `TestCollectBackupSummaries_TimeRangeBoundsInclusivity`
(backup_timerange_internal_test.go, whitebox package `dynamodb`) constructs a
backup with a zero-fractional-second `CreationDateTime` so an exact-boundary
comparison is meaningful, and drives `collectBackupSummaries` directly;
hand-verified to fail against unfixed code (0 backups returned for the
inclusive-boundary case, expected 1). No prior test exercised
TimeRangeLowerBound/TimeRangeUpperBound at all.

Also examined and confirmed correct, no bug: `ScanIndexForward` default
(true/ascending) at item_ops_query.go:102; `contains`/`begins_with` string
comparison is case-sensitive (Go's `strings.Contains`/`HasPrefix`, matching
real DynamoDB expression-function semantics -- no case-insensitive mode is
documented for these); parallel-Scan `applySegmentFilter`'s FNV-hash-mod-
TotalSegments partitioning (item_ops_scan.go) gives every item exactly one
owning segment, matching AWS's documented total-coverage guarantee;
`filterGlobalTables`'s RegionName membership filter (global_tables.go)
matches ListGlobalTablesInput's documented "results only include global
tables which have replicas in the selected region."

Coverage is a slice, stated as one: the legacy Query/Scan/PutItem/UpdateItem/
DeleteItem parameter-translation layer (already extensively fixed in prior
lze5/yvs8 passes) and the modern `expr` evaluator's comparison/function
semantics, checked deeply; GSI/LSI-specific filter interactions beyond
projection-type handling, PartiQL's `WHERE`-clause evaluator
(`filterEAVByExpression`, partiql.go), and streams' `appendMatchingRecords`
were not re-examined this pass.

## 2026-08-31 unnamed-in-PARITY sweep (gopherstack-6flj/21my continuation)

Targeted the six `List*`/`Describe*` operations whose names appeared
nowhere in this file before today: `DescribeContinuousBackups`,
`DescribeEndpoints`, `DescribeImport`, `DescribeKinesisStreamingDestination`,
`DescribeLimits`, `DescribeTimeToLive`. Confirmed protocol from the
deserializer directly: `dynamodb@v1.63.1` is `awsAwsjson10_` (JSON RPC 1.0),
not XML -- no case-folding, so a casing mismatch here is a hard decode
failure rather than a latent one. All six read against their own
deserializer/type in `deserializers.go`/`types/types.go`, per op and per
nested item type.

**Bug found and fixed**: `DescribeImport`/`ImportTable`'s
`ImportTableDescription.InputCompressionType` (real member, confirmed at
`types/types.go:2005` and deserializer case `"InputCompressionType"` in
`awsAwsjson10_deserializeDocumentImportTableDescription`) was tracked on the
backend the whole time -- `ImportTable` stores the caller's
`InputCompressionType` on `storedImport.InputCompression` (`store.go:114`)
-- but neither wire converter (`importDescriptionFromRecord`,
`import_export_s3.go`; `importDescriptionWireFromSDK`,
`handler_import.go`) ever read it back out. Every `DescribeImport`/
`ImportTable` response reported an empty compression type regardless of
what GZIP/NONE the import was created with. `ImportSummary` (the
`ListImports` item type) genuinely has no such member, so this was
Describe/ImportTable-only, not a sibling disagreement.
Test: `TestDescribeImport_InputCompressionType`
(`import_input_compression_test.go`), drives the real
`aws-sdk-go-v2/service/dynamodb` client through `ImportTable` then
`DescribeImport` and asserts `InputCompressionType == GZIP` on both
responses. Verified failing pre-fix (`actual: ""`).

**Recorded, not fixed** -- real per-SDK gaps with no ready backing state:
- `DescribeImport`/`ImportTable`'s `ImportTableDescription` is also missing
  `TableCreationParameters` (`*types.TableCreationParameters`,
  `types/types.go:3323`, optional -- not `This member is required`). The
  backend only stores the request's `TableCreationParameters` transiently
  to drive `CreateTable`; `storedImport` never retains the struct itself,
  so echoing it back would need either a new stored field or a
  reconstruction from the resulting `TableDescription`. Left unfixed this
  pass; a straightforward correct fix is to store `input.TableCreationParameters`
  directly on `storedImport` at `ImportTable` time (it is the caller's own
  request value, not synthesized) and thread it through both wire
  converters and the corresponding wire struct.
- `DescribeKinesisStreamingDestination`'s `KinesisDataStreamDestination` is
  missing `DestinationStatusDescription` (`*string`, confirmed real member
  and deserializer case at `deserializers.go` around
  `awsAwsjson10_deserializeDocumentKinesisDataStreamDestination`). Real but
  currently unobservable: this backend's `DestinationStatus` is hardcoded
  to `ACTIVE` (`kinesisDestinationsRLocked`, `kinesis_streaming.go`) and
  never models a FAILED/DISABLING transition, and AWS only populates this
  description field for non-nominal states -- so no legal input through
  this backend would ever produce a non-empty value.

**Clean at both layers, no bug**: `DescribeContinuousBackups` (uses the
real SDK types directly as the backend's return type --
`ContinuousBackupsDescription`/`PointInTimeRecoveryDescription` -- and a
dedicated wire converter, `continuousBackupsOutputFromSDK`, correctly
re-encodes the two `*time.Time` fields as Unix-epoch-seconds floats
matching the JSON protocol's wire format rather than encoding/json's
default RFC3339 string); `DescribeEndpoints` (`Endpoint.Address`/
`CachePeriodInMinutes`, both required members, both present);
`DescribeLimits` (four flat int64 quota fields, all present under their
real names); `DescribeTimeToLive` (`TimeToLiveDescription.AttributeName`/
`TimeToLiveStatus`, both present, wrapper key correct).

Every list in this batch's scope was already correctly member-wrapped
(`DescribeEndpoints`' `Endpoints` list; no other list-shaped ops in this
batch's six).

Gates: `go build ./services/iam/... ./services/dynamodb/...`, `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/dynamodb/...` (pass),
`golangci-lint run ./services/dynamodb/...` (0 issues). No `nolint`
directives in either file touched (`handler_import.go`,
`import_export_s3.go`).
