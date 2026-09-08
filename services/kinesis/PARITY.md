---
service: kinesis
sdk_module: aws-sdk-go-v2/service/kinesis@v1.53.0
last_audit_commit: a4d4c728b
last_audit_date: 2026-08-19
overall: A            # this pass (gopherstack-nbg8): the 2026-07-23 audit's "wire: ok" claim was false for DescribeAccountSettings/UpdateAccountSettings/UpdateMaxRecordSize/UpdateStreamWarmThroughput -- all four decoded wholly fabricated request/response shapes with no basis in the real SDK. Rebuilt all four around their real Input/Output shapes (MinimumThroughputBillingCommitmentInput/Output; MaxRecordSizeInKiB, not Bytes; WarmThroughputMiBps + the previously-unmodeled WarmThroughput Output object). Also found and fixed, while reading the whole operation rather than just the flagged field: DescribeLimits was silently dropping two of its four *required* output members (OnDemandStreamCount/OnDemandStreamCountLimit) -- also marked "wire: ok" -- and UpdateMaxRecordSize's Input had a StreamName field with no basis in the real shape (only StreamARN exists). This is the second and third time this service's manifest has positively claimed verification that was false (see gopherstack-3jqz for the first). Only remaining gap is KMSAccessDeniedException, honestly undeliverable without an IAM policy engine. gopherstack-r80d (required-OUTPUT-member sweep): re-extracted every "This member is required." field from every *Output struct across all 39 ops in the pinned SDK (17 required fields across 11 ops) and cross-checked each against the handler's success path. DescribeLimits (above) was the only miss, already fixed; the other 10 ops (DescribeStream, DescribeStreamConsumer, DescribeStreamSummary, GetRecords, GetResourcePolicy, ListStreams, ListTagsForStream, PutRecord, PutRecords, RegisterStreamConsumer) all populate their required members correctly. Service is settled for this bug class.
ops:
  IncreaseStreamRetentionPeriod: {wire: fixed, errors: ok, state: ok, persist: ok, note: "reverted 2b2086c9: that commit made equal-to-current RetentionPeriodHours return InvalidArgumentException (a strict reading of the aws-sdk-go-v2 doc comment 'Must be more than the current retention period'), which broke TestTerraform_Kinesis in CI -- terraform's aws_kinesis_stream resource issues IncreaseStreamRetentionPeriod even when the requested value already equals the stream's current retention (confirmed live: CreateStream -> 24h default -> Increase(48) OK -> a second Increase(48) against the already-48h stream 400'd with InvalidArgumentException before this fix). Real AWS tolerates the equal case rather than erroring on every no-drift re-apply, so restored equal-value == no-op success. Strictly-lower and out-of-[24,8760] values are still rejected. gopherstack-enpq (2026-08-22): Input had no StreamARN member at all (api_op_IncreaseStreamRetentionPeriod.go:43-58 (StreamARN:52) -- 'you must use either the StreamARN or the StreamName parameter, or both'); an ARN-only caller silently resolved to an empty stream name and 400'd. Fixed via resolveStreamNameAndRegion."}
  DecreaseStreamRetentionPeriod: {wire: fixed, errors: ok, state: ok, persist: ok, note: "reverted 2b2086c9, mirrored: equal-to-current RetentionPeriodHours is a no-op success again (not InvalidArgumentException), matching real AWS/terraform tolerance. Strictly-greater and below-24h-min values are still rejected. gopherstack-enpq (2026-08-22): same missing-StreamARN bug as IncreaseStreamRetentionPeriod (api_op_DecreaseStreamRetentionPeriod.go:39-54 (StreamARN:48)), fixed the same way."}
  CreateStream: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: ON_DEMAND now defaults to 4 shards (was 1); inline Tags now validated pre-mutation and persisted via TagResource instead of a lost handler-local map. 2026-08-23 (request-side accept-and-drop sweep): CreateStreamInput's own MaxRecordSizeInKiB/WarmThroughputMiBps members (api_op_CreateStream.go:101-121 -- distinct from the same-named UpdateMaxRecordSize/UpdateStreamWarmThroughput fields) had no Go field at all; the backend already tracks both (Stream.MaxRecordSizeBytes/WarmThroughputMiBps, read back by DescribeStreamSummary), so a caller specifying either at creation time silently got the 1 MiB default / zero throughput instead. Fixed via resolveCreateStreamMaxRecordSize plus the same range checks UpdateMaxRecordSize/UpdateStreamWarmThroughput already apply."}
  DeleteStream: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "fixed (gopherstack-enpq, cmd/structfielddiff): EnforceConsumerDeletion (real DeleteStreamInput member) was not accepted at all, so this backend deleted a stream unconditionally regardless of registered enhanced fan-out consumers -- more permissive than AWS, whose own doc comment says 'If this parameter is unset (null) or if you set it to false, and the stream has registered consumers, the call to DeleteStream fails with a ResourceInUseException.' Now checked against stream.Consumers before any mutation; new ErrStreamHasConsumers sentinel (ResourceInUseException) wired through resourceErrorDetails. Consumers themselves need no separate deletion step -- they are already keyed off the parent Stream struct (stream.Consumers), not a standalone global table, so they vanish with the stream regardless of EnforceConsumerDeletion's value once the delete is allowed to proceed. FIXED (gopherstack-6kj0, b8484292f): also left b.resourcePolicies[region][streamARN] behind, inherited by a recreated stream of the same name via GetResourcePolicy; now cleared alongside the FIS fault-injection entry. Regression: TestDeleteStream_ClearsResourcePolicyOnRecreate."}
  DescribeStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: Shards list now paginates (Limit/ExclusiveStartShardId/HasMoreShards); previously returned every shard in one page with HasMoreShards hardcoded false"}
  DescribeStreamSummary: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq (2026-08-22): MaxRecordSizeInKiB and WarmThroughput (both real, optional StreamDescriptionSummary members, types/types.go) had no Go field at all -- the backend already tracks the underlying Stream.MaxRecordSizeBytes/WarmThroughputMiBps (set by UpdateMaxRecordSize/UpdateStreamWarmThroughput) but never surfaced either back on describe, so a client had no way to read back settings it had itself just applied. Fixed by adding both to DescribeStreamOutput and the wire response (WarmThroughput.Current/Target both mirror the synchronous-apply model UpdateStreamWarmThroughput already documents)."}
  ListStreams: {wire: ok, errors: ok, state: ok, persist: ok, note: "StreamNames (required) correctly populated; StreamSummaries (optional, richer per-stream shape) is not -- see gaps. gopherstack-wksw (constraint-not-honoured sweep, 2026-08-29): Limit's documented default AND max of 100 (api_op_ListStreams.go: 'The default value is 100. If you specify a value greater than 100, at most 100 results are returned.') was not applied -- an omitted Limit returned the entire account's stream inventory in one page instead of capping at 100, and a Limit > 100 was accepted uncapped rather than clamped. Fixed: both directions now resolve to 100. TestListStreams_DefaultLimit (streams_test.go) confirmed failing pre-fix (105 streams, 0 Limit -> 105 returned, HasMoreStreams false)."}
  PutRecord: {wire: ok, errors: ok, state: ok, persist: ok, note: "MD5 hash routing, explicit hash key, per-shard monotonic sequence numbers verified correct. SequenceNumberForOrdering is accepted-and-ignored: confirmed non-issue (gopherstack-enpq) -- it is a client-side ordering hint only ('If this parameter is not set, records are coarsely ordered based on arrival time'), not a server-enforced/validated field, and this backend already assigns strictly increasing per-shard sequence numbers regardless of it."}
  PutRecords: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: empty Records list now rejected (was silently 200); stream-not-found now fails the whole call with top-level ResourceNotFoundException instead of InternalFailure on every result entry"}
  GetShardIterator: {wire: ok, errors: ok, state: ok, persist: n/a, note: "TRIM_HORIZON/LATEST/AT_(AFTER_)SEQUENCE_NUMBER/AT_TIMESTAMP all verified; iterator token carries region so cross-region record stores stay isolated; fixed: AT_TIMESTAMP with a genuinely omitted Timestamp (JSON field absent, distinguished from an explicit epoch-zero value via *float64) now rejected InvalidArgumentException instead of silently reading from position 0"}
  GetRecords: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-enpq, cmd/structfielddiff): ChildShards (real GetRecordsOutput member, populated 'only when the end of the current shard is reached') had no Go field at all and was never returned, even though this backend already computes the exact end-of-shard condition (Closed && fully-consumed) to null out NextShardIterator. Same fix also caught a second, independent bug in that shared condition: NextShardIterator was always sent as an explicit empty string rather than omitted, and the real SDK deserializer reads an explicit \"\" as a non-nil *string, not nil -- so GetRecordsOutput's own doc-documented end-of-shard signal ('If set to null, the shard has been closed...') never actually fired for a real client, only json:\",omitempty\" makes that true. New childShardsOf walks stream.Shards for ParentShardID/AdjacentParentShardID matches (split children have one parent, merge children have two) and builds the real ChildShard{ShardId,ParentShards,HashKeyRange} shape. 10k-record / 10MiB caps and MillisBehindLatest re-verified unchanged. gopherstack-wksw (2026-08-29): Limit's documented default of 10,000 (api_op_GetRecords.go: 'Specify a value of up to 10,000 ... The default value is 10,000.') was wired as 1,000 (defaultGetRecordsLimit, models.go) -- a real client omitting Limit got a 10x-smaller page than AWS returns, silently changing pagination cadence (not a data-loss bug: the shard iterator still advances correctly and a follow-up GetRecords reads the rest, but every omitted-Limit call under-returned relative to the documented contract). Fixed: constant corrected to 10000. TestGetRecords_ZeroLimitDefaultsTo10000 (records_get_test.go) confirmed failing pre-fix (10500 records seeded, 0 Limit -> 1000 returned, not 10000); the pre-existing TestGetRecords_ZeroLimitUsesDefault only used 5 records so never crossed either candidate default and could not have caught this."}
  ListShards: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "fixed: deleted invented 'AT_SHARD_ID' ShardFilterType (not in the real SDK enum) and its lineage-matching behavior; AFTER_SHARD_ID now implements the real exclusive-start-cursor-over-all-shards semantics; AT_TRIM_HORIZON/AT_TIMESTAMP/FROM_TIMESTAMP now do true per-shard-timestamp filtering (Shard.StartedAt/ClosedAt) instead of approximating as 'include everything'; AT_TIMESTAMP/FROM_TIMESTAMP now require ShardFilterTimestamp (InvalidArgumentException if omitted). gopherstack-enpq (2026-08-22): Input also had no StreamARN member (api_op_ListShards.go:46-126 (StreamARN:110)); fixed via resolveStreamNameAndRegion, also added StreamARN to the NextToken mutual-exclusion check. gopherstack-wksw (2026-08-29): MaxResults' documented default AND max of 1000 (api_op_ListShards.go) was applied only when MaxResults was explicitly set and smaller than the result -- an omitted MaxResults (or one > 1000) returned every matching shard unbounded. Ordinarily masked because the default filter (open shards only) is capped by maxShardsPerStream=100, but AT_TRIM_HORIZON/FROM_TRIM_HORIZON/FROM_TIMESTAMP include CLOSED lineage shards too, which DescribeStream's own comment notes 'accumulates ... forever' for a heavily-resharded stream -- a real account can cross 1000. Fixed both directions to resolve to 1000. TestListShards_DefaultMaxResults (whitebox_test.go, package kinesis -- 1500 shards fabricated directly since reaching this count via real resharding isn't the thing under test) confirmed failing pre-fix (1500 returned)."}
  RegisterStreamConsumer: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: added missing 20-consumers-per-stream limit (LimitExceededException). gopherstack-enpq (2026-08-22): Tags (real, optional RegisterStreamConsumerInput member -- api_op_RegisterStreamConsumer.go: 'You can add tags to the registered consumer when making a RegisterStreamConsumer request by setting the Tags parameter') had no Go field at all and was silently dropped. Fixed: Consumer gained a Tags map (additive, no snapshot version bump), and ListTagsForResource/TagResource/UntagResource now route to it for a consumer ARN -- previously these three only ever resolved a *stream* ARN (streamNameFromARN unconditionally), so a consumer ARN always 404'd even after this fix's own Tags parameter worked. 2026-08-19 wrapper-key/nested-shape sweep: the Consumer object in the response was wired from the same jsonConsumer struct DescribeStreamConsumer uses, which carries a StreamARN key -- but the real types.Consumer (deserializers.go:6279-6349, used by RegisterStreamConsumer and ListStreamConsumers) has no StreamARN member at all; only types.ConsumerDescription (deserializers.go:6353-6432, DescribeStreamConsumer only) does. Fabricated key with no case in the real per-field switch (falls to its silent default, so a real client never broke, just received an extra ignored key). Split into jsonConsumer (no StreamARN) and jsonConsumerDescription (StreamARN); handler_consumers.go."}
  DescribeStreamConsumer: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-19: ConsumerDescription's StreamARN confirmed correct (real types.ConsumerDescription member, deserializers.go:6403-6410) -- only the sibling ops' fabricated copy of it was wrong; see RegisterStreamConsumer."}
  ListStreamConsumers: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-19: same fabricated Consumer.StreamARN key as RegisterStreamConsumer (real types.Consumer has no StreamARN), same fix (jsonConsumer). gopherstack-wksw (2026-08-29): MaxResults' documented default of 100 (api_op_ListStreamConsumers.go) is also only applied when explicitly set and smaller than the result (same pattern as ListShards, consumers.go:197) -- judged NOT to need fixing: RegisterStreamConsumer enforces maxConsumersPerStream=20 (models.go:93) as a hard cap with no deletion-then-recreation-past-the-cap path modeled, so the unbounded branch can never actually return more than 20 consumers, structurally under the 100 default. Left as-is per RESTRAINT (medialive ListOfferings precedent) rather than fixed defensively."}
  DeregisterStreamConsumer: {wire: ok, errors: ok, state: ok, persist: ok}
  SubscribeToShard: {wire: ok, errors: ok, state: ok, persist: n/a, note: "event-stream binary framing verified byte-for-byte (prelude/CRC/headers); polling goroutine bounded by idle-poll count and 5-min deadline, no leak; fixed: AT_TIMESTAMP with a genuinely omitted Timestamp now rejected InvalidArgumentException (was previously ambiguous between omitted and explicit-zero, both silently read from position 0). 2026-08-19: prior byte-level framing checks never ran the real aws-sdk-go-v2 client's own event-stream reader end to end -- new TestSubscribeToShard_RoundTrip (subscribe_roundtrip_test.go) drives client.SubscribeToShard + out.GetStream().Events() for real and confirms the SDK decodes a SubscribeToShardEvent with the record; SubscribeToShardEvent field names (ContinuationSequenceNumber/MillisBehindLatest/Records, deserializers.go:5549-5605) re-confirmed against the per-field switch. ChildShards (optional member of the same event, deserializers.go:5570-5573) is not populated on SubscribeToShardEvent -- see gaps."}
  UpdateShardCount: {wire: fixed, errors: ok, state: ok, persist: ok, note: "double/half scaling window, parent/adjacent-parent lineage, old shards kept CLOSED verified. gopherstack-enpq (2026-08-22): Input had no StreamARN member (api_op_UpdateShardCount.go:77-108 (StreamARN:102)); fixed via resolveStreamNameAndRegion."}
  EnableEnhancedMonitoring: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq (2026-08-22): Input had no StreamARN member (api_op_EnableEnhancedMonitoring.go:34-71 (StreamARN:65)); fixed via resolveStreamNameAndRegion."}
  DisableEnhancedMonitoring: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq (2026-08-22): same missing-StreamARN bug as EnableEnhancedMonitoring (api_op_DisableEnhancedMonitoring.go:34-71 (StreamARN:65)), fixed the same way (shared jsonEnhancedMonitoringReq)."}
  DescribeLimits: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "gopherstack-nbg8: OnDemandStreamCount/OnDemandStreamCountLimit are both required output members (api_op_DescribeLimits.go:34-51, alongside ShardLimit/OpenShardCount) that were silently dropped -- a real client decoded zero values for both regardless of backend state. Wired to new CountOnDemandStreams (region-scoped, mirrors CountOpenShards) and OnDemandStreamCountLimit (the account-level ON_DEMAND cap CreateStream already enforced -- previously only reachable, incorrectly, through UpdateAccountSettings' fabricated shape below)."}
  DescribeAccountSettings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-nbg8: the prior wire: ok claim was false. The real Output has exactly one member, MinimumThroughputBillingCommitment (api_op_DescribeAccountSettings.go:34-45); ShardLimit/OnDemandStreamCount/OnDemandStreamCountLimit were never real members of this op -- they belong to DescribeLimits (see above), suggesting the original audit confused the two sibling ops. Rebuilt around the real MinimumThroughputBillingCommitmentOutput shape (Status/StartedAt/EndedAt/EarliestAllowedEndAt, all epoch-seconds timestamps via pkgs/awstime.Epoch). This backend has no billing engine: no billing behaviour follows from an ENABLED commitment, and EarliestAllowedEndAt is never populated (it needs a commitment-window model this backend doesn't have -- see gaps). Status/StartedAt/EndedAt now persist across snapshot/restore."}
  UpdateAccountSettings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-nbg8: the prior wire: ok claim was false. The real Input has exactly one, required, member, MinimumThroughputBillingCommitment (api_op_UpdateAccountSettings.go:42-51); ShardLimit/OnDemandStreamCount/OnDemandStreamCountLimit were fabricated with no basis in the real shape, so every real client's request was silently ignored. Now decodes and validates the real Status enum (ENABLED/DISABLED -> InvalidArgumentException otherwise), stores it as account-level state, and returns the real Output shape. The on-demand-stream cap this field used to (mis)configure is real internal state (CreateStream's checkOnDemandLimit via b.onDemandStreamCountLimit) -- real AWS manages it as a Service Quota, not via this op, so it moved to a Go-level-only SetOnDemandStreamCountLimit config knob (no wire equivalent, mirroring WithKMSValidator's cross-service config pattern) rather than being deleted."}
  UpdateMaxRecordSize: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-nbg8: the prior wire: ok claim was false. Decoded a JSON key MaxRecordSizeBytes; the real (and only) required field is MaxRecordSizeInKiB (api_op_UpdateMaxRecordSize.go:30-47), and the unit is KiB, not bytes -- a real request left the value at zero, which the existing bounds check then always rejected with InvalidArgumentException (every real call 400'd). Also found while reading the whole operation, not just the flagged field: the real Input has no StreamName member at all -- only StreamARN, plus StreamId (reserved for future use, not modeled) -- but gopherstack was additionally decoding and consuming a fabricated StreamName field. Now resolves the stream from StreamARN only and converts the requested KiB value to bytes via bytesPerKiB before applying it to Stream.MaxRecordSizeBytes."}
  UpdateStreamWarmThroughput: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-nbg8: the prior wire: ok claim was false, and 'intentional no-op' was not: the op decoded fabricated WriteCapacityUnits/ReadCapacityUnits fields with no basis in the real shape, so every real client's request silently no-op'd. The real required field is WarmThroughputMiBps (api_op_UpdateStreamWarmThroughput.go:63-70). Also unmodeled: the real Output (StreamARN/StreamName/WarmThroughput{CurrentMiBps,TargetMiBps}, api_op_UpdateStreamWarmThroughput.go:76-88) -- the handler returned an empty struct{}. Now decodes/validates WarmThroughputMiBps (bounds-checked against AWS's documented 10 GiBps default cap, maxWarmThroughputMiBps), stores it on Stream.WarmThroughputMiBps, and returns the real Output shape. Applied synchronously since this backend has no UPDATING transient-state model (unlike real AWS, which returns the stream to ACTIVE asynchronously) -- Current/Target always match on read; see gaps."}
  MergeShards: {wire: ok, errors: ok, state: ok, persist: ok, note: "adjacency check (either shard may be passed first), closed-parent lineage verified"}
  SplitShard: {wire: ok, errors: ok, state: ok, persist: ok, note: "NewStartingHashKey must be strictly inside parent range, verified"}
  StartStreamEncryption: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: KeyId is now required and format-validated (UUID/key ARN/alias ARN/alias name, matching the four shapes the SDK doc comment enumerates) -- InvalidArgumentException if malformed; optional KMSKeyValidator (WithKMSValidator, wired to the real kms backend by cli.go's wireKinesisKMS) additionally verifies the key exists and is usable, returning KMSNotFoundException/KMSDisabledException/KMSInvalidStateException -- all three are real types.KMSNotFoundException-class exceptions confirmed present in the SDK's StartStreamEncryption error set (deserializers.go), contradicting the previous audit's claim that no KMS-specific exception exists for this op. With no validator wired, only the format check applies (a well-formed but nonexistent KeyId is accepted, same permissive behavior as before)."}
  StopStreamEncryption: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: KeyId is now required and format-validated like StartStreamEncryption (matches the SDK's required-field validator); never calls the KMS validator since disabling encryption must succeed even if the key was later disabled/deleted"}
  DeleteResourcePolicy: {wire: ok, errors: ok, state: ok, persist: n/a, note: "resource policies not yet in backendSnapshot; see gaps"}
  GetResourcePolicy: {wire: ok, errors: ok, state: ok, persist: n/a}
  PutResourcePolicy: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now reads backend stream.Tags via Backend.ListTagsForResource instead of a handler-local map that was previously the ONLY store for tags applied via CreateStream/AddTagsToStream/RemoveTagsFromStream"}
  AddTagsToStream: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: now writes through Backend.TagResource (stream.Tags) instead of a handler-local map dropped on Snapshot. gopherstack-enpq (2026-08-22): Input also had no StreamARN member (api_op_AddTagsToStream.go:39-54 (StreamARN:48)); fixed via resolveStreamNameAndRegion."}
  RemoveTagsFromStream: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: now writes through Backend.UntagResource. gopherstack-enpq (2026-08-22): same missing-StreamARN bug (api_op_RemoveTagsFromStream.go:38-52 (StreamARN:46)), fixed the same way."}
  ListTagsForStream: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: now reads Backend.ListTagsForResource. gopherstack-enpq (2026-08-22): same missing-StreamARN bug (api_op_ListTagsForStream.go:35-53 (StreamARN:47)), fixed the same way."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now enforces the 50-tag cap consistently with AddTagsToStream (previously uncapped)"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateStreamMode: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: PROVISIONED -> ON_DEMAND now auto-reshards up to defaultOnDemandShardCount (4, matching CreateStream's ON_DEMAND default) when the stream is currently below that floor, closing the old open shards (CLOSED, retained for lineage) and opening new ones spanning the full hash range -- reuses the same reshardTo helper UpdateShardCount uses. This approximates AWS's documented 'scale to double the max/peak-30-day throughput, whichever is higher' behavior, which requires a throughput-history model this emulator doesn't have; see gaps for the remaining approximation gap. ON_DEMAND -> PROVISIONED never reshards (keeps current shard count as the new baseline), matching AWS. 2026-08-23 (request-side accept-and-drop sweep): WarmThroughputMiBps (real, optional UpdateStreamModeInput member, 'only valid when the stream mode is being updated to on-demand') had no Go field at all in the decode struct -- dropped even on an ON_DEMAND transition, though the backend already tracks Stream.WarmThroughputMiBps and reads it back on DescribeStreamSummary. Fixed: applied (with the same range check UpdateStreamWarmThroughput uses) only when the transition target is ON_DEMAND, matching the documented constraint; ignored on a PROVISIONED transition rather than erroring."}
families:
  hash_key_routing: {status: ok, note: "MD5-based partition-key routing and explicit-hash-key routing verified against big.Int range math; shardForHashKey fallback-to-first-open-shard behavior documented"}
  sequence_numbers: {status: ok, note: "per-shard monotonic NextSeq counter, 49-prefixed AWS-shaped sequence string, persisted via Shard.NextSeq"}
  reshard_lineage: {status: ok, note: "SplitShard/MergeShards/UpdateShardCount/UpdateStreamMode all set ParentShardID/AdjacentParentShardID correctly; closed shards retained forever for DescribeStream/ListShards lineage (see leaks note); Shard gained StartedAt/ClosedAt (set by every shard-creation/closeShard call site) so ListShards' timestamp-bounded ShardFilter types can do real time-bounded filtering instead of approximating"}
  error_codes: {status: ok, note: "ResourceNotFoundException/ResourceInUseException/InvalidArgumentException/ProvisionedThroughputExceededException/ExpiredIteratorException/LimitExceededException/UnknownOperationException all verified exact string + 400 status. fixed: KMSNotFoundException/KMSDisabledException/KMSInvalidStateException are now modeled and reachable via StartStreamEncryption's optional KMSKeyValidator (see StartStreamEncryption note) -- the previous audit's claim that Kinesis has no KMS-specific exceptions was wrong; deserializers.go's awsAwsjson11_deserializeOpErrorStartStreamEncryption lists KMSAccessDeniedException/KMSDisabledException/KMSInvalidStateException/KMSNotFoundException/KMSOptInRequired/KMSThrottlingException/AccessDeniedException as real modeled errors for this op. KMSAccessDeniedException specifically remains unreachable -- see gaps."}
gaps:
  - "KMSAccessDeniedException (types.KMSAccessDeniedException) is a real modeled StartStreamEncryption/StopStreamEncryption error but has no trigger path: it requires evaluating a KMS key policy/grant against a calling principal, and gopherstack has no IAM policy evaluation engine anywhere (not just in kinesis) to produce an access-denied decision from. The sentinel (ErrKMSAccessDenied) and its InvalidArgumentException-style wire mapping (KMSAccessDeniedException, 400) are defined for wire-shape completeness, matching the real error type string exactly, but nothing in the backend can ever return it. Fabricating a fake denial rule (e.g. 'deny if KeyId contains X') would itself be a stub, so this stays an honest gap rather than a fake implementation. (bd: gopherstack-ud2)"
  - "UpdateStreamMode's PROVISIONED -> ON_DEMAND auto-reshard (see UpdateStreamMode note) approximates AWS's real throughput-history-based scaling with a fixed floor (defaultOnDemandShardCount = 4); it does not scale further for streams whose sustained load would earn a higher on-demand shard count in real AWS, since that requires tracking throughput history this emulator has no model for. Low priority: most callers re-describe the stream after the transition and adapt to whatever shard count comes back. (bd: gopherstack-ud2)"
  - "AT_TRIM_HORIZON's trim-horizon instant is computed from the stream's RetentionPeriod but clamped to never predate the stream's own oldest tracked shard StartedAt (see trimHorizon in shards.go), so it degrades gracefully for young streams instead of AWS's true 'oldest data still available' semantics that would require tracking exactly when each record was trimmed, not just when its shard opened/closed. Close enough for shard-lineage filtering (the documented ShardFilter use case); would diverge from AWS in a scenario with partial mid-shard trimming, which this emulator's record ring-buffer model doesn't represent per-shard trim timestamps for."
  - "CORRECTED this pass: the previous gap entry claiming resource policies (PutResourcePolicy/GetResourcePolicy/DeleteResourcePolicy) are lost across a persistence restart was stale/incorrect. persistence.go's backendSnapshot already has a ResourcePolicies field wired into both Snapshot (line ~60) and Restore (line ~119), and TestInMemoryBackend_FullStateSnapshotRestoreRoundTrip already exercises PutResourcePolicy through an actual snapshot/restore cycle and passes. No code change needed; this was a documentation-only correction (carried forward unchanged from the prior ledger)."
  - "CORRECTED this pass: the deferred entry below claiming Lambda event-source-mapping trigger wiring 'lives in cli.go per task constraints; not touched' was stale -- cli.go's wireKinesisLambda (called at cli.go:2657) already wires services/kinesis to services/lambda's event-source poller via kinesisReaderAdapter, and this has been true since before this pass. Moved out of deferred; documentation-only correction, no code changed for this item."
  - "AccessDeniedException is declared in UpdateMaxRecordSize's and UpdateStreamWarmThroughput's error switches (deserializers.go's awsAwsjson11_deserializeOpErrorUpdateMaxRecordSize / ...UpdateStreamWarmThroughput both list it) but has no trigger path, for the same reason as KMSAccessDeniedException above: no IAM policy evaluation engine anywhere in gopherstack to produce an access-denied decision from. Not fabricated a fake rule for it; stays an honest gap. (gopherstack-nbg8)"
  - "ResourceInUseException is declared for both UpdateMaxRecordSize and UpdateStreamWarmThroughput (real AWS returns it when the target stream isn't ACTIVE, e.g. mid-UPDATING from a concurrent operation) but is unreachable here: every stream in this backend is ACTIVE immediately on creation and stays so (no transient CREATING/UPDATING/DELETING window is modeled anywhere in kinesis, not just for these two ops), so there is never a state where the check could fire honestly. (gopherstack-nbg8)"
  - "UpdateStreamWarmThroughput applies the requested WarmThroughputMiBps synchronously; real AWS documents this as asynchronous (stream goes UPDATING, then back to ACTIVE, 'could take a few minutes to complete' for a large stream). This backend has no transient-state model for any stream-level operation (see the ResourceInUseException gap above), so WarmThroughputObject.CurrentMiBps and TargetMiBps always match immediately on read instead of Current lagging Target during a scale-up window. (gopherstack-nbg8)"
  - "2026-08-14 (gopherstack-enpq): mechanical struct-field diff (cmd/structfielddiff) against aws-sdk-go-v2/service/kinesis@v1.46.4, a different method than the op-by-op audits above (gopherstack-nbg8/r80d, 2026-08-13). Only 4 of 39 ops had a real candidate field miss after excluding ResultMetadata/StreamId (both known noise -- StreamId is 'Not Implemented. Reserved for future use.' on every op that has it, same class as sqs's already-known-noise fields), a low false-positive rate that independently re-confirms this service's very recent A-grade sweep by a different method than the reads that earned it. 2 real bugs FOUND AND FIXED: DeleteStream's missing EnforceConsumerDeletion gate (see DeleteStream ops entry) and GetRecords' missing ChildShards plus the NextShardIterator empty-string-vs-null bug it uncovered alongside it (see GetRecords ops entry). PutRecordInput.SequenceNumberForOrdering confirmed a non-issue (see PutRecord ops entry). ListStreamsOutput.StreamSummaries disclosed below, not fixed."
  - "CORRECTED (gopherstack-enpq, 2026-08-22): the 2026-08-14 entry above's 'only 4 of 39 ops had a real candidate field miss' / 'low false-positive rate ... independently re-confirms' claim was overstated -- re-running structfielddiff and hand-verifying every op's request-side identifier contract against its own api_op_*.go doc comment (not just its field list) found 9 more real bugs the prior pass missed: AddTagsToStream/RemoveTagsFromStream/ListTagsForStream/IncreaseStreamRetentionPeriod/DecreaseStreamRetentionPeriod/ListShards/EnableEnhancedMonitoring/DisableEnhancedMonitoring/UpdateShardCount all had no Go field for StreamARN despite each op's own doc comment requiring support for it ('you must use either the StreamARN or the StreamName parameter... recommended that you use the StreamARN input parameter') -- plus RegisterStreamConsumer.Tags (structurally absent) and DescribeStreamSummary.MaxRecordSizeInKiB/WarmThroughput (tracked internally, never surfaced). All 11 fixed this pass; see the dated section below and each op's own ops: entry."
  - "DISCLOSED, not fixed (2026-08-19 wrapper-key/nested-shape sweep, Layer 3 -- never-emitted optional members, explicitly out of scope as a hunt per that sweep's charter): StreamDescriptionSummary is missing three optional real members it could populate from backend state already tracked on Stream -- MaxRecordSizeInKiB (Stream.MaxRecordSizeBytes / bytesPerKiB), StreamId (n/a -- real AWS documents this as 'Not Implemented. Reserved for future use.', same known-noise class as other StreamId fields), and WarmThroughput (Stream.WarmThroughputMiBps, same WarmThroughputObject shape UpdateStreamWarmThroughput already emits correctly). UpdateShardCountOutput.StreamARN (optional) is also never emitted -- UpdateShardCountOutput (models.go) has no StreamARN field at the backend-output level, so this needs backend plumbing, not a one-line wire fix. Record.EncryptionType (optional, both GetRecordsOutput.Records and SubscribeToShardEvent.Records) is never emitted -- this backend does track Stream.EncryptionType but doesn't thread it onto individual jsonRecord entries. None of these are wrong keys or wrong types; they are members with no case reached at all because nothing writes them. (bd: gopherstack-ud2)"
  - "DISCLOSED, not fixed (gopherstack-enpq): ListStreamsOutput.StreamSummaries ([]types.StreamSummary -- ARN/name/status/creation-timestamp/mode per stream, optional not required) is not populated; only the required StreamNames is. Real AWS's newer SDKs/console traffic favor StreamSummaries over the legacy StreamNames-only shape, so a client reading only StreamSummaries would see an empty list even though StreamNames (the field the real validator actually requires) is correct. Not fixed this pass: the backend's ListStreams pagination is built entirely around a sorted []string of names (streams.go), and building StreamSummaries correctly means carrying the full *Stream (or at least ARN/Status/CreatedAt/StreamMode) through that same pagination window rather than bolting a lookup on afterward -- a real reshape of ListStreamsOutput/the backend method signature, not a one-line add, so it was left disclosed rather than rushed. (bd: gopherstack-ud2)"
deferred:
  - "Enhanced fan-out SubscribeToShard real streaming cadence / HTTP2 push semantics beyond the polling emulation already in place"
leaks: {status: clean, note: "stream.mu (lockmetrics) and stream.Tags always Close()'d on DeleteStream/Purge; SubscribeToShard polling goroutine bounded by subscribeToShardMaxIdlePolls (3) and a 5-minute deadline, exits on ctx.Done(); FIS throughput-fault goroutines bound to experiment ctx or scheduled cleanup, lazily evict on read; janitor retention sweep is a single ticker goroutine stopped via context cancellation, no per-stream goroutines; this pass's reshardTo/closeShard/KMSKeyValidator additions introduce no goroutines, tickers, or new lock-acquisition orderings -- KMS validation is a synchronous in-process call into the kms package's own locked backend while kinesis holds stream.mu, safe because kms never calls back into kinesis"}
---

## Notes

### 2026-08-29 constraint-not-honoured sweep (gopherstack-wksw, this pass)

New bug class for this campaign: a parameter that constrains a result (filter/sort/page
limit) present in the real Input but not correctly honoured by the handler/backend --
distinct from the wire-shape (wrong key/type) bugs the 16 passes above already swept for.
Read every collection-returning op's real `<Op>Input` (kinesis@v1.46.4) against its
handler and backend method.

**3 real bugs found and fixed** (all the same "documented default silently unapplied"
shape -- see `ListStreams`/`ListShards`/`GetRecords` ops entries above for full detail,
SDK line references, and the failing-pre-fix test for each): `ListStreams.Limit` (default
100 not applied, unbounded instead), `ListShards.MaxResults` (default 1000 not applied for
filter types that include closed shards), `GetRecords.Limit` (default wired as 1000
instead of the documented 10000 -- the one case in this sweep that under-returns relative
to spec rather than over-returns). **1 gap judged not worth fixing**: `ListStreamConsumers`
has the identical unapplied-default shape but is structurally bounded to 20 consumers by
`RegisterStreamConsumer`'s own limit, well under the 100 default, so the missing cap is
unobservable (see its own ops entry).

Everything else checked out already correct: `DescribeStream`'s `Limit`/
`ExclusiveStartShardId` default-100/max-10000 pagination (the "good sibling" that showed
the other three were wrong); `ListShards`' `ShardFilter` semantics (already fixed by a
prior pass, re-confirmed here); `ListStreamConsumers`/`ListStreams` `NextToken` cursor
round-trip; `ListTagsForStream`'s `Limit`/`ExclusiveStartTagKey` (default 10/max 50,
`handler_tags.go:180-198`) -- correctly plumbed at the handler layer, not the backend;
`ListTagsForResource` correctly has no pagination member in the real Input (`ResourceARN`
only) so nothing was missing there. No mismatched required-vs-optional-ARN target filters
found (`ListStreamConsumers.StreamARN`, `ListTagsForStream`/`ListTagsForResource`'s
resource-ARN resolution) -- all correctly scope results to the named resource, verified by
reading `regionFromARNOrCtx`/`streamNameFromARN`/`consumerInfoFromARN` call sites.

Real-SDK-client round trip not used for these 3 fixes: each bug is purely in what the
backend does with a value it already decodes correctly (`input.Limit`/`input.MaxResults`
already arrive as the right Go int; the bug is the `<= 0` fallback branch), so a hand-built
`*Input` struct via the exported Go backend method inherits the bug identically to a real
typed client call -- the narrow exception the campaign brief allows. `go vet ./...`
(repo-wide, no backend signatures changed), `go test -race -count=1 ./services/kinesis/...`,
and `golangci-lint run ./services/kinesis/...` (0 issues) all clean after the fix.

### 2026-08-23 request-side accept-and-drop sweep (this pass)

kinesis's request side had not been swept in this campaign before now: its only prior lead
(`StreamId` missing from ~25 ops) was already correctly closed as not-a-bug -- every op that carries
it marks it `// Not Implemented. Reserved for future use.` on the real SDK Input, so an absent Go
field there is inert, not a defect. This pass instead compared every one of the 39 real `<Op>Input`
struct's members (`api_op_*.go`, kinesis@v1.46.4) against each op's actual decode struct, looking
for a member the real Input declares that the decode struct silently drops.

Kinesis is pure `awsjson1.1` (JSON-RPC) end to end -- confirmed no op has a
`serializeOpHttpBindings<Op>Input` function in `serializers.go` -- so every member lives in the body;
there is no query- or URI-bound member anywhere in this service to trip on (rule 3 from the sweep
charter is a non-issue here, unlike dynamodb's mixed query+body binding).

**Signal size**: 39 ops read end to end; 34 wired handlers plus `SubscribeToShard`'s separate
event-stream path checked against their decode structs. **2 real accept-and-drop bugs found and
fixed** (both optional-member-dropped / silent-no-op class, both backed by state the backend already
tracks and reads back elsewhere -- see `CreateStream`/`UpdateStreamMode` ops entries above for detail
and SDK line references). **0 false positives hand-checked away** -- every other candidate absence
(`GetRecordsInput.StreamName`, `PutRecordInput.SequenceNumberForOrdering`, every `StreamId`) had
already been individually confirmed as not-a-bug by a prior pass (see those ops' own entries and the
"Rejected candidates" note above); this pass re-confirmed rather than re-litigating them and found no
new false positive.

Both fixes proven via a real `aws-sdk-go-v2/service/kinesis` client round trip
(`TestCreateStream_MaxRecordSizeAndWarmThroughput`, `TestUpdateStreamMode_WarmThroughputMiBps`,
`wire_field_fixes_test.go`): each confirmed failing against the unfixed code first (CreateStream
reported the 1024 KiB default / a nil `WarmThroughput` instead of the requested values;
UpdateStreamMode reported `0` instead of the requested `WarmThroughputMiBps`), then passing after the
fix. Hand-reverted (`streams.go`, `stream_modes.go`, `handler_streams.go`, `handler_stream_modes.go`,
`models.go` restored from `git show HEAD:...`) -- both tests failed against the reverted tree with the
same errors, then restored byte-identical (`md5sum` match). `streams.go`'s `CreateStream` was
decomposed into `resolveCreateStreamModeAndShardCount`/`resolveCreateStreamMaxRecordSize` helpers to
stay under `cyclop`'s complexity ceiling after the added validation branches -- no `//nolint` used.
`golangci-lint run ./services/kinesis/...` 0 issues; `go test -race ./services/kinesis/...` and this
service's `persistence_test.go` both green; `go build ./cmd/pgoload/... ./test/integration/...`
confirmed no out-of-package literal broke (both structs' new fields are purely additive, all call
sites use named-field literals). Neither `CreateStreamInput` nor `UpdateStreamModeInput` participates
in `Stream`'s persisted JSON shape directly -- both are transient wire/domain input types, not the
persisted struct itself -- so no `kinesisSnapshotVersion` bump was needed or made.

### 2026-08-19 wrapper-key / nested-shape sweep (this pass)

Full sweep of all 39 ops against `aws-sdk-go-v2/service/kinesis@v1.46.4`'s live deserializer path
(`awsAwsjson11_*`, JSON-RPC 1.1 confirmed from `api_client.go`'s `X-Amz-Target:
Kinesis_20131202.<Op>` headers in `serializers.go`). For JSON-RPC, every op's `HandleDeserialize`
calls its `deserializeOpDocument<Op>Output` directly on the raw JSON body -- there is no REST-style
httpPayload dead-code trap here (that trap is specific to REST protocols with a single-structure-member
Output and no header bindings); confirmed via `grep -c` on a sample of OpDocument helpers (count 2:
one definition, one live call site).

**1 real bug found and fixed**: `RegisterStreamConsumer`/`ListStreamConsumers` (see those ops'
entries) shared one `jsonConsumer` wire struct with `DescribeStreamConsumer` that carried a `StreamARN`
key -- but real `types.Consumer` (used by the first two) has no such member; only `types.ConsumerDescription`
(`DescribeStreamConsumer` only) does. Benign in practice (the real deserializer's per-field switch
silently drops unrecognized keys via its `default` case, so no client call ever broke), but a genuine
fabricated-member bug per the sweep's bug-class definition. Fixed by splitting into `jsonConsumer`
(no `StreamARN`) and `jsonConsumerDescription` (`StreamARN`); `handler_consumers.go`. An existing test
(`TestConsumerLifecycle` in `consumers_test.go`) asserted `regResp.Consumer.StreamARN` on the
RegisterStreamConsumer response, locking in the fabricated key -- corrected to a raw-JSON absence
check instead (a typed client can't observe an extra key it has no field for). New dedicated regression
test: `TestConsumerWireShape_StreamARN` (`consumer_wire_shape_test.go`), table-driven over
RegisterStreamConsumer/ListStreamConsumers (must NOT carry `StreamARN`) vs DescribeStreamConsumer
(must carry it), via raw-body checks since this is exactly the "typed client can't see it" case the
sweep's method calls out. Hand-revert reproduced the exact predicted symptom (both
`TestConsumerLifecycle` subtests and both `TestConsumerWireShape_StreamARN` negative subtests failed
with the leaked key present; `describe_has_StreamARN` correctly kept passing throughout since that
path was never wrong); restore confirmed byte-identical.

**Nested shapes re-verified clean** against their own deserializers (`Shard`/`HashKeyRange`/
`SequenceNumberRange`, `StreamDescription`/`StreamDescriptionSummary` as a genuine summary-vs-full
pair, `Consumer`/`ConsumerDescription`, `Record` including its base64 `Data` and epoch-seconds
`ApproximateArrivalTimestamp`, `EnhancedMetrics`, `StreamModeDetails`, `ChildShard`,
`PutRecordsResultEntry`, `WarmThroughputObject`, `MinimumThroughputBillingCommitmentOutput`): every
key name, nesting level, and JSON type in `handler_streams.go`, `handler_records.go`,
`handler_shards.go`, `handler_resharding.go`, `handler_account_settings.go`, `handler_stream_modes.go`
matches its deserializer's per-field switch exactly. No summary-vs-full type confusion (the dominant
bug class found elsewhere this campaign) -- `StreamDescription` and `StreamDescriptionSummary` are
each read from their own, separately-verified deserializer.

**New real-SDK-client round-trip proof for `SubscribeToShard`**: no prior test drove the real
`aws-sdk-go-v2` client's own event-stream reader end to end (existing tests parsed the binary frames
by hand). `TestSubscribeToShard_RoundTrip` (`subscribe_roundtrip_test.go`) does: CreateStream ->
RegisterStreamConsumer -> PutRecord -> `client.SubscribeToShard` -> `out.GetStream().Events()`,
and confirms the SDK's own reader decodes a `SubscribeToShardEvent` carrying the record. Confirms the
`:event-type`/`:message-type` event-stream framing (`buildEventStreamHeaders`/`encodeEventStreamMsg`
in `handler_consumers.go`) is correct, not just byte-identical to a hand-checked expectation.

Three new disclosed (not fixed, Layer 3 -- never-emitted optional members, out of scope as a hunt
per this sweep's charter) gaps recorded below: `StreamDescriptionSummary`'s
`MaxRecordSizeInKiB`/`WarmThroughput`, `UpdateShardCountOutput.StreamARN`, and `Record.EncryptionType`.

### Account-settings and throughput ops decoded wholly fabricated shapes (this pass: gopherstack-nbg8)

The 2026-07-23 audit claimed `wire: ok` for `DescribeAccountSettings`, `UpdateAccountSettings`,
`UpdateMaxRecordSize`, and `UpdateStreamWarmThroughput`. All four were false, verified against the
pinned `aws-sdk-go-v2/service/kinesis@v1.46.4` (resolved from `go.mod`, read only from that path under
`$(go env GOMODCACHE)`; kinesis confirmed JSON-RPC 1.1, `awsAwsjson11_*` serializer prefix).

**`UpdateAccountSettings` / `DescribeAccountSettings`.** The real `UpdateAccountSettingsInput`
(`api_op_UpdateAccountSettings.go:42-51`) has exactly one member, `MinimumThroughputBillingCommitment
*types.MinimumThroughputBillingCommitmentInput` (required), whose own only member is `Status`
(`ENABLED`/`DISABLED`, `types/types.go:168-176`). `DescribeAccountSettingsInput` has *no* members at
all, and `DescribeAccountSettingsOutput`/`UpdateAccountSettingsOutput` both echo a single
`MinimumThroughputBillingCommitment *types.MinimumThroughputBillingCommitmentOutput`
(`types/types.go:178-197`: `Status` required, plus optional `StartedAt`/`EndedAt`/`EarliestAllowedEndAt`
timestamps). gopherstack instead modeled `ShardLimit`/`OnDemandStreamCount`/`OnDemandStreamCountLimit` --
none of which are real members of *either* op. `ShardLimit`/`OnDemandStreamCount`/
`OnDemandStreamCountLimit` are real, but they belong to the sibling op `DescribeLimits`
(`api_op_DescribeLimits.go:34-51`, all four members required) -- the original audit appears to have
conflated the two.

Rebuilt around the real shape: `MinimumThroughputBillingCommitmentInput`/`Output` (Go-level, mirroring
the SDK types), decoded/encoded via `Status` and epoch-seconds timestamps
(`pkgs/awstime.Epoch`, confirmed against `deserializers.go:6758-6790` -- this protocol's Timestamp shape
is `unixTimestamp`, a JSON number, not RFC3339). `UpdateAccountSettings` validates `Status` against the
two real input values (`InvalidArgumentException` otherwise, matching the op's declared error switch:
`InvalidArgumentException`/`LimitExceededException`/`ValidationException`), stamps `StartedAt` on a
`DISABLED -> ENABLED` transition and `EndedAt` on `ENABLED -> DISABLED`, and persists across
snapshot/restore. Per the task's fixing rule for a real-but-unbacked billing concept: this backend has no
billing engine, so **no billing behaviour follows from `ENABLED`** -- it is state that is stored and
echoed, nothing more. `EarliestAllowedEndAt` and the output-only `ENABLED_UNTIL_EARLIEST_ALLOWED_END`
status are never produced, since computing them needs a commitment-window model this backend doesn't
have (see `gaps`).

`OnDemandStreamCountLimit` was real internal state (`CreateStream`'s `checkOnDemandLimit`, backing the
actual per-account ON_DEMAND-stream cap), just reachable through the wrong op. There is no real AWS wire
operation that lets a caller change it (AWS manages it as a Service Quota); it is now set via a
Go-level-only `InMemoryBackend.SetOnDemandStreamCountLimit`, mirroring `WithKMSValidator`'s
cross-service-config-outside-the-wire-protocol pattern, rather than deleted outright or left reachable
through a fabricated field.

**`UpdateMaxRecordSize`.** Decoded a JSON key `MaxRecordSizeBytes`; the real (and only required) field is
`MaxRecordSizeInKiB` (`api_op_UpdateMaxRecordSize.go:30-47`), and the unit differs too -- KiB, not bytes
-- so this was not a simple rename. A real client's request left the decoded value at its Go zero (0),
which the backend's own bounds check (`< defaultMaxRecordSizeBytes || > absoluteMaxRecordSizeBytes`) then
always rejected with `InvalidArgumentException`: **the op 400'd on every real call**, reproduced and
confirmed via a hand-revert of the fix. Reading the whole operation (not just the flagged field) surfaced
a second, independent fabrication: the real Input has **no `StreamName` member at all** -- only
`StreamARN` and `StreamId` (the latter explicitly "Not Implemented. Reserved for future use" per the
SDK's own doc comment) -- yet gopherstack was decoding and consuming a `StreamName` field with no basis
in the real shape. Fixed by converting the requested KiB value to bytes (`bytesPerKiB`) before applying
it to `Stream.MaxRecordSizeBytes` (kept as the internal representation; only the wire unit changed), and
by resolving the target stream from `StreamARN` alone.

**`UpdateStreamWarmThroughput`.** Decoded fabricated `WriteCapacityUnits`/`ReadCapacityUnits`; the real
required field is `WarmThroughputMiBps` (`api_op_UpdateStreamWarmThroughput.go:63-70`), so a real client's
request **silently no-op'd** -- reproduced and confirmed via hand-revert. The real Output
(`StreamARN`/`StreamName`/`WarmThroughput *types.WarmThroughputObject{CurrentMiBps,TargetMiBps}`,
`api_op_UpdateStreamWarmThroughput.go:76-88`) was entirely unmodeled: the handler returned an empty
`struct{}{}`. Fixed: `WarmThroughputMiBps` is decoded, bounds-checked against AWS's documented default cap
("you cannot scale to more than 10 GiBps for an on-demand stream", `maxWarmThroughputMiBps`), stored on
`Stream.WarmThroughputMiBps`, and echoed in the real Output shape (`StreamARN`/`StreamName` resolved from
the target stream, `WarmThroughput.CurrentMiBps`/`TargetMiBps` both set to the requested value). Applied
synchronously, since this backend has no `UPDATING` transient-state model for any stream-level op (real
AWS applies this asynchronously); see `gaps`.

**`DescribeLimits`** (also flagged `wire: ok`, not itself in the reported bug but immediately adjacent):
its real Output (`api_op_DescribeLimits.go:34-51`) has **four required members**
(`ShardLimit`/`OpenShardCount`/`OnDemandStreamCount`/`OnDemandStreamCountLimit`); gopherstack modeled only
the first two, so a real client decoded `0` for `OnDemandStreamCount`/`OnDemandStreamCountLimit`
regardless of actual backend state -- the exact "required-member drop" bug class this campaign is
hunting, found only because fixing the account-settings ops required tracing where the real
`OnDemandStreamCount`/`OnDemandStreamCountLimit` concept actually belongs. Fixed by adding
`InMemoryBackend.CountOnDemandStreams` (region-scoped, mirrors the existing `CountOpenShards` convention)
and `InMemoryBackend.OnDemandStreamCountLimit`, both wired into `handleDescribeLimits`.

**Test fallout.** Two existing tests encoded the fabricated shapes and asserted only a status code, not a
real round trip: `TestKinesis_UpdateAccountSettings` sent an unrelated `ShardLevelMetrics` field (matching
neither the fabricated nor the real shape) and asserted 200 only; `TestKinesis_UpdateStreamWarmThroughput`
sent `ConsumersToPut`/`WriteProvisionedUnits` (matching neither shape either) and accepted `200-299 or
400` as passing -- a test that could not fail. Both replaced with real-`aws-sdk-go-v2`-client round trips
(`TestUpdateAccountSettings_RoundTrip`, `TestUpdateStreamWarmThroughput_RoundTrip`) that assert on decoded
response fields, not just status. Five more tests only used
`UpdateAccountSettingsInput.OnDemandStreamCountLimit` as a setup mechanism for the real
`checkOnDemandLimit` behavior in `streams_test.go`/`persistence_test.go`/`persistence_roundtrip_test.go`;
converted to `SetOnDemandStreamCountLimit`, preserving their actual assertions unchanged. Four
`UpdateMaxRecordSize` call sites in `records_get_test.go` used the old `MaxRecordSizeBytes` field name and
(for one) a `StreamName` the real op doesn't accept; converted to `MaxRecordSizeInKiB` plus
`StreamARN` resolved via a new `mustStreamARN` test helper. New round-trip tests
(`TestUpdateAccountSettings_RoundTrip`, `TestUpdateMaxRecordSize_RoundTrip`,
`TestUpdateStreamWarmThroughput_RoundTrip`, `TestDescribeLimits_OnDemandStreamCount_RoundTrip`) were each
verified to fail against the pre-fix code by hand-reverting the relevant wire-field rename/addition and
re-running just that test, then restoring the fix.

### KMS KeyId validation (this pass: closed the KMSAccessDeniedException gap for real, minus the truly undeliverable part)

The previous audit's `gaps:` entry claimed "there is no KMS-specific exception in the Kinesis API
itself" for `StartStreamEncryption`. This was wrong: `aws-sdk-go-v2/service/kinesis`'s
`deserializers.go` (`awsAwsjson11_deserializeOpErrorStartStreamEncryption`) explicitly lists
`KMSAccessDeniedException`, `KMSDisabledException`, `KMSInvalidStateException`, `KMSNotFoundException`,
`KMSOptInRequired`, and `KMSThrottlingException` as modeled errors for this op, and
`types/errors.go` defines all of them with `ErrorFault() smithy.FaultClient` (400-class). The op was
also missing `KeyId`'s required-field validation entirely -- any string, including empty, was accepted.

Fixed in two layers:

1. **Format validation** (`validateKMSKeyIDFormat` in `stream_encryption.go`, always active): `KeyId`
   must match one of the four shapes the SDK's own doc comment enumerates -- a bare key UUID, a key
   ARN (`arn:*:kms:*:*:key/<uuid>`), an alias ARN (`arn:*:kms:*:*:alias/<name>`), or an alias name
   (`alias/<name>`, including the Kinesis-owned `alias/aws/kinesis`). A malformed or empty `KeyId`
   returns `InvalidArgumentException`. This applies to both `StartStreamEncryption` and
   `StopStreamEncryption` (the SDK requires `KeyId` on both, even though stopping encryption never
   needs to resolve it).
2. **Optional cross-service existence/state check** (`KMSKeyValidator` interface + `WithKMSValidator`
   setter, mirroring `services/ssm`'s `KMSEncryptor`/`WithKMS` pattern exactly): when wired --
   `cli.go`'s `wireKinesisKMS` does this in production, adapting the real `services/kms` backend's
   `DescribeKey` -- `StartStreamEncryption` additionally verifies the key exists and is `Enabled`,
   returning `KMSNotFoundException` (key doesn't exist), `KMSDisabledException` (key is `Disabled`), or
   `KMSInvalidStateException` (any other non-`Enabled` state, e.g. `PendingDeletion`/`PendingImport`).
   With no validator wired (e.g. a bare `kinesis.NewInMemoryBackend()` in a unit test), only the format
   check applies -- a well-formed but nonexistent key is accepted, identical to pre-existing permissive
   behavior, so no existing caller regresses.

`KMSAccessDeniedException` remains genuinely undeliverable: it requires evaluating a KMS key policy or
grant against a calling principal, and gopherstack has no IAM policy evaluation engine to produce that
decision from anywhere in the codebase, not just kinesis. The sentinel and wire mapping are defined (so
the error type string is correct if a future IAM engine ever wires into it), but nothing can trigger it
today -- see `gaps`.

### UpdateStreamMode PROVISIONED -> ON_DEMAND auto-reshard (this pass: closed for real, with a documented approximation)

AWS's docs for `UpdateStreamMode` state that switching to on-demand "automatically scales your data
stream to handle up to double the maximum throughput ... or up to double the peak throughput within
the last 30 days, whichever is higher." This emulator tracks no throughput history, so an exact
implementation isn't possible without inventing one. Instead, `UpdateStreamMode` now reuses the same
`reshardTo` helper `UpdateShardCount` uses (extracted from it this pass) to reshard a stream up to
`defaultOnDemandShardCount` (4 -- the same floor `CreateStream` gives a fresh `ON_DEMAND` stream)
whenever the transitioning stream is currently below it, closing the old open shards (retained CLOSED
for lineage, exactly like `UpdateShardCount`/`MergeShards`/`SplitShard` do) and opening new ones
spanning the full hash range. A stream already at or above the floor is left alone. The reverse
transition (`ON_DEMAND -> PROVISIONED`) still never reshards, matching AWS (the current auto-scaled
shard count becomes the new provisioned baseline). See
`TestUpdateStreamMode_OnDemandTransitionReshardsUpToFloor` and
`TestUpdateStreamMode_OnDemandToProvisionedKeepsShardCount`.

### GetShardIterator / SubscribeToShard AT_TIMESTAMP now requires Timestamp (this pass: closed for real)

Both ops silently treated an *omitted* `Timestamp` the same as an *explicit* epoch-zero `Timestamp`
(both decoded to the Go zero value), reading from position 0 either way. The wire types for both
(`jsonGetShardIteratorReq.Timestamp`, `jsonStartingPosition.Timestamp`) are now `*float64` instead of
`float64`, so JSON-field-absent (nil) is distinguished from an explicit `"Timestamp": 0` (non-nil,
pointing at 0.0) -- the existing `TestGetShardIteratorAtTimestamp` test, which explicitly sends
`"Timestamp": 0` and expects success, continues to pass unchanged, while a genuinely omitted
`Timestamp` on `AT_TIMESTAMP` now returns `InvalidArgumentException` from both the backend
(`GetShardIteratorInput.Timestamp`/`SubscribeToShardInput.StartingPosition.Timestamp` are now
`*time.Time`) and the HTTP layer. See `TestGetShardIterator_AtTimestampRequiresTimestamp`,
`TestGetShardIterator_AtTimestampNilRejectedAtBackend`, `TestSubscribeToShard_AtTimestampRequiresTimestamp`.

### ListShards ShardFilter: deleted an invented type, implemented the real ones for real (this pass)

Two separate problems, found while field-diffing `ListShards` against
`aws-sdk-go-v2/service/kinesis/types.ShardFilterType`'s real enum
(`AFTER_SHARD_ID`/`AT_TRIM_HORIZON`/`FROM_TRIM_HORIZON`/`AT_LATEST`/`AT_TIMESTAMP`/`FROM_TIMESTAMP`):

1. **Invented op**: the backend special-cased a `"AT_SHARD_ID"` filter type that does not exist in the
   real SDK, with behavior (`listShardsAtShardID`: return every shard whose ID *or*
   `ParentShardID`/`AdjacentParentShardID` equals the target) that doesn't correspond to any real
   `ShardFilterType`'s documented semantics either. No test exercised it. Deleted per the no-invented-ops
   rule, replaced with a real implementation of `AFTER_SHARD_ID` (exclusive-start cursor over *all*
   shards, open and closed -- unlike the default/`AT_LATEST` filter, which only ever considers open
   shards).
2. **Approximated filters**: `AT_TRIM_HORIZON`/`AT_TIMESTAMP`/`FROM_TIMESTAMP` were previously
   approximated as "include every shard, open and closed" (`shardFilterIncludesAll`), which is not what
   any of them mean -- `AT_TIMESTAMP`/`AT_TRIM_HORIZON` should return only shards *open at* a given
   instant, and `FROM_TIMESTAMP` only open shards plus closed shards whose end postdates the instant.
   `Shard` gained `StartedAt`/`ClosedAt time.Time` fields (set at every shard-creation site --
   `buildInitialShards`, `SplitShard`, `MergeShards`, `reshardTo` -- and by a new `closeShard` helper
   that keeps `Closed`/`ClosedAt` in sync everywhere a shard retires), and `resolveShardFilter` in
   `shards.go` now implements real per-shard timestamp predicates (`shardOpenAt`/`shardClosedAtOrAfter`)
   against them. `AT_TIMESTAMP`/`FROM_TIMESTAMP` now require `ShardFilterTimestamp`
   (`InvalidArgumentException` if omitted, using the same `*float64`-presence-detection fix as
   `GetShardIterator`). `AT_TRIM_HORIZON`'s trim-horizon instant is clamped to never predate the
   stream's own oldest shard (see `trimHorizon`), so a freshly created stream doesn't spuriously return
   an empty result just because its retention window is mathematically older than the stream itself --
   see `gaps` for the residual approximation this clamp represents.
   Legacy Go-level callers that set the plain `ShardFilter` string field (not `ShardFilterType`, which
   only the HTTP handler populates) continue to work unchanged -- `resolveShardFilter` falls back to
   `ShardFilter` when `ShardFilterType` is empty.
   See `TestListShards_ShardFilterType_AfterShardID`, `_AtTimestamp`, `_FromTimestamp`,
   `_AtTrimHorizon`, `_TimestampRequired`, `_UnrecognizedRejected`.

### Retention-period equality bug — reverted after breaking real terraform apply (CI: TestTerraform_Kinesis)

A previous pass (`2b2086c9`) changed `IncreaseStreamRetentionPeriod`/`DecreaseStreamRetentionPeriod`
from treating an equal `RetentionPeriodHours` as an idempotent no-op (200 OK) to rejecting it with
`InvalidArgumentException`, reasoning from the literal aws-sdk-go-v2 doc comments:
`IncreaseStreamRetentionPeriodInput.RetentionPeriodHours` "Must be more than the current retention
period" and `DecreaseStreamRetentionPeriodInput.RetentionPeriodHours` "Must be less than the current
retention period" — both strict inequalities on their face.

That change broke `TestTerraform_Kinesis` in CI: `aws_kinesis_stream` with `retention_period = 48`
started failing at `apply` with `IncreaseStreamRetentionPeriod, 400, InvalidArgumentException`.
Reproduced live against a running gopherstack instance with the real `aws-sdk-go-v2/service/kinesis`
client: `CreateStream` (shard count 1) → stream ACTIVE at the 24h default → first
`IncreaseStreamRetentionPeriod(48)` succeeds → **a second `IncreaseStreamRetentionPeriod(48)` issued
against the now-already-48h stream (mimicking the OpenTofu/Terraform AWS provider's create-then-set /
idempotent-reapply flow) returns `InvalidArgumentException`** even though the stream is already at the
requested value. Real AWS tolerates this — the terraform provider relies on the API being idempotent
for a no-drift re-apply, and a strict reading of the doc comment that rejects the equal case does not
survive contact with the provider's actual call pattern.

Reverted both backend methods to treat an equal `RetentionPeriodHours` as a no-op success again, while
keeping strict rejection of the wrong direction (lower value on Increase, higher value on Decrease) and
the min/max bounds (24h floor / 8760h ceiling, unaffected). Updated the three tests that `2b2086c9` had
changed to assert the strict-rejection behavior
(`backend_test.go`'s `increase_same_value_rejected`/`decrease_same_value_rejected` table cases, and
`handler_refinement3_test.go`'s `TestRefinement3_RetentionPeriod_IncreaseToSameValueRejected`) back to
asserting the no-op-success behavior, and added
`TestRefinement3_RetentionPeriod_IncreaseFromDefaultEqualsDefault` to cover the exact default-24h
create-then-set-24h terraform pattern.

### Resource-policy persistence gap correction (documentation-only)

The previous ledger's `gaps:` list claimed `PutResourcePolicy`/`GetResourcePolicy`/
`DeleteResourcePolicy` state was not part of `backendSnapshot` and was lost across a persistence
restart. This was stale/incorrect: `persistence.go`'s `backendSnapshot.ResourcePolicies` field is
already wired into both `Snapshot` and `Restore`, and
`TestInMemoryBackend_FullStateSnapshotRestoreRoundTrip` already exercises `PutResourcePolicy` through
an actual snapshot→restore cycle and passes. No backend/handler code changed for this item — the gap
entry has been corrected to reflect reality.

### Tag persistence bug (fixed in a prior pass)

Before this pass, `Handler` kept a **second, parallel tag store** (`h.tags map[string]*svcTags.Tags`,
keyed by `region+"/"+streamName`) that was the *only* backing store for tags applied via
`CreateStream` (inline `Tags`), `AddTagsToStream`, `RemoveTagsFromStream`, `ListTagsForStream`, and
`ListTagsForResource`. The backend's own `stream.Tags` field — the one that actually participates in
`Snapshot`/`Restore` (`backendSnapshot.Streams[region][name].Tags` via the `Stream` struct's `json:"tags"`
tag) — was only ever written by `TagResource`/`UntagResource` (the ARN-based API), and the handler's
`ListTagsForResource` never read it (it read `h.tags` too, so this went unnoticed operationally). Net
effect: **every tag applied through the legacy AddTagsToStream/CreateStream API path silently vanished
on process restart**, even though the stream itself, its shards, and its records persisted correctly —
a textbook "persist when persistence is enabled" violation per the no-stub rule, made worse by two
existing tests (`TestRefinement1_ListTagsForResource_SortedOutput`,
`TestRefinement2_ListTagsForResource_UsesHandlerTags`) that had *rationalized the bug as intentional
design* rather than flagging it (the exact "looks-wrong-but-correct" trap the parity playbook warns
about, except here the previous audit got the call wrong).

Fix: every tag-mutating handler now writes through `Backend.TagResource`/`Backend.UntagResource`
(single source of truth = `stream.Tags`), and every tag-reading handler reads through
`Backend.ListTagsForResource`. The handler-local `h.tags`/`tagsMu` map, `tagKey`/`setTags`/`getTags`/
`removeTags`, and the `OnStreamPurged` tag-cleanup closure in `WithJanitor` were deleted entirely (dead
weight once the backend is the only store — `stream.Tags.Close()` is already called by the backend on
`DeleteStream`/`Purge`). `CreateStream` now validates inline `Tags` (length + 50-tag cap) *before*
creating the stream, matching AWS's all-or-nothing semantics, and `TagResource` now enforces the same
50-tag cap `AddTagsToStream` always enforced (previously uncapped). See
`TestRefinement2_Tags_SurvivePersistenceRestore` for the regression test that exercises all three
write paths (CreateStream / AddTagsToStream / TagResource) through an actual Snapshot→Restore cycle.

### PutRecords request-level vs. per-record errors

AWS's `PutRecords` contract distinguishes **request-level** failures (fail the whole call with a
single top-level exception, HTTP 4xx, no `Records` envelope) from **per-record** failures (200 OK,
each failed entry gets its own `ErrorCode`/`ErrorMessage`, `FailedRecordCount` > 0). Stream-not-found
and an empty `Records` list are request-level. Before this pass, the backend looped
`PutRecord` per entry with no upfront existence check, so a `PutRecords` call against a nonexistent
stream returned **200 OK** with every entry marked `"InternalFailure"` — wrong on three counts (wrong
HTTP status class, wrong error code, wrong response shape). Fixed by resolving the stream once before
the loop and returning `ErrStreamNotFound` at the top level; an empty `Records` slice is now rejected
the same way (AWS's SDK model has `MinItems: 1`).

### ON_DEMAND default shard count

AWS allocates **4 shards** to a freshly created `ON_DEMAND` stream (capacity is auto-managed
thereafter); a caller-supplied `ShardCount` is ignored for `ON_DEMAND`. The backend previously fell
through to `defaultShardCount = 1` for `ON_DEMAND` streams with no explicit `ShardCount`, which is
wrong for any test/tool that inspects shard count immediately after creating an on-demand stream (e.g.
to compute expected parallelism). `streamMode` is now resolved *before* `shardCount`, and `ON_DEMAND`
always gets `defaultOnDemandShardCount = 4` regardless of the caller's `ShardCount`.

### DescribeStream shard pagination

`DescribeStream`'s `Shards` list has an AWS-documented page contract: default 100, max 10000,
resumed via `ExclusiveStartShardId`, with `HasMoreShards` signaling truncation. The previous
implementation returned **every** shard in the stream in one response and hardcoded
`HasMoreShards: false` — invisible on a fresh stream (≤100 shards from `CreateStream`, capped by
`maxShardsPerStream`), but real once a stream has been resharded enough times: `MergeShards`/
`SplitShard`/`UpdateShardCount` never remove CLOSED shards from `stream.Shards` (correctly — AWS keeps
closed shards visible for lineage), so a long-lived, heavily-resharded stream's total shard count
(open + closed) is unbounded and can exceed one page. `DescribeStreamInput` gained
`ExclusiveStartShardID`/`Limit` fields (additive; every existing call site that only sets `StreamName`
is unaffected) and `DescribeStreamOutput` gained `HasMoreShards`.

### Shard hash-range and sequence-number traps (unchanged this pass, re-confirmed correct)

- **Hash key range**: the full space is `[0, 2^128-1]`. `shardForHashKey` matches a partition key's
  MD5-derived `big.Int` against `[HashKeyRangeStart, HashKeyRangeEnd]` inclusive on both ends; the
  fallback to "first open shard" (and then `shards[0]` even if closed) only fires if no shard's stored
  range covers the hash, which should not happen for internally-generated shards but protects against
  a corrupted/hand-seeded stream (see `AddStreamInternal`, test-only) from panicking on `nil`.
- **Sequence numbers** are per-shard monotonic (`Shard.NextSeq`), formatted as
  `49<14-digit ms timestamp><4-digit shard idx><20-digit seq>` — this is a plausible-shaped AWS
  sequence number (49-prefix) but is **not** globally comparable across shards the way real AWS
  sequence numbers are within a single call; `findSequencePosition` binary-searches assuming
  ascending order *within one shard's own record list*, which holds because records are always
  appended in arrival order — do not assume cross-shard ordering from the string value.
  `EndingSequenceNumber` is only populated once a shard is `Closed` — an open shard with records
  reports `SequenceNumberRangeEnd: ""` deliberately (real AWS/KCL treats the *presence* of
  `EndingSequenceNumber` as the "this shard is closed, move to children" signal; populating it on an
  open shard would make consumers abandon it prematurely). This is intentional, not a gap.
- **Reshard lineage**: `MergeShards` accepts either shard as "first" as long as they are hash-range
  adjacent (`s1.end+1 == s2.start` or `s2.end+1 == s1.start`); `SplitShard` requires
  `NewStartingHashKey` strictly inside `(shard.start, shard.end)` (not equal to either bound).
  `UpdateShardCount`'s `findOverlappingParents` assigns up to 2 parent IDs per new shard based on hash
  range overlap with the previously-open shard set — this can only ever find 0, 1, or 2 overlapping
  parents given the reshard math, matching AWS's parent/adjacent-parent model.

### Consumer registration limit

`RegisterStreamConsumer` had no upper bound; AWS caps enhanced fan-out consumers at 20 per stream
(`LimitExceededException` beyond that). Added the check; see
`TestAudit2_RegisterStreamConsumer_LimitExceeded`.

### Account settings persistence

`UpdateAccountSettings`'s `OnDemandStreamCountLimit` was backend in-memory state not included in
`backendSnapshot` — every restart silently reset the account's on-demand limit back to the compiled-in
default (10), even if an operator had explicitly raised or lowered it. Added to the snapshot.

### KMS error codes (deferred, not fabricated)

The task brief calls out `KMSAccessDenied` as an error code to verify. The real Kinesis API (per the
`aws-sdk-go-v2/service/kinesis` model) does not define a KMS-specific exception for
`StartStreamEncryption`/`StopStreamEncryption`/`UpdateMaxRecordSize` — the modeled exceptions are
`InvalidArgumentException`, `LimitExceededException`, `ResourceInUseException`,
`ResourceNotFoundException`, and a generic `AccessDeniedException` (not currently in this package's
error set at all). Actually validating a `KeyId` would require calling into the `kms` service's
backend, which is a cross-service dependency out of scope for a `services/kinesis/`-only pass per this
sweep's constraints. Fabricating a KMS validation error path not backed by real state would itself be
a stub, so this is left as a documented gap rather than implemented.

### structfielddiff pass (gopherstack-enpq, 2026-08-22): re-audited from scratch, found a real StreamARN family and a consumer-tagging gap the prior "SETTLED" note missed

This issue's own notes claimed kinesis was SETTLED after a prior pass (structfielddiff pass 3,
2026-08-14) that reported "only 4 of 39 ops had a real candidate field miss" and fixed 2
(`DeleteStream.EnforceConsumerDeletion`, `GetRecords.ChildShards`/`NextShardIterator`). Per this
session's explicit instruction not to trust that ledger, this pass re-ran
`go run ./cmd/structfielddiff -service kinesis` against the pinned
`aws-sdk-go-v2/service/kinesis@v1.46.4` (confirmed via `go.mod` -- `kinesis` is not in any
`dirModuleOverride` table, so the directory name IS the module name; siblings
`kinesisanalytics`/`kinesisanalyticsv2`/`kinesisvideo`/`firehose` are separate modules and were not
touched) and hand-verified every one of the 39 ops' request/response fields against gopherstack's
actual wire structs (not just the two the prior pass's table showed as fixed), reading each op's
`api_op_*.go` doc comment for its required-identifier contract as well as its struct fields.

**9 ops had no `StreamARN` Go field at all** -- `AddTagsToStream`, `RemoveTagsFromStream`,
`ListTagsForStream`, `IncreaseStreamRetentionPeriod`, `DecreaseStreamRetentionPeriod`, `ListShards`,
`EnableEnhancedMonitoring`, `DisableEnhancedMonitoring`, `UpdateShardCount`. Every one of these ops'
own doc comment states, verbatim: "When invoking this API, you must use either the StreamARN or the
StreamName parameter, or both... It is recommended that you use the StreamARN input parameter." A
real client identifying the stream by ARN alone (the AWS-recommended pattern) silently resolved to an
empty stream name in gopherstack and failed with `ResourceNotFoundException` even though the ARN was
valid -- structurally absent field, genuinely reachable (this is the documented preferred calling
convention, not an edge case). Fixed via a new `resolveStreamNameAndRegion` helper in `store.go`
(mirrors the ARN-resolution pattern already used at the backend layer by
`MergeShards`/`SplitShard`/`StartStreamEncryption`/`UpdateStreamMode`, lifted to the handler layer for
these 9 ops whose domain Input types have no StreamARN member): each handler now resolves the target
stream name and per-request region from either identifier before calling the backend. `ListShards`
additionally needed `StreamARN` added to its existing NextToken-mutual-exclusion validation (AWS
rejects `NextToken` combined with `StreamARN` the same as with `StreamName`). Proven via
`TestStreamIdentifiedByARNOnly` (`wire_field_fixes_test.go`), one subtest per op, each driving the
real `aws-sdk-go-v2` client with `StreamARN` set and `StreamName` omitted entirely -- confirmed to
fail with `ResourceNotFoundException` against the unfixed code (hand-reverted: `store.go`,
`handler_tags.go`, `handler_stream_retention.go`, `handler_shards.go`, `handler_monitoring.go`,
`handler_resharding.go` restored from `git show HEAD:<path>`, all 9 subtests failed, then restored
byte-identical via md5sum).

**`RegisterStreamConsumer.Tags`** (real, optional member -- its own doc comment: "You can add tags to
the registered consumer when making a RegisterStreamConsumer request by setting the Tags parameter...
Tags will take effect from the CREATING status of the consumer") had no Go field at all and was
silently dropped. Worse, `ListTagsForResource`/`TagResource`/`UntagResource` (all three real,
resource-agnostic per their own doc comments -- "the specified Kinesis resource") only ever resolved
a *stream* ARN (`streamNameFromARN` unconditionally, which mis-parses a consumer ARN's
`stream/{s}/consumer/{c}` resource segment as a literal stream name that can never match), so even a
caller using `TagResource` directly against a consumer's own ARN had no way to tag it. Fixed:
`Consumer` gained a `Tags map[string]string` field (purely additive to the JSON shape `Stream`/
`Consumer` are persisted under directly -- confirmed against `persistence.go`'s
`kinesisSnapshotVersion` doc comment, no version bump needed, same class as `gopherstack-hjdd`'s
precedent for additive-only persisted-model changes); `RegisterStreamConsumer` now validates
(`validateTagKVs`, 50-tag cap) and stores the caller's `Tags`; `ListTagsForResource`/`TagResource`/
`UntagResource` now detect a consumer ARN via the existing `consumerInfoFromARN` helper and route to
new `listConsumerTags`/`tagConsumer`/`untagConsumer` backend methods that read/write `Consumer.Tags`
under the same `stream.mu` the consumer map itself is already guarded by (no new lock or goroutine).
Proven via `TestRegisterStreamConsumer_TagsRoundTrip`: registers a consumer with `Tags`, confirms
`ListTagsForResource` against the *consumer's* ARN sees them, then drives `TagResource`/
`UntagResource` against the same consumer ARN. Hand-reverted (`models.go`, `consumers.go`,
`handler_consumers.go`, `tags.go` restored from HEAD; also had to revert `streams.go`/
`handler_streams.go` alongside to keep the tree compiling, since `models.go` is shared with the
DescribeStreamSummary fix below) -- failed with `ResourceNotFoundException` on `ListTagsForResource`
against the unfixed code, then restored byte-identical.

**`DescribeStreamSummary`** was missing `MaxRecordSizeInKiB` and `WarmThroughput`, both real optional
`StreamDescriptionSummary` members (`types/types.go`). The backend already tracks the underlying
values (`Stream.MaxRecordSizeBytes`/`WarmThroughputMiBps`, set by `UpdateMaxRecordSize`/
`UpdateStreamWarmThroughput`) but never surfaced either back through `DescribeStream`/
`DescribeStreamSummary` -- `DescribeStreamOutput` (the shared domain type) carried neither field, so
a client had no way to read back settings it had itself just applied. (`DescribeStream`'s own
`StreamDescription` shape genuinely has neither field in the real SDK, confirmed via
`structfielddiff` -- only the Summary variant does, so `DescribeStream` itself needed no change.)
Fixed by adding both to `DescribeStreamOutput` and wiring `handleDescribeStreamSummary` to convert
bytes to KiB and echo `WarmThroughput.Current`/`Target` both equal to the stored value, consistent
with `UpdateStreamWarmThroughput`'s already-documented synchronous-apply model (no `UPDATING`
transient state in this backend). Proven via
`TestDescribeStreamSummary_MaxRecordSizeAndWarmThroughput`: asserts the 1024 KiB default is reported
even with no prior `UpdateMaxRecordSize` call, then that both fields reflect a subsequent
`UpdateMaxRecordSize`/`UpdateStreamWarmThroughput` call. Hand-reverted (`streams.go`,
`handler_streams.go` restored from HEAD) -- all three assertions failed against the unfixed code
(reported `0` instead of `1024`/`2048`, and a nil `WarmThroughput`), then restored byte-identical.

**Rejected candidates, not counted:** `GetRecordsInput` genuinely has no `StreamName` member in the
real SDK (only `StreamARN`/`StreamId`, both optional) despite carrying the same "StreamARN or
StreamName" doc-comment boilerplate as its siblings -- `ShardIterator` alone already fully
disambiguates the target in both the real API and this backend, so an unread `StreamARN` here is
more-permissive-not-less (a real client's optional cross-check is simply skipped), not a functional
gap; left as-is. `PutRecordOutput.EncryptionType` (non-pointer `types.EncryptionType`) was
already correctly populated and is a non-pointer enum in any case (not provable per this campaign's
own rule). `RegisterStreamConsumerInput.StreamId`/every other op's `StreamId` sibling field remain
unmodeled: all are the SDK's own "Not Implemented. Reserved for future use." fields, the same noise
class already documented for this service.

**No existing test ratified either defect this pass** -- both were previously untested gaps (no test
exercised `StreamARN`-only calling or a consumer-ARN tag round trip), not asserted-wrong behavior.

All three fix groups: full gate suite green (`go build`, `go vet`, `gofmt -l` clean, `go test -race`,
`golangci-lint run` 0 findings / 0 nolints added after fixing 1 `err113` + 3 `modernize` (mapsloop) +
1 `golines` finding by refactoring, `go fix -diff` no diff, `make build-check` clean). Working tree
left uncommitted per this session's constraints.
