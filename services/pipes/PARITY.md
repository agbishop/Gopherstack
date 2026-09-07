---
service: pipes
sdk_module: aws-sdk-go-v2/service/pipes@v1.26.4
last_audit_commit: 7f68d2d24
last_audit_date: 2026-08-23
overall: A            # both execution gaps closed for real (runner.go source pollers + cli.go target/DLQ wiring); the only remaining gap is a proven genuine impossibility (no in-repo Kafka/AMQP broker)
ops:
  CreatePipe: {wire: ok, errors: ok, state: ok, persist: ok, note: "added max-50-tags validation to match TagResource's existing limit; RoleArn is now enforced as a required field (ValidationException when absent/empty), matching validateOpCreatePipeInput -- closes the gap previously left open in the 2026-07-13 pass. ~40 call sites across the test suite (Go CreatePipeInput{} literals and raw-HTTP JSON bodies) updated to supply RoleArn now that it's enforced. 2026-08-21: KinesisStreamSourceParameters.StartingPositionTimestamp (a Kinesis-source-only filter) decoded straight into *time.Time, which encoding/json cannot unmarshal from the epoch-seconds JSON number restjson1 actually sends -- rejecting the entire request body for any real client setting it (gopherstack-5mr2). Fixed via wire_time.go's MarshalJSON/UnmarshalJSON pair, not a field-type change, since the same struct also serves DescribePipe's response and the persistence snapshot round trip. FIXED 2026-08-21 (gopherstack-us9u kind-mismatch sweep) -- BatchContainerOverrides.Environment was map[string]string; the real types.BatchContainerOverrides.Environment is []BatchEnvironmentVariable ({Name, Value} objects), and serializers.go/deserializers.go reuse the identical type for both CreatePipe's request and DescribePipe's response, so a real client setting a Batch environment variable override failed CreatePipe's request decode outright (json: cannot unmarshal array into ... of type map[string]string). Fixed by changing the field's Go type directly to []BatchEnvironmentVariable (no domain/wire split needed, since both directions share one struct). Proven via a real aws-sdk-go-v2/service/pipes client round trip (wire_batch_environment_test.go), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical. FIXED (gopherstack-101r): RuntimeMetricsStreaming (request, response, and the Pipe model) was a wholly invented concept -- absent from CreatePipeInput/UpdatePipeInput/types.Pipe in the real SDK, no such feature exists anywhere in EventBridge Pipes. Removed entirely (models.go/handler.go/pipe_lifecycle.go), including the two raw-body tests that asserted it round-tripped (TestRuntimeMetricsStreaming_Create/Update, pipe_lifecycle_test.go) -- replaced by wire_field_fixes_test.go's TestCreateUpdatePipe_RuntimeMetricsStreamingNotAccepted, which asserts the key is gone. FIXED (gopherstack-gcjw): added nested source union required-field validation matching validatePipeSourceParameters' per-type validators in aws-sdk-go-v2 pipes validators.go -- ActiveMQ/RabbitMQ Credentials+QueueName, MSK/SelfManagedKafka TopicName (validateSourceRequiredFields, sources.go), alongside the pre-existing Kinesis/DynamoDB StartingPosition check (validateSourceStartingPosition). Also added target union fields beyond the pre-existing KinesisStreamParameters.PartitionKey: EcsTaskParameters.TaskDefinitionArn, BatchJobParameters.JobDefinition/JobName, RedshiftDataParameters.Database/Sqls, SageMakerPipelineParameters.PipelineParameterList[].Name/Value, and TimestreamParameters.TimeValue/VersionValue/DimensionMappings (plus each DimensionMappings entry's DimensionName/DimensionValue/DimensionValueType) -- validateTargetRequiredFields, targets.go. FOLLOW-UP (gopherstack-gcjw, 2026-09-06): added TimestreamParameters.SingleMeasureMappings per-entry MeasureName/MeasureValue/MeasureValueType and MultiMeasureMappings per-entry MultiMeasureName/MultiMeasureAttributeMappings (plus each attribute mapping's MeasureValue/MeasureValueType/MultiMeasureAttributeName) -- validateSingleMeasureMappingsRequiredFields/validateMultiMeasureMappingsRequiredFields/validateMultiMeasureAttrMappingsRequiredFields, targets.go. Added ECS NetworkConfiguration.AwsvpcConfiguration.Subnets (required when AwsvpcConfiguration is set) and CapacityProviderStrategy[].CapacityProvider per entry -- validateECSTargetRequiredFields, targets.go. Added PipeLogConfigurationParameters.Level, required whenever LogConfiguration is set -- validateLogConfigurationRequiredFields, pipe_lifecycle.go. FIXED (gopherstack-iyc2, 2026-09-06): the ECS/Batch override wire shapes that d4e234022 silently removed (see the addendum after Bug 3) are restored -- EcsEnvironmentVariable, EcsEnvironmentFile, EcsResourceRequirement, EcsContainerOverride, EcsEphemeralStorage, EcsInferenceAcceleratorOverride, and EcsTaskOverride's ContainerOverrides/EphemeralStorage/InferenceAcceleratorOverrides fields; BatchResourceRequirement and BatchContainerOverrides.ResourceRequirements; ECSTaskTargetParameters' PropagateTags/ReferenceId/Tags (as a new shared Tag type, matching real types.Tag -- the prior prose's proposed name EcsTag was never actually landed). Re-derived from the pinned pipes@v1.26.4 SDK directly rather than restored verbatim from the old diff, because the old diff had five wire-casing bugs the SDK's own serializers.go/deserializers.go contradict: EcsResourceRequirement's Type/Value, EcsEnvironmentFile's Type/Value, EcsEnvironmentVariable's Name/Value, and EcsEphemeralStorage's SizeInGiB all serialize lowercase (type/value, type/value, name/value, sizeInGiB) -- Pipes passes ECS's own RunTask-override casing straight through for these leaf shapes rather than using its usual PascalCase -- and EcsInferenceAcceleratorOverride's DeviceName/DeviceType likewise serialize lowercase (deviceName/deviceType). The old diff had all of these capitalized, which would have silently dropped every one of these fields on both request decode and response encode for any real client. Now attached: the nested required-field validation this gap had blocked -- validateEcsTaskOverrideRequiredFields (EnvironmentFiles/ResourceRequirements Type/Value required per ContainerOverrides entry, EphemeralStorage.SizeInGiB required when set, matching validators.go:505-525's validateEcsTaskOverride) and validateBatchContainerOverridesRequiredFields (ResourceRequirements Type/Value required per entry, matching validators.go:244-259's validateBatchContainerOverrides). Round-trip proof: targets_ecs_batch_test.go's TestECS_ContainerOverrides_RoundTrip/TestECS_EphemeralStorage_RoundTrip/TestECS_InferenceAcceleratorOverrides_RoundTrip/TestECS_PropagateTagsReferenceIdTags_RoundTrip/TestBatch_ResourceRequirements_RoundTrip, hand-reverted against HEAD (pre-fix, which has none of these types) and confirmed failing, then restored byte-identical. Validation proof: targets_required_fields_test.go's TestTargetEcsTaskOverrideRequiredFields/TestTargetBatchContainerOverridesRequiredFields, hand-neutered (validation call sites replaced with `return nil`, keeping the types so the package still compiles) and confirmed every rejection subtest wrongly passes, then restored byte-identical. TestSnapshotVersionGuard now reports pipes' backendSnapshot changed without a version bump for this additive-only field set -- left to a human decision per gopherstack-5i6p's guidance, not bumped or -update'd here."}
  DescribePipe: {wire: ok, errors: ok, state: ok, persist: ok, note: "DesiredState now reports DELETED while CurrentState=DELETING, per RequestedPipeStateDescribeResponse. 2026-08-21: StartingPositionTimestamp response encoding fixed by the same wire_time.go change as CreatePipe (see its note) -- it was previously emitted as an RFC3339 string, which the real client's deserializer (expecting the epoch-seconds number restjson1's own serializer always used on the request side) would have rejected. FIXED 2026-08-21 (gopherstack-us9u) -- BatchContainerOverrides.Environment fixed to []BatchEnvironmentVariable; see CreatePipe's note (same shared type, same fix). FIXED (gopherstack-101r): RuntimeMetricsStreaming removed from the response (pipeResponse/toPipeResponse); see CreatePipe's note."}
  UpdatePipe: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added ConflictException guard against updating a pipe that is DELETING (was silently resurrecting it, corrupting the pending async delete); RoleArn is now enforced as a required field on every UpdatePipe call (ValidationException when absent/empty), matching validateOpUpdatePipeInput -- real AWS requires RoleArn to be resupplied on every update, even when unchanged. Validation order is Name/DesiredState/SourceParameters-batch-size -> RoleArn -> pipe-lookup, so a request missing RoleArn against a nonexistent pipe now correctly surfaces ValidationException, not NotFoundException (adjusted TestErrors/update_nonexistent_pipe_returns_404 to supply a valid RoleArn so it still exercises the NotFound path specifically). Note: types.UpdatePipeSourceKinesisStreamParameters has no StartingPositionTimestamp member in the real SDK at all, so this field is not reachable via UpdatePipe by any real client regardless of the 2026-08-21 fix. write-only-state sweep (this pass): KmsKeyIdentifier was a plain string guarded by != \"\" (not *string like the real UpdatePipeInput.KmsKeyIdentifier, api_op_UpdatePipe.go), whose doc says \"To update a pipe that is using a customer managed key to use the default Amazon Web Services owned key, specify an empty string\" -- a client's documented, explicit clear was silently dropped. Now *string with a nil check (pipe_lifecycle.go). Response side (pipeResponse.KmsKeyIdentifier, handler.go) intentionally kept `json:\"KmsKeyIdentifier,omitempty\"` -- TestKmsKeyIdentifier (pipe_lifecycle_test.go) already asserts the key is absent from CreatePipe's response when no custom key is set, matching real AWS's default-owned-key omission; stripping omitempty would break that documented, correct behavior for the overwhelmingly common no-custom-key case. Round-trip test: wire_field_fixes_test.go (TestUpdatePipe_KmsKeyIdentifierCanBeCleared). FIXED (gopherstack-101r): RuntimeMetricsStreaming removed from UpdatePipeInput/applyUpdateFields; see CreatePipe's note (same wholly invented field, same fix). FIXED (gopherstack-gcjw): added validateUpdateSourceRequiredFields (sources.go), matching validateUpdatePipeSourceParameters in aws-sdk-go-v2 pipes validators.go -- ActiveMQ/RabbitMQ Credentials only. This is deliberately narrower than CreatePipe's validateSourceRequiredFields: the real SDK's UpdatePipeSourceParameters type has no QueueName, TopicName, or StartingPosition requirement at all (those fields can't be changed after creation), so UpdatePipe must NOT reject a request missing them. TargetParameters route through the same validateTargetRequiredFields as CreatePipe (see its note) -- unlike source parameters, target validation is symmetric across Create/Update in the real SDK. FOLLOW-UP (gopherstack-gcjw, 2026-09-06): validateLogConfigurationRequiredFields (LogConfiguration.Level) is likewise reached identically from CreatePipe and UpdatePipe -- validateOpUpdatePipeInput nests into validatePipeLogConfigurationParameters exactly like validateOpCreatePipeInput does, both only when LogConfiguration != nil. See CreatePipe's note for the full list of what this pass added and what remains a modeling gap."}
  DeletePipe: {wire: ok, errors: ok, state: ok, persist: ok, note: "DesiredState now reports DELETED, matching UpdatePipe fix's shared toPipeResponse"}
  ListPipes: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-3me4): sortedPipeNames (pipes.go) hand-rolled an O(n^2) bubble/selection sort over every pipe name under ListPipes' read lock; replaced with slices.Sort. Behaviour-identical (pipesTable is keyed by Name, so the input can never contain duplicates -- stability is moot); benchmarked at n=10000 names: ~305ms/op before, ~1.25ms/op after (~240x). The read lock still has to be held across the whole call regardless (collectMatchingPipes/buildNextToken also read pipesTable), so this is a pure algorithmic fix, not a locking change."}
  StartPipe: {wire: ok, errors: ok, state: ok, persist: ok}
  StopPipe: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  route_matcher: {status: ok, note: "verified every op's (method,path) against aws-sdk-go-v2/service/pipes v1.23.18 serializers.go opPath/request.Method literals; added handler_route_matcher_test.go driving RouteMatcher(c)+Handler()(c) end-to-end (prior tests all bypassed RouteMatcher via h.Handler()(c) directly, and the /tags/ prefix is shared across many services -- test also pins that a pipes-shaped path with a non-pipes SigV4 credential-scope service is correctly rejected)"}
  filter_semantics: {status: fixed, note: "2026-08-30 (gopherstack-uox6 value-semantics pass): FilterCriteria.Filters[].Pattern (types.Filter, a bare *string -- the wire type documents no grammar of its own, linking out to the EventBridge event-pattern guide) was hand-rolled in filter.go against that guide. Two documented operators, both marked 'Pipe support: Yes' on the operator table, were broken: (1) exists (true/false) was structurally unreachable -- matchesJSONPattern short-circuited to false whenever a pattern's field was absent from the message, before ever consulting the rule, so {\"exists\":false} (which must match on absence) could never match, and matchesRule had no exists case at all, so {\"exists\":true} on a *present* field also always returned false. (2) anything-but only accepted a JSON array of strings; the guide's own primary example (`\"state\": [ { \"anything-but\": \"initializing\" } ]`) is a single bare string, which failed to unmarshal into []string and fell through to an unconditional false -- excluding every message regardless of value, the opposite of anything-but's purpose. Fixed: matchesJSONPattern/fieldMatchesRule/matchesRule now thread field-presence through explicitly so exists can be evaluated before (and independent of) presence-gating everything else; matchesAnythingBut accepts both the single-value and list forms. TestFilter_PatternOperators (filter_test.go) gained 6 new table cases (2 anything-but-single-value, 4 exists true/false x present/absent), each driven end-to-end through CreatePipe+Runner+mock SQS reader/deleter, not a unit call into the matcher directly; all 6 hand-verified to fail against unfixed code first. filterKinesisRecords/filterDynamoDBRecords (sources_poll.go) both call the same matchesAnyFilter entrypoint, so the fix covers all three source types without separate changes. Not touched, disclosed as gaps below (2026-08-30): numeric matching ({\"numeric\":[...]}) and $or are both documented ('Pipe support: Yes') but entirely unimplemented -- msgStr's json.Unmarshal-into-string silently fails for any non-string message value, and $or is not special-cased as a pattern key at all, so both are structural feature absences rather than a value applied wrong (this class's scope), not fixed here. FIXED (2026-09-06, gopherstack-a2vk): matchesJSONPattern only ever compared top-level pattern fields, so a nested EventBridge-style pattern such as {\"dynamodb\":{\"NewImage\":{\"id\":{\"S\":[\"1\"]}}}} (the real DynamoDB Streams shape, and the dominant real-world Pipes filter pattern) could never match past the first level. Fixed: matchesJSONPattern/fieldMatchesRule now detect whether a field's pattern value is a nested object (isJSONObject, byte-sniffed on the first non-whitespace char rather than probed via json.Unmarshal-into-map, since unmarshalling JSON null into a map succeeds with err=nil and would otherwise misclassify a literal null array element as an object) versus an array, and recurse via matchesPatternObject/matchesNestedRule when it is one. Also closed the numeric gap disclosed above ({\"numeric\":[\">\",5]}) plus added cidr ({\"cidr\":\"10.0.0.0/24\"}), both algorithmically ported from services/eventbridge/pattern.go's matchNumeric/matchCIDR (see this file's dated Notes entry below for the reuse-vs-rewrite decision -- eventbridge's matcher is unexported and pipes/ is a separate package, so this is a parallel, independently-tested implementation of the same documented AWS semantics, not a shared import). $or, wildcard, and the nested {\"prefix\":{\"equals-ignore-case\":...}} form remain unimplemented; matchesRuleObject's default case (any matcher-object key it does not recognize) deliberately returns false rather than silently matching everything. Exact-match comparison (matchesExactRule) was also hardened from a string-only equality check with a lossy quote-stripping fallback to a type-sensitive reflect.DeepEqual over both sides' json.Unmarshal(...,&any)-decoded values (DeepEqual, not ==, because a malformed pattern or event value can decode to a non-comparable Go type -- a JSON array or object -- which would panic under ==). ListPipesInput.NamePrefix's own SDK doc comment reads 'will return all endpoints with \"ABC\" in the name' (substring, and 'endpoints' is the wrong noun for this API -- looks like a codegen doc-comment artifact copied from another service's template) but matchesFilter (pipes.go) implements prefix matching, matching the field's literal name and the universal AWS List-filter-prefix convention across the SDK; left as-is, doc comment treated as unreliable rather than followed literally, consistent with the PatchOrchestratorFilter lesson (read the operation's own type/behavior, not a possibly-templated doc string) -- flagged here rather than silently changed."}
gaps:
  - "MSK, self-managed Kafka, RabbitMQ, and ActiveMQ pipe sources are modeled in full in CreatePipe/UpdatePipe/DescribePipe wire shapes (sources.go) but are never polled by the runner, and this is a genuine impossibility rather than a deferred implementation: gopherstack has no in-process Kafka-wire-protocol broker or AMQP/OpenWire broker anywhere in the repo to read messages from. Verified by inspecting both candidate backends before writing this line: services/kafka (Amazon MSK) implements only the AWS *control-plane* HTTP API (CreateCluster/DescribeCluster/GetBootstrapBrokers/topic metadata CRUD) -- confirmed via `grep -rl 'func.*Produce\\|func.*Consume\\|func.*SendMessage\\|func.*ReceiveMessage'` returning nothing message-plane-shaped; services/mq (Amazon MQ, backs both RabbitMQ and ActiveMQ engine types) is the same shape (broker/user/configuration lifecycle CRUD only, zero produce/consume methods anywhere in the package). Neither package speaks the real wire protocol (Kafka's binary TCP protocol; AMQP 0-9-1 for RabbitMQ; OpenWire/STOMP for ActiveMQ), so even a cluster/broker created via those services' control planes has no data-plane to poll. runner.go's pollPipe routes only SQS/Kinesis/DynamoDB-Streams ARNs and leaves these four source types unrouted (with a doc comment explaining why) rather than faking delivery."
deferred: []
leaks: {status: clean, note: "runDelayed goroutines are tracked by b.wg and tied to svcCtx (cancelled via Handler.Shutdown -> Backend.Shutdown); Runner.Start/Wait use the same wg+done-channel pattern; enrichmentCounts entries are pruned in completeDeleteTransition alongside the pipe row, so no unbounded growth. Runner.shardIterators (new: caches Kinesis/DynamoDB-Streams shard iterator tokens, keyed by pipe ARN + shard ID, since those source types have no message-level ack/delete like SQS to drive cleanup off of) is swept every poll tick in pollAllPipes: any cached key whose pipe ARN is not in the current RUNNING set is dropped, so a stopped or deleted stream-sourced pipe's iterator entries do not accumulate."}
---

## Notes

Protocol: restjson1. Timestamps (`CreationTime`/`LastModifiedTime`) are epoch
**seconds** as a JSON number with fractional milliseconds precision (verified
against `smithytime.ParseEpochSeconds` in the real SDK's deserializers.go --
NOT epoch milliseconds despite the `epochMillis` helper's name; the helper
divides `UnixMilli()` by 1000, which produces exactly the epoch-seconds-with-
millisecond-fraction shape the deserializer expects. Confusing name, correct
value -- did not rename to avoid unnecessary churn across the package).

`DesiredState` has two different wire shapes depending on the op, both
confirmed directly from `aws-sdk-go-v2/service/pipes/types/enums.go`:
- `RequestedPipeState` (`RUNNING`/`STOPPED` only) on CreatePipeOutput,
  UpdatePipeOutput, StartPipeOutput, StopPipeOutput, and the `Pipe` summary
  type used by ListPipesOutput.
- `RequestedPipeStateDescribeResponse` (`RUNNING`/`STOPPED`/`DELETED`) on
  DescribePipeOutput and DeletePipeOutput only. This pass fixed the shared
  `toPipeResponse` (used by all six single-pipe ops) to substitute `DELETED`
  for `DesiredState` whenever `CurrentState == DELETING` -- previously it
  echoed the pipe's last real desired state (RUNNING/STOPPED) even while
  being deleted. `pipeSummary` (List) intentionally does NOT get this
  substitution since its wire type has no DELETED value and a
  fully-DELETING-but-not-yet-removed pipe can only be observed transiently.

`CreatePipe`/`UpdatePipe`/`DeletePipe`/`StartPipe`/`StopPipe` all share the
full `pipeResponse` struct (same one `DescribePipe` uses), which includes
several fields (`SourceParameters`, `TargetParameters`, `DeadLetterConfig`,
`LogConfiguration`, `RuntimeMetricsStreaming`, `Tags`, `RoleArn`, `Source`,
`Description`, `Enrichment`, `KmsKeyIdentifier`) that the real
CreatePipeOutput/UpdatePipeOutput/DeletePipeOutput/StartPipeOutput/
StopPipeOutput shapes do NOT have (those five ops' real outputs are just
`Arn`/`CreationTime`/`CurrentState`/`DesiredState`/`LastModifiedTime`/`Name`).
Confirmed via the real SDK's restjson1 deserializers that unrecognized JSON
keys are silently skipped (`for key, value := range shape { switch key {
...no default case... } }`), so this is NOT a client-breaking bug -- extra
fields are ignored by every language's generated SDK. Left as-is rather than
splitting into five narrower response structs; flag if a future audit wants
stricter shape-for-shape parity, but it's cosmetic today. Also note
`DescribePipeOutput` (this SDK version, v1.23.18) has no top-level
`DeadLetterConfig` or `RuntimeMetricsStreaming` fields at all -- those only
exist nested inside `SourceParameters.{Kinesis,DynamoDBStream}Parameters` and
inside the pipe's own `RuntimeMetricsStreaming` sub-object respectively (the
latter genuinely doesn't exist as a top-level Pipe field in this SDK
version). Gopherstack's `Pipe.DeadLetterConfig`/`Pipe.RuntimeMetricsStreaming`
top-level fields are therefore emulator-only extensions with no real-API
equivalent at this SDK version; harmless (real requests never populate them,
so they're always empty on the wire) but worth knowing if diffing shapes.

Route paths (verified against `aws-sdk-go-v2/service/pipes@v1.23.18/
serializers.go` opPath literals, one `httpbinding.SplitURI` call per
`awsRestjson1_serializeOp*`):
- `POST /v1/pipes/{Name}` = CreatePipe
- `GET /v1/pipes/{Name}` = DescribePipe
- `PUT /v1/pipes/{Name}` = UpdatePipe
- `DELETE /v1/pipes/{Name}` = DeletePipe
- `GET /v1/pipes` = ListPipes
- `POST /v1/pipes/{Name}/start` = StartPipe
- `POST /v1/pipes/{Name}/stop` = StopPipe
- `GET /tags/{resourceArn}` = ListTagsForResource
- `POST /tags/{resourceArn}` = TagResource
- `DELETE /tags/{resourceArn}` = UntagResource

All ten match `Handler.ExtractOperation`/`extractPipeCRUDOp`/
`extractPipeActionOp`/`extractTagsOp` exactly -- no route-matcher bugs found.
`RouteMatcher()` additionally gates on `httputils.ExtractServiceFromRequest`
(the SigV4 credential-scope service name) before checking path prefix, which
is what prevents the shared `/tags/{resourceArn}` prefix from colliding with
every other AWS service that also serves tag ops off that path; this gate
did not previously have direct test coverage (all existing tests call
`h.Handler()(c)` directly, bypassing `RouteMatcher` entirely) -- added
`handler_route_matcher_test.go` to close that gap, following the pattern
established in `services/guardduty/handler_route_matcher_test.go`.

State machine: `PipeState` enum (`RUNNING`/`STOPPED`/`CREATING`/`UPDATING`/
`DELETING`/`STARTING`/`STOPPING`/`*_FAILED`/`*_ROLLBACK_FAILED`) matches the
real SDK's `types.PipeState.Values()` exactly. The `*_FAILED` states
(`CREATE_FAILED`, `UPDATE_FAILED`, `DELETE_FAILED`, `START_FAILED`,
`STOP_FAILED`) and `MarkPipeFailed` are defined but never triggered by any
internal code path -- every async transition (`completeCreateTransition`,
`completeUpdateTransition`, `completeDeleteTransition`,
`completeStartTransition`, `completeStopTransition`) always succeeds
optimistically after a fixed 10ms delay. This is consistent with the rest of
the emulator (no synthetic failure injection outside pkgs/chaos) and was not
treated as a bug -- `MarkPipeFailed` is exported and unit-tested
(`TestAudit_MarkPipeFailed`) for callers (e.g. a future chaos hook) that want
to force a failed state.

`StartPipe`/`StopPipe` already had a transitional-state guard
(`changePipeDesiredState`: reject if `CurrentState` matches the op's own
in-flight transitional state, or if `CurrentState == DELETING`).
`UpdatePipe` had NO such guard before this pass -- it unconditionally
overwrote `CurrentState` to `UPDATING` regardless of what state the pipe was
actually in. Concretely: `DeletePipe` sets `CurrentState = DELETING` and
schedules `completeDeleteTransition` (which only actually removes the row if
`CurrentState` is *still* `DELETING` when the delayed goroutine fires). If
`UpdatePipe` ran in that ~10ms window, it flipped `CurrentState` to
`UPDATING`, which made `completeDeleteTransition`'s guard fail silently --
the pipe was never removed, `DeletePipe`'s own response (which already
claimed `CurrentState: DELETING`) became a lie, and the pipe was
permanently stuck in `UPDATING` (since `completeUpdateTransition` would
still fire and "complete" it into a state the caller never asked for). Fixed
by rejecting `UpdatePipe` with `ConflictException` when
`CurrentState == DELETING`, mirroring the existing Start/Stop guard pattern.
`DeletePipe` itself was NOT given a symmetric guard against CREATING/
UPDATING/STARTING/STOPPING pipes -- letting a delete win over any of those
in-flight transitions is standard AWS async-resource behavior (delete is
terminal) and already correctly implemented (delete always overwrites
`CurrentState`/`DesiredState` and its own completion check is unconditional
on `CurrentState == DELETING`, so a later competing transition simply loses,
same failure mode analysis as above but in delete's favor by design).

Tag limits: `TagResource` already enforced `maxTagsPerPipe` (50, matching
real AWS), but `CreatePipe`'s initial `Tags` map was never checked against
the same limit -- a single `CreatePipe` call with >50 tags in the request
body succeeded, and the row could then only be discovered as over-limit via
`ListTagsForResource`. Added the same `len(tags) > maxTagsPerPipe` check to
`CreatePipe`, returning the same `ValidationException` shape `TagResource`
already uses.

RoleArn required-field validation (2026-07-24 pass): the prior audit
(2026-07-13) found that `CreatePipe`/`UpdatePipe` never validated `RoleArn`
as non-empty, even though `aws-sdk-go-v2/service/pipes@v1.23.18`'s
`validateOpCreatePipeInput`/`validateOpUpdatePipeInput` both mark it a
required member (confirmed directly against `validators.go`), and left it
open citing high test-churn (~340 subtests) for a raw-HTTP-only edge case.
This pass closed the gap for real: `CreatePipe` now rejects an empty
`RoleARN` with `ValidationException` (checked after `Source`/`Target`, so
those checks' error precedence is unchanged), and `UpdatePipe` now rejects
an empty `RoleARN` on *every* call -- matching real AWS, which requires
`RoleArn` to be resupplied on every `UpdatePipe` request even when its value
doesn't change (confirmed via the `// This member is required` doc comment
on `UpdatePipeInput.RoleArn` in `api_op_UpdatePipe.go`, not just the
smithy-generated client-side validator). `UpdatePipe`'s check runs before
the pipe-name lookup, matching real AWS's validate-before-execute ordering;
one test (`TestErrors/update_nonexistent_pipe_returns_404`) previously sent
a structurally-invalid request (missing `RoleArn`) against a nonexistent
pipe and asserted `NotFoundException` -- it was updated to supply a valid
`RoleArn` so it continues to exercise the not-found path specifically,
rather than incidentally asserting the wrong exception for a request that
was never valid to begin with. ~41 call sites across the test suite (34
`CreatePipeInput{}`/7 `UpdatePipeInput{}` Go struct literals, plus ~65
raw-HTTP JSON request bodies) were updated to supply `RoleArn` now that
it's enforced; no test assertions about *other* validation paths
(missing `Source`, missing `Target`, invalid `DesiredState`, etc.) changed
behavior, since those checks all run before the new `RoleArn` check.

Execution gaps closed for real (2026-07-24 second pass, parity-3 final phase):
the two gaps below were previously deferred citing "cross-service, out of
services/pipes/'s edit scope." That reasoning is no longer accepted in this
phase; both are now closed with real code plus tests proving delivery, not
just closed on paper.

1. **Runner source pollers (Kinesis, DynamoDB Streams).** `pollPipe`
   previously only had an `isSQSARN` gate; a RUNNING pipe with a Kinesis or
   DynamoDB Streams source never polled anything. `runner.go` gained
   `PipeKinesisReader`/`PipeDynamoDBStreamsReader` source interfaces (mirroring
   the shapes `services/lambda/event_source_poller.go`'s ESM poller already
   uses for the same two source types) plus `Runner.SetKinesisReader`/
   `SetDynamoDBStreamsReader`. A new `sources_poll.go` implements
   `pollKinesisPipe`/`pollDynamoDBStreamPipe`: each shard's iterator is cached
   in `Runner.shardIterators` (a `pkgs/safemap.Map[string,string]`, per the
   pkgs-catalog.md rule that an isolated single-map cache belongs in safemap
   rather than a bespoke mutex) and advances unconditionally once `GetRecords`
   succeeds -- matching the Lambda ESM poller's established precedent, since
   Kinesis/DynamoDB Streams have no message-level ack/delete to make
   checkpoint-only-on-success safe (one poison record would otherwise wedge
   the shard forever). Records are filtered through the same `FilterCriteria`
   engine SQS sources use (`filter.go`'s `matchesAnyFilter` was generalized
   from `(*SQSMessage, []Filter)` to `(body string, []Filter)` so all three
   source types share one matcher), enriched and dispatched through the same
   `dispatchTarget`/DLQ path SQS uses (`invokeTargetWithPayload` was split
   into that reusable `dispatchTarget` plus a thin SQS-receipt-handle wrapper).
   `cli.go`'s `wirePipesRunner` wires both against the **real** Kinesis and
   DynamoDB backends (new `pipesKinesisReaderAdapter`/
   `pipesDDBStreamsReaderAdapter`, modeled on the existing
   `kinesisReaderAdapter`/`ddbStreamsReaderAdapter` Lambda ESM adapters).
   MSK/self-managed Kafka/RabbitMQ/ActiveMQ remain unpolled -- see `gaps:`
   above for the proof this is a genuine impossibility, not a deferral.

2. **cli.go target/DLQ wiring.** `wirePipesRunner` previously only wired an
   SQS source reader and Lambda/StepFunctions target invokers, even though
   `runner.go` already had full `SNSPublisher`/`SQSSender`/`PipeKinesisPutter`/
   `PipeEventBridgePutter`/`PipeCloudWatchLogsPutter`/`PipeFirehosePutter`
   interfaces, `Set*` methods, and `invoke*Target` implementations sitting
   unused -- every one of those six target types (and both DLQ paths, which
   reuse the SNS/SQS interfaces) returned `ErrTargetInvokerUnwired` in the
   real binary. `wirePipesRunner` was split into `wirePipesSources`/
   `wirePipesInvokers`/`wirePipesTargets` and now wires all six against real
   backends via six new adapter structs (`pipesSNSPublisherAdapter`,
   `pipesSQSSenderAdapter`, `pipesKinesisPutterAdapter`,
   `pipesEventBridgePutterAdapter`, `pipesCloudWatchLogsPutterAdapter`,
   `pipesFirehosePutterAdapter`), each a thin delegate to that service's own
   `InMemoryBackend` method (`Publish`/`SendMessage`/`PutRecord`/`PutEvents`/
   `PutLogEvents`/`PutRecord`), following the existing
   `pipesSQSReaderAdapter`/`pipesSFNStarterAdapter` pattern. `SetSNSPublisher`/
   `SetSQSSender` cover both the direct-target case and the DLQ case
   (`handlePipeFailure`/`sendToDLQIfConfigured`) since `runner.go` already
   shared those two interfaces between the two call sites.

   Proof of delivery (not just "no error returned"): `cli_pipes_wiring_test.go`
   (root package) builds every backend for real (no mocks), calls the exact
   `wirePipesRunner` cli.go invokes, starts the real `Runner` ticker, and
   asserts the record actually landed in each target's own backend state --
   an SNS topic's message archive, a real SQS queue's `ReceiveMessage`, a
   Kinesis shard's `GetRecords`, an EventBridge archive's `EventCount`, a
   CloudWatch Logs stream's `GetLogEvents`, and an S3 object written by a
   flushed Firehose delivery stream -- plus a DLQ-delivery test (a Lambda
   target fails deterministically with no Docker runtime available, and the
   failure is redirected to a real SQS DLQ) and both new source pollers
   (a real `kinesisBk.PutRecord` / `ddbBk.PutItem` is picked up by the running
   poller and forwarded to a real SQS target). `services/pipes/`'s own test
   suite (`sources_kinesis_ddb_test.go`) additionally covers the poller logic
   itself against fakes: filter application, DLQ-on-target-failure, iterator
   advancement (no re-delivery), `GetRecords`-error recovery, and the shard
   iterator sweep bounding cache growth once a pipe stops.

3. **2026-08-21: `KinesisStreamSourceParameters.StartingPositionTimestamp`
   epoch-seconds fix (gopherstack-5mr2).** `sources.go` declared this field as
   plain `*time.Time` with a `json` tag, decoded/encoded by encoding/json's
   default machinery. Real `CreatePipe`/`UpdatePipe` requests carry it as a
   restjson1 epoch-seconds JSON number
   (`aws-sdk-go-v2/service/pipes@v1.26.4/serializers.go:1903-1905`:
   `ok.Double(smithytime.FormatEpochSeconds(*v.StartingPositionTimestamp))`),
   which `time.Time.UnmarshalJSON` rejects outright
   (`Time.UnmarshalJSON: input is not a JSON string`) -- failing the whole
   CreatePipe request body for any real client that sets an `AT_TIMESTAMP`
   Kinesis source. The same struct is shared, unconverted, between the
   CreatePipe/UpdatePipe decode target, `Pipe.SourceParameters` (the domain
   model), DescribePipe's response encoding, and the persistence snapshot
   round trip, so a plain field-type change to `*float64` (the pattern used
   in `services/emr/handler_clusters.go`) was not a fit -- it would have
   pushed raw epoch floats into the domain model and every other consumer.
   Instead, `wire_time.go` adds `KinesisStreamSourceParameters.MarshalJSON`/
   `UnmarshalJSON` (the alias-embedding pattern already used by
   `services/eventbridge/wire_time.go` and `services/cloudtrail/models.go`'s
   `Event`), keeping the domain field a real `time.Time` while the wire
   encoding/decoding goes through `*float64` + `epochSecondsPtr`/
   `timeFromEpochSecondsPtr`. This also fixes a second, previously-unnoticed
   bug for free: DescribePipe was encoding this field as an RFC3339 string,
   which a real client's deserializer (expecting the same epoch-seconds
   shape on responses; deserializers.go:4988-4996) would also have rejected.
   `UpdatePipe` cannot exercise this field at all in the real API --
   `types.UpdatePipeSourceKinesisStreamParameters` has no
   `StartingPositionTimestamp` member -- so only CreatePipe/DescribePipe are
   reachable by a real client; `wire_time_test.go` covers both, plus a
   direct-`UnmarshalJSON` table test proving the exact old failure mode and
   that an RFC3339 string is still correctly rejected (not silently
   misparsed) post-fix.

   Scope check performed for this pass (not just the four fields named in
   gopherstack-5mr2's floor count): a go/packages-based static analyzer
   walked every `json.Unmarshal`/`json.Decoder.Decode`/echo `Context.Bind`
   call site across all of `services/` (164 packages, generic `decodeBody[T]`/
   `parseBody[T]`/`unmarshalAction[T]` helpers included) and flagged every
   struct field of type `time.Time`/`*time.Time` reachable from a decode
   target, recursively through nested/slice fields. Every other hit was a
   false positive already covered by an existing fix: `services/eventbridge`
   (`EventEntry`, `StartReplayInput`) and `services/cloudtrail` (`Event`)
   already have the alias/custom-`UnmarshalJSON` pattern; `services/kinesis`
   (`ShardIterator`) round-trips only through its own opaque, never-AWS-wire
   base64 token; `services/sagemaker` hits are the pre-existing (and
   off-limits for this pass) alias pattern. `KinesisStreamSourceParameters`
   was the only remaining genuine gap in the whole tree.

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, unbumped retype

`pipesSnapshotVersion` bumped 1 -> 2. `d83f4b5d3` retyped
`BatchContainerOverrides.Environment` (nested inside a registered `pipes/<region>` table's
value type via `Pipe.TargetParameters.BatchJobParameters`) from `map[string]string` to
`[]BatchEnvironmentVariable`, matching the real deserializer, without bumping the snapshot
version. A pre-fix (v1) snapshot's `"Environment"` object no longer unmarshals into the
new array field at all -- `RestoreAll` now errors outright rather than silently losing
data, but the whole backend then fails to restore, which the version guard exists to
convert into a clean, recoverable "discard and start empty" instead.

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration.

**Proof:** `TestRestore_V1BatchEnvironmentDiscarded` (persistence_test.go) builds a
v1-shaped `pipes/eu-west-1` snapshot with an object-shaped `Environment` and asserts
`Restore` succeeds (discarding cleanly) rather than erroring. Hand-reverted to version 1:
the same test then fails with `Restore` returning `json: cannot unmarshal object into Go
struct field BatchContainerOverrides.targetParameters.BatchJobParameters.
ContainerOverrides.Environment of type []pipes.BatchEnvironmentVariable`, confirming the
symptom; restored and `md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).
## pipes (this session, 2026-08-20)

Wrapper-key / nested-shape wire-parity sweep, re-verified against the pinned
`aws-sdk-go-v2/service/pipes@v1.26.4` (this pass confirmed the version bumped
from the v1.23.18 the older notes above cite; no shape referenced below moved
between those two versions except where noted). Protocol reconfirmed
restjson1; `awsRestjson1_deserializeOpDocument<Op>Output` is live for every op
(traced `HandleDeserialize` for CreatePipe: JSON-decodes the whole body into
`shape interface{}` then calls the per-op `deserializeOpDocument...Output`
directly on it -- no httpPayload single-member indirection, so the "cnhp
trap" does not apply to this service; all ten ops' outputs are flat top-level
objects).

**Bug 1 -- `LogConfiguration` fabricated `Destinations` wrapper (dominant class,
wrong nesting + missing members).** Real `types.PipeLogConfiguration`
(`types.go:769`) and `types.PipeLogConfigurationParameters` (`types.go:816`)
both put `CloudwatchLogsLogDestination`, `FirehoseLogDestination`, and
`S3LogDestination` as three direct top-level pointer fields alongside `Level`
and `IncludeExecutionData` -- confirmed against the live
`awsRestjson1_deserializeDocumentPipeLogConfiguration`
(`deserializers.go:4628-4686`), whose `switch key` only recognizes those five
flat keys. `services/pipes/models.go`'s `LogConfiguration` instead wrapped the
three destinations in a fabricated `Destinations []LogDestination` array that
does not exist anywhere in the real API. Effect: a real client sending the
correct flat shape (`{"Level":"INFO","CloudwatchLogsLogDestination":{...}}`)
had every destination field silently dropped by `json.Unmarshal` (unknown
keys ignored), so `CreatePipe`/`UpdatePipe` accepted the call but configured
no log destination at all, and `DescribePipe` echoed back an empty
`Destinations` list forever. Fixed by flattening `LogConfiguration` to match
the real shape exactly and deleting the `LogDestination` wrapper type;
updated `clonePipe`'s deep-copy accordingly. Hand-revert proof: reverted
`models.go` via `cp` from `git show HEAD`, re-ran `TestLogConfiguration` --
all four subtests failed with "log destination not found" (the flat-shape
request body the test now sends round-trips to nothing under the old
wrapper). `TestLogConfiguration` (`pipe_lifecycle_test.go`) was itself an
existing wrong-key test (built `"Destinations": [...]` bodies) and is
corrected to the flat shape. The **Cloudwatch casing is correct** in both old
and new code (`CloudwatchLogsLogDestination`, not `CloudWatch...`) -- matches
`serializers.go`/`deserializers.go` exactly; the managedblockchain-style
casing bug named in the brief does not exist here.

**Bug 2 -- `EcsTaskOverride` missing three real members (missing-from-narrower
class).** Real `types.EcsTaskOverride` (`types.go`, via `PipeTargetEcsTaskParameters.Overrides`)
has `ContainerOverrides []EcsContainerOverride`, `EphemeralStorage
*EcsEphemeralStorage`, and `InferenceAcceleratorOverrides
[]EcsInferenceAcceleratorOverride` in addition to `Cpu`/`ExecutionRoleArn`/
`Memory`/`TaskRoleArn` (7 fields total); `services/pipes/targets.go`'s
`EcsTaskOverride` only had the last four -- the entire per-container override
capability named explicitly in the brief was absent. Added
`EcsContainerOverride` (`Command`/`Cpu`/`Environment`/`EnvironmentFiles`/
`Memory`/`MemoryReservation`/`Name`/`ResourceRequirements`, matching
`types.EcsContainerOverride` field-for-field), `EcsEphemeralStorage`,
`EcsInferenceAcceleratorOverride`, `EcsEnvironmentVariable`,
`EcsEnvironmentFile`, `EcsResourceRequirement`, and wired all three into
`EcsTaskOverride` plus `cloneECSTaskParameters`.

**Bug 3 -- `ECSTaskTargetParameters` missing `PropagateTags`/`ReferenceId`/`Tags`
(missing-from-narrower class).** Real `types.PipeTargetEcsTaskParameters` has
15 fields; gopherstack's `ECSTaskTargetParameters` had 12, missing exactly
`PropagateTags` (`PropagateTags` enum), `ReferenceId` (`*string`), and `Tags`
(`[]types.Tag`, `Key`/`Value`). Added all three (`Tags` as a new `EcsTag`
struct with `Key`/`Value` string fields, matching real `types.Tag`) and wired
`Tags` into `cloneECSTaskParameters`.

**Addendum (2026-09-06, gopherstack-gcjw; superseded 2026-09-06,
gopherstack-iyc2): Bug 2 and Bug 3's fixes were dropped by `d4e234022`, and
have since been restored.** `EcsContainerOverride`, `EcsResourceRequirement`,
`EcsEphemeralStorage`, `EcsInferenceAcceleratorOverride`,
`EcsEnvironmentVariable`, `EcsEnvironmentFile`, `BatchResourceRequirement`,
and `ECSTaskTargetParameters.PropagateTags`/`ReferenceId`/`Tags` -- everything
both bugs above describe adding -- were removed by `d4e234022` (2026-08-23), a
large multi-topic squash merge, three days after this section was written,
with no corresponding edit to this prose or to the commit's own message (the
only pipes change its message documents is the unrelated
`BatchContainerOverrides.Environment` map-to-array fix folded into the same
squash -- nothing in it justifies dropping these types, so this reads as
collateral damage from that pass rather than a deliberate reversal).
gopherstack-iyc2 (2026-09-06) restored all of it, re-derived from the pinned
SDK rather than copied verbatim from Bug 2/3's original diff -- that diff
turned out to have five wire-casing bugs of its own (see CreatePipe's
ops-table note above for the full list). Bug 2/Bug 3's field lists above are
still accurate for what exists in the *current* tree; only their struct
field's exact JSON tags differ from what's now on disk (`Tags` is a plain
`Tag` type now, not `EcsTag`, and the ECS-casing leaf fields are lowercase,
not the `PascalCase` those two entries implied). This is also why
gopherstack-gcjw's ECS-Overrides and Batch-ResourceRequirements follow-up
items (CreatePipe's ops-table note above) could not be validated at the time
gcjw ran: the fields to validate did not exist yet.

**Bug 4 -- `BatchContainerOverrides.Environment` wrong JSON type + missing
`ResourceRequirements` (wrong-type class + missing-from-narrower).** Real
`types.BatchContainerOverrides.Environment` is `[]BatchEnvironmentVariable`
(an array of `{Name,Value}` objects) and also has a `ResourceRequirements
[]BatchResourceRequirement` field; gopherstack had `Environment
map[string]string` (a JSON *object*, not an array) and no
`ResourceRequirements` field at all. A real client's request body
`"Environment":[{"Name":"FOO","Value":"bar"}]` would fail to unmarshal into
gopherstack's map field entirely (`json: cannot unmarshal array into Go
struct field ... of type map[string]string`), a hard 400 on every Batch
target configured with container env overrides -- confirmed by hand-revert:
restoring the old `targets.go` against the already-fixed test file produces a
compile error (`undefined: pipes.BatchEnvironmentVariable`) proving the type
mismatch, and re-running `TestBatch_ContainerOverrides` against the fix
passes. Fixed by adding `BatchEnvironmentVariable`/`BatchResourceRequirement`
types and correcting the field; updated `cloneBatchJobParameters`.
`TestBatch_ContainerOverrides` (`targets_ecs_batch_test.go`) and
`TestClone_BatchDependsIsolation` were existing wrong-key tests (built/read
`Environment` as a JSON object) and are corrected to the array-of-objects
shape; the container-overrides test now also asserts the `Environment` value
round-trips, which it previously did not check at all.

**Bug 5 -- `pipeSummary` fabricated `Description` field (missing-from-narrower
/ response-shape-conflation class).** `ListPipesOutput.Pipes []types.Pipe`
(`api_op_ListPipes.go:74`) uses the 10-field summary `types.Pipe`
(`types.go`), which has no `Description` field -- only the full
`DescribePipeOutput` (18 fields) does. `services/pipes/handler.go`'s
`pipeSummary` struct carried a fabricated `Description` field generalized
from the wider `pipeResponse` sibling. Removed it from both the struct and
`handleListPipes`'s population. `TestHandler_ListPipesIncludesSourceTarget`
(`handler_test.go`) was an existing wrong-key test asserting `ListPipes`
returns `Description`; corrected to assert the key is *absent*, and confirmed
by hand-revert (reverting `handler.go` makes the corrected assertion fail).

**`Pipe` vs `DescribePipeOutput`: confirmed distinct**, and now correctly so
after Bug 5 -- `pipeSummary` (10 wire fields) vs `pipeResponse` (18 wire
fields matching `DescribePipeOutput` exactly, modulo the pre-existing
known-cosmetic extra fields noted below) no longer share a fabricated field.

**Full field-list diffs performed (optional included, types checked), all
CLEAN except the four bugs above:**
`PipeSourceParameters` (8/8 fields) and all seven variants --
`PipeSourceSqsQueueParameters`, `PipeSourceKinesisStreamParameters` (9/9),
`PipeSourceDynamoDBStreamParameters` (8/8),
`PipeSourceManagedStreamingKafkaParameters` (6/6),
`PipeSourceSelfManagedKafkaParameters` (9/9) plus
`SelfManagedKafkaAccessConfigurationVpc`,
`PipeSourceRabbitMQBrokerParameters` (5/5),
`PipeSourceActiveMQBrokerParameters` (4/4) -- and all three credential unions
(`MSKAccessCredentials`, `SelfManagedKafkaAccessConfigurationCredentials`,
`MQBrokerAccessCredentials`), verified against their live
`awsRestjson1_serializeDocument*`/`deserializeDocument*` switch statements,
not just the type list. `PipeTargetParameters` (13/13) and all twelve
variants except the two bugged ones above:
`PipeTargetLambdaFunctionParameters`, `PipeTargetStateMachineParameters`,
`PipeTargetSqsQueueParameters`, `PipeTargetKinesisStreamParameters`,
`PipeTargetCloudWatchLogsParameters`,
`PipeTargetEventBridgeEventBusParameters`, `PipeTargetRedshiftDataParameters`,
`PipeTargetSageMakerPipelineParameters`, `PipeTargetBatchJobParameters` (7/7,
container-overrides bug aside), `PipeTargetTimestreamParameters` (8/8) plus
`DimensionMapping`/`SingleMeasureMapping`/`MultiMeasureMapping`/
`MultiMeasureAttributeMapping`, `PipeTargetHttpParameters`.
`PipeEnrichmentParameters`/`PipeEnrichmentHttpParameters` -- CLEAN, exact
field-for-field match including nesting.

**Enum check, both directions.** Every enum named in the brief
(`AssignPublicIp`, `BatchJobDependencyType`, `BatchResourceRequirementType`,
`DimensionValueType`, `DynamoDBStreamStartPosition`,
`EcsEnvironmentFileType`, `EcsResourceRequirementType`, `EpochTimeUnit`,
`IncludeExecutionDataOption`, `KinesisStreamStartPosition`, `LaunchType`,
`LogLevel`, `MSKStartPosition`, `MeasureValueType`,
`OnPartialBatchItemFailureStreams`, `PipeTargetInvocationType`,
`PlacementConstraintType`, `PlacementStrategyType`, `PropagateTags`,
`RequestedPipeState`, `RequestedPipeStateDescribeResponse`, `S3OutputFormat`,
`SelfManagedKafkaStartPosition`, `TimeFieldType`) backs a plain-`string`
gopherstack field with no hand-written enum-constant list of its own (this
service never re-declares AWS's enum constants -- it passes the wire string
through), so every real SDK value is representable and no fabricated
constant can ever be emitted; verified this pattern holds for the fields this
pass touched (`EcsResourceRequirementType`, `BatchResourceRequirementType`,
`EcsEnvironmentFileType`, `PropagateTags` on the new `ECSTaskTargetParameters`
fields) same as pre-existing ones. **Exception found and disclosed, not
fixed:** `PipeState` (internal `CurrentState` state-machine constants in
`models.go`) defines 12 of 15 real values (`enums.go`), missing
`CREATE_ROLLBACK_FAILED`/`DELETE_ROLLBACK_FAILED`/`UPDATE_ROLLBACK_FAILED`.
Not a wire bug (every value gopherstack emits is a valid real one) -- it's
that gopherstack's synchronous state machine never models an async-rollback
failure path, so it can never reach those three terminal states. Structural
gap, left unfixed (Layer 3 territory, out of scope for this sweep).
`RequestedPipeState`/`RequestedPipeStateDescribeResponse` and the
`DesiredState` DELETED-substitution logic from the prior audit reconfirmed
correct at v1.26.4.

**Gap found and disclosed, not fixed (new this pass, more significant than
the pre-existing "cosmetic extra fields" note below suggested):** real
`CreatePipeInput`/`UpdatePipeInput`/`DescribePipeOutput` have **no top-level
`DeadLetterConfig` field at all** (confirmed by reading the full
`CreatePipeInput` struct in `api_op_CreatePipe.go` and `DescribePipeOutput`
in `api_op_DescribePipe.go`) -- the real API only has DLQ config nested
inside `SourceParameters.KinesisStreamParameters.DeadLetterConfig` and
`SourceParameters.DynamoDBStreamParameters.DeadLetterConfig` (both of which
gopherstack's `sources.go` already models correctly, per the clean diff
above). `services/pipes/models.go`'s top-level `Pipe.DeadLetterConfig` /
`CreatePipeInput.DeadLetterConfig` wire field is therefore fabricated -- and
unlike the already-documented cosmetic extra-field notes elsewhere in this
file, this one is load-bearing: `runner.go:405-409` and
`sources_poll.go:268-274` (the actual DLQ-delivery code path) read
**exclusively** from that fabricated top-level field, never from the two real
nested fields. Net effect: a real AWS SDK client that configures DLQ the only
way the real API allows (nested under its Kinesis/DynamoDB source
parameters) gets silent non-delivery to its DLQ in gopherstack, while
gopherstack's own test suite (`runner_dlq_test.go` etc.) only ever exercises
the fabricated top-level field and so never catches this. Not fixed this pass
-- wiring the runner to read the two real nested fields (with the top-level
field either removed or kept only as an internal fallback) touches
`runner.go`, `sources_poll.go`, `pipe_lifecycle.go`, persistence, and a
double-digit number of existing DLQ tests, which is more rework than this
sweep's time budget covers safely; flagging for a follow-up pass rather than
risking a rushed, under-tested change to a currently-passing feature area.
**FIXED 2026-08-22 (gopherstack-6ffg, commit `c1b8de09a`) -- see the dated
entry below.** This paragraph is left as the original record of the gap;
do not read it as still-open.

**Pre-existing cosmetic-extra-field notes (from the 2026-07-13/07-24 audits)
reconfirmed still true at v1.26.4, not touched this pass:**
`pipeResponse` (shared by Create/Update/Delete/Start/Stop/DescribePipe) still
carries `RuntimeMetricsStreaming`, which does not exist anywhere in
`aws-sdk-go-v2/service/pipes@v1.26.4` (`grep -rn RuntimeMetrics` across the
whole SDK package returns zero hits); and `DeadLetterConfig`/
`RuntimeMetricsStreaming` still appear as extra unrecognized keys on
`CreatePipeOutput`/`UpdatePipeOutput`/`StartPipeOutput`/`StopPipeOutput`
(whose real shapes are just `Arn`/`Name`/`RoleArn`/`Source`/`Target`/
`Description`/`Enrichment`/`KmsKeyIdentifier`/`DesiredState`/`CurrentState`/
`StateReason`/`CreationTime`/`LastModifiedTime`). Real deserializers ignore
unrecognized JSON keys (confirmed via the `default: _, _ = key, value` case
pattern throughout `deserializers.go`), so these remain harmless to real SDK
clients round-tripping -- distinct from the `DeadLetterConfig` gap above,
which actually breaks a feature.

**Families clean:** `route_matcher` (untouched, re-confirmed against the
op list, no changes needed), all ten ops' HTTP method/path bindings, error
sets for `CreatePipe`/`DescribePipe` spot-checked against their live
`deserializeOpError<Op>` switches (`ConflictException`/`InternalException`/
`NotFoundException`/`ServiceQuotaExceededException`/`ThrottlingException`/
`ValidationException`) and match `errors.go` exactly.

**Provenance verdict:** `last_audit_commit: 5d5b2188` dated 2026-07-13 by
`git show -s --format=%ad`, but `last_audit_date` read `2026-07-24` before
this pass -- an 11-day gap with the commit predating the date. Checked
whether the stamp advanced across the two passes in between
(`efc42cbc4`->`3c8a7ff5f`->`d39bf33e4`) via `git log -p --follow -- PARITY.md`:
`efc42cbc4` set commit=`5d5b2188`/date=2026-07-13 (self-consistent, real
audit); `3c8a7ff5f` bumped only the date to 2026-07-24, leaving the commit
pointer unchanged; `d39bf33e4` touched neither. So this is the same
"date-only bump, stuck commit pointer" pattern as the cloudcontrol example in
the brief -- one real audit (`efc42cbc4`) followed by at least one pass that
advanced the date without doing new work reflected in the commit pointer.
This pass sets both to current HEAD (`17458c2f2`, 2026-08-20) since real work
was done and verified above.

**Gates:** `go build ./services/pipes/...` clean. `go vet ./services/pipes/...`
clean. `gofmt -l services/pipes/` empty. `go test ./services/pipes/... -race
-count=1` -- PASS. `golangci-lint run ./services/pipes/...` and `go fix -diff`
-- see session gate output for exact result; no `//nolint` for
cyclop/gocyclo/gocognit/funlen added, no `export_test.go` changes.

**Files touched:** `services/pipes/models.go`, `services/pipes/targets.go`,
`services/pipes/handler.go`, `services/pipes/pipe_lifecycle_test.go`,
`services/pipes/targets_ecs_batch_test.go`, `services/pipes/handler_test.go`,
`services/pipes/PARITY.md`. No file outside `services/pipes/` touched.

## 2026-08-23: DeadLetterConfig gap re-audit -- already fixed, PARITY.md was stale

bd filed a second, independently-worded bug (`gopherstack-a2y2`) describing
the exact `DeadLetterConfig` fabrication documented above and requesting a
fix. Re-verification against the pinned SDK (`aws-sdk-go-v2/service/
pipes@v1.26.4/types/types.go`) found the fix already shipped two days
earlier: commit `c1b8de09a` ("fix(pipes): read the dead-letter queue from the
source parameters, where the real API puts it", 2026-08-22) closed
`gopherstack-6ffg` -- a differently-numbered issue for the same defect. The
duplicate filing traces to this file: the "Gap found and disclosed, not
fixed" paragraph above was never updated after the fix landed, so it read as
an open gap to whatever produced `gopherstack-a2y2`. Corrected in place
rather than deleted (see the amendment appended to that paragraph).

**Confirmed still correct today, independently:**
- Real SDK: `CreatePipeInput`/`UpdatePipeInput`/`DescribePipeOutput` carry
  zero `DeadLetterConfig` occurrences (`grep -n DeadLetterConfig
  api_op_{Create,Update,Describe}Pipe.go` in the pinned module -- empty).
  `DeadLetterConfig` exists on exactly four real types:
  `PipeSourceKinesisStreamParameters`, `PipeSourceDynamoDBStreamParameters`,
  `UpdatePipeSourceKinesisStreamParameters`, and
  `UpdatePipeSourceDynamoDBStreamParameters` (`types.go:886,926,1816,1851`).
  No other source type (SQS, MSK, self-managed Kafka, RabbitMQ, ActiveMQ) has
  it, on either the create or update side -- matches `c1b8de09a`'s own claim.
- Gopherstack: `models.go`'s top-level `Pipe.DeadLetterConfig` /
  `CreatePipeInput.DeadLetterConfig` / `UpdatePipeInput.DeadLetterConfig`
  fields are gone (only the `DeadLetterConfig` type itself and its clone
  helper remain, still used by the two correct nested fields in
  `sources.go`). `runner.go`'s `pipeDeadLetterARN` and both its call sites
  (`handlePipeFailure`, `sources_poll.go`'s `sendToDLQIfConfigured`) read
  exclusively from `SourceParameters.KinesisStreamParameters.DeadLetterConfig`
  / `.DynamoDBStreamParameters.DeadLetterConfig`.

**No snapshot-version bump was needed for this fix** (unlike the
`gopherstack-hjdd` `BatchContainerOverrides` retype above), and this was
verified rather than assumed: `pkgs/persistence/snapshot.go`'s
`UnmarshalSnapshot` calls plain `json.Unmarshal` with no
`DisallowUnknownFields`, so a pre-fix snapshot's stray top-level
`"deadLetterConfig"` key is silently ignored on restore rather than causing a
decode error -- this is a field *removal*, not the incompatible type change
(`map`->`slice`) the version guard exists to catch. `pipesSnapshotVersion`
stays at 2. `go test ./pkgs/persistence/...` passes unmodified (no `-update`
run).

**Proof re-run today, both hand-reverted against `c1b8de09a^`'s
`models.go`/`runner.go`/`sources_poll.go`/`pipe_lifecycle.go` (via `cp`, not
`git checkout`) and restored:**
- `TestRunner_EnrichmentFailure_RoutesToDLQ/enrichment_unwired_with_dlq`
  (`runner_dlq_test.go`) failed on the reverted code:
  `expected: []string{"arn:aws:sqs:...:dlq"} / actual: []string(nil)` --
  the SQS-path DLQ never receives the failed batch.
- `TestPipesRunner_StreamSourcePolling/kinesis_target_failure_dlq`
  (`sources_test.go`) failed on the reverted code: `"[]" should have 1
  item(s), but has 0` -- the stream-poller DLQ path has the same symptom.
  Both are exactly the "failed delivery never reaches the DLQ" symptom the
  bug describes. Restored via `cp`; `md5sum` of all four files matched their
  pre-revert values exactly. `go build ./services/pipes/...` and
  `go test ./services/pipes/... -race -count=1` both clean after restore.

**Gates this pass:** `go build ./...` clean. `go test ./services/pipes/...
-race -count=1` -- PASS. `go test ./pkgs/persistence/...` -- PASS (no
`-update`). `go vet -tags integration ./...` clean. `go vet -tags e2e ./...`
fails, but on `test/e2e/timestreamquery_test.go` (a `CreateScheduledQuery`
call-site arity mismatch) -- unrelated to `pipes`, not touched by this pass,
pre-existing on `HEAD` before this session started.

**Left undone:** nothing in `services/pipes/`. `gopherstack-a2y2` should be
closed as a duplicate of the already-closed `gopherstack-6ffg`; that bd
bookkeeping was left to the orchestrator per this pass's scope (git/bd
mutations reserved for the orchestrator). The unrelated `test/e2e` vet
failure is also left for whichever session owns that file.

## 2026-09-06 (gopherstack-a2vk): nested event-pattern filtering

Confirmed the filed premise: `filter.go`'s `matchesJSONPattern` only ever
walked the top level of `FilterCriteria.Filters[].Pattern` against the
message body. A pattern shaped like the real-world DynamoDB Streams filter
`{"dynamodb":{"NewImage":{"id":{"S":["1"]}}}}` -- the shape EventBridge's own
docs (`eb-event-patterns-content-based-filtering.html`) use as the canonical
example, and the shape a real DynamoDB Streams record actually produces --
could never match past `dynamodb`, because the code had no branch for "this
field's pattern value is itself an object, not an array of matchers." The
old doc comment called this "future work"; it was a real, reachable gap, not
overcautious hedging.

**Sources consulted, in the order the brief specifies:**

1. **In-repo precedent.** `services/eventbridge/pattern.go` already
   implements a full nested EventBridge pattern matcher (`matchObject`/
   `matchObjectField` recursion, `$or`, numeric, cidr, wildcard,
   equals-ignore-case, anything-but) used by that service's `PutRule`/
   `PutEvents` matching. Every one of its matcher functions is unexported
   (`matchPattern`, `compilePattern`, `matchObject`, ... -- confirmed via
   `grep -n '^func [A-Z]' services/eventbridge/pattern.go`, zero hits), so
   `services/pipes/` cannot import it as-is; doing so would require either
   exporting eventbridge's functions or lifting shared logic into a new
   `pkgs/` package, both edits outside `services/eventbridge/`/`pkgs/` and
   therefore outside this pass's file scope (`services/pipes/` only).
   **Decision: do not rewrite a second, independently-designed matcher from
   scratch.** Instead, `filter.go`'s existing `json.RawMessage`-based,
   field-at-a-time matcher (already correctly handling `exists`/`prefix`/
   `suffix`/`anything-but` per the 2026-08-30 pass above) was extended with
   nested-object recursion plus `numeric`/`cidr`, each ported algorithmically
   from `pattern.go`'s `matchNumeric`/`compareNumeric`/`matchCIDR` (same
   op-pair-list numeric algorithm, same `net.ParseCIDR`/`ipNet.Contains`
   cidr check) rather than reinvented. This keeps pipes' engine on its
   existing `json.RawMessage` representation (avoiding a costly rewrite onto
   eventbridge's `map[string]any` representation, which the rest of
   `filter.go` and its tests are not built around) while not diverging on
   the semantics that matter. **Flagging for a human/orchestrator decision,
   not resolved here (out of this pass's file scope):** consolidating both
   into one `pkgs/eventpattern`-style package is the structurally correct
   long-term fix, since gopherstack now has two independently-tested but
   textually different implementations of the same documented AWS matching
   semantics; filed as a reuse-not-done gap rather than silently accepted.
2. **Pinned SDK** (`aws-sdk-go-v2/service/pipes@v1.26.4`): `types.Filter`
   (`types/types.go`) is `struct { Pattern *string }` with a doc comment
   that says only "The event pattern." and links to the EventBridge content-
   filtering guide -- confirming the SDK itself defines no pattern grammar of
   its own; Pipes patterns are EventBridge patterns by reference, matching
   this file's existing `filter_semantics` note.
3. **AWS documentation**
   (`docs.aws.amazon.com/eventbridge/latest/userguide/
   eb-event-patterns-content-based-filtering.html`, the same source the
   2026-08-30 pass already cited): "For each key in the event pattern, the
   value ... must be an array" -- the array-not-bare-scalar rule; nested
   objects "let you match nested JSON"; array entries within one key are
   ORed and multiple keys are ANDed ("all the fields ... must match").

**Exactly which content filters are supported, and what's not:**

| Filter | Status |
|---|---|
| Nested object recursion | Supported (this pass) |
| Exact-match array (`["v1","v2"]`), multi-key AND, multi-entry OR | Supported (nested-recursion pass reuses the pre-existing top-level AND/OR logic at every level) |
| `{"exists": true/false}` | Supported (2026-08-30 pass) |
| `{"prefix": "..."}` / `{"suffix": "..."}` (plain string form only) | Supported |
| `{"anything-but": "x"}` / `{"anything-but": ["x","y"]}` (string only) | Supported (2026-08-30 pass) |
| `{"numeric": [">", 5]}` | Supported (this pass) -- event value must decode as a true JSON number; a string-encoded number (e.g. DynamoDB's `{"N": "5"}` wrapper) does not match, matching `services/eventbridge/pattern.go`'s own `toFloat64` restriction |
| `{"cidr": "10.0.0.0/24"}` | Supported (this pass) |
| `{"prefix": {"equals-ignore-case": "..."}}` / `{"suffix": {"equals-ignore-case": ...}}` / bare `{"equals-ignore-case": "..."}` | **Not supported** -- falls through `matchesRuleObject`'s unrecognized-key default, never matches |
| `{"wildcard": "..."}` | **Not supported** -- same fail-closed default |
| `$or` combinator | **Not supported** -- not special-cased as a pattern key, so a literal `"$or"` field is matched as an ordinary (never-present) field name and the whole pattern fails to match |
| A bare non-array, non-object pattern value (e.g. `{"type":"order"}` instead of `{"type":["order"]}`) | **Not supported** -- invalid per real EventBridge (`eventbridge/pattern.go`'s `validatePatternObject` rejects it outright); Pipes has no separate pattern-validation step at `CreatePipe` time, so this fails closed (never matches) rather than being silently treated as an exact-match scalar |
| Event field itself is a JSON array (any-element-matches semantics) | **Not supported** -- an array-valued event field is compared as one opaque JSON value; exact-match compares it structurally (rarely useful), and every content filter (`prefix`/`suffix`/`numeric`/`cidr`/`anything-but`) fails to decode it and returns no-match |

**Deliberate, tested fail-closed behavior:** any matcher-object key not in
the supported list above falls through every `if _, ok := ruleObj["..."]`
check in `matchesRuleObject` to its final `return false`, so an unsupported
operator can only ever cause records to be *dropped*, never records that
should have been filtered out to *leak through*. Covered by
`TestFilter_NestedPatterns/unrecognized_matcher_object_never_matches`
(`filter_test.go`), using `{"wildcard": "ord*"}` as the unsupported
operator.

**Exact-match type-sensitivity pinned by tests (2026-09-06, gopherstack-50hq):**
`matchesExactRule`'s type-sensitive comparison and its `reflect.DeepEqual`
(not `==`, which panics on a non-comparable decoded array/object) are now
covered by `TestFilter_ExactMatchTypeSensitivity` (`filter_test.go`).

**`filter.go`'s doc comment corrected** (was inaccurate after the
2026-08-30 pass already fixed `exists`/`anything-but`, and is now further
out of date):

Old (`matchesJSONPattern`'s doc comment):
```
// matchesJSONPattern tests whether msgBody satisfies the EventBridge-style
// JSON event pattern. Only top-level field matching is implemented; nested
// field paths and advanced operators (prefix, suffix, numeric range, cidr,
// exists, anything-but) are left as future work.
//
// Pattern shape:  {"field": ["value1", "value2", ...], ...}
// Each field in the pattern must exist in the message and its value must
// equal at least one of the listed rule values.
```

New:
```
// matchesJSONPattern tests whether msgBody satisfies the EventBridge-style
// JSON event pattern (eb-event-patterns-content-based-filtering.html).
// A nested pattern object recurses into the matching message field
// (e.g. {"dynamodb":{"NewImage":{"id":{"S":["1"]}}}}); multiple fields at
// one level are ANDed, multiple array entries for one field are ORed.
//
// Supported content filters (array elements shaped as an object): exists,
// prefix, suffix, numeric, anything-but, cidr. Unsupported operators
// (wildcard, equals-ignore-case, the nested {"prefix":{"equals-ignore-case":
// ...}} form, $or) and any other unrecognized matcher object never match --
// see matchesRuleObject -- rather than silently matching everything.
//
// Pattern shape:  {"field": ["value1", "value2", ...], "nested": {"field2": [...]}}
// A field's pattern value must be either an array (of exact-match values
// and/or content-filter objects) or a nested object; a bare scalar is not a
// valid EventBridge pattern value and never matches (see isJSONObject).
```

**Tests:** `filter_test.go` gained `TestFilter_NestedPatterns` (14
subtests, each driven end-to-end through `CreatePipe`+`Runner`+mock SQS
reader/deleter, matching this file's existing test style, not a unit call
into the matcher directly): `nested_dynamodb_newimage_matches`/
`nested_dynamodb_newimage_value_mismatch` (the issue's own DynamoDB
`NewImage` shape), `nested_field_absent_no_match`/
`nested_parent_field_absent_no_match`, `multi_key_and_both_present_matches`/
`multi_key_and_one_mismatched_no_match`, `multi_entry_or_second_value_matches`/
`multi_entry_or_no_value_matches`, `numeric_operator_matches`/
`numeric_operator_no_match`, `cidr_operator_matches`/`cidr_operator_no_match`,
`bare_scalar_pattern_value_never_matches`, and
`unrecognized_matcher_object_never_matches`.

**Revert-proof (hand-reverted via `cp` from `git show HEAD:services/pipes/
filter.go`, not `git checkout`):** with the production change reverted,
`go test -race -run TestFilter_NestedPatterns -v ./services/pipes/...`
failed 3 of the 14 subtests --
`nested_dynamodb_newimage_matches`, `numeric_operator_matches`, and
`cidr_operator_matches` -- each with `Not equal: expected: []string{"rh1"} /
actual: []string(nil)` (the message that should have passed the filter was
dropped instead). The other 11 subtests happen to also pass against the
unmodified top-level-only matcher (their AND/absence/OR semantics hold at
the top level too, or their content filter -- `anything-but` fails a
different way pre-existing), so they are not revert-proof on their own, but
are kept as regression coverage for AND/OR/absence semantics at the new
nested-matcher code paths going forward. Restored via `cp` from the fixed
version and `md5sum`-verified byte-identical
(`af5b8b470628559e7b3da7103a9b6133`) before/after the revert-and-restore
cycle.

**Gates:** `GOTOOLCHAIN=go1.26.6 go build ./services/pipes/...` clean.
`GOTOOLCHAIN=go1.26.6 go vet ./services/pipes/...` clean. `gofmt -l
services/pipes/*.go` empty. `GOTOOLCHAIN=go1.26.6 go test -race -count=1
./services/pipes/...` -- PASS (all subtests, including the 14 new ones).
`GOTOOLCHAIN=go1.26.6 golangci-lint run services/pipes/...` -- `0 issues.`
No `//nolint` for `cyclop`/`gocyclo`/`gocognit`/`funlen` added; the matcher
was decomposed into one small named helper per matcher type instead
(`matchesPrefixRule`/`matchesSuffixRule`/`matchesNumericRule`/
`matchesCIDRRule`/`matchesAnythingBut`/`matchesNestedRule`/
`matchesExactRule`/`matchesRuleObject`).

**Files touched:** `services/pipes/filter.go` (nested recursion, numeric,
cidr, `reflect.DeepEqual` exact-match hardening, corrected doc comment),
`services/pipes/filter_test.go` (`TestFilter_NestedPatterns`),
`services/pipes/PARITY.md` (this entry plus the `filter_semantics` YAML note
update above). No file outside `services/pipes/` touched; no shared/`pkgs/`
code lifted this pass (see the reuse-vs-rewrite decision above).

## 2026-09-06 (gopherstack-sphp): Filter.Pattern syntax validation at CreatePipe/UpdatePipe

**Premise verified against real AWS, not assumed.** The filed issue claimed
real AWS Pipes rejects a malformed `FilterCriteria.Filters[].Pattern` at
creation time, the way this repo's own `services/eventbridge` already does
for `EventPattern` at `PutRule`/`PutTargets`/`TestEventPattern`
(`validatePatternObject`/`validateMatcherObject`/`isKnownMatcher`,
`pattern.go`). Checked three sources:

1. **aws-sdk-go-v2 error sets.** Both ops list `ValidationException`:
   `CreatePipe`: `UnknownError, ConflictException, InternalException,
   NotFoundException, ServiceQuotaExceededException, ThrottlingException,
   ValidationException`. `UpdatePipe`: same minus
   `ServiceQuotaExceededException`. (`aws-sdk-go-v2/service/pipes@v1.26.4/
   deserializers.go`, `awk`-extracted.) Necessary but not sufficient --
   `ValidationException`'s doc comment ("Indicates that an error has occurred
   while performing a validate operation") is generic and doesn't confirm
   pattern-shape checking specifically.
2. **SDK client-side validator.** `validateOpCreatePipeInput` /
   `validatePipeSourceParameters` in `validators.go` walk every
   `SourceParameters` variant (Kinesis, DynamoDB, brokers, Kafka) but never
   call into `FilterCriteria` at all -- no `validateFilterCriteria` function
   exists in the SDK. The client-side validator is pure-required-field
   checking; it does not confirm server-side pattern validation either way.
3. **AWS docs, decisive.** `eb-pipes-event-filtering.html`'s
   "Filtering Kinesis and DynamoDB messages" table states outright: *"Non-JSON
   | Any | EventBridge throws an exception at the time of Pipe creation or
   update. The filter pattern must be valid JSON format."* Real AWS Pipes
   does reject a malformed pattern at `CreatePipe`/`UpdatePipe`, not only at
   delivery time. **Verdict: real bug, confirmed. Implemented.**

**Design decision: pipes' own operator subset, not eventbridge's validator
verbatim.** `services/eventbridge/pattern.go`'s `isKnownMatcher` accepts
`prefix, suffix, exists, numeric, anything-but, cidr, wildcard,
equals-ignore-case`, plus a top-level `$or` combinator, plus anything-but's
object-negation forms. `services/pipes/filter.go`'s runtime matcher
(`matchesRuleObject`) implements only `prefix, suffix, exists, numeric,
anything-but, cidr` -- and its `anything-but` only decodes a string or a
list of strings (not numbers, not object-form), and its `prefix`/`suffix`
only decode a plain string (not the nested `{"equals-ignore-case": ...}`
form). This gap is tracked separately as **gopherstack-5eok** (not this
issue's job). Real AWS's own comparison-operators table
(`eb-create-pattern-operators.html`, "Pipe support" column) confirms pipes
*does* support `$or`, `equals-ignore-case`, and ignore-case prefix/suffix in
real life -- only bare `wildcard` and anything-but's object-negation forms
are genuinely eventbridge-only. So the filed issue's claim that pipes
supports "none of" `$or`/wildcard/equals-ignore-case/object-form
anything-but is only half right: two of those four (`$or`,
`equals-ignore-case`) are real, documented AWS Pipes syntax that
**gopherstack's own matcher just hasn't implemented yet**.

Reusing eventbridge's validator verbatim would therefore accept `$or` and
`equals-ignore-case` patterns as syntactically valid (correctly, per real
AWS) while `filter.go` can never actually match them -- `CreatePipe` says
yes, delivery silently never matches, exactly the anti-pattern the issue
warns against. The validator added here (`filter_validation.go`) instead
recognizes only the six matcher keys `filter.go` implements, and further
requires `prefix`/`suffix` values to be plain strings and `anything-but`
values to be a string or list of strings -- mirroring exactly what
`matchesPrefixRule`/`matchesSuffixRule`/`matchesAnythingBut` can actually
decode. `$or` gets no special top-level handling (eventbridge's validator
special-cases it to descend into each alternative; pipes' does not), so a
pattern using it is rejected. Every rejection message for an
eventbridge-real/pipes-unimplemented construct cites gopherstack-5eok, so
whoever picks that issue up knows to loosen this validator in step.

**What happens to an eventbridge-only or eventbridge-real-but-pipes-
unimplemented operator:** rejected at `CreatePipe`/`UpdatePipe` with
`ValidationException`, not silently accepted. Covers: `$or` (top-level and
nested), bare `wildcard`, standalone `equals-ignore-case`, the nested
`{"prefix"/"suffix": {"equals-ignore-case": ...}}` form, and `anything-but`
in numeric, mixed-list, or object-negation form.

**Non-JSON (substring) patterns are untouched.** `filter.go`'s
`matchesSingleFilter` only treats a pattern as an EventBridge JSON pattern
when it starts with `{`; anything else is a documented backward-compatible
literal substring match (matches real AWS's own SQS filtering table, which
allows a plain-string pattern against a plain-string message body). The new
validator mirrors this split exactly: only `{`-prefixed patterns are
structurally validated; a plain string like `"order"` is accepted
unconditionally, same as before.

**Files changed:**
- `services/pipes/filter_validation.go` (new): `validateFilterCriteria`,
  `validateFilterPattern`, `validatePipePatternObject`,
  `validatePipeMatcherArray`, `validatePipeMatcherObject`,
  `validatePipeAnythingButValue`, `isKnownPipeMatcher`.
- `services/pipes/pipe_lifecycle.go`: `validateCreatePipeInput` and
  `UpdatePipe` both now call `validateFilterCriteria` on
  `in.SourceParameters.FilterCriteria` when `SourceParameters` is set.
- `services/pipes/export_test.go`: added `SetFilterPatternForTest`, a
  test-only backend method that overwrites a pipe's first filter pattern
  directly, bypassing the new validation -- needed to keep exercising
  `filter.go`'s runtime fail-closed behavior (the defense-in-depth layer
  behind creation-time validation, gopherstack-lrgk) for patterns that are
  no longer constructible via `CreatePipe`.
- `services/pipes/filter_test.go`: `TestFilter_NestedPatterns`'s
  `bare_scalar_pattern_value_never_matches` and
  `unrecognized_matcher_object_never_matches` subtests, and
  `TestFilter_ExactMatchTypeSensitivity`'s
  `array_pattern_element_vs_array_value_no_match_no_panic` subtest (a raw
  JSON array as a matcher-array element -- also not a valid EventBridge
  matcher shape, so also now rejected by `CreatePipe`), now create their
  pipe with an empty pattern and inject the invalid pattern afterward via
  `SetFilterPatternForTest`, instead of passing it directly to `CreatePipe`.
  The runtime fail-closed assertion (`wantMatch: false`, no message
  delivered) is unchanged and still exercised -- only how the pipe gets its
  invalid pattern changed. See the `rawPattern` field added to both test
  tables and its doc comment for the mechanics.
- `services/pipes/filter_validation_test.go` (new):
  `TestFilterPatternValidation_CreatePipe` and
  `TestFilterPatternValidation_UpdatePipe`, 20 shared table cases each
  (10 valid, 10 invalid) run against both ops.

**Revert-proof.** Neutered `validateFilterCriteria` to `return nil`
in-place, rebuilt (compiles -- `fc` is an unused parameter, which Go does
not flag), and ran
`GOTOOLCHAIN=go1.26.6 go test ./services/pipes/... -run 'TestFilterPatternValidation_CreatePipe|TestFilterPatternValidation_UpdatePipe' -v`:
all 10 invalid-pattern subtests failed per op (`invalid_bare_scalar_field_value`,
`invalid_number_field_value`, `invalid_malformed_json`,
`invalid_unknown_matcher_wildcard`, `invalid_unknown_matcher_equals_ignore_case`,
`invalid_or_combinator`, `invalid_nested_prefix_ignore_case`,
`invalid_anything_but_numeric`, `invalid_anything_but_object_form`,
`invalid_anything_but_mixed_list`), each `Error: An error is expected but
got nil.`; the 10 valid-pattern subtests still passed. Restored
`validateFilterCriteria`'s real body immediately after.

**Gates:** `GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/pipes/...`
-- PASS. `GOTOOLCHAIN=go1.26.6 golangci-lint run services/pipes/...` --
`0 issues.` No `//nolint` for `cyclop`/`gocyclo`/`gocognit`/`funlen` added.

**Fragile points, for the record:** `isKnownPipeMatcher` is the single
source of truth for what `CreatePipe`/`UpdatePipe` accept; it must stay in
lockstep with `matchesRuleObject`'s dispatch table in `filter.go` as
gopherstack-5eok lands new operators, or the validator will start rejecting
patterns the runtime can newly match. `validatePipePatternObject` has an
explicit `if key == "$or"` branch that returns a clear error before falling
into the generic array/matcher-object path; if that branch is ever removed,
`$or` still gets rejected (its nested pattern objects fail the generic
matcher-object check), but the error message degrades to a confusing
"unknown matcher %q for field \"$or\"" naming the wrong thing. Keep the
explicit branch.
